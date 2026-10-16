package main

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

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
	sy := NewSyncer(s, func() {})
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sy.Stop(a.ID) })
	waitImport(t, sy, a.ID)
	phone := otherDevice(t, imapSrv)

	// Quiet when nothing changes: Inboxed's own commands mustn't keep waking it.
	before := sy.cycles.Load()
	time.Sleep(200 * time.Millisecond)
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
	sy := NewSyncer(s, func() {})
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sy.Stop(a.ID) })
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
