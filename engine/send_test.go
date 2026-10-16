package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/zalando/go-keyring"
)

// recorder is an SMTP server that keeps what it's sent, or refuses every recipient.
type recorder struct {
	mu     sync.Mutex
	msgs   []string
	refuse bool
}

type recorderSession struct{ r *recorder }

func (r *recorder) NewSession(*smtp.Conn) (smtp.Session, error) { return recorderSession{r}, nil }
func (recorderSession) AuthMechanisms() []string                { return []string{sasl.Plain} }
func (s recorderSession) Auth(string) (sasl.Server, error)      { return smtpSession{}.Auth("") }
func (recorderSession) Mail(string, *smtp.MailOptions) error    { return nil }
func (s recorderSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.r.refuse {
		return &smtp.SMTPError{Code: 550, Message: "no such user here"}
	}
	return nil
}
func (s recorderSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	s.r.mu.Lock()
	s.r.msgs = append(s.r.msgs, string(b))
	s.r.mu.Unlock()
	return err
}
func (recorderSession) Reset()        {}
func (recorderSession) Logout() error { return nil }

func (r *recorder) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.msgs)
}

// folder lists a server folder's Message-IDs and flags, as another client sees them.
func folder(t *testing.T, srv Server, name string) map[string][]imap.Flag {
	t.Helper()
	c := otherDevice(t, srv)
	if _, err := c.Select(name, nil).Wait(); err != nil {
		t.Fatal(err)
	}
	msgs, err := c.Fetch(imap.SeqSet{imap.SeqRange{Start: 1, Stop: 0}}, &imap.FetchOptions{
		Flags: true, BodySection: []*imap.FetchItemBodySection{idHeader}}).Collect()
	if err != nil && !strings.Contains(err.Error(), "No such message") {
		t.Fatal(err)
	}
	out := map[string][]imap.Flag{}
	for _, m := range msgs {
		out[messageID(m.FindBodySection(idHeader))] = m.Flags
	}
	return out
}

func sendingAccount(t *testing.T, rec *recorder) (s *Store, sy *Syncer, acc string, imapSrv Server) {
	t.Helper()
	imapSrv, _, _ = testServers(t)
	s, sy, acc = syncedAccount(t, imapSrv, serveSMTP(t, rec))
	return s, sy, acc, imapSrv
}

func TestSendReplyFilesInSent(t *testing.T) {
	rec := &recorder{}
	s, sy, acc, imapSrv := sendingAccount(t, rec)
	before := len(folder(t, imapSrv, "Sent"))
	err := sy.Send(Draft{AccountID: acc, To: "Maya Chen <maya@example.com>", Subject: "Re: Dinner plans", Body: "See you!", ThreadID: acc + "/d1@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.sent(); len(got) != 1 || !strings.Contains(got[0], "In-Reply-To: <d3@example.com>") {
		t.Fatalf("SMTP got %q", got)
	}
	// Filed in the background, after the send already returned.
	eventually(t, "a copy is filed in Sent", func() bool { return len(folder(t, imapSrv, "Sent")) == before+1 })
	sent := folder(t, imapSrv, "Sent")
	for id, flags := range sent {
		if id != "d2@example.com" && !slices.Contains(flags, imap.FlagSeen) {
			t.Errorf("the Sent copy isn't marked read: %v", flags)
		}
	}
	eventually(t, "the reply is in the thread here", func() bool {
		th, _ := s.Thread(acc + "/d1@example.com")
		return len(th.Messages) == 4
	})
	th, _ := s.Thread(acc + "/d1@example.com")
	if th.Messages[3].Body != "See you!" || th.Messages[3].Draft {
		t.Errorf("the reply isn't in the thread here: %d messages", len(th.Messages))
	}
}

func TestSendRefusedKeepsNothing(t *testing.T) {
	rec := &recorder{refuse: true}
	s, sy, acc, imapSrv := sendingAccount(t, rec)
	err := sy.Send(Draft{AccountID: acc, To: "nobody@example.com", Subject: "Re: Dinner plans", Body: "Hi", ThreadID: acc + "/d1@example.com"})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "no such user here") {
		t.Errorf("refused send: %v", err)
	}
	if n := len(folder(t, imapSrv, "Sent")); n != 1 {
		t.Errorf("Sent holds %d after a refused send", n)
	}
	if th, _ := s.Thread(acc + "/d1@example.com"); len(th.Messages) != 3 {
		t.Errorf("thread has %d messages after a refused send", len(th.Messages))
	}
}

func TestSendWithoutSentFolder(t *testing.T) {
	keyring.MockInit()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("you@example.com", "secret")
	u.Create("INBOX", nil)
	mem.AddUser(u)
	rec := &recorder{}
	_, sy, acc := syncedAccount(t, serveIMAP(t, mem, nil), serveSMTP(t, rec))
	if err := sy.Send(Draft{AccountID: acc, To: "maya@example.com", Subject: "Hi", Body: "Hello"}); err != nil {
		t.Errorf("sent mail with nowhere to file it counts as sent: %v", err)
	}
	if len(rec.sent()) != 1 {
		t.Errorf("SMTP got %d messages", len(rec.sent()))
	}
}

func TestDraftsRoundTrip(t *testing.T) {
	rec := &recorder{}
	s, sy, acc, imapSrv := sendingAccount(t, rec)
	d := Draft{AccountID: acc, To: "maya@example.com", Subject: "Plans", Body: "First try"}
	id, err := sy.SaveDraft(d)
	if err != nil {
		t.Fatal(err)
	}
	drafts := folder(t, imapSrv, "Drafts")
	if len(drafts) != 1 {
		t.Fatalf("Drafts holds %d", len(drafts))
	}
	for _, flags := range drafts {
		if !slices.Contains(flags, imap.FlagDraft) {
			t.Errorf("no \\Draft flag: %v", flags)
		}
	}
	var threadID string
	s.db.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, id).Scan(&threadID)
	if th, _ := s.Thread(threadID); len(th.Messages) != 1 || !th.Messages[0].Draft || th.Messages[0].Subject != "Plans" {
		t.Errorf("the draft here: %+v", th.Messages)
	}

	d.DraftID, d.Body = id, "Second try"
	id2, err := sy.SaveDraft(d)
	if err != nil || id2 == id {
		t.Fatalf("second save: %q, %v", id2, err)
	}
	if n := len(folder(t, imapSrv, "Drafts")); n != 1 {
		t.Errorf("Drafts holds %d after saving again, want 1", n)
	}
	var old int
	s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE id = ?`, id).Scan(&old)
	if old != 0 {
		t.Error("the replaced version is still here")
	}

	d.DraftID = id2
	if err := sy.Send(d); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the sent draft leaves Drafts", func() bool { return len(folder(t, imapSrv, "Drafts")) == 0 })

	id3, _ := sy.SaveDraft(Draft{AccountID: acc, Subject: "Never mind"})
	s.db.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, id3).Scan(&threadID)
	if err := sy.DeleteDraft(id3); err != nil {
		t.Fatal(err)
	}
	if n := len(folder(t, imapSrv, "Drafts")); n != 0 {
		t.Errorf("Drafts holds %d after discarding", n)
	}
	if _, err := s.Thread(threadID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the discarded draft's thread is still here: %v", err)
	}
}

func TestSavesSentItself(t *testing.T) {
	for host, want := range map[string]bool{
		"smtp.gmail.com": true, "smtp.office365.com": true, "smtp-mail.outlook.com": true, "smtp.fastmail.com": false,
	} {
		if got := savesSentItself(host); got != want {
			t.Errorf("%s: %v", host, got)
		}
	}
}

// An SMTP server that greets and then never answers can't hang the add-account form or a send.
func TestSMTPGivesUpOnASilentServer(t *testing.T) {
	old := smtpTimeout
	smtpTimeout = 300 * time.Millisecond
	t.Cleanup(func() { smtpTimeout = old })
	ln := listen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			fmt.Fprint(conn, "220 ready\r\n")
			go io.Copy(io.Discard, conn) // and then silence
		}
	}()
	t.Cleanup(func() { ln.Close() })
	start := time.Now()
	err := checkSMTP(local(ln), "you@example.com", "secret")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("after %v: %v", time.Since(start), err)
	}
}

// Mail flagged \Deleted but not expunged (a client without UIDPLUS, or one that expunges later) is gone here.
func TestDeletedButNotExpungedIsGone(t *testing.T) {
	old := fullInterval
	fullInterval = 0 // a flag change shows in STATUS only with CONDSTORE; otherwise the hourly full check finds it
	t.Cleanup(func() { fullInterval = old })
	imapSrv, smtpSrv, _ := testServers(t)
	s, sy, acc := syncedAccount(t, imapSrv, smtpSrv)
	phone := otherDevice(t, imapSrv)
	uid := uidOf(t, phone, "INBOX", "d1@example.com")
	phone.Store(imap.UIDSetNum(uid), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
	sy.Start(acc)
	eventually(t, "the deleted message leaves the thread", func() bool {
		th, _ := s.Thread(acc + "/d1@example.com")
		return len(th.Messages) == 2
	})
}

// The draft endpoints only ever delete drafts.
func TestDeleteDraftRefusesOtherMail(t *testing.T) {
	imapSrv, smtpSrv, _ := testServers(t)
	s, sy, acc := syncedAccount(t, imapSrv, smtpSrv)
	if err := sy.DeleteDraft(acc + "/d1@example.com"); !errors.Is(err, ErrInvalid) {
		t.Errorf("deleting an inbox message as a draft: %v", err)
	}
	if th, _ := s.Thread(acc + "/d1@example.com"); len(th.Messages) != 3 {
		t.Errorf("the message was deleted here: %d left", len(th.Messages))
	}
}
