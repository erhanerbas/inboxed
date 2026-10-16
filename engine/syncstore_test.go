package main

import (
	"errors"
	"slices"
	"testing"
)

// syncedThread builds acc/a@x with two messages: a@x in the inbox (UID 1) and b@x in Sent (UID 1).
func syncedThread(t *testing.T) (s *Store, inbox, sent string) {
	t.Helper()
	s = newTestStore(t)
	addTestAccount(t, s)
	inbox, _ = s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	sent, _ = s.UpsertMailbox("acc", "Sent", "Sent", ptr("sent"), "", 1)
	s.UpsertMailbox("acc", "Archive", "Archive", ptr("archive"), "", 1)
	s.AddMessage(Incoming{AccountID: "acc", MailboxID: inbox, UID: 1, parsed: parsed{HeaderID: "a@x", Root: "a@x", Subject: "Plans",
		From: Address{"Maya", "maya@x"}, Date: seedNow, Text: "Dinner?"}})
	s.AddMessage(Incoming{AccountID: "acc", MailboxID: sent, UID: 1, Seen: true, parsed: parsed{HeaderID: "b@x", Root: "a@x",
		Subject: "Re: Plans", From: Address{"You", "you@example.com"}, Date: seedNow.Add(3600e9), Text: "Yes"}})
	return s, inbox, sent
}

func TestServerFlagsDriveThreadFlags(t *testing.T) {
	s, inbox, _ := syncedThread(t)
	if th, _ := s.Thread("acc/a@x"); !th.Unread || th.Starred {
		t.Fatalf("before: unread %v starred %v", th.Unread, th.Starred)
	}
	if err := s.SetLocationFlags(inbox, map[uint32][2]bool{1: {true, true}}); err != nil {
		t.Fatal(err)
	}
	if th, _ := s.Thread("acc/a@x"); th.Unread || !th.Starred {
		t.Errorf("after the server marked it read and flagged: unread %v starred %v", th.Unread, th.Starred)
	}
}

func TestRemoveLocationsThenPrune(t *testing.T) {
	s, inbox, sent := syncedThread(t)
	if err := s.RemoveLocations(inbox, []uint32{1}); err != nil {
		t.Fatal(err)
	}
	// Until the folder sync is over, a message in no folder may just be on its way to another one: it stays.
	if th, err := s.Thread("acc/a@x"); err != nil || len(th.Messages) != 2 || !slices.Equal(th.MailboxIDs, []string{sent}) || th.Unread {
		t.Fatalf("after the inbox copy vanished: %v, %d messages, boxes %v, unread %v", err, len(th.Messages), th.MailboxIDs, th.Unread)
	}
	if err := s.Prune("acc"); err != nil {
		t.Fatal(err)
	}
	if th, _ := s.Thread("acc/a@x"); len(th.Messages) != 1 || th.Snippet != "Yes" {
		t.Fatalf("after pruning: %d messages, snippet %q", len(th.Messages), th.Snippet)
	}
	s.RemoveLocations(sent, []uint32{1})
	s.Prune("acc")
	if _, err := s.Thread("acc/a@x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a thread with no messages left should be gone: %v", err)
	}
	var rows int
	s.db.QueryRow(`SELECT COUNT(*) FROM search s JOIN threads t ON t.doc = s.rowid WHERE t.account_id = 'acc'`).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d search rows left", rows)
	}
	if th, _ := s.Thread("t1"); th.ID != "t1" {
		t.Error("pruning an IMAP account touched demo mail")
	}
}

func TestMoveLocationsKeepsTheMessage(t *testing.T) {
	s, inbox, _ := syncedThread(t)
	if err := s.MoveLocations(inbox, []uint32{1}, "acc/Archive", []uint32{7}); err != nil {
		t.Fatal(err)
	}
	if known, _ := s.KnownUIDs("acc/Archive"); !known[7] {
		t.Errorf("archive UIDs %v", known)
	}
	s.Prune("acc")
	if th, _ := s.Thread("acc/a@x"); len(th.Messages) != 2 || !slices.Contains(th.MailboxIDs, "acc/Archive") {
		t.Errorf("after the move: %d messages, boxes %v", len(th.Messages), th.MailboxIDs)
	}
}

// The server marks the thread read while a star is on its way; once the star is sent, the thread follows the server.
func TestFinishedChangeFollowsTheServer(t *testing.T) {
	s, inbox, _ := syncedThread(t)
	yes := true
	s.SetFlags("acc/a@x", nil, &yes)
	ch, _ := s.PendingChanges("acc")
	s.SetLocationFlags(inbox, map[uint32][2]bool{1: {true, false}}) // read elsewhere, while the star waits
	s.MarkLocations(inbox, []uint32{1}, "flagged", true)            // the star reached the server
	s.FinishChange(ch[0].ID)
	if th, _ := s.Thread("acc/a@x"); th.Unread || !th.Starred {
		t.Errorf("unread %v starred %v, want read and starred as on the server", th.Unread, th.Starred)
	}
}

func TestPendingThreadsKeepLocalState(t *testing.T) {
	s, inbox, _ := syncedThread(t)
	yes := true
	if err := s.SetFlags("acc/a@x", nil, &yes); err != nil { // starred locally, not yet on the server
		t.Fatal(err)
	}
	if err := s.Move("acc/a@x", "archive"); err != nil {
		t.Fatal(err)
	}
	s.SetLocationFlags(inbox, map[uint32][2]bool{1: {true, false}}) // a sync arrives before the changes were sent
	th, _ := s.Thread("acc/a@x")
	if !th.Starred || slices.Contains(th.MailboxIDs, inbox) {
		t.Errorf("a sync undid pending local changes: starred %v, boxes %v", th.Starred, th.MailboxIDs)
	}
}

func TestMarkLocationsAndMailboxes(t *testing.T) {
	s, inbox, sent := syncedThread(t)
	if err := s.MarkLocations(inbox, []uint32{1}, "flagged", true); err != nil {
		t.Fatal(err)
	}
	var flagged bool
	s.db.QueryRow(`SELECT flagged FROM message_locations WHERE mailbox_id = ? AND uid = 1`, inbox).Scan(&flagged)
	if !flagged {
		t.Error("MarkLocations didn't record the flag")
	}
	s.SetMailboxStatus(inbox, "1/2/3/0", 42)
	if got, modseq := s.MailboxStatus(inbox); got != "1/2/3/0" || modseq != 42 {
		t.Errorf("status %q, modseq %d", got, modseq)
	}
	boxes, _ := s.ServerMailboxes("acc")
	if boxes["INBOX"] != inbox || boxes["Sent"] != sent || len(boxes) != 3 {
		t.Errorf("server mailboxes %v (the Starred entry isn't one)", boxes)
	}
	if err := s.DeleteMailbox(sent); err != nil {
		t.Fatal(err)
	}
	s.Prune("acc")
	if th, _ := s.Thread("acc/a@x"); len(th.Messages) != 1 {
		t.Errorf("a removed folder's messages should go with it: %d left", len(th.Messages))
	}
}
