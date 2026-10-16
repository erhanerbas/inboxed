package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
)

// mailAccount syncs the test server's account with a Syncer that records every batch of new mail it reports.
func mailAccount(t *testing.T) (got func() [][]NewMail, acc string, deliver func(raw string, flags ...imap.Flag)) {
	t.Helper()
	imapSrv, smtpSrv, user := testServers(t)
	s := newTestStore(t)
	var mu sync.Mutex
	var batches [][]NewMail
	sy := NewSyncer(s, func() {}, func(b []NewMail) {
		mu.Lock()
		batches = append(batches, b)
		mu.Unlock()
	})
	t.Cleanup(func() { sy.Stop(acc); sy.running.Wait() })
	a, err := sy.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	acc = a.ID
	waitImport(t, sy, acc)
	got = func() [][]NewMail {
		mu.Lock()
		defer mu.Unlock()
		return append([][]NewMail(nil), batches...)
	}
	return got, acc, func(raw string, flags ...imap.Flag) {
		appendRaw(t, user, "INBOX", raw, flags...)
		sy.Start(acc)
	}
}

func TestFirstSyncDoesNotNotify(t *testing.T) {
	got, _, _ := mailAccount(t) // INBOX already holds unread mail when the account is added
	time.Sleep(300 * time.Millisecond)
	if b := got(); len(b) != 0 {
		t.Errorf("importing history notified: %+v", b)
	}
}

func TestNewMailIsReported(t *testing.T) {
	got, acc, deliver := mailAccount(t)
	deliver(newsletter)
	eventually(t, "the new mail is reported", func() bool { return len(got()) == 1 })
	b := got()[0]
	if len(b) != 1 || b[0].From != "News" || b[0].Subject != "This week" || b[0].ThreadID != acc+"/n1@example.com" ||
		b[0].AccountID != acc || b[0].Snippet != "Headlines." {
		t.Errorf("reported %+v", b)
	}
}

func TestOwnMailDoesNotNotify(t *testing.T) {
	got, _, deliver := mailAccount(t)
	deliver("From: You <you@example.com>\nTo: You <you@example.com>\nSubject: Note to self\nMessage-ID: <self@example.com>\n\nRemember.\n")
	deliver(strings.Replace(newsletter, "n1@", "n2@", 1), imap.FlagSeen)
	deliver(strings.Replace(newsletter, "n1@", "n3@", 1)) // a marker: once this is reported, the others weren't
	eventually(t, "the marker is reported", func() bool {
		for _, b := range got() {
			for _, m := range b {
				if strings.HasSuffix(m.ThreadID, "/n3@example.com") {
					return true
				}
			}
		}
		return false
	})
	for _, b := range got() {
		for _, m := range b {
			if !strings.HasSuffix(m.ThreadID, "/n3@example.com") {
				t.Errorf("reported %+v", m)
			}
		}
	}
}

func TestEventsCarryMail(t *testing.T) {
	ev := NewEvents()
	srv := httptest.NewServer(ev)
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	lines := make(chan string, 20)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	<-lines // ": connected"
	<-lines
	ev.Mail([]NewMail{{ThreadID: "acc/x", From: "Maya", Subject: "Hi"}})
	next := func() string {
		select {
		case l := <-lines:
			return l
		case <-time.After(2 * time.Second):
			return "(nothing)"
		}
	}
	if l := next(); l != "event: mail" {
		t.Fatalf("got %q", l)
	}
	var batch []NewMail
	if err := json.Unmarshal([]byte(strings.TrimPrefix(next(), "data: ")), &batch); err != nil || len(batch) != 1 || batch[0].From != "Maya" {
		t.Errorf("data %+v (%v)", batch, err)
	}
}
