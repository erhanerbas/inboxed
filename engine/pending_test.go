package main

import (
	"slices"
	"testing"
)

func kinds(changes []Change) []string {
	var out []string
	for _, c := range changes {
		out = append(out, c.Kind+"="+c.Value)
	}
	return out
}

func TestLocalActionsQueueForIMAPOnly(t *testing.T) {
	s, _, _ := syncedThread(t)
	yes, no := true, false
	s.SetFlags("acc/a@x", &no, &yes)
	s.Move("acc/a@x", "archive")
	s.SetFlags("t1", &no, nil) // a demo thread: nothing to send
	got, _ := s.PendingChanges("acc")
	if want := []string{"seen=1", "flagged=1", "move=archive"}; !slices.Equal(kinds(got), want) {
		t.Errorf("queued %v, want %v", kinds(got), want)
	}
	if demo, _ := s.PendingChanges("a2"); len(demo) != 0 {
		t.Errorf("demo account queued %v", kinds(demo))
	}
	s.SetFlags("acc/a@x", &yes, nil)
	s.MarkAllRead("acc", "acc/Archive") // where the thread is now
	if got, _ := s.PendingChanges("acc"); kinds(got)[len(got)-1] != "seen=1" {
		t.Errorf("mark all as read queued %v", kinds(got))
	}
}

func TestChangeTargets(t *testing.T) {
	s, inbox, sent := syncedThread(t)
	target := func(kind, value string) []Target {
		got, err := s.ChangeTargets(Change{AccountID: "acc", ThreadID: "acc/a@x", Kind: kind, Value: value})
		if err != nil {
			t.Fatalf("%s=%s: %v", kind, value, err)
		}
		return got
	}
	// Read: every message. Unread and star: the newest message only (b@x in Sent).
	if got := target("seen", "1"); len(got) != 2 {
		t.Errorf("read targets %+v", got)
	}
	if got := target("seen", "0"); len(got) != 1 || got[0].MailboxID != sent {
		t.Errorf("unread targets %+v", got)
	}
	if got := target("flagged", "1"); len(got) != 1 || got[0].MailboxID != sent {
		t.Errorf("star targets %+v", got)
	}
	if got := target("move", "archive"); len(got) != 1 || got[0].MailboxID != inbox || got[0].Path != "INBOX" || got[0].Dest != "Archive" || !slices.Equal(got[0].UIDs, []uint32{1}) {
		t.Errorf("archive targets %+v", got)
	}
	if _, err := s.ChangeTargets(Change{AccountID: "acc", ThreadID: "acc/a@x", Kind: "move", Value: "trash"}); err == nil {
		t.Error("moving to trash on an account without a trash folder should fail")
	}
}

func TestFailedChangeIsDroppedAndReverted(t *testing.T) {
	s, inbox, _ := syncedThread(t)
	s.Move("acc/a@x", "archive")
	changes, _ := s.PendingChanges("acc")
	for attempt := 1; attempt <= 5; attempt++ {
		dropped, err := s.FailChange(changes[0].ID)
		if err != nil || dropped != (attempt == 5) {
			t.Fatalf("attempt %d: dropped %v, %v", attempt, dropped, err)
		}
	}
	if left, _ := s.PendingChanges("acc"); len(left) != 0 {
		t.Errorf("still queued: %v", kinds(left))
	}
	if th, _ := s.Thread("acc/a@x"); !slices.Contains(th.MailboxIDs, inbox) {
		t.Errorf("after giving up, the thread should be back where the server has it: %v", th.MailboxIDs)
	}
}
