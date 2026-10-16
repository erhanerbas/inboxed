package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/zalando/go-keyring"
)

const (
	dinner = "From: Maya Chen <maya@example.com>\nTo: You <you@example.com>\nSubject: Dinner plans\n" +
		"Message-ID: <d1@example.com>\nDate: Thu, 15 Oct 2026 10:00:00 +0000\n\nAre you free Saturday?\n"
	myReply = "From: You <you@example.com>\nTo: Maya Chen <maya@example.com>\nSubject: Re: Dinner plans\n" +
		"Message-ID: <d2@example.com>\nIn-Reply-To: <d1@example.com>\nReferences: <d1@example.com>\n" +
		"Date: Thu, 15 Oct 2026 11:00:00 +0000\n\nSaturday works.\n\nOn Thu, Maya Chen wrote:\n> Are you free Saturday?\n"
	herReply = "From: Maya Chen <maya@example.com>\nTo: You <you@example.com>\nSubject: Re: Dinner plans\n" +
		"Message-ID: <d3@example.com>\nIn-Reply-To: <d2@example.com>\nReferences: <d1@example.com> <d2@example.com>\n" +
		"Date: Thu, 15 Oct 2026 12:00:00 +0000\n\nGreat, see you then.\n"
	checklist = "From: Priya Nair <priya@example.com>\nTo: You <you@example.com>\nSubject: Launch checklist\n" +
		"Message-ID: <l1@example.com>\nDate: Thu, 15 Oct 2026 09:00:00 +0000\n\nThree things left.\n"
	newsletter = "From: News <news@example.com>\nTo: You <you@example.com>\nSubject: This week\n" +
		"Message-ID: <n1@example.com>\nDate: Fri, 16 Oct 2026 08:00:00 +0000\n\nHeadlines.\n"
)

type literal struct{ *strings.Reader }

func (l literal) Size() int64 { return int64(l.Len()) }

func appendRaw(t *testing.T, u *imapmemserver.User, mailbox, raw string, flags ...imap.Flag) {
	t.Helper()
	msg := strings.ReplaceAll(raw, "\n", "\r\n")
	if _, err := u.Append(mailbox, literal{strings.NewReader(msg)}, &imap.AppendOptions{Flags: flags, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func local(ln net.Listener) Server {
	return Server{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Security: "none"}
}

type smtpBackend struct{}
type smtpSession struct{}

func (smtpBackend) NewSession(*smtp.Conn) (smtp.Session, error) { return smtpSession{}, nil }
func (smtpSession) AuthMechanisms() []string                    { return []string{sasl.Plain} }
func (smtpSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		if username == "you@example.com" && password == "secret" {
			return nil
		}
		return errors.New("bad credentials")
	}), nil
}
func (smtpSession) Mail(string, *smtp.MailOptions) error { return nil }
func (smtpSession) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (smtpSession) Data(io.Reader) error                 { return nil }
func (smtpSession) Reset()                               {}
func (smtpSession) Logout() error                        { return nil }

// testServers runs an in-memory IMAP server and an SMTP server on this machine for the length of a test, with
// a fake keychain.
func testServers(t *testing.T) (imapSrv, smtpSrv Server, user *imapmemserver.User) {
	t.Helper()
	keyring.MockInit()
	mem := imapmemserver.New()
	user = imapmemserver.NewUser("you@example.com", "secret")
	for _, name := range []string{"INBOX", "Sent", "Archive", "Work", "Work/Projects"} {
		user.Create(name, nil)
	}
	appendRaw(t, user, "INBOX", dinner)
	appendRaw(t, user, "INBOX", herReply, imap.FlagSeen)
	appendRaw(t, user, "Sent", myReply, imap.FlagSeen)
	appendRaw(t, user, "Work/Projects", checklist, imap.FlagSeen, imap.FlagFlagged)
	mem.AddUser(user)
	return serveIMAP(t, mem, nil), serveSMTP(t, smtpBackend{}), user
}

// serveIMAP serves mem on this machine for the test. wrap, when set, can make the session misbehave.
func serveIMAP(t *testing.T, mem *imapmemserver.Server, wrap func(imapserver.Session) imapserver.Session) Server {
	t.Helper()
	is := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			var s imapserver.Session = mem.NewSession()
			if wrap != nil {
				s = wrap(s)
			}
			return s, nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	ln := listen(t)
	go is.Serve(ln)
	t.Cleanup(func() { is.Close() })
	return local(ln)
}

func serveSMTP(t *testing.T, backend smtp.Backend) Server {
	t.Helper()
	ss := smtp.NewServer(backend)
	ss.AllowInsecureAuth = true
	ln := listen(t)
	go ss.Serve(ln)
	t.Cleanup(func() { ss.Close() })
	return local(ln)
}

func waitImport(t *testing.T, im *Syncer, id string) Progress {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if p := im.Progress(id); p != nil && p.State != "importing" {
			return *p
		}
	}
	t.Fatal("import never finished")
	return Progress{}
}

func TestAddAccountRejectsBadSignIns(t *testing.T) {
	imapSrv, smtpSrv, _ := testServers(t)
	s := newTestStore(t)
	im := NewSyncer(s, func() {})
	closed := listen(t)
	nothing := local(closed)
	closed.Close()
	remote := imapSrv
	remote.Host = "imap.example.com"
	cases := []struct {
		name string
		edit func(*AccountSettings)
		want string
	}{
		{"wrong password", func(a *AccountSettings) { a.Password = "nope" }, "didn't accept the username or password"},
		{"nothing listening", func(a *AccountSettings) { a.IMAP = nothing }, "Couldn't connect"},
		{"unencrypted to another computer", func(a *AccountSettings) { a.IMAP = remote }, "SSL/TLS or STARTTLS"},
		{"missing server", func(a *AccountSettings) { a.SMTP.Host = "" }, "Fill in"},
	}
	for _, c := range cases {
		set := AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv}
		c.edit(&set)
		if _, err := im.AddAccount(set); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if accounts, _ := s.Accounts(); len(accounts) != 2 {
		t.Errorf("a failed sign-in saved an account (%d accounts)", len(accounts))
	}
}

func TestImportAccount(t *testing.T) {
	imapSrv, smtpSrv, user := testServers(t)
	s := newTestStore(t)
	im := NewSyncer(s, func() {})
	a, err := im.AddAccount(AccountSettings{Name: "Home", Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	if p := waitImport(t, im, a.ID); p.State != "done" || p.Done != 4 || p.Total != 4 {
		t.Fatalf("progress %+v", p)
	}
	if pw, _ := keyring.Get(keyringService, a.ID); pw != "secret" {
		t.Errorf("keychain holds %q", pw)
	}

	boxes := map[string]Mailbox{}
	all, _ := s.Mailboxes()
	for _, m := range all {
		boxes[m.ID] = m
	}
	role := func(id string) string {
		if r := boxes[id].Role; r != nil {
			return *r
		}
		return ""
	}
	if role(a.ID+"/INBOX") != "inbox" || role(a.ID+"/Sent") != "sent" || role(a.ID+"/Archive") != "archive" ||
		role(a.ID+"/Work") != "" || boxes[a.ID+"/INBOX"].Name != "Inbox" {
		t.Errorf("roles and names: %+v", boxes)
	}
	if p := boxes[a.ID+"/Work/Projects"].ParentID; p == nil || *p != a.ID+"/Work" || boxes[a.ID+"/Work/Projects"].Name != "Projects" {
		t.Errorf("Work/Projects: %+v", boxes[a.ID+"/Work/Projects"])
	}

	th, err := s.Thread(a.ID + "/d1@example.com")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(th.MailboxIDs)
	if len(th.Messages) != 3 || th.Subject != "Dinner plans" || !th.Unread || th.Snippet != "Great, see you then." ||
		!slices.Equal(th.MailboxIDs, []string{a.ID + "/INBOX", a.ID + "/Sent"}) {
		t.Errorf("dinner thread: %d messages, %q, unread %v, snippet %q, boxes %v",
			len(th.Messages), th.Subject, th.Unread, th.Snippet, th.MailboxIDs)
	}
	if got, _ := s.Threads(a.ID, a.ID+":starred", ""); !slices.Equal(ids(got), []string{a.ID + "/l1@example.com"}) {
		t.Errorf("starred: %v", ids(got))
	}

	// The next pass fetches only what's new.
	appendRaw(t, user, "INBOX", newsletter)
	im.Start(a.ID) // wakes the running sync
	eventually(t, "the new message imported on the next pass", func() bool { return accountMessages(t, s, a.ID) == 5 })

	if err := im.RemoveAccount(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(keyringService, a.ID); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("keychain entry left behind: %v", err)
	}
	if _, err := s.Thread(a.ID + "/d1@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("mail left behind: %v", err)
	}
	if im.Progress(a.ID) != nil {
		t.Error("a removed account still reports progress")
	}
}

// flaky wraps a session to misbehave like real servers do: a folder that can't be opened, a message that can't be
// fetched. It counts whole-message downloads.
type flaky struct {
	imapserver.SessionIMAP4rev2 // the full interface, or the server refuses a session that hides IMAP4rev2
	failSelect                  string
	failUID                     imap.UID
	bodies                      *atomic.Int32
}

func (f flaky) Select(name string, options *imap.SelectOptions) (*imap.SelectData, error) {
	if name == f.failSelect {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "can't open this folder"}
	}
	return f.SessionIMAP4rev2.Select(name, options)
}

func (f flaky) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	whole := slices.ContainsFunc(options.BodySection, func(s *imap.FetchItemBodySection) bool {
		return s.Specifier == imap.PartSpecifierNone && len(s.HeaderFields) == 0
	})
	if set, ok := numSet.(imap.UIDSet); ok && whole {
		if f.failUID != 0 && set.Contains(f.failUID) {
			return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "some messages could not be fetched"}
		}
		if uids, ok := set.Nums(); ok && f.bodies != nil {
			f.bodies.Add(int32(len(uids)))
		}
	}
	return f.SessionIMAP4rev2.Fetch(w, numSet, options)
}

func accountMessages(t *testing.T, s *Store, accountID string) int {
	t.Helper()
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE id LIKE ?`, accountID+"/%").Scan(&n)
	return n
}

func TestImportSkipsWhatItCantRead(t *testing.T) {
	keyring.MockInit()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("you@example.com", "secret")
	u.Create("INBOX", nil)
	u.Create("Broken", nil)
	appendRaw(t, u, "INBOX", dinner)
	appendRaw(t, u, "INBOX", herReply) // UID 2: the server can't send it
	appendRaw(t, u, "INBOX", newsletter)
	appendRaw(t, u, "Broken", checklist)
	mem.AddUser(u)
	imapSrv := serveIMAP(t, mem, func(s imapserver.Session) imapserver.Session {
		return flaky{SessionIMAP4rev2: s.(imapserver.SessionIMAP4rev2), failSelect: "Broken", failUID: 2}
	})
	s := newTestStore(t)
	im := NewSyncer(s, func() {})
	a, err := im.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: serveSMTP(t, smtpBackend{})})
	if err != nil {
		t.Fatal(err)
	}
	if p := waitImport(t, im, a.ID); p.State != "done" {
		t.Fatalf("one bad folder and one bad message stopped the import: %+v", p)
	}
	if n := accountMessages(t, s, a.ID); n != 2 {
		t.Errorf("%d messages imported, want the 2 readable ones", n)
	}
}

func TestImportDownloadsEachMessageOnce(t *testing.T) {
	keyring.MockInit()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("you@example.com", "secret")
	u.Create("INBOX", nil)
	u.Create("Archive", nil)
	appendRaw(t, u, "INBOX", dinner)
	appendRaw(t, u, "Archive", dinner) // the same message again, like a Gmail label or All Mail
	appendRaw(t, u, "Archive", newsletter)
	mem.AddUser(u)
	var bodies atomic.Int32
	imapSrv := serveIMAP(t, mem, func(s imapserver.Session) imapserver.Session {
		return flaky{SessionIMAP4rev2: s.(imapserver.SessionIMAP4rev2), bodies: &bodies}
	})
	s := newTestStore(t)
	im := NewSyncer(s, func() {})
	a, err := im.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: serveSMTP(t, smtpBackend{})})
	if err != nil {
		t.Fatal(err)
	}
	if p := waitImport(t, im, a.ID); p.State != "done" || p.Done != 3 {
		t.Fatalf("progress %+v", p)
	}
	if got := bodies.Load(); got != 2 {
		t.Errorf("downloaded %d whole messages, want 2 (the copy in Archive needs only its headers)", got)
	}
	th, _ := s.Thread(a.ID + "/d1@example.com")
	slices.Sort(th.MailboxIDs)
	if !slices.Equal(th.MailboxIDs, []string{a.ID + "/Archive", a.ID + "/INBOX"}) {
		t.Errorf("dinner thread folders %v", th.MailboxIDs)
	}
}

// silentServer greets, optionally answers LOGIN, and then never answers again.
func silentServer(t *testing.T, login bool) Server {
	t.Helper()
	ln := listen(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				fmt.Fprint(conn, "* OK [CAPABILITY IMAP4rev1] ready\r\n")
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					tag, rest, _ := strings.Cut(line, " ")
					if login && strings.HasPrefix(strings.ToUpper(rest), "LOGIN") {
						fmt.Fprintf(conn, "%s OK logged in\r\n", tag)
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return local(ln)
}

func TestSignInGivesUpOnASilentServer(t *testing.T) {
	defer func(old time.Duration) { signInTimeout = old }(signInTimeout)
	signInTimeout = 200 * time.Millisecond
	_, smtpSrv, _ := testServers(t)
	start := time.Now()
	_, err := NewSyncer(newTestStore(t), func() {}).AddAccount(AccountSettings{Email: "you@example.com", Password: "secret",
		IMAP: silentServer(t, false), SMTP: smtpSrv})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "stopped answering") || time.Since(start) > 3*time.Second {
		t.Errorf("after %v: %v", time.Since(start), err)
	}
}

func TestImportGivesUpOnAStalledServer(t *testing.T) {
	defer func(old time.Duration) { idleTimeout = old }(idleTimeout)
	idleTimeout = 300 * time.Millisecond
	keyring.MockInit()
	s := newTestStore(t)
	srv := silentServer(t, true)
	if err := s.CreateAccount(Account{ID: "acc", Name: "Home", Email: "you@example.com", Provider: "imap", Color: "c"},
		AccountSettings{Username: "you@example.com", IMAP: srv, SMTP: srv}); err != nil {
		t.Fatal(err)
	}
	keyring.Set(keyringService, "acc", "secret")
	im := NewSyncer(s, func() {})
	im.Start("acc")
	if p := waitImport(t, im, "acc"); p.State != "error" || !strings.Contains(p.Error, "stopped answering") {
		t.Errorf("progress %+v", p)
	}
}

func TestGmailAllMailIsTheArchive(t *testing.T) {
	all := &imap.ListData{Mailbox: "[Gmail]/All Mail", Delim: '/', Attrs: []imap.MailboxAttr{imap.MailboxAttrAll}}
	if skipFolder(all) {
		t.Error("All Mail is skipped, but on Gmail archived mail lives only there")
	}
	if r := mailboxRole(all); r == nil || *r != "archive" {
		t.Errorf("All Mail role %v, want archive (as Apple Mail treats it)", r)
	}
}

func TestSizedBatches(t *testing.T) {
	const mb = 1 << 20
	sizes := map[imap.UID]int64{1: 15 * mb, 2: 10 * mb, 3: 1024, 4: 30 * mb, 5: 1024}
	got := sizedBatches([]imap.UID{1, 2, 3, 4, 5}, sizes, 50, 20*mb)
	want := [][]imap.UID{{1}, {2, 3}, {4}, {5}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("by size: %v, want %v", got, want)
	}
	many := make([]imap.UID, 120)
	for i := range many {
		many[i] = imap.UID(i + 1)
	}
	if b := sizedBatches(many, nil, 50, 20*mb); len(b) != 3 || len(b[0]) != 50 || len(b[2]) != 20 {
		t.Errorf("by count: %d batches", len(b))
	}
}

// loginOnly is an SMTP server that offers AUTH LOGIN and not PLAIN, like many Exchange servers.
type loginOnly struct{ smtpSession }
type loginBackend struct{}

func (loginBackend) NewSession(*smtp.Conn) (smtp.Session, error) { return loginOnly{}, nil }
func (loginOnly) AuthMechanisms() []string                       { return []string{sasl.Login} }
func (loginOnly) Auth(string) (sasl.Server, error)               { return &loginServer{}, nil }

type loginServer struct {
	step int
	user string
}

func (l *loginServer) Next(response []byte) ([]byte, bool, error) {
	l.step++
	switch {
	case l.step == 1 && response == nil:
		return []byte("Username:"), false, nil
	case l.user == "":
		l.user = string(response)
		return []byte("Password:"), false, nil
	case l.user == "you@example.com" && string(response) == "secret":
		return nil, true, nil
	}
	return nil, true, errors.New("bad credentials")
}

func TestSMTPFallsBackToAuthLogin(t *testing.T) {
	if err := checkSMTP(serveSMTP(t, loginBackend{}), "you@example.com", "secret"); err != nil {
		t.Errorf("a LOGIN-only server: %v", err)
	}
}
