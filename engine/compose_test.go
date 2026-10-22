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

// The compose panel quotes a name only when it must, so plain names with accents, apostrophes and initials parse as is.
func TestRecipientsWithUnquotedNames(t *testing.T) {
	got, err := recipients("Seán O'Brien-Núñez Jr. <sean@x>, J. R. Smith <jr@x>, \"Chen, Maya\" <maya@x>")
	if err != nil || len(got) != 3 || got[0].Name != "Seán O'Brien-Núñez Jr." || got[2].Address != "maya@x" {
		t.Fatalf("%v, %+v", err, got)
	}
}

func TestBccOnlyInEnvelope(t *testing.T) {
	s, _, _ := syncedThread(t)
	out, err := s.Compose(Draft{AccountID: "acc", To: "a@x", Bcc: "Hidden <h@x>", Body: "Hi"}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Recipients, []string{"a@x", "h@x"}) || strings.Contains(string(out.Raw), "Bcc") {
		t.Errorf("recipients %v; the sent message must not name Bcc:\n%s", out.Recipients, out.Raw)
	}
	sent, err := parseMessage(out.SentCopy, seedNow)
	if raw, _ := parseMessage(out.Raw, seedNow); err != nil || len(sent.Bcc) != 1 || sent.HeaderID != raw.HeaderID {
		t.Errorf("the Sent copy names Bcc %v and is the same message (%q vs %q): %v", sent.Bcc, sent.HeaderID, raw.HeaderID, err)
	}
	if _, err := s.Compose(Draft{AccountID: "acc", Bcc: "h@x", Body: "Hi"}, seedNow, false); err != nil {
		t.Errorf("Bcc alone is a recipient: %v", err)
	}
}

func TestDraftKeepsBcc(t *testing.T) {
	s, _, _ := syncedThread(t)
	out, err := s.Compose(Draft{AccountID: "acc", Bcc: "Hidden <h@x>", Body: "Hi"}, seedNow, true)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseMessage(out.Raw, seedNow)
	if err != nil || len(p.Bcc) != 1 || p.Bcc[0] != (Address{"Hidden", "h@x"}) {
		t.Fatalf("draft Bcc %v (%v)", p.Bcc, err)
	}
	drafts, _ := s.UpsertMailbox("acc", "Drafts", "Drafts", ptr("drafts"), "", 1)
	if err := s.AddMessage(Incoming{AccountID: "acc", MailboxID: drafts, UID: 1, parsed: p}); err != nil {
		t.Fatal(err)
	}
	th, err := s.Thread("acc/" + p.Root)
	if err != nil || len(th.Messages) != 1 || len(th.Messages[0].Bcc) != 1 {
		t.Errorf("stored draft lost its Bcc: %+v (%v)", th.Messages, err)
	}
}

func TestHTMLIsAlternative(t *testing.T) {
	s, _, _ := syncedThread(t)
	for _, files := range [][]Attachment{nil, {{Name: "a.txt", Type: "text/plain", Hash: mustSave(t, s, "hello")}}} {
		out, err := s.Compose(Draft{AccountID: "acc", To: "a@x", Body: "Hi there", HTML: "<p>Hi <b>there</b></p>", Attachments: files}, seedNow, false)
		if err != nil {
			t.Fatal(err)
		}
		p, err := parseMessage(out.Raw, seedNow)
		if err != nil || strings.TrimSpace(p.Text) != "Hi there" || !strings.Contains(p.HTML, "<b>there</b>") ||
			len(p.Attachments) != len(files) || !strings.Contains(string(out.Raw), "multipart/alternative") {
			t.Errorf("with %d files: text %q html %q attachments %v (%v)\n%s", len(files), p.Text, p.HTML, p.Attachments, err, out.Raw)
		}
	}
}

func mustSave(t *testing.T, s *Store, data string) string {
	t.Helper()
	hash, err := s.saveFile([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
