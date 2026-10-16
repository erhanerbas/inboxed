package main

import (
	"errors"
	"maps"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/zalando/go-keyring"
)

// testSyncer is a Syncer that is stopped, and its goroutines finished, when the test ends (before the test's earlier
// cleanups put shortened timeouts back).
func testSyncer(t *testing.T, s *Store) *Syncer {
	t.Helper()
	sy := NewSyncer(s, func() {})
	t.Cleanup(func() {
		sy.mu.Lock()
		ids := slices.Collect(maps.Keys(sy.jobs))
		sy.mu.Unlock()
		for _, id := range ids {
			sy.Stop(id)
		}
		sy.running.Wait()
	})
	return sy
}

// otherDevice is a second IMAP client changing the mailbox, like a phone would.
func otherDevice(t *testing.T, srv Server) *imapclient.Client {
	t.Helper()
	c, err := imapclient.DialInsecure(address(srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login("you@example.com", "secret").Wait(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("never happened: %s", what)
}

// shortIntervals makes the regular checks frequent, so a test never waits minutes for one.
func shortIntervals(t *testing.T) {
	t.Helper()
	oldCheck, oldPoll := checkInterval, pollInterval
	checkInterval, pollInterval = 300*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { checkInterval, pollInterval = oldCheck, oldPoll })
}

// uidOf finds a message in a folder by its Message-ID; 0 when it isn't there.
func uidOf(t *testing.T, c *imapclient.Client, mailbox, messageID string) imap.UID {
	t.Helper()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		t.Fatal(err)
	}
	found, err := c.UIDSearch(&imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Message-ID", Value: messageID}}}, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if uids := found.AllUIDs(); len(uids) == 1 {
		return uids[0]
	}
	return 0
}

// The regular check stays at 5 minutes here: inbox changes must arrive through IDLE.
func TestSyncFollowsTheServer(t *testing.T) {
	imapSrv, smtpSrv, user := testServers(t)
	s := newTestStore(t)
	sy := testSyncer(t, s)
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	waitImport(t, sy, a.ID)
	phone := otherDevice(t, imapSrv)

	// Quiet when nothing changes: Inboxed's own commands mustn't keep waking it.
	before := sy.cycles.Load()
	time.Sleep(time.Second)
	if extra := sy.cycles.Load() - before; extra > 0 {
		t.Errorf("%d sync passes with nothing changing", extra)
	}

	appendRaw(t, user, "INBOX", newsletter)
	eventually(t, "new mail shows up", func() bool { _, err := s.Thread(a.ID + "/n1@example.com"); return err == nil })

	uid := uidOf(t, phone, "INBOX", "d1@example.com")
	phone.Store(imap.UIDSetNum(uid), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close()
	eventually(t, "read on the phone is read here", func() bool { th, _ := s.Thread(a.ID + "/d1@example.com"); return !th.Unread })

	uid = uidOf(t, phone, "INBOX", "n1@example.com")
	phone.Store(imap.UIDSetNum(uid), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
	phone.UIDExpunge(imap.UIDSetNum(uid)).Close()
	eventually(t, "deleted on the phone is gone here", func() bool {
		_, err := s.Thread(a.ID + "/n1@example.com")
		return errors.Is(err, ErrNotFound)
	})

	phone.Delete("Work/Projects").Wait()
	sy.Start(a.ID) // other folders aren't watched live; Sync now checks them
	eventually(t, "a folder removed on the server is removed here", func() bool {
		boxes, _ := s.ServerMailboxes(a.ID)
		_, ok := boxes["Work/Projects"]
		return !ok
	})
}

func TestLocalChangesReachTheServer(t *testing.T) {
	shortIntervals(t)
	imapSrv, smtpSrv, _ := testServers(t)
	s := newTestStore(t)
	sy := testSyncer(t, s)
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	waitImport(t, sy, a.ID)
	phone := otherDevice(t, imapSrv)
	thread := a.ID + "/d1@example.com"
	flags := func(mailbox, id string) []imap.Flag {
		uid := uidOf(t, phone, mailbox, id)
		if uid == 0 {
			return nil
		}
		msgs, _ := phone.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{Flags: true}).Collect()
		return msgs[0].Flags
	}

	yes, no := true, false
	s.SetFlags(thread, &no, &yes) // read and starred, here
	sy.WakeAll()
	eventually(t, "read and star reach the server", func() bool {
		return slices.Contains(flags("INBOX", "d1@example.com"), imap.FlagSeen) &&
			slices.Contains(flags("INBOX", "d3@example.com"), imap.FlagFlagged) // star: the newest message
	})

	s.Move(thread, "archive")
	sy.WakeAll()
	eventually(t, "archive moves it out of the inbox on the server", func() bool {
		return uidOf(t, phone, "INBOX", "d1@example.com") == 0 && uidOf(t, phone, "Archive", "d1@example.com") != 0
	})
	eventually(t, "and here it stays archived and starred once synced", func() bool {
		var pending int
		s.db.QueryRow(`SELECT COUNT(*) FROM pending_changes`).Scan(&pending)
		th, _ := s.Thread(thread)
		return pending == 0 && th.Starred && slices.Contains(th.MailboxIDs, a.ID+"/Archive") && !slices.Contains(th.MailboxIDs, a.ID+"/INBOX")
	})
}

// syncedAccount adds the test server's account and waits for its first pass.
func syncedAccount(t *testing.T, imapSrv, smtpSrv Server) (*Store, *Syncer, string) {
	t.Helper()
	s := newTestStore(t)
	sy := testSyncer(t, s)
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	waitImport(t, sy, a.ID)
	return s, sy, a.ID
}

func queueEmpty(s *Store) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM pending_changes`).Scan(&n)
	return n == 0
}

// Changes queued one after another while offline all arrive, even when the first one moves the mail.
func TestQueuedChangesAfterAMoveAllArrive(t *testing.T) {
	shortIntervals(t)
	imapSrv, smtpSrv, user := testServers(t)
	appendRaw(t, user, "INBOX", newsletter) // one message, nowhere else
	s, sy, acc := syncedAccount(t, imapSrv, smtpSrv)
	sy.Stop(acc) // offline
	phone := otherDevice(t, imapSrv)
	news := acc + "/n1@example.com"
	dinner := acc + "/d1@example.com"
	yes := true
	s.Move(news, "archive")
	s.SetFlags(news, nil, &yes) // star the archived mail
	s.Move(dinner, "trash")
	s.Move(dinner, "inbox") // undo
	sy.Start(acc)
	eventually(t, "the queue is sent", func() bool { return queueEmpty(s) })
	time.Sleep(500 * time.Millisecond) // and a folder sync after it
	if th, _ := s.Thread(news); !th.Starred || !slices.Equal(th.MailboxIDs, []string{acc + "/Archive"}) {
		t.Errorf("the star queued after the archive was lost: starred %v, boxes %v", th.Starred, th.MailboxIDs)
	}
	if uid := uidOf(t, phone, "Trash", "d1@example.com"); uid != 0 {
		t.Errorf("undo lost: d1 still in Trash on the server")
	}
	if th, _ := s.Thread(dinner); !slices.Contains(th.MailboxIDs, acc+"/INBOX") {
		t.Errorf("undo lost here: %v", th.MailboxIDs)
	}
}

// Trash and restore leave the mail I sent in Sent: it never lands in Trash or the inbox.
func TestTrashLeavesMySentMail(t *testing.T) {
	shortIntervals(t)
	imapSrv, smtpSrv, _ := testServers(t)
	s, sy, acc := syncedAccount(t, imapSrv, smtpSrv)
	phone := otherDevice(t, imapSrv)
	thread := acc + "/d1@example.com"
	s.Move(thread, "trash")
	sy.WakeAll()
	eventually(t, "trashed on the server", func() bool { return queueEmpty(s) && uidOf(t, phone, "Trash", "d1@example.com") != 0 })
	if uidOf(t, phone, "Sent", "d2@example.com") == 0 {
		t.Fatal("my reply left Sent")
	}
	s.Move(thread, "inbox")
	sy.WakeAll()
	eventually(t, "restored on the server", func() bool { return queueEmpty(s) && uidOf(t, phone, "INBOX", "d1@example.com") != 0 })
	if uidOf(t, phone, "INBOX", "d2@example.com") != 0 || uidOf(t, phone, "Sent", "d2@example.com") == 0 {
		t.Error("restore moved my reply into the inbox")
	}
}

// Mail moved on another device keeps its downloaded copy: only the header is fetched again.
func TestMovedElsewhereIsNotDownloadedAgain(t *testing.T) {
	keyring.MockInit()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("you@example.com", "secret")
	u.Create("INBOX", nil)
	u.Create("Archive", nil)
	appendRaw(t, u, "INBOX", dinner)
	mem.AddUser(u)
	var bodies atomic.Int32
	imapSrv := serveIMAP(t, mem, func(s imapserver.Session) imapserver.Session {
		return flaky{SessionIMAP4rev2: s.(imapserver.SessionIMAP4rev2), bodies: &bodies}
	})
	s, sy, acc := syncedAccount(t, imapSrv, serveSMTP(t, smtpBackend{}))
	phone := otherDevice(t, imapSrv)
	uid := uidOf(t, phone, "INBOX", "d1@example.com")
	if _, err := phone.Move(imap.UIDSetNum(uid), "Archive").Wait(); err != nil {
		t.Fatal(err)
	}
	sy.Start(acc)
	eventually(t, "it shows in Archive here", func() bool {
		th, _ := s.Thread(acc + "/d1@example.com")
		return slices.Equal(th.MailboxIDs, []string{acc + "/Archive"})
	})
	if got := bodies.Load(); got != 1 {
		t.Errorf("downloaded %d whole messages, want 1", got)
	}
}

// A server without IDLE: waiting between polls is not the server going silent.
func TestPollingDoesNotTripTheWatchdog(t *testing.T) {
	oldIdle, oldPoll := idleTimeout, pollInterval
	idleTimeout, pollInterval = 300*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { idleTimeout, pollInterval = oldIdle, oldPoll })
	keyring.MockInit()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("you@example.com", "secret")
	u.Create("INBOX", nil)
	appendRaw(t, u, "INBOX", dinner)
	mem.AddUser(u)
	is := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}}, // no IDLE
		InsecureAuth: true,
	})
	ln := listen(t)
	go is.Serve(ln)
	t.Cleanup(func() { is.Close() })
	_, sy, acc := syncedAccount(t, local(ln), serveSMTP(t, smtpBackend{}))
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if p := sy.Progress(acc); p != nil && p.State == "error" {
			t.Fatalf("polling produced an error: %s", p.Error)
		}
	}
}
