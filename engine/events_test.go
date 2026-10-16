package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEventsStream(t *testing.T) {
	ev := NewEvents()
	srv := httptest.NewServer(ev)
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type %q", ct)
	}
	lines := make(chan string, 10)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	next := func() string {
		select {
		case l := <-lines:
			return l
		case <-time.After(2 * time.Second):
			return "(nothing)"
		}
	}
	if l := next(); !strings.HasPrefix(l, ":") {
		t.Fatalf("first line %q, want the connected comment", l)
	}
	next() // blank line after the comment
	ev.Publish()
	ev.Publish() // a burst while the first is unsent is one event
	if l := next(); l != "event: change" {
		t.Errorf("got %q, want event: change", l)
	}
}
