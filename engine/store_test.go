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
