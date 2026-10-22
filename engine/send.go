package main

import (
	"bytes"
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/zalando/go-keyring"
)

// savesSentItself: these providers file what their SMTP server sends in Sent on their own; a copy would be a duplicate.
func savesSentItself(host string) bool {
	host = strings.ToLower(host)
	for _, d := range []string{"gmail.com", "googlemail.com", "outlook.com", "office365.com"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (sy *Syncer) credentials(accountID string) (AccountSettings, string, error) {
	set, err := sy.store.AccountSettings(accountID)
	if err != nil {
		return set, "", err
	}
	pw, err := keyring.Get(keyringService, accountID)
	if err != nil {
		return set, "", problem("Inboxed couldn't read this account's password from the keychain. Remove the account and add it again.")
	}
	return set, pw, nil
}

// file appends a message to the account's folder with that role and returns its message id. When the server says
// where it went (APPENDUID) it's stored here at once; otherwise the next sync brings it. ErrNotFound: no such folder.
func (sy *Syncer) file(accountID, role string, raw []byte, flags ...imap.Flag) (string, error) {
	var mailboxID, path string
	err := sy.store.db.QueryRow(`SELECT id, path FROM mailboxes WHERE account_id = ? AND role = ? AND path IS NOT NULL
		ORDER BY position LIMIT 1`, accountID, role).Scan(&mailboxID, &path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	set, pw, err := sy.credentials(accountID)
	if err != nil {
		return "", err
	}
	c, err := loginIMAP(set.IMAP, set.Username, pw, nil)
	if err != nil {
		return "", err
	}
	stalled := time.AfterFunc(idleTimeout, func() { c.Close() }) // go-imap doesn't time out commands
	defer func() { stalled.Stop(); c.Logout().Wait(); c.Close() }()
	now := time.Now()
	cmd := c.Append(path, int64(len(raw)), &imap.AppendOptions{Flags: flags, Time: now})
	if _, err := cmd.Write(raw); err != nil {
		return "", err
	}
	if err := cmd.Close(); err != nil {
		return "", err
	}
	data, err := cmd.Wait()
	if err != nil {
		return "", err
	}
	p, err := parseMessage(raw, now)
	if err != nil {
		return "", err
	}
	// The server has it now, so this can't fail any more: if it can't be stored here, the next sync brings it.
	if data == nil || data.UID == 0 {
		sy.Wake(accountID)
	} else if err := sy.store.AddMessage(Incoming{AccountID: accountID, MailboxID: mailboxID, UID: uint32(data.UID),
		Seen: true, parsed: p}); err != nil {
		log.Printf("file %s: stored on the server, not here yet: %v", accountID, err)
		sy.Wake(accountID)
	}
	return accountID + "/" + p.HeaderID, nil
}

// Send sends over SMTP and returns once the server accepted it. Filing a copy in Sent and deleting the draft it came
// from happen in the background and are only logged: the mail is sent, and reporting anything else as a failure, or
// keeping the panel waiting on it, would invite sending twice.
func (sy *Syncer) Send(d Draft) error {
	out, err := sy.store.Compose(d, time.Now(), false)
	if err != nil {
		return err
	}
	set, pw, err := sy.credentials(d.AccountID)
	if err != nil {
		return err
	}
	c, err := dialSMTP(set.SMTP, set.Username, pw)
	if err != nil {
		return err
	}
	if err := c.SendMail(out.From, out.Recipients, bytes.NewReader(out.Raw)); err != nil {
		c.Close()
		return problem(fmt.Sprintf("%s didn't accept the message: %v", set.SMTP.Host, err))
	}
	c.Quit()
	sy.running.Go(func() {
		if savesSentItself(set.SMTP.Host) {
			sy.Wake(d.AccountID)
		} else if _, err := sy.file(d.AccountID, "sent", out.SentCopy, imap.FlagSeen); err != nil && !errors.Is(err, ErrNotFound) {
			log.Printf("send %s: sent, but filing a copy in Sent failed: %v", d.AccountID, err)
		}
		if d.DraftID != "" {
			if err := sy.DeleteDraft(d.DraftID); err != nil {
				log.Printf("send %s: sent, but deleting the draft failed: %v", d.AccountID, err)
			}
		}
		sy.onChange()
	})
	return nil
}

// SaveDraft saves a draft in the server's Drafts folder, replacing the version it was opened from, and returns its id.
func (sy *Syncer) SaveDraft(d Draft) (string, error) {
	out, err := sy.store.Compose(d, time.Now(), true)
	if err != nil {
		return "", err
	}
	id, err := sy.file(d.AccountID, "drafts", out.Raw, imap.FlagDraft, imap.FlagSeen)
	if errors.Is(err, ErrNotFound) {
		return "", problem("This account has no Drafts folder on the server.")
	} else if err != nil {
		return "", err
	}
	if d.DraftID != "" {
		if err := sy.DeleteDraft(d.DraftID); err != nil {
			log.Printf("draft %s: saved, but deleting the previous version failed: %v", d.AccountID, err)
		}
	}
	sy.onChange()
	return id, nil
}

// DeleteDraft removes a draft from the server and from here. Without UIDPLUS it's only flagged \Deleted, since a
// plain EXPUNGE could remove other clients' deleted mail.
func (sy *Syncer) DeleteDraft(id string) error {
	accountID, _, _ := strings.Cut(id, "/")
	_, path, uid, err := sy.store.Copy(id, "drafts")
	if errors.Is(err, ErrNotFound) {
		// Not in Drafts on the server. Only a draft not stored there yet (no locations at all) is deleted here; mail
		// anywhere else is never touched through this.
		if located, err := sy.store.Located(id); err != nil || located {
			return cmp.Or(err, error(problem("That message isn't a draft.")))
		}
		return sy.store.DeleteMessage(id)
	} else if err != nil {
		return err
	}
	set, pw, err := sy.credentials(accountID)
	if err != nil {
		return err
	}
	c, err := loginIMAP(set.IMAP, set.Username, pw, nil)
	if err != nil {
		return err
	}
	stalled := time.AfterFunc(idleTimeout, func() { c.Close() })
	defer func() { stalled.Stop(); c.Logout().Wait(); c.Close() }()
	if _, err := c.Select(path, nil).Wait(); err != nil {
		return err
	}
	set1 := uidSet([]uint32{uid})
	if err := c.Store(set1, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return err
	}
	if c.Caps().Has(imap.CapUIDPlus) {
		if err := c.UIDExpunge(set1).Close(); err != nil {
			return err
		}
	}
	if err := sy.store.DeleteMessage(id); err != nil {
		return err
	}
	sy.onChange()
	return nil
}
