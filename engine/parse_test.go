package main

import (
	"strings"
	"testing"
	"time"
)

var fallback = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func crlf(s string) []byte { return []byte(strings.ReplaceAll(s, "\n", "\r\n")) }

func TestParseMultipartLatin1WithAttachment(t *testing.T) {
	raw := crlf("From: Maya Chen <maya@example.com>\nTo: You <you@example.com>, Sam <sam@example.com>\nCc: Priya <priya@example.com>\n" +
		"Subject: =?UTF-8?Q?Caf=C3=A9_plans?=\nMessage-ID: <a1@example.com>\nDate: Thu, 15 Oct 2026 10:00:00 +0000\n" +
		"MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain; charset=iso-8859-1\n\n" +
		"Caf\xe9 at 8?\n--b\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=menu.pdf\n\n%PDF-1.4 tiny\n--b--\n")
	p, err := parseMessage(raw, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Café plans" || p.Text != "Café at 8?" || p.HeaderID != "a1@example.com" || p.Root != "a1@example.com" {
		t.Errorf("parsed %+v", p)
	}
	if p.From != (Address{"Maya Chen", "maya@example.com"}) || len(p.To) != 2 || len(p.Cc) != 1 || !p.Date.Equal(time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("addresses or date: %+v", p)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Name != "menu.pdf" || p.Attachments[0].Size != 13 {
		t.Errorf("attachments %+v", p.Attachments)
	}
}

func TestParseReplyThreadsToTheFirstReference(t *testing.T) {
	raw := crlf("From: You <you@example.com>\nSubject: Re: Plans\nMessage-ID: <c3@example.com>\nIn-Reply-To: <b2@example.com>\n" +
		"References: <a1@example.com> <b2@example.com>\nContent-Type: text/plain\n\nSounds good.\n")
	p, _ := parseMessage(raw, fallback)
	if p.Root != "a1@example.com" {
		t.Errorf("root %q, want the first reference", p.Root)
	}
	raw = crlf("From: You <you@example.com>\nMessage-ID: <c4@example.com>\nIn-Reply-To: <b2@example.com>\n\nOK\n")
	if p, _ := parseMessage(raw, fallback); p.Root != "b2@example.com" {
		t.Errorf("root %q, want In-Reply-To when there are no References", p.Root)
	}
}

func TestParseHTMLOnly(t *testing.T) {
	raw := crlf("From: Shop <shop@example.com>\nSubject: Receipt\nMessage-ID: <r@example.com>\nContent-Type: text/html; charset=utf-8\n\n" +
		"<html><head><style>p{color:red}</style></head><body><p>Hello&nbsp;<b>there</b></p><p>Line two</p><script>x()</script></body></html>\n")
	p, _ := parseMessage(raw, fallback)
	if p.Text != "Hello there\nLine two" {
		t.Errorf("text %q", p.Text)
	}
}

func TestParseWithoutMessageIDOrDate(t *testing.T) {
	raw := crlf("From: someone@example.com\nSubject: Hi\n\nHello\n")
	p, err := parseMessage(raw, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.HeaderID, "sha256:") || p.Root != p.HeaderID || !p.Date.Equal(fallback) || p.From.Email != "someone@example.com" {
		t.Errorf("parsed %+v", p)
	}
	if again, _ := parseMessage(raw, fallback); again.HeaderID != p.HeaderID {
		t.Error("the stand-in id must be stable, or a re-import duplicates the message")
	}
}
