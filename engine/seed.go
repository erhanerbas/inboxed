package main

import (
	_ "embed"
	"encoding/json"
	"time"
)

// seed.json is the small set tests rely on (INBOXED_SEED=1); demo.json is fuller mail for screenshots
// (INBOXED_SEED=demo, which `npm run demo` sets).
var (
	//go:embed seed.json
	seedJSON []byte
	//go:embed demo.json
	demoJSON []byte
)

type seedMessage struct {
	Message
	MinutesAgo int `json:"minutesAgo"`
}

type seedThread struct {
	Thread
	Messages []seedMessage `json:"messages"`
}

// Seed fills an empty database with demo mailboxes from data (seedJSON or demoJSON), dated relative to now.
// Does nothing if any account exists.
func (s *Store) Seed(now time.Time, data []byte) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil || n > 0 {
		return err
	}
	var seed struct {
		Accounts  []Account
		Mailboxes []Mailbox
		Threads   []seedThread
	}
	if err := json.Unmarshal(data, &seed); err != nil {
		return err
	}
	for i, a := range seed.Accounts {
		if _, err := s.db.Exec(`INSERT INTO accounts (id, name, email, provider, color, position) VALUES (?, ?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Email, a.Provider, a.Color, i); err != nil {
			return err
		}
	}
	for i, m := range seed.Mailboxes {
		if _, err := s.db.Exec(`INSERT INTO mailboxes (id, account_id, name, role, parent_id, position) VALUES (?, ?, ?, ?, ?, ?)`,
			m.ID, m.AccountID, m.Name, m.Role, m.ParentID, i); err != nil {
			return err
		}
	}
	for _, st := range seed.Threads {
		t := st.Thread
		for _, sm := range st.Messages {
			m := sm.Message
			m.Date = now.Add(-time.Duration(sm.MinutesAgo) * time.Minute).UTC().Format(isoDate)
			t.Messages = append(t.Messages, m)
		}
		if err := s.PutThread(t); err != nil {
			return err
		}
		for _, m := range t.Messages {
			if m.HTML == "" {
				continue
			}
			if _, err := s.db.Exec(`UPDATE messages SET html = ? WHERE id = ?`, m.HTML, m.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
