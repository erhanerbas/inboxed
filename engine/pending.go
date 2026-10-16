package main

import (
	"database/sql"
	"errors"
	"fmt"
)

// Change is a local action on an IMAP thread waiting to reach the server.
type Change struct {
	ID                               int64
	AccountID, ThreadID, Kind, Value string
}

// Target is where a change applies on the server: some UIDs in one folder, and for a move, the destination.
type Target struct {
	MailboxID, Path, Dest string
	UIDs                  []uint32
}

const maxAttempts = 5

func flagValue(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

// queueChange records a local action for the syncer to send. Only IMAP accounts have a server to send it to.
func queueChange(tx *sql.Tx, threadID, kind, value string) error {
	_, err := tx.Exec(`INSERT INTO pending_changes (account_id, thread_id, kind, value)
		SELECT t.account_id, t.id, ?, ? FROM threads t JOIN accounts a ON a.id = t.account_id
		WHERE t.id = ? AND a.provider = 'imap'`, kind, value, threadID)
	return err
}

func (s *Store) PendingChanges(accountID string) ([]Change, error) {
	rows, err := s.db.Query(`SELECT id, account_id, thread_id, kind, value FROM pending_changes WHERE account_id = ? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.ID, &c.AccountID, &c.ThreadID, &c.Kind, &c.Value); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ChangeTargets finds where on the server a change applies. Mark read touches every message, mark unread and star
// only the newest (as Mail and Gmail do), unstar all. Moves: archive takes inbox copies to the archive folder, trash
// takes every other copy to the trash, restore brings archived or trashed copies back to the inbox.
func (s *Store) ChangeTargets(c Change) ([]Target, error) {
	where, dest := "", ""
	switch {
	case c.Kind == "seen" && c.Value == "0", c.Kind == "flagged" && c.Value == "1":
		where = `AND m.id = (SELECT id FROM messages WHERE thread_id = ?1 ORDER BY date DESC LIMIT 1)`
	case c.Kind == "seen", c.Kind == "flagged":
	case c.Kind == "move" && c.Value == "archive":
		where, dest = `AND mb.role = 'inbox'`, "archive"
	case c.Kind == "move" && c.Value == "trash":
		where, dest = `AND COALESCE(mb.role, '') != 'trash'`, "trash"
	case c.Kind == "move" && c.Value == "inbox":
		where, dest = `AND mb.role IN ('archive', 'trash')`, "inbox"
	default:
		return nil, fmt.Errorf("%w: unknown change %s=%s", ErrInvalid, c.Kind, c.Value)
	}
	var destPath string
	if dest != "" {
		err := s.db.QueryRow(`SELECT path FROM mailboxes WHERE account_id = ? AND role = ? AND path IS NOT NULL ORDER BY position LIMIT 1`,
			c.AccountID, dest).Scan(&destPath)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, problem(fmt.Sprintf("This account has no %s folder on the server.", dest))
		} else if err != nil {
			return nil, err
		}
	}
	rows, err := s.db.Query(`SELECT ml.mailbox_id, mb.path, ml.uid FROM message_locations ml
		JOIN messages m ON m.id = ml.message_id JOIN mailboxes mb ON mb.id = ml.mailbox_id
		WHERE m.thread_id = ?1 AND mb.path IS NOT NULL `+where+` ORDER BY mb.position, ml.uid`, c.ThreadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var id, path string
		var uid uint32
		if err := rows.Scan(&id, &path, &uid); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].MailboxID != id {
			out = append(out, Target{MailboxID: id, Path: path, Dest: destPath})
		}
		out[len(out)-1].UIDs = append(out[len(out)-1].UIDs, uid)
	}
	return out, rows.Err()
}

func (s *Store) FinishChange(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pending_changes WHERE id = ?`, id)
	return err
}

// FailChange counts a refused attempt. After maxAttempts the change is dropped and its thread re-derived, so it shows
// the server's state again rather than an action that will never happen.
func (s *Store) FailChange(id int64) (dropped bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var attempts int
	var threadID string
	if err := tx.QueryRow(`UPDATE pending_changes SET attempts = attempts + 1 WHERE id = ? RETURNING attempts, thread_id`, id).
		Scan(&attempts, &threadID); err != nil {
		return false, err
	}
	if attempts >= maxAttempts {
		if _, err := tx.Exec(`DELETE FROM pending_changes WHERE id = ?`, id); err != nil {
			return false, err
		}
		if err := syncThread(tx, threadID); err != nil {
			return false, err
		}
		dropped = true
	}
	return dropped, tx.Commit()
}
