package main

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

func ptr(s string) *string { return &s }

func addTestAccount(t *testing.T, s *Store) {
	t.Helper()
	set := AccountSettings{Email: "you@example.com", Username: "you@example.com",
		IMAP: Server{Host: "imap.example.com", Port: 993, Security: "tls"},
		SMTP: Server{Host: "smtp.example.com", Port: 465, Security: "tls"}}
	if err := s.CreateAccount(Account{ID: "acc", Name: "Home", Email: "you@example.com", Provider: "imap", Color: "c"}, set); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAndDeleteAccount(t *testing.T) {
	s := newTestStore(t)
	addTestAccount(t, s)
	set, err := s.AccountSettings("acc")
	if err != nil || set.IMAP.Host != "imap.example.com" || set.SMTP.Port != 465 || set.Username != "you@example.com" || set.Password != "" {
		t.Fatalf("settings %+v, %v", set, err)
	}
	if ids, _ := s.IMAPAccounts(); !slices.Equal(ids, []string{"acc"}) {
		t.Errorf("IMAP accounts %v (demo accounts aren't IMAP)", ids)
	}
	boxes, _ := s.Mailboxes()
	if !slices.ContainsFunc(boxes, func(m Mailbox) bool { return m.ID == "acc:starred" && m.Role != nil && *m.Role == "starred" }) {
		t.Error("new account has no Starred entry")
	}
	if err := s.DeleteAccount("acc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AccountSettings("acc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted account still there: %v", err)
	}
	if err := s.DeleteAccount("acc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestAddMessageThreadsAndLocations(t *testing.T) {
	s := newTestStore(t)
	addTestAccount(t, s)
	inbox, err := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	sent, _ := s.UpsertMailbox("acc", "Sent", "Sent", ptr("sent"), "", 1)
	at := func(h int) time.Time { return seedNow.Add(time.Duration(h) * time.Hour) }
	original := parsed{HeaderID: "a@x", Root: "a@x", Subject: "Plans", From: Address{"Maya", "maya@x"},
		To: []Address{{"You", "you@example.com"}}, Date: at(0), Text: "Dinner?"}
	reply := parsed{HeaderID: "b@x", Root: "a@x", Subject: "Re: Plans", From: Address{"You", "you@example.com"},
		To: []Address{{"Maya", "maya@x"}}, Cc: []Address{{"Sam", "sam@x"}}, Date: at(1), Text: "Yes!\n\nOn Tue, Maya wrote:\n> Dinner?"}
	for _, in := range []Incoming{
		{AccountID: "acc", MailboxID: inbox, UID: 7, parsed: original},
		{AccountID: "acc", MailboxID: sent, UID: 3, Seen: true, parsed: reply},
		{AccountID: "acc", MailboxID: inbox, UID: 8, Seen: true, Flagged: true, parsed: reply}, // the same reply, also in the inbox
	} {
		if err := s.AddMessage(in); err != nil {
			t.Fatal(err)
		}
	}
	th, err := s.Thread("acc/a@x")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(th.MailboxIDs)
	if len(th.Messages) != 2 || th.Subject != "Plans" || !th.Unread || !th.Starred || th.Snippet != "Yes!" ||
		!slices.Equal(th.MailboxIDs, []string{"acc/INBOX", "acc/Sent"}) {
		t.Fatalf("thread: %d messages, %q, unread %v, starred %v, snippet %q, boxes %v",
			len(th.Messages), th.Subject, th.Unread, th.Starred, th.Snippet, th.MailboxIDs)
	}
	if known, _ := s.KnownUIDs(inbox); !known[7] || !known[8] || known[3] {
		t.Errorf("inbox UIDs %v", known)
	}
	if got, _ := s.Threads("acc", inbox, "sam"); len(got) != 1 {
		t.Errorf("Cc isn't searchable: %d results", len(got))
	}
	// A server that renumbers a folder (new UIDVALIDITY) has it imported again, so its old UIDs are forgotten.
	if _, err := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 2); err != nil {
		t.Fatal(err)
	}
	if known, _ := s.KnownUIDs(inbox); len(known) != 0 {
		t.Errorf("UIDs kept across a UIDVALIDITY change: %v", known)
	}
}

func TestUpsertMailboxParents(t *testing.T) {
	s := newTestStore(t)
	addTestAccount(t, s)
	s.UpsertMailbox("acc", "Work", "Work", nil, "", 1)
	child, _ := s.UpsertMailbox("acc", "Work/Projects", "Projects", nil, "Work", 1)
	orphan, _ := s.UpsertMailbox("acc", "[Gmail]/Sent Mail", "Sent Mail", ptr("sent"), "[Gmail]", 1)
	boxes, _ := s.Mailboxes()
	parent := map[string]*string{}
	for _, m := range boxes {
		parent[m.ID] = m.ParentID
	}
	if p := parent[child]; p == nil || *p != "acc/Work" {
		t.Errorf("Work/Projects parent %v", p)
	}
	if parent[orphan] != nil {
		t.Errorf("a folder whose parent isn't listed should be top-level, got %v", *parent[orphan])
	}
}

// Removing an account cascades through message_locations; without an index on message_id that is quadratic.
func TestRemoveLargeAccountIsFast(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing test")
	}
	s := newTestStore(t)
	addTestAccount(t, s)
	inbox, _ := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	for i := range 4000 {
		id := fmt.Sprintf("m%d@x", i)
		if err := s.AddMessage(Incoming{AccountID: "acc", MailboxID: inbox, UID: uint32(i + 1), Seen: true,
			parsed: parsed{HeaderID: id, Root: id, Subject: "Note", From: Address{"Bot", "bot@x"}, Date: seedNow, Text: "hi"}}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	if err := s.DeleteAccount("acc"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Errorf("removing 4,000 messages took %v", took)
	}
}

func TestAddLocationForKnownMessage(t *testing.T) {
	s := newTestStore(t)
	addTestAccount(t, s)
	inbox, _ := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	archive, _ := s.UpsertMailbox("acc", "[Gmail]/All Mail", "All Mail", ptr("archive"), "", 1)
	s.AddMessage(Incoming{AccountID: "acc", MailboxID: inbox, UID: 3, Seen: true,
		parsed: parsed{HeaderID: "a@x", Root: "a@x", Subject: "Hi", Date: seedNow, Text: "Hello"}})
	if ok, err := s.AddLocation("acc", archive, 9, "a@x", true, true); !ok || err != nil {
		t.Fatalf("known message: %v %v", ok, err)
	}
	if ok, _ := s.AddLocation("acc", archive, 10, "unknown@x", true, false); ok {
		t.Error("an unknown message can't gain a location without its body")
	}
	th, _ := s.Thread("acc/a@x")
	slices.Sort(th.MailboxIDs)
	if !th.Starred || !slices.Equal(th.MailboxIDs, []string{"acc/INBOX", "acc/[Gmail]/All Mail"}) {
		t.Errorf("thread after AddLocation: starred %v, boxes %v", th.Starred, th.MailboxIDs)
	}
	if known, _ := s.KnownUIDs(archive); !known[9] || known[10] {
		t.Errorf("All Mail UIDs %v", known)
	}
}
