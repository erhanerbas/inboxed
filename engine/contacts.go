package main

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// Contact is someone in the mail: suggested in the address fields and for @mentions.
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Contacts is everyone the mail involves except the accounts themselves, most relevant first: people written to
// count three times as much as people who wrote, and both fade with time since the last contact.
// ponytail: scans every message per call (the UI asks once per compose); keep a contacts table on insert if that gets slow.
func (s *Store) Contacts(now time.Time) ([]Contact, error) {
	rows, err := s.db.Query(`WITH mine AS (SELECT lower(email) AS email FROM accounts),
		msgs AS (SELECT date, sender, recipients, cc,
			lower(json_extract(sender, '$.email')) IN (SELECT email FROM mine) AS out FROM messages),
		people AS (
			SELECT json_extract(j.value, '$.name') AS name, json_extract(j.value, '$.email') AS email, date, 1 AS sent
				FROM msgs, json_each(msgs.recipients) j WHERE out
			UNION ALL SELECT json_extract(j.value, '$.name'), json_extract(j.value, '$.email'), date, 1
				FROM msgs, json_each(msgs.cc) j WHERE out
			UNION ALL SELECT json_extract(sender, '$.name'), json_extract(sender, '$.email'), date, 0 FROM msgs WHERE NOT out)
		SELECT coalesce(name, ''), lower(email), date, sent FROM people
		WHERE email LIKE '%_@_%' AND lower(email) NOT IN (SELECT email FROM mine)
		ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type tally struct {
		Contact
		sent, received int
		last           time.Time
	}
	byEmail := map[string]*tally{}
	for rows.Next() {
		var name, email, date string
		var sent bool
		if err := rows.Scan(&name, &email, &date, &sent); err != nil {
			return nil, err
		}
		t := byEmail[email]
		if t == nil {
			last, _ := time.Parse(isoDate, date)
			t = &tally{Contact: Contact{Email: email}, last: last}
			byEmail[email] = t
		}
		if t.Name == "" && !strings.EqualFold(name, email) { // newest first: the first real name is the latest
			t.Name = strings.TrimSpace(name)
		}
		if sent {
			t.sent++
		} else {
			t.received++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	score := func(t *tally) float64 {
		days := max(now.Sub(t.last).Hours()/24, 0)
		return float64(3*t.sent+t.received) / (1 + days/30)
	}
	all := make([]*tally, 0, len(byEmail))
	for _, t := range byEmail {
		all = append(all, t)
	}
	slices.SortFunc(all, func(a, b *tally) int {
		return cmp.Or(cmp.Compare(score(b), score(a)), strings.Compare(a.Email, b.Email))
	})
	out := []Contact{}
	for _, t := range all[:min(len(all), 5000)] {
		out = append(out, t.Contact)
	}
	return out, nil
}
