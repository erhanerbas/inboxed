package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var seedNow = time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Seed(seedNow); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenStoreMigratesAndSeedsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for range 2 {
		s, err := OpenStore(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Seed(seedNow); err != nil {
			t.Fatal(err)
		}
		var version, accounts, threads int
		s.db.QueryRow("PRAGMA user_version").Scan(&version)
		s.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&accounts)
		s.db.QueryRow("SELECT COUNT(*) FROM threads").Scan(&threads)
		if version != len(migrations) || accounts != 2 || threads != 7 {
			t.Fatalf("version %d, accounts %d, threads %d", version, accounts, threads)
		}
		s.Close()
	}
}

func TestThreadDerivesSummaryFields(t *testing.T) {
	s := newTestStore(t)
	th, err := s.Thread("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Messages) != 4 || th.Messages[3].From.Name != "Maya Chen" || th.Messages[0].Cc == nil {
		t.Fatalf("messages: %+v", th.Messages)
	}
	var names []string
	for _, p := range th.Participants {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"Maya Chen", "Daniel Okafor"}) {
		t.Errorf("participants %v, want everyone but you, in order of appearance", names)
	}
	if !strings.HasPrefix(th.Snippet, "Done. I updated both screens") || strings.Contains(th.Snippet, "wrote:") {
		t.Errorf("snippet %q should be the latest reply without its quote", th.Snippet)
	}
	if want := seedNow.Add(-35 * time.Minute).Format(isoDate); th.LastDate != want {
		t.Errorf("lastDate %s, want %s", th.LastDate, want)
	}
	if _, err := s.Thread("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing thread: %v, want ErrNotFound", err)
	}
}

func ids(threads []Thread) []string {
	out := []string{}
	for _, t := range threads {
		out = append(out, t.ID)
	}
	return out
}

func unreadByMailbox(t *testing.T, s *Store) map[string]int {
	t.Helper()
	boxes, err := s.Mailboxes()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, m := range boxes {
		out[m.ID] = m.Unread
	}
	return out
}

func TestAccountsAndMailboxes(t *testing.T) {
	s := newTestStore(t)
	accounts, err := s.Accounts()
	if err != nil || len(accounts) != 2 || accounts[0].Name != "Personal" {
		t.Fatalf("accounts %+v, %v", accounts, err)
	}
	boxes, _ := s.Mailboxes()
	if len(boxes) != 18 || boxes[9].ID != "a1-travel-2026" || *boxes[9].ParentID != "a1-travel" || boxes[9].Role != nil {
		t.Fatalf("mailboxes out of order or missing parent/role: %+v", boxes[9])
	}
	want := map[string]int{"a1-inbox": 1, "a2-inbox": 2, "a1-travel-2026": 1, "a2-projects-inboxed": 1, "a1-starred": 0, "a1-spam": 0}
	got := unreadByMailbox(t, s)
	for id, n := range want {
		if got[id] != n {
			t.Errorf("%s unread %d, want %d", id, got[id], n)
		}
	}
}

func TestThreadsByView(t *testing.T) {
	s := newTestStore(t)
	cases := []struct {
		account, key, query string
		want                []string
	}{
		{"all", "inbox", "", []string{"t1", "t2", "t4", "t3", "t5", "t7"}},
		{"a1", "a1-receipts", "", []string{"t5"}},
		{"a1", "a1-sent", "", []string{"t6"}},
		{"all", "starred", "", []string{"t3"}},
		{"a2", "a2-inbox", "", []string{"t1", "t4", "t7"}},
		{"all", "inbox", "lisbon", []string{"t2"}},
		{"all", "inbox", "MAYA", []string{"t1"}},
		{"all", "inbox", "brand guide", []string{"t7"}},
		{"all", "inbox", `"unbalanced`, []string{}},
		{"all", "inbox", `zzz* OR -qqq NEAR`, []string{}},
	}
	for _, c := range cases {
		got, err := s.Threads(c.account, c.key, c.query)
		if err != nil {
			t.Errorf("%s/%s %q: %v", c.account, c.key, c.query, err)
			continue
		}
		if !slices.Equal(ids(got), c.want) {
			t.Errorf("%s/%s %q: got %v, want %v", c.account, c.key, c.query, ids(got), c.want)
		}
	}
	if list, _ := s.Threads("all", "inbox", ""); list[0].Messages != nil || len(list[0].Participants) != 2 {
		t.Errorf("list rows carry summaries only: %+v", list[0])
	}
	if _, err := s.Threads("a1", "a2-inbox", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("another account's mailbox: %v, want ErrNotFound", err)
	}
}

func TestSetFlags(t *testing.T) {
	s := newTestStore(t)
	yes, no := true, false
	if err := s.SetFlags("t2", nil, &yes); err != nil {
		t.Fatal(err)
	}
	s.SetFlags("t3", nil, &no)
	if got, _ := s.Threads("all", "starred", ""); !slices.Equal(ids(got), []string{"t2"}) {
		t.Errorf("starred after flags: %v", ids(got))
	}
	s.SetFlags("t1", &no, nil)
	if got := unreadByMailbox(t, s); got["a2-inbox"] != 1 || got["a2-projects-inboxed"] != 0 {
		t.Errorf("reading t1 should lower both of its mailboxes: %v", got)
	}
	if err := s.SetFlags("nope", &yes, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing thread: %v", err)
	}
}

func TestMove(t *testing.T) {
	s := newTestStore(t)
	boxes := func(id string) []string {
		th, err := s.Thread(id)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(th.MailboxIDs)
		return th.MailboxIDs
	}
	steps := []struct {
		id, to string
		want   []string
	}{
		{"t2", "archive", []string{"a1-archive", "a1-travel-2026"}},
		{"t2", "inbox", []string{"a1-inbox", "a1-travel-2026"}},
		{"t4", "trash", []string{"a2-trash"}},
		{"t4", "inbox", []string{"a2-inbox"}},
	}
	for _, st := range steps {
		if err := s.Move(st.id, st.to); err != nil {
			t.Fatalf("%s → %s: %v", st.id, st.to, err)
		}
		if got := boxes(st.id); !slices.Equal(got, st.want) {
			t.Errorf("%s → %s: %v, want %v", st.id, st.to, got, st.want)
		}
	}
	if err := s.Move("t4", "nowhere"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad target: %v", err)
	}
	if err := s.Move("nope", "trash"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing thread: %v", err)
	}
}

func TestMarkAllRead(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkAllRead("all", "inbox"); err != nil {
		t.Fatal(err)
	}
	if got := unreadByMailbox(t, s); got["a1-inbox"]+got["a2-inbox"] != 0 {
		t.Errorf("inboxes still unread: %v", got)
	}
}
