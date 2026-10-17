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
// deliver appends to INBOX and wakes the sync; put appends to any folder without waking it; wake runs a pass.
func mailAccount(t *testing.T) (got func() [][]NewMail, acc string, deliver func(raw string, flags ...imap.Flag)) {
	got, acc, put, wake := mailAccountWith(t)
	return got, acc, func(raw string, flags ...imap.Flag) { put("INBOX", raw, flags...); wake() }
}

func mailAccountWith(t *testing.T) (got func() [][]NewMail, acc string, put func(folder, raw string, flags ...imap.Flag), wake func()) {
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
	return got, acc, func(folder, raw string, flags ...imap.Flag) { appendRaw(t, user, folder, raw, flags...) }, func() { sy.Start(acc) }
}

func reported(got func() [][]NewMail) (n int) {
	for _, b := range got() {
		n += len(b)
	}
	return n
}

// Two copies of one message arriving together (on the list and in Cc) are one notification.
func TestCopiesNotifyOnce(t *testing.T) {
	got, _, put, wake := mailAccountWith(t)
	put("INBOX", newsletter)
	put("INBOX", newsletter)
	wake()
	eventually(t, "reported", func() bool { return reported(got) > 0 })
	time.Sleep(300 * time.Millisecond)
	if n := reported(got); n != 1 {
		t.Errorf("%d notifications for one message", n)
	}
}

// Mail that reached another folder first (Gmail's All Mail, a filing rule) still notifies when it reaches the inbox.
func TestStoredElsewhereFirstStillNotifies(t *testing.T) {
	got, acc, put, wake := mailAccountWith(t)
	put("Archive", newsletter)
	wake()
	time.Sleep(500 * time.Millisecond) // a pass stores it from Archive: not new mail yet
	if n := reported(got); n != 0 {
		t.Fatalf("mail outside the inbox notified: %d", n)
	}
	put("INBOX", newsletter)
	wake()
	eventually(t, "it notifies once it's in the inbox", func() bool { return reported(got) == 1 })
	if m := got()[0][0]; m.ThreadID != acc+"/n1@example.com" || m.Subject != "This week" {
		t.Errorf("reported %+v", m)
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

// Mail found before any UI is listening (the first pass at launch) waits for the first one that connects.
func TestMailWaitsForTheFirstListener(t *testing.T) {
	ev := NewEvents()
	ev.Mail([]NewMail{{ThreadID: "acc/early", From: "Maya"}})
	srv := httptest.NewServer(ev)
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	deadline := time.After(2 * time.Second)
	lines := make(chan string)
	go func() {
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	for {
		select {
		case l := <-lines:
			if strings.Contains(l, "acc/early") {
				return
			}
		case <-deadline:
			t.Fatal("early mail never reached the first listener")
		}
	}
}
