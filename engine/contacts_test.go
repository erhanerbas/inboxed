package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestContactsRank(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	addTestAccount(t, s)
	inbox, _ := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	n := 0
	put := func(from Address, to, cc []Address, date time.Time) {
		n++
		id := fmt.Sprintf("m%d@x", n)
		if err := s.AddMessage(Incoming{AccountID: "acc", MailboxID: inbox, UID: uint32(n), parsed: parsed{HeaderID: id, Root: id,
			Subject: "Hi", From: from, To: to, Cc: cc, Date: date, Text: "Hi"}}); err != nil {
			t.Fatal(err)
		}
	}
	me := Address{"You", "You@Example.com"}
	old := seedNow.Add(-200 * 24 * time.Hour)
	// Carol: written to twice (once in Cc). Dave: wrote to me three times. Sent weighs more.
	put(me, []Address{{"carol@x", "Carol@X"}}, nil, seedNow)
	put(me, []Address{{"Erin", "erin@x"}}, []Address{{"Carol King", "carol@x"}}, old)
	for range 3 {
		put(Address{"Dave", "dave@x"}, []Address{me}, nil, seedNow)
	}
	// Frank and Gina wrote once each; Gina recently, so she passes Erin, written to long ago.
	put(Address{"Frank", "frank@x"}, []Address{me}, nil, old)
	put(Address{"", "gina@x"}, []Address{me}, nil, seedNow)

	got, err := s.Contacts(seedNow)
	if err != nil {
		t.Fatal(err)
	}
	var emails []string
	for _, c := range got {
		emails = append(emails, c.Email)
	}
	if want := []string{"carol@x", "dave@x", "gina@x", "erin@x", "frank@x"}; !slices.Equal(emails, want) {
		t.Fatalf("order %v, want %v", emails, want)
	}
	if got[0].Name != "Carol King" {
		t.Errorf("carol's name %q: the newest real name, not her address", got[0].Name)
	}
	if got[2].Name != "" {
		t.Errorf("gina has no name, got %q", got[2].Name)
	}
}
