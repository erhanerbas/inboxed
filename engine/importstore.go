package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// parsed is what Inboxed keeps of a raw RFC 5322 message (filled by parseMessage).
type parsed struct {
	HeaderID    string // Message-ID, or a content hash when the message has none
	Root        string // thread key: the first message the conversation refers back to
	Subject     string
	From        Address
	To, Cc      []Address
	Date        time.Time
	Text        string
	Attachments []Attachment
	HTML        string   // the HTML part, as sent; rendered only in a sandboxed frame
	Refs        []string // every Message-ID it refers to (References, then In-Reply-To), for threading
	Files       [][]byte // attachment content, parallel to Attachments
}

// Incoming is one message as the importer found it on the server.
type Incoming struct {
	AccountID, MailboxID string
	UID                  uint32
	Seen, Flagged        bool
	parsed
}

// CreateAccount saves a new IMAP/SMTP account, plus its Starred entry (a flag-based view, not a server folder).
// The password lives in the keychain, never here.
func (s *Store) CreateAccount(a Account, set AccountSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO accounts (id, name, email, provider, color, position, username, imap, smtp)
		VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM accounts), ?, ?, ?)`,
		a.ID, a.Name, a.Email, a.Provider, a.Color, set.Username, toJSON(set.IMAP), toJSON(set.SMTP)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO mailboxes (id, account_id, name, role, parent_id, position)
		VALUES (?, ?, 'Starred', 'starred', NULL, (SELECT COALESCE(MAX(position), -1) + 1 FROM mailboxes))`,
		a.ID+":starred", a.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// AccountSettings returns how to reach an account's servers. Password is always empty.
func (s *Store) AccountSettings(id string) (AccountSettings, error) {
	var set AccountSettings
	var imapJSON, smtpJSON string
	err := s.db.QueryRow(`SELECT name, email, username, imap, smtp FROM accounts WHERE id = ?`, id).
		Scan(&set.Name, &set.Email, &set.Username, &imapJSON, &smtpJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return set, ErrNotFound
	} else if err != nil {
		return set, err
	}
	if err := json.Unmarshal([]byte(imapJSON), &set.IMAP); err != nil {
		return set, err
	}
	return set, json.Unmarshal([]byte(smtpJSON), &set.SMTP)
}

// DeleteAccount removes an account and, by cascade, its folders, threads, messages, locations and search rows.
func (s *Store) DeleteAccount(id string) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// IMAPAccounts lists the accounts that import over IMAP (demo accounts don't).
func (s *Store) IMAPAccounts() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM accounts WHERE provider = 'imap' ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpsertMailbox records a server folder and returns its id. A folder whose parent wasn't listed (Gmail's [Gmail]
// is \Noselect) is top-level. When UIDVALIDITY changed the server has renumbered the folder, so its remembered UIDs
// are forgotten and it is imported again.
func (s *Store) UpsertMailbox(accountID, path, name string, role *string, parentPath string, uidValidity uint32) (string, error) {
	id := accountID + "/" + path
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var parent *string
	if parentPath != "" {
		p := accountID + "/" + parentPath
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM mailboxes WHERE id = ?`, p).Scan(&n); err != nil {
			return "", err
		}
		if n > 0 {
			parent = &p
		}
	}
	var old int64
	err = tx.QueryRow(`SELECT uid_validity FROM mailboxes WHERE id = ?`, id).Scan(&old)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.Exec(`INSERT INTO mailboxes (id, account_id, name, role, parent_id, position, path, uid_validity)
			VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM mailboxes), ?, ?)`,
			id, accountID, name, role, parent, path, uidValidity)
	case err != nil:
		return "", err
	default:
		if old != int64(uidValidity) {
			if _, err := tx.Exec(`DELETE FROM message_locations WHERE mailbox_id = ?`, id); err != nil {
				return "", err
			}
		}
		_, err = tx.Exec(`UPDATE mailboxes SET name = ?, role = ?, parent_id = ?, uid_validity = ? WHERE id = ?`,
			name, role, parent, uidValidity, id)
	}
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// KnownUIDs is the set of a folder's UIDs already imported.
func (s *Store) KnownUIDs(mailboxID string) (map[uint32]bool, error) {
	rows, err := s.db.Query(`SELECT uid FROM message_locations WHERE mailbox_id = ?`, mailboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[uint32]bool{}
	for rows.Next() {
		var uid uint32
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		known[uid] = true
	}
	return known, rows.Err()
}

// AddMessage files one imported message: it joins (or starts) its thread and records where it lives on the server.
// The same message found in another folder (a Sent copy, a Gmail label) only gains another location.
func (s *Store) AddMessage(in Incoming) error {
	msgID := in.AccountID + "/" + in.HeaderID
	for i := range in.Attachments { // outside the transaction: a file nobody refers to is cleaned at the next start
		if i < len(in.Files) {
			hash, err := s.saveFile(in.Files[i])
			if err != nil {
				return err
			}
			in.Attachments[i].Hash = hash
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var threadID string
	err = tx.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, msgID).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		if threadID, err = joinThread(tx, in, msgID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO messages (id, thread_id, sender, recipients, cc, date, body, attachments, header_id, subject, html)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			msgID, threadID, toJSON(in.From), toJSON(orEmpty(in.To)), toJSON(orEmpty(in.Cc)), in.Date.UTC().Format(isoDate),
			in.Text, toJSON(orEmpty(in.Attachments)), in.HeaderID, in.Subject, in.HTML); err != nil {
			return err
		}
		for _, ref := range in.Refs {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO message_refs (ref, message_id) VALUES (?, ?)`, in.AccountID+"/"+ref, msgID); err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO message_locations (mailbox_id, uid, message_id, seen, flagged) VALUES (?, ?, ?, ?, ?)`,
		in.MailboxID, in.UID, msgID, in.Seen, in.Flagged); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO thread_mailboxes (thread_id, mailbox_id) VALUES (?, ?)`, threadID, in.MailboxID); err != nil {
		return err
	}
	// A thread is unread while any copy of any of its messages is unseen, and starred while any is flagged.
	if _, err := tx.Exec(`UPDATE threads SET unread = unread OR ?, starred = starred OR ? WHERE id = ?`,
		!in.Seen, in.Flagged, threadID); err != nil {
		return err
	}
	if err := syncThread(tx, threadID); err != nil {
		return err
	}
	if err := refreshThread(tx, threadID); err != nil {
		return err
	}
	return tx.Commit()
}

// joinThread picks the thread a new message belongs to, by every link it has to stored mail: its root's thread, the
// threads of stored messages it refers to, and of stored messages that refer to it (a reply that arrived first). So a
// message joins its conversation even when a mailing list trimmed its References or its original came later. When it
// links several threads, they're one conversation: the others merge into the first. With no link, it starts
// <account>/<root>.
func joinThread(tx *sql.Tx, in Incoming, msgID string) (string, error) {
	var linked []string
	collect := func(query string, args ...any) error {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			if !slices.Contains(linked, id) {
				linked = append(linked, id)
			}
		}
		return rows.Err()
	}
	root := in.AccountID + "/" + in.Root
	if err := collect(`SELECT id FROM threads WHERE id = ?`, root); err != nil {
		return "", err
	}
	if len(in.Refs) > 0 {
		refs := make([]any, len(in.Refs))
		for i, r := range in.Refs {
			refs[i] = in.AccountID + "/" + r
		}
		in := `(?` + strings.Repeat(", ?", len(refs)-1) + `)`
		if err := collect(`SELECT thread_id FROM messages WHERE id IN `+in, refs...); err != nil {
			return "", err
		}
		// Siblings: stored messages answering the same (perhaps never imported) original.
		if err := collect(`SELECT m.thread_id FROM message_refs r JOIN messages m ON m.id = r.message_id WHERE r.ref IN `+in, refs...); err != nil {
			return "", err
		}
	}
	if err := collect(`SELECT m.thread_id FROM message_refs r JOIN messages m ON m.id = r.message_id WHERE r.ref = ?`, msgID); err != nil {
		return "", err
	}
	// A conversation with a change still on its way to the server isn't merged or joined: the change was made on
	// the mail it had (a trash must not take other mail along). The links still count once it's sent.
	var free []string
	for _, id := range linked {
		var pending bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM pending_changes WHERE thread_id = ?)`, id).Scan(&pending); err != nil {
			return "", err
		}
		if !pending {
			free = append(free, id)
		}
	}
	if len(free) == 0 {
		if slices.Contains(linked, root) {
			return root, nil // its own root conversation is busy; joining it is still right
		}
		_, err := tx.Exec(`INSERT INTO threads (id, account_id, subject, snippet, participants, last_date, has_attachments)
			VALUES (?, ?, ?, '', '[]', '', 0)`, root, in.AccountID, in.Subject)
		return root, err
	}
	return free[0], mergeThreads(tx, free[0], free[1:])
}

// mergeThreads moves threads' messages, folders and queued changes into one, then deletes them (and, by trigger,
// their search rows). Flags combine; syncThread re-derives them from the server where it can.
func mergeThreads(tx *sql.Tx, into string, from []string) error {
	for _, f := range from {
		for _, q := range []string{
			`UPDATE messages SET thread_id = ?1 WHERE thread_id = ?2`,
			`INSERT OR IGNORE INTO thread_mailboxes (thread_id, mailbox_id) SELECT ?1, mailbox_id FROM thread_mailboxes WHERE thread_id = ?2`,
			`UPDATE pending_changes SET thread_id = ?1 WHERE thread_id = ?2`,
			`UPDATE threads SET unread = unread OR (SELECT unread FROM threads WHERE id = ?2),
				starred = starred OR (SELECT starred FROM threads WHERE id = ?2) WHERE id = ?1`,
			`DELETE FROM threads WHERE id = ?2`,
		} {
			if _, err := tx.Exec(q, into, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// refreshThread recomputes a thread's summary fields and search row from its stored messages. Its subject is the
// earliest message's, so a thread reads "Plans", not "Re: Plans".
func refreshThread(tx *sql.Tx, id string) error {
	var doc int64
	var me string
	if err := tx.QueryRow(`SELECT t.doc, a.email FROM threads t JOIN accounts a ON a.id = t.account_id WHERE t.id = ?`, id).
		Scan(&doc, &me); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT sender, recipients, cc, date, body, attachments, subject FROM messages
		WHERE thread_id = ? ORDER BY date`, id)
	if err != nil {
		return err
	}
	var msgs []Message
	var subject string
	attached := false
	var people, bodies []string
	for rows.Next() {
		var m Message
		var from, to, cc, attachments, subj string
		if err := rows.Scan(&from, &to, &cc, &m.Date, &m.Body, &attachments, &subj); err != nil {
			rows.Close()
			return err
		}
		json.Unmarshal([]byte(from), &m.From)
		json.Unmarshal([]byte(to), &m.To)
		json.Unmarshal([]byte(cc), &m.Cc)
		attached = attached || attachments != "[]"
		if subject == "" {
			subject = subj
		}
		for _, a := range append(append([]Address{m.From}, m.To...), m.Cc...) {
			people = append(people, a.Name, a.Email)
		}
		bodies = append(bodies, m.Body)
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(msgs) == 0 {
		return err
	}
	last := msgs[len(msgs)-1]
	if _, err := tx.Exec(`UPDATE threads SET subject = COALESCE(NULLIF(?, ''), subject), snippet = ?, participants = ?,
		last_date = ?, has_attachments = ? WHERE id = ?`,
		subject, snippet(last.Body), toJSON(participants(msgs, me)), last.Date, attached, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM search WHERE rowid = ?`, doc); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO search (rowid, subject, people, body) VALUES (?, ?, ?, ?)`,
		doc, subject, strings.Join(people, " "), strings.Join(bodies, "\n"))
	return err
}

// AddLocation records that a message already stored also lives in another folder (a Gmail label, All Mail), without
// downloading it again. It reports false when the message isn't stored yet, so its body must be fetched.
func (s *Store) AddLocation(accountID, mailboxID string, uid uint32, headerID string, seen, flagged bool) (bool, error) {
	msgID := accountID + "/" + headerID
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var threadID string
	err = tx.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, msgID).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO message_locations (mailbox_id, uid, message_id, seen, flagged) VALUES (?, ?, ?, ?, ?)`,
		mailboxID, uid, msgID, seen, flagged); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO thread_mailboxes (thread_id, mailbox_id) VALUES (?, ?)`, threadID, mailboxID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE threads SET unread = unread OR ?, starred = starred OR ? WHERE id = ?`,
		!seen, flagged, threadID); err != nil {
		return false, err
	}
	if err := syncThread(tx, threadID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
