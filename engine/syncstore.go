package main

import (
	"database/sql"
	"errors"
	"maps"
	"slices"
)

// notPending: threads with a local change still on its way to the server keep their local state.
const notPending = `NOT EXISTS (SELECT 1 FROM pending_changes WHERE thread_id = ?1)`

// syncThread re-derives a thread's flags and folders from where its messages are on the server. Demo threads (no
// locations) and threads with pending changes are left alone; a thread with no messages left is deleted.
func syncThread(tx *sql.Tx, threadID string) error {
	var messages, located int
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT m.id), COUNT(ml.message_id) FROM messages m
		LEFT JOIN message_locations ml ON ml.message_id = m.id WHERE m.thread_id = ?`, threadID).Scan(&messages, &located); err != nil {
		return err
	}
	if messages == 0 {
		_, err := tx.Exec(`DELETE FROM threads WHERE id = ?`, threadID)
		return err
	}
	if located == 0 {
		return nil
	}
	const here = `FROM message_locations ml JOIN messages m ON m.id = ml.message_id WHERE m.thread_id = ?1`
	if _, err := tx.Exec(`UPDATE threads SET unread = EXISTS (SELECT 1 `+here+` AND NOT ml.seen),
		starred = EXISTS (SELECT 1 `+here+` AND ml.flagged) WHERE id = ?1 AND `+notPending, threadID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM thread_mailboxes WHERE thread_id = ?1 AND `+notPending+`
		AND mailbox_id NOT IN (SELECT ml.mailbox_id `+here+`)`, threadID); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT OR IGNORE INTO thread_mailboxes (thread_id, mailbox_id)
		SELECT DISTINCT ?1, ml.mailbox_id `+here+` AND `+notPending, threadID)
	return err
}

// SetLocationFlags records the server's flags for a folder's messages (UID → [seen, flagged]) and re-derives the
// threads whose flags changed.
func (s *Store) SetLocationFlags(mailboxID string, flags map[uint32][2]bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	threads := map[string]bool{}
	for uid, f := range flags {
		var msgID string
		err := tx.QueryRow(`UPDATE message_locations SET seen = ?, flagged = ? WHERE mailbox_id = ? AND uid = ?
			AND (seen != ? OR flagged != ?) RETURNING message_id`, f[0], f[1], mailboxID, uid, f[0], f[1]).Scan(&msgID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return err
		}
		var threadID string
		if err := tx.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, msgID).Scan(&threadID); err != nil {
			return err
		}
		threads[threadID] = true
	}
	for id := range threads {
		if err := syncThread(tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveLocations forgets messages a folder no longer has (deleted, or moved away on the server) and re-derives their
// threads. The messages themselves stay until Prune: one moved to another folder only needs its new location.
func (s *Store) RemoveLocations(mailboxID string, uids []uint32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	threads := map[string]bool{}
	for _, uid := range uids {
		var threadID string
		err := tx.QueryRow(`DELETE FROM message_locations WHERE mailbox_id = ? AND uid = ?
			RETURNING (SELECT thread_id FROM messages WHERE id = message_id)`, mailboxID, uid).Scan(&threadID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return err
		}
		threads[threadID] = true
	}
	for id := range threads {
		if err := syncThread(tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MoveLocations records that the server moved messages (src UIDs → dst UIDs, in order, as COPYUID reports them).
func (s *Store) MoveLocations(src string, srcUIDs []uint32, dst string, dstUIDs []uint32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	threads := map[string]bool{}
	for i, uid := range srcUIDs {
		var threadID string
		err := tx.QueryRow(`UPDATE OR REPLACE message_locations SET mailbox_id = ?, uid = ? WHERE mailbox_id = ? AND uid = ?
			RETURNING (SELECT thread_id FROM messages WHERE id = message_id)`, dst, dstUIDs[i], src, uid).Scan(&threadID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return err
		}
		threads[threadID] = true
	}
	for id := range threads {
		if err := syncThread(tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Prune deletes an account's messages that are in no folder on the server any more, and threads left with no messages.
// The syncer runs it once every folder is synced, so mail that only moved has its new location by then.
func (s *Store) Prune(accountID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`DELETE FROM messages WHERE id IN (SELECT m.id FROM messages m JOIN threads t ON t.id = m.thread_id
		WHERE t.account_id = ? AND NOT EXISTS (SELECT 1 FROM message_locations WHERE message_id = m.id))
		RETURNING thread_id`, accountID)
	if err != nil {
		return err
	}
	threads := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		threads[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id := range threads {
		if err := syncThread(tx, id); err != nil {
			return err
		}
		if err := refreshThread(tx, id); err != nil && !errors.Is(err, sql.ErrNoRows) { // deleted threads have nothing to refresh
			return err
		}
	}
	return tx.Commit()
}

// MarkLocations records a flag Inboxed just set on the server, so later re-derivations agree with it.
func (s *Store) MarkLocations(mailboxID string, uids []uint32, kind string, on bool) error {
	column := map[string]string{"seen": "seen", "flagged": "flagged"}[kind]
	if column == "" {
		return nil
	}
	for _, uid := range uids {
		if _, err := s.db.Exec(`UPDATE message_locations SET `+column+` = ? WHERE mailbox_id = ? AND uid = ?`, on, mailboxID, uid); err != nil {
			return err
		}
	}
	return nil
}

// ServerMailboxes maps an account's server folder paths to their mailbox ids (local entries like Starred aren't
// server folders).
func (s *Store) ServerMailboxes(accountID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT path, id FROM mailboxes WHERE account_id = ? AND path IS NOT NULL`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var path, id string
		if err := rows.Scan(&path, &id); err != nil {
			return nil, err
		}
		out[path] = id
	}
	return out, rows.Err()
}

// DeleteMailbox drops a folder that no longer exists on the server, forgetting its locations first (Prune then
// deletes the messages that were only there).
func (s *Store) DeleteMailbox(id string) error {
	known, err := s.KnownUIDs(id)
	if err != nil {
		return err
	}
	if err := s.RemoveLocations(id, slices.Collect(maps.Keys(known))); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM mailboxes WHERE id = ?`, id)
	return err
}

// MailboxStatus is a folder's STATUS when it was last synced, and its HIGHESTMODSEQ (CONDSTORE servers; else 0).
func (s *Store) MailboxStatus(id string) (status string, modseq uint64) {
	s.db.QueryRow(`SELECT status, modseq FROM mailboxes WHERE id = ?`, id).Scan(&status, &modseq)
	return status, modseq
}

func (s *Store) SetMailboxStatus(id, status string, modseq uint64) error {
	_, err := s.db.Exec(`UPDATE mailboxes SET status = ?, modseq = ? WHERE id = ?`, status, modseq, id)
	return err
}

// Copy is where a message lives in a folder with that role, or ErrNotFound.
func (s *Store) Copy(messageID, role string) (mailboxID, path string, uid uint32, err error) {
	err = s.db.QueryRow(`SELECT ml.mailbox_id, mb.path, ml.uid FROM message_locations ml JOIN mailboxes mb ON mb.id = ml.mailbox_id
		WHERE ml.message_id = ? AND mb.role = ? AND mb.path IS NOT NULL LIMIT 1`, messageID, role).Scan(&mailboxID, &path, &uid)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// DeleteMessage deletes one message here (a discarded or replaced draft); its thread follows, or goes when empty.
func (s *Store) DeleteMessage(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var threadID string
	err = tx.QueryRow(`DELETE FROM messages WHERE id = ? RETURNING thread_id`, id).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if err := syncThread(tx, threadID); err != nil {
		return err
	}
	if err := refreshThread(tx, threadID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return tx.Commit()
}

// Located says whether a message is in any folder on the server.
func (s *Store) Located(messageID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM message_locations WHERE message_id = ?`, messageID).Scan(&n)
	return n > 0, err
}
