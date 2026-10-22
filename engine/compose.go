package main

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	netmail "net/mail"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
)

// Draft is a message being written, as the compose panel sends it. ThreadID makes it a reply; DraftID is the saved
// draft it replaces (send or save deletes that one).
type Draft struct {
	AccountID string `json:"accountId"`
	To        string `json:"to"`
	Cc        string `json:"cc"`
	Bcc       string `json:"bcc"` // sent to, never named in what's sent; a saved draft keeps them
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	HTML      string `json:"html"` // the formatted message; Body is its plain-text version, sent alongside
	ThreadID  string `json:"threadId"`
	DraftID   string `json:"draftId"`
	// Files by content hash: uploads (POST /files) and the stored attachments a forward or a resumed draft carries.
	Attachments []Attachment `json:"attachments"`
}

// Outgoing is a message ready to send: RFC 5322 bytes and the SMTP envelope.
type Outgoing struct {
	Raw        []byte
	SentCopy   []byte // what's filed in Sent: Raw, plus the Bcc header when there is one
	From       string
	Recipients []string
}

// recipients parses what was typed in To or Cc. A bad address is named, so the person can find it.
func recipients(field string) ([]*mail.Address, error) {
	if strings.TrimSpace(field) == "" {
		return nil, nil
	}
	list, err := netmail.ParseAddressList(field)
	if err != nil {
		for part := range strings.SplitSeq(field, ",") {
			if _, err := netmail.ParseAddress(part); err != nil && strings.TrimSpace(part) != "" {
				return nil, problem(fmt.Sprintf("Check the address “%s”.", strings.TrimSpace(part)))
			}
		}
		return nil, problem(fmt.Sprintf("Check the addresses “%s”.", strings.TrimSpace(field)))
	}
	out := make([]*mail.Address, len(list))
	for i, a := range list {
		out[i] = (*mail.Address)(a)
	}
	return out, nil
}

// Compose builds a plain-text message from an account. A reply (ThreadID) answers the thread's newest message that
// isn't a draft: In-Reply-To names it, References starts with the thread's root, so it joins the conversation here
// and in other clients. A draft may be saved before it has recipients.
func (s *Store) Compose(d Draft, now time.Time, draft bool) (Outgoing, error) {
	var name, email, provider string
	err := s.db.QueryRow(`SELECT name, email, provider FROM accounts WHERE id = ?`, d.AccountID).Scan(&name, &email, &provider)
	if errors.Is(err, sql.ErrNoRows) {
		return Outgoing{}, ErrNotFound
	} else if err != nil {
		return Outgoing{}, err
	}
	if provider != "imap" {
		return Outgoing{}, problem("Demo accounts can't send mail.")
	}
	to, err := recipients(d.To)
	if err != nil {
		return Outgoing{}, err
	}
	cc, err := recipients(d.Cc)
	if err != nil {
		return Outgoing{}, err
	}
	bcc, err := recipients(d.Bcc)
	if err != nil {
		return Outgoing{}, err
	}
	if len(to)+len(cc)+len(bcc) == 0 && !draft {
		return Outgoing{}, problem("Add at least one recipient.")
	}

	var h mail.Header
	if name == email {
		name = "" // the account's name defaults to its address: From: "you@x" <you@x> reads oddly, and spam filters notice
	}
	h.SetAddressList("From", []*mail.Address{{Name: name, Address: email}})
	if len(to) > 0 {
		h.SetAddressList("To", to)
	}
	if len(cc) > 0 {
		h.SetAddressList("Cc", cc)
	}
	if len(bcc) > 0 && draft {
		h.SetAddressList("Bcc", bcc)
	}
	h.SetSubject(d.Subject)
	h.SetDate(now)
	_, domain, _ := strings.Cut(email, "@")
	h.SetMessageID(strings.ToLower(rand.Text()) + "@" + domain)
	if d.ThreadID != "" {
		var parent string
		err := s.db.QueryRow(`SELECT header_id FROM messages m WHERE thread_id = ? AND header_id != ''
			AND NOT EXISTS (SELECT 1 FROM message_locations ml JOIN mailboxes mb ON mb.id = ml.mailbox_id
				WHERE ml.message_id = m.id AND mb.role = 'drafts')
			ORDER BY date DESC LIMIT 1`, d.ThreadID).Scan(&parent)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Outgoing{}, err
		}
		_, root, _ := strings.Cut(d.ThreadID, "/")
		refs := []string{root}
		if parent != "" && parent != root {
			refs = append(refs, parent)
		}
		h.SetMsgIDList("In-Reply-To", []string{refs[len(refs)-1]})
		h.SetMsgIDList("References", refs)
	}
	crlf := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n") }
	sentHeader := mail.Header{Header: h.Header.Copy()} // writing the body changes the header it's given
	var buf bytes.Buffer
	if err := s.writeBody(&buf, h, crlf(d.Body), crlf(d.HTML), d.Attachments); err != nil {
		return Outgoing{}, err
	}
	out := Outgoing{Raw: buf.Bytes(), SentCopy: buf.Bytes(), From: email}
	// Your Sent copy shows who you Bcc'd, as in other mail apps; what's transmitted never does.
	if len(bcc) > 0 && !draft {
		sentHeader.SetAddressList("Bcc", bcc)
		var sent bytes.Buffer
		if err := s.writeBody(&sent, sentHeader, crlf(d.Body), crlf(d.HTML), d.Attachments); err != nil {
			return Outgoing{}, err
		}
		out.SentCopy = sent.Bytes()
	}
	for _, a := range slices.Concat(to, cc, bcc) {
		out.Recipients = append(out.Recipients, a.Address)
	}
	return out, nil
}

// writeBody writes the text, or the text and HTML as multipart/alternative, alone or (with files) as the first part
// of multipart/mixed followed by the files.
func (s *Store) writeBody(buf *bytes.Buffer, h mail.Header, body, html string, files []Attachment) error {
	texts := []struct{ kind, content string }{{"text/plain", body}}
	if html != "" {
		texts = append(texts, struct{ kind, content string }{"text/html", html})
	}
	write := func(w io.WriteCloser, content string) error {
		if _, err := io.WriteString(w, content); err != nil {
			return err
		}
		return w.Close()
	}
	part := func(kind string) mail.InlineHeader {
		var ph mail.InlineHeader
		ph.Set("Content-Type", kind+"; charset=utf-8")
		ph.Set("Content-Transfer-Encoding", "quoted-printable")
		return ph
	}
	if len(files) == 0 && html == "" {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := mail.CreateSingleInlineWriter(buf, h)
		if err != nil {
			return err
		}
		return write(w, body)
	}
	if len(files) == 0 {
		iw, err := mail.CreateInlineWriter(buf, h)
		if err != nil {
			return err
		}
		for _, t := range texts {
			w, err := iw.CreatePart(part(t.kind))
			if err == nil {
				err = write(w, t.content)
			}
			if err != nil {
				return err
			}
		}
		return iw.Close()
	}
	mw, err := mail.CreateWriter(buf, h)
	if err != nil {
		return err
	}
	if html == "" {
		tw, err := mw.CreateSingleInline(part("text/plain"))
		if err == nil {
			err = write(tw, body)
		}
		if err != nil {
			return err
		}
	} else {
		iw, err := mw.CreateInline()
		if err != nil {
			return err
		}
		for _, t := range texts {
			w, err := iw.CreatePart(part(t.kind))
			if err == nil {
				err = write(w, t.content)
			}
			if err != nil {
				return err
			}
		}
		if err := iw.Close(); err != nil {
			return err
		}
	}
	for _, f := range files {
		data, err := s.File(f.Hash)
		if err != nil {
			return problem(fmt.Sprintf("Inboxed doesn't have “%s” any more. Remove it and attach it again.", f.Name))
		}
		var ah mail.AttachmentHeader
		ah.Set("Content-Type", cmp.Or(f.Type, "application/octet-stream"))
		ah.Set("Content-Transfer-Encoding", "base64")
		ah.SetFilename(f.Name)
		aw, err := mw.CreateAttachment(ah)
		if err == nil {
			_, err = aw.Write(data)
		}
		if err == nil {
			err = aw.Close()
		}
		if err != nil {
			return err
		}
	}
	return mw.Close()
}
