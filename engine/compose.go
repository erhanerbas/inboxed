package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	netmail "net/mail"
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
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	ThreadID  string `json:"threadId"`
	DraftID   string `json:"draftId"`
}

// Outgoing is a message ready to send: RFC 5322 bytes and the SMTP envelope.
type Outgoing struct {
	Raw        []byte
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
	if len(to)+len(cc) == 0 && !draft {
		return Outgoing{}, problem("Add at least one recipient.")
	}

	var h mail.Header
	h.SetAddressList("From", []*mail.Address{{Name: name, Address: email}})
	if len(to) > 0 {
		h.SetAddressList("To", to)
	}
	if len(cc) > 0 {
		h.SetAddressList("Cc", cc)
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
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Transfer-Encoding", "quoted-printable")

	var buf bytes.Buffer
	w, err := mail.CreateSingleInlineWriter(&buf, h)
	if err != nil {
		return Outgoing{}, err
	}
	body := strings.ReplaceAll(strings.ReplaceAll(d.Body, "\r\n", "\n"), "\n", "\r\n")
	if _, err := w.Write([]byte(body)); err != nil {
		return Outgoing{}, err
	}
	if err := w.Close(); err != nil {
		return Outgoing{}, err
	}
	out := Outgoing{Raw: buf.Bytes(), From: email}
	for _, a := range append(to, cc...) {
		out.Recipients = append(out.Recipients, a.Address)
	}
	return out, nil
}
