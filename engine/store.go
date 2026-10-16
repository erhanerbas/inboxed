package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid request")
)

// isoDate sorts as text in time order and parses in JavaScript.
const isoDate = "2006-01-02T15:04:05.000Z"

// Applied in order, once each; PRAGMA user_version records how many ran. Never edit a shipped entry, append a new one.
var migrations = []string{
	`CREATE TABLE accounts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		provider TEXT NOT NULL,
		color TEXT NOT NULL,
		position INTEGER NOT NULL
	);
	CREATE TABLE mailboxes (
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		role TEXT,
		parent_id TEXT REFERENCES mailboxes(id) ON DELETE CASCADE,
		position INTEGER NOT NULL
	);
	CREATE TABLE threads (
		id TEXT PRIMARY KEY,
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		subject TEXT NOT NULL,
		unread INTEGER NOT NULL DEFAULT 0,
		starred INTEGER NOT NULL DEFAULT 0,
		snippet TEXT NOT NULL,
		participants TEXT NOT NULL,
		last_date TEXT NOT NULL,
		has_attachments INTEGER NOT NULL
	);
	CREATE INDEX threads_last_date ON threads(last_date);
	CREATE TABLE thread_mailboxes (
		thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
		mailbox_id TEXT NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
		PRIMARY KEY (thread_id, mailbox_id)
	);
	CREATE INDEX thread_mailboxes_mailbox ON thread_mailboxes(mailbox_id);
	CREATE TABLE messages (
		id TEXT PRIMARY KEY,
		thread_id TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
		sender TEXT NOT NULL,
		recipients TEXT NOT NULL,
		cc TEXT NOT NULL,
		date TEXT NOT NULL,
		body TEXT NOT NULL,
		attachments TEXT NOT NULL
	);
	CREATE INDEX messages_thread ON messages(thread_id, date);
	CREATE VIRTUAL TABLE search USING fts5(thread_id UNINDEXED, subject, people, body);`,
}

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// ponytail: one connection serializes all access; plenty for one user. Pool readers if Phase 4 sync contends.
	db.SetMaxOpenConns(1)
	s := &Store{db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// PutThread writes a thread with its messages and mailboxes, replacing any earlier version. It recomputes the
// summary fields lists read (snippet, participants, last date), so listing never loads messages.
func (s *Store) PutThread(t Thread) error {
	if len(t.Messages) == 0 {
		return fmt.Errorf("%w: thread %s has no messages", ErrInvalid, t.ID)
	}
	var me string
	if err := s.db.QueryRow(`SELECT email FROM accounts WHERE id = ?`, t.AccountID).Scan(&me); err != nil {
		return fmt.Errorf("account %s: %w", t.AccountID, err)
	}
	last := t.Messages[len(t.Messages)-1]
	attached := false
	for _, m := range t.Messages {
		attached = attached || len(m.Attachments) > 0
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO threads (id, account_id, subject, unread, starred, snippet, participants, last_date, has_attachments)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET subject = excluded.subject, unread = excluded.unread, starred = excluded.starred,
			snippet = excluded.snippet, participants = excluded.participants, last_date = excluded.last_date,
			has_attachments = excluded.has_attachments`,
		t.ID, t.AccountID, t.Subject, t.Unread, t.Starred, snippet(last.Body),
		toJSON(participants(t.Messages, me)), last.Date, attached); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM thread_mailboxes WHERE thread_id = ?`,
		`DELETE FROM messages WHERE thread_id = ?`,
		`DELETE FROM search WHERE thread_id = ?`,
	} {
		if _, err := tx.Exec(q, t.ID); err != nil {
			return err
		}
	}
	for _, m := range t.MailboxIDs {
		if _, err := tx.Exec(`INSERT INTO thread_mailboxes (thread_id, mailbox_id) VALUES (?, ?)`, t.ID, m); err != nil {
			return fmt.Errorf("thread %s in mailbox %s: %w", t.ID, m, err)
		}
	}
	var people, bodies []string
	for _, m := range t.Messages {
		if _, err := tx.Exec(`
			INSERT INTO messages (id, thread_id, sender, recipients, cc, date, body, attachments)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, t.ID, toJSON(m.From), toJSON(orEmpty(m.To)), toJSON(orEmpty(m.Cc)), m.Date, m.Body,
			toJSON(orEmpty(m.Attachments))); err != nil {
			return err
		}
		for _, a := range append([]Address{m.From}, m.To...) {
			people = append(people, a.Name, a.Email)
		}
		bodies = append(bodies, m.Body)
	}
	if _, err := tx.Exec(`INSERT INTO search (thread_id, subject, people, body) VALUES (?, ?, ?, ?)`,
		t.ID, t.Subject, strings.Join(people, " "), strings.Join(bodies, "\n")); err != nil {
		return err
	}
	return tx.Commit()
}

// participants is everyone in the thread except the account owner, in order of first appearance.
func participants(msgs []Message, me string) []Address {
	seen := map[string]bool{me: true}
	out := []Address{}
	for _, m := range msgs {
		for _, a := range append([]Address{m.From}, m.To...) {
			if !seen[a.Email] {
				seen[a.Email] = true
				out = append(out, a)
			}
		}
	}
	return out
}

const summaryColumns = `t.id, t.account_id, t.subject, t.snippet, t.participants, t.last_date, t.unread, t.starred,
	t.has_attachments, (SELECT json_group_array(mailbox_id) FROM thread_mailboxes WHERE thread_id = t.id)`

func scanSummary(row interface{ Scan(...any) error }) (Thread, error) {
	var t Thread
	var people, boxes string
	if err := row.Scan(&t.ID, &t.AccountID, &t.Subject, &t.Snippet, &people, &t.LastDate, &t.Unread, &t.Starred,
		&t.HasAttachments, &boxes); err != nil {
		return t, err
	}
	if err := json.Unmarshal([]byte(people), &t.Participants); err != nil {
		return t, err
	}
	return t, json.Unmarshal([]byte(boxes), &t.MailboxIDs)
}

// Thread returns one conversation with its messages, oldest first.
func (s *Store) Thread(id string) (Thread, error) {
	t, err := scanSummary(s.db.QueryRow(`SELECT `+summaryColumns+` FROM threads t WHERE t.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	} else if err != nil {
		return t, err
	}
	rows, err := s.db.Query(`SELECT id, sender, recipients, cc, date, body, attachments FROM messages
		WHERE thread_id = ? ORDER BY date`, id)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		m := Message{ThreadID: id}
		var from, to, cc, attachments string
		if err := rows.Scan(&m.ID, &from, &to, &cc, &m.Date, &m.Body, &attachments); err != nil {
			return t, err
		}
		for _, f := range []struct {
			src string
			dst any
		}{{from, &m.From}, {to, &m.To}, {cc, &m.Cc}, {attachments, &m.Attachments}} {
			if err := json.Unmarshal([]byte(f.src), f.dst); err != nil {
				return t, err
			}
		}
		t.Messages = append(t.Messages, m)
	}
	return t, rows.Err()
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only plain structs and slices reach here
	}
	return string(b)
}

// orEmpty keeps lists as [] in JSON, never null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
