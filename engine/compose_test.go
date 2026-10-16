package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestBuildReplyThreads(t *testing.T) {
	s, _, _ := syncedThread(t)
	out, err := s.Compose(Draft{AccountID: "acc", To: "Maya <maya@x>", Subject: "Re: Plans", Body: "Sounds good", ThreadID: "acc/a@x"}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(out.Raw)
	if !strings.Contains(raw, "In-Reply-To: <b@x>") || !strings.Contains(raw, "References: <a@x> <b@x>") {
		t.Errorf("reply headers missing:\n%s", raw)
	}
	if !slices.Equal(out.Recipients, []string{"maya@x"}) || out.From != "you@example.com" {
		t.Errorf("envelope %q → %v", out.From, out.Recipients)
	}
	if p, err := parseMessage(out.Raw, seedNow); err != nil || p.Root != "a@x" {
		t.Errorf("the reply's thread root is %q (%v), want a@x", p.Root, err)
	}
}

func TestBuildEncodesUnicode(t *testing.T) {
	s, _, _ := syncedThread(t)
	out, err := s.Compose(Draft{AccountID: "acc", To: "José <jose@x>", Subject: "Café ☕", Body: "Grüße 😀"}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range out.Raw {
		if b > 127 {
			t.Fatalf("raw message isn't ASCII:\n%s", out.Raw)
		}
	}
	p, err := parseMessage(out.Raw, seedNow)
	if err != nil || p.Subject != "Café ☕" || strings.TrimSpace(p.Text) != "Grüße 😀" || len(p.To) != 1 || p.To[0].Name != "José" {
		t.Errorf("read back: %q %q %+v (%v)", p.Subject, p.Text, p.To, err)
	}
}

func TestBuildRejectsBadInput(t *testing.T) {
	s, _, _ := syncedThread(t)
	_, err := s.Compose(Draft{AccountID: "acc", To: "maya@x, not an address", Subject: "Hi"}, seedNow, false)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "Check the address “not an address”") {
		t.Errorf("bad address: %v", err)
	}
	if _, err := s.Compose(Draft{AccountID: "acc", Subject: "Hi"}, seedNow, false); !errors.Is(err, ErrInvalid) {
		t.Errorf("no recipients: %v", err)
	}
	if _, err := s.Compose(Draft{AccountID: "acc", Subject: "Unfinished"}, seedNow, true); err != nil {
		t.Errorf("a draft may have no recipients yet: %v", err)
	}
	if _, err := s.Compose(Draft{AccountID: "a1", To: "maya@x"}, seedNow, false); !errors.Is(err, ErrInvalid) || err.Error() != "Demo accounts can't send mail." {
		t.Errorf("demo account: %v", err)
	}
}

func TestBuildNewMessageHasNoThreadHeaders(t *testing.T) {
	s, _, _ := syncedThread(t)
	out, err := s.Compose(Draft{AccountID: "acc", To: "maya@x", Subject: "Hello"}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(out.Raw)
	if strings.Contains(raw, "In-Reply-To") || strings.Contains(raw, "References") {
		t.Errorf("a new message has thread headers:\n%s", raw)
	}
	if p, _ := parseMessage(out.Raw, seedNow); !strings.HasSuffix(p.HeaderID, "@example.com") {
		t.Errorf("Message-ID %q", p.HeaderID)
	}
}
