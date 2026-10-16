package main

import (
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trailStore is a store with an IMAP account "acc" and its inbox.
func trailStore(t *testing.T) (*Store, string) {
	t.Helper()
	s := newTestStore(t)
	addTestAccount(t, s)
	inbox, _ := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	return s, inbox
}

func add(t *testing.T, s *Store, mailbox string, uid uint32, raw []byte) {
	t.Helper()
	p, err := parseMessage(raw, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessage(Incoming{AccountID: "acc", MailboxID: mailbox, UID: uid, parsed: p}); err != nil {
		t.Fatal(err)
	}
}

func note(id, refs string) []byte {
	if refs != "" {
		refs += "\n"
	}
	return crlf("From: Maya <maya@x>\nSubject: Plans\nMessage-ID: <" + id + ">\n" + refs + "\nHi from " + id + "\n")
}

func threadOf(t *testing.T, s *Store, msgID string) string {
	t.Helper()
	var id string
	s.db.QueryRow(`SELECT thread_id FROM messages WHERE id = ?`, "acc/"+msgID).Scan(&id)
	return id
}

func TestThreadsJoinByLinks(t *testing.T) {
	s, inbox := trailStore(t)
	// A reply arrives first; its root x was never imported, but it names p as the message it answers.
	add(t, s, inbox, 1, note("r@x", "References: <x@x> <p@x>"))
	// p's own headers point elsewhere (y), yet the stored reply refers to it: it joins the reply's thread.
	add(t, s, inbox, 2, note("p@x", "References: <y@x>"))
	if threadOf(t, s, "p@x") != threadOf(t, s, "r@x") {
		t.Errorf("an original arriving after its reply: %q vs %q", threadOf(t, s, "p@x"), threadOf(t, s, "r@x"))
	}
	// A mailing list trimmed the root from References: the message still joins through the reply it names.
	add(t, s, inbox, 3, note("l@x", "References: <r@x>"))
	if threadOf(t, s, "l@x") != threadOf(t, s, "r@x") {
		t.Errorf("trimmed References: %q vs %q", threadOf(t, s, "l@x"), threadOf(t, s, "r@x"))
	}

	// Two separate threads, then a message that links them: they merge, with folders and queued changes.
	add(t, s, inbox, 4, note("a@x", ""))
	add(t, s, inbox, 5, note("b@x", ""))
	yes := true
	s.SetFlags("acc/b@x", nil, &yes)
	add(t, s, inbox, 6, note("m@x", "References: <a@x> <b@x>"))
	if threadOf(t, s, "b@x") != "acc/a@x" || threadOf(t, s, "m@x") != "acc/a@x" {
		t.Errorf("merge: b in %q, m in %q", threadOf(t, s, "b@x"), threadOf(t, s, "m@x"))
	}
	if _, err := s.Thread("acc/b@x"); err == nil {
		t.Error("the merged-away thread is still here")
	}
	var pending string
	s.db.QueryRow(`SELECT thread_id FROM pending_changes`).Scan(&pending)
	if pending != "acc/a@x" {
		t.Errorf("queued change follows the merge: %q", pending)
	}
	var orphans int
	s.db.QueryRow(`SELECT COUNT(*) FROM search WHERE rowid NOT IN (SELECT doc FROM threads)`).Scan(&orphans)
	if orphans != 0 {
		t.Errorf("%d search rows without a thread", orphans)
	}
}

func pdfMessage(id string) []byte {
	return crlf("From: Maya <maya@x>\nSubject: Menu\nMessage-ID: <" + id + ">\nMIME-Version: 1.0\n" +
		"Content-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain\n\nMenu attached.\n" +
		"--b\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=menu.pdf\n\n%PDF-1.4 tiny\n--b--\n")
}

func TestAttachmentFiles(t *testing.T) {
	s, inbox := trailStore(t)
	add(t, s, inbox, 1, pdfMessage("one@x"))
	add(t, s, inbox, 2, pdfMessage("two@x"))
	files, _ := os.ReadDir(s.files)
	if len(files) != 1 {
		t.Fatalf("%d files for one attachment sent twice", len(files))
	}
	th, _ := s.Thread("acc/one@x")
	if got, err := s.File(th.Messages[0].Attachments[0].Hash); err != nil || string(got) != "%PDF-1.4 tiny" {
		t.Errorf("file %q, %v", got, err)
	}
	s.DeleteMessage("acc/one@x")
	s.CleanFiles()
	if files, _ := os.ReadDir(s.files); len(files) != 1 {
		t.Error("a file still in use was cleaned")
	}
	s.DeleteMessage("acc/two@x")
	s.CleanFiles()
	if files, _ := os.ReadDir(s.files); len(files) != 0 {
		t.Error("an unused file was kept")
	}
}

func TestThreadServesHTMLWithInlineImages(t *testing.T) {
	s, inbox := trailStore(t)
	add(t, s, inbox, 1, richMessage)
	th, err := s.Thread("acc/a@x")
	if err != nil {
		t.Fatal(err)
	}
	if html := th.Messages[0].HTML; !strings.Contains(html, `src="data:image/png;base64,iVBORw0KGgo="`) || strings.Contains(html, "cid:") {
		t.Errorf("html %q", html)
	}
}

func TestInlineImagesCapped(t *testing.T) {
	s, inbox := trailStore(t)
	big := base64.StdEncoding.EncodeToString(make([]byte, 6<<20))
	add(t, s, inbox, 1, crlf("From: a@x\nMessage-ID: <big@x>\nMIME-Version: 1.0\nContent-Type: multipart/related; boundary=r\n\n"+
		"--r\nContent-Type: text/html\n\n<img src=\"cid:big@x\">\n"+
		"--r\nContent-Type: image/png\nContent-ID: <big@x>\nContent-Transfer-Encoding: base64\n\n"+big+"\n--r--\n"))
	th, _ := s.Thread("acc/big@x")
	if !strings.Contains(th.Messages[0].HTML, "cid:big@x") {
		t.Error("a 6 MB image was inlined")
	}
}

func TestMigrationForgetsImportedMail(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "old.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	t.Cleanup(func() { db.Close() })
	if err := s.migrateTo(5); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(seedNow); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts (id, name, email, provider, color, position) VALUES ('acc', 'Home', 'you@x', 'imap', 'c', 9)`,
		`INSERT INTO mailboxes (id, account_id, name, role, position, path, uid_validity, status) VALUES ('acc/INBOX', 'acc', 'Inbox', 'inbox', 99, 'INBOX', 1, '1/1/2/0/0')`,
		`INSERT INTO threads (id, account_id, subject, snippet, participants, last_date, has_attachments) VALUES ('acc/a@x', 'acc', 'Hi', '', '[]', '', 0)`,
		`INSERT INTO search (rowid, subject, people, body) VALUES ((SELECT doc FROM threads WHERE id = 'acc/a@x'), 'Hi', '', '')`,
		`INSERT INTO messages (id, thread_id, sender, recipients, cc, date, body, attachments) VALUES ('acc/a@x', 'acc/a@x', '{}', '[]', '[]', '', 'Hi', '[]')`,
		`INSERT INTO message_locations (mailbox_id, uid, message_id) VALUES ('acc/INBOX', 1, 'acc/a@x')`,
		`INSERT INTO pending_changes (account_id, thread_id, kind, value) VALUES ('acc', 'acc/a@x', 'seen', '1')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	count := func(q string) (n int) { db.QueryRow(q).Scan(&n); return }
	if n := count(`SELECT COUNT(*) FROM threads WHERE account_id = 'acc'`); n != 0 {
		t.Errorf("%d imported threads left", n)
	}
	if n := count(`SELECT COUNT(*) FROM search WHERE rowid NOT IN (SELECT doc FROM threads)`); n != 0 {
		t.Errorf("%d orphaned search rows", n)
	}
	if n := count(`SELECT COUNT(*) FROM pending_changes`) + count(`SELECT COUNT(*) FROM message_locations`); n != 0 {
		t.Errorf("%d pending changes or locations left", n)
	}
	if n := count(`SELECT COUNT(*) FROM mailboxes WHERE status != ''`); n != 0 {
		t.Errorf("%d folders keep a status, so they wouldn't sync again", n)
	}
	if n := count(`SELECT COUNT(*) FROM threads`); n == 0 {
		t.Error("demo mail was forgotten too")
	}
}
