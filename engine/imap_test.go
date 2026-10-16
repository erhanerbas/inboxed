package main

import (
	"errors"
	"io"
	"net"
	"slices"
	"strings"
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

	is := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	iln := listen(t)
	go is.Serve(iln)
	t.Cleanup(func() { is.Close() })

	ss := smtp.NewServer(smtpBackend{})
	ss.AllowInsecureAuth = true
	sln := listen(t)
	go ss.Serve(sln)
	t.Cleanup(func() { ss.Close() })
	return local(iln), local(sln), user
}

func waitImport(t *testing.T, im *Importer, id string) Progress {
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
	im := NewImporter(s)
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
	im := NewImporter(s)
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

	// A second run fetches only what's new.
	appendRaw(t, user, "INBOX", newsletter)
	im.Start(a.ID)
	if p := waitImport(t, im, a.ID); p.Done != 1 || p.Total != 1 {
		t.Errorf("second run: %+v", p)
	}
	var messages int
	s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE id LIKE ?`, a.ID+"/%").Scan(&messages)
	if messages != 5 {
		t.Errorf("%d messages after the second run, want 5", messages)
	}

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
