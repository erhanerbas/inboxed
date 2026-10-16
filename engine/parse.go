package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io"
	"regexp"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset" // decodes non-UTF-8 bodies and headers
	"github.com/emersion/go-message/mail"
)

// parseMessage keeps what Inboxed shows of a raw message: headers, plain text (or HTML reduced to text) and
// attachment names and sizes. fallbackDate (the server's INTERNALDATE) stands in for a missing or broken Date.
func parseMessage(raw []byte, fallbackDate time.Time) (parsed, error) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return parsed{}, err
	}
	h := r.Header
	var p parsed
	p.Subject, _ = h.Subject()
	p.HeaderID, _ = h.MessageID()
	if p.HeaderID == "" {
		sum := sha256.Sum256(raw)
		p.HeaderID = "sha256:" + hex.EncodeToString(sum[:12])
	}
	refs, _ := h.MsgIDList("References")
	replyTo, _ := h.MsgIDList("In-Reply-To")
	p.Root = cmp.Or(first(refs), first(replyTo), p.HeaderID)
	if from := addresses(h, "From"); len(from) > 0 {
		p.From = from[0]
	}
	p.To, p.Cc = addresses(h, "To"), addresses(h, "Cc")
	if p.Date, err = h.Date(); err != nil || p.Date.IsZero() {
		p.Date = fallbackDate
	}

	var plain, htmlParts []string
	for {
		part, err := r.NextPart()
		if err != nil {
			break // io.EOF, or a broken part: keep what was read so far
		}
		switch ph := part.Header.(type) {
		case *mail.InlineHeader:
			ct, params, _ := ph.ContentType()
			if !strings.HasPrefix(ct, "text/") {
				if name := params["name"]; name != "" { // an inline image or file sent without "attachment"
					n, _ := io.Copy(io.Discard, part.Body)
					p.Attachments = append(p.Attachments, Attachment{Name: name, Size: n})
				}
				continue
			}
			b, _ := io.ReadAll(part.Body)
			switch ct {
			case "text/plain":
				plain = append(plain, string(b))
			case "text/html":
				htmlParts = append(htmlParts, string(b))
			}
		case *mail.AttachmentHeader:
			name, _ := ph.Filename()
			n, _ := io.Copy(io.Discard, part.Body)
			p.Attachments = append(p.Attachments, Attachment{Name: cmp.Or(name, "attachment"), Size: n})
		}
	}
	switch {
	case len(plain) > 0:
		p.Text = strings.Join(plain, "\n")
	case len(htmlParts) > 0:
		p.Text = htmlToText(strings.Join(htmlParts, "\n"))
	}
	p.Text = strings.TrimSpace(strings.ReplaceAll(p.Text, "\r\n", "\n"))
	return p, nil
}

func first(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func addresses(h mail.Header, key string) []Address {
	list, _ := h.AddressList(key)
	out := []Address{}
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Email: a.Address})
	}
	return out
}

var (
	htmlDrop  = regexp.MustCompile(`(?is)<(script|style|head)\b.*?</(script|style|head)>`)
	htmlBreak = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|tr|h[1-6]|blockquote)>`)
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	blankRuns = regexp.MustCompile(`\n{3,}`)
)

// htmlToText stands in for HTML-only mail until Phase 6 renders HTML in a sandbox: words and paragraph breaks only.
func htmlToText(s string) string {
	s = htmlDrop.ReplaceAllString(s, "")
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = html.UnescapeString(htmlTag.ReplaceAllString(s, ""))
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
