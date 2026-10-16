package main

import (
	"cmp"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "dev.inboxed.app"
	dialTimeout    = 15 * time.Second
	headerBatch    = 200      // UIDs per header-only fetch
	bodyBatch      = 50       // whole messages per fetch at most
	bodyBatchBytes = 20 << 20 // …and roughly this many bytes, so big attachments never pile up in memory
)

// Variables so tests can shorten them. go-imap only times out the greeting, so a server that stops answering would
// otherwise hang a sign-in or an import forever.
var (
	signInTimeout = 45 * time.Second // the add-account form waits on this
	smtpTimeout   = time.Minute      // per SMTP command or reply; ponytail: raise for big attachments (Phase 6) on slow uplinks
	idleTimeout   = 2 * time.Minute  // a sync gives up when the server has answered nothing for this long (IDLE excepted)
)

// problem is an error meant for a person (a form, a status line). The API returns its text as is, with status 400.
type problem string

func (p problem) Error() string      { return string(p) }
func (problem) Is(target error) bool { return target == ErrInvalid }

// commandFailed is true when the server answered NO or BAD to one command (a folder it can't open, a message it
// can't send). The connection is still fine, so an import skips that item instead of stopping.
func commandFailed(err error) bool {
	var e *imap.Error
	return errors.As(err, &e)
}

func address(s Server) string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

func cantConnect(s Server, err error) error {
	return problem(fmt.Sprintf("Couldn't connect to %s. Check the server name, port and security. (%v)", address(s), err))
}

func refused(s Server, err error) error {
	return problem(fmt.Sprintf("%s didn't accept the username or password. (%v)", s.Host, err))
}

func stoppedAnswering(s Server) error {
	return problem(fmt.Sprintf("%s stopped answering. Try again later.", s.Host))
}

// Unencrypted connections are only for servers on this computer (tests); anything else needs TLS.
func checkSecurity(s Server) error {
	switch s.Security {
	case "tls", "starttls":
		return nil
	case "none":
		if s.Host == "localhost" || net.ParseIP(s.Host).IsLoopback() {
			return nil
		}
	}
	return problem(fmt.Sprintf("%s needs SSL/TLS or STARTTLS. Unencrypted connections are only allowed to this computer.", s.Host))
}

// loginIMAP connects and signs in, or explains in plain words what went wrong.
// loginIMAP signs in. handler, when set, receives what the server reports on its own (new mail, changed flags).
func loginIMAP(s Server, username, password string, handler *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	if err := checkSecurity(s); err != nil {
		return nil, err
	}
	opts := &imapclient.Options{Dialer: &net.Dialer{Timeout: dialTimeout}, UnilateralDataHandler: handler}
	var c *imapclient.Client
	var err error
	switch s.Security {
	case "tls":
		c, err = imapclient.DialTLS(address(s), opts)
	case "starttls":
		c, err = imapclient.DialStartTLS(address(s), opts)
	default:
		c, err = imapclient.DialInsecure(address(s), opts)
	}
	if err != nil {
		return nil, cantConnect(s, err)
	}
	timer := time.AfterFunc(signInTimeout, func() { c.Close() })
	err = c.Login(username, password).Wait()
	if !timer.Stop() {
		return nil, stoppedAnswering(s)
	}
	switch {
	case commandFailed(err): // the server said NO: that's the credentials
		c.Close()
		return nil, refused(s, err)
	case err != nil:
		c.Close()
		return nil, problem(fmt.Sprintf("The connection to %s dropped while signing in. Try again. (%v)", s.Host, err))
	}
	return c, nil
}

// checkSMTP signs in to the outgoing server, so a typo shows up now rather than at the first send.
func checkSMTP(s Server, username, password string) error {
	c, err := dialSMTP(s, username, password)
	if err != nil {
		return err
	}
	c.Quit()
	return nil
}

// cappedConn keeps every deadline within limit from now. go-smtp sets its own deadline before each command (5 to 12
// minutes, and none while idle), replacing any set on the connection, and its STARTTLS client can't be given shorter
// ones; capping them here is the one place that covers plain, TLS and STARTTLS alike.
type cappedConn struct {
	net.Conn
	limit time.Duration
}

func (c cappedConn) SetDeadline(t time.Time) error {
	if latest := time.Now().Add(c.limit); t.IsZero() || t.After(latest) {
		t = latest
	}
	return c.Conn.SetDeadline(t)
}

// dialSMTP connects and signs in. No SMTP command or reply can take longer than smtpTimeout, so a server that stops
// answering can't hang the add-account form or a send.
func dialSMTP(s Server, username, password string) (*smtp.Client, error) {
	if err := checkSecurity(s); err != nil {
		return nil, err
	}
	raw, err := (&net.Dialer{Timeout: dialTimeout}).Dial("tcp", address(s))
	if err != nil {
		return nil, cantConnect(s, err)
	}
	conn := cappedConn{raw, smtpTimeout}
	conn.SetDeadline(time.Time{}) // the greeting and TLS handshake, before go-smtp sets any
	var c *smtp.Client
	switch s.Security {
	case "tls":
		tc := tls.Client(conn, &tls.Config{ServerName: s.Host})
		if err := tc.Handshake(); err != nil {
			conn.Close()
			return nil, cantConnect(s, err)
		}
		c = smtp.NewClient(tc)
	case "starttls":
		if c, err = smtp.NewClientStartTLS(conn, &tls.Config{ServerName: s.Host}); err != nil {
			conn.Close()
			return nil, cantConnect(s, err)
		}
	default:
		c = smtp.NewClient(conn)
	}
	// PLAIN where offered; many Exchange servers only offer LOGIN.
	auth := sasl.NewPlainClient("", username, password)
	if !c.SupportsAuth(sasl.Plain) && c.SupportsAuth(sasl.Login) {
		auth = sasl.NewLoginClient(username, password)
	}
	if err := c.Auth(auth); err != nil {
		c.Close()
		return nil, refused(s, err)
	}
	return c, nil
}

// sizedBatches splits UIDs into fetches of at most maxCount messages and roughly maxBytes, so a run of large
// attachments never sits in memory all at once. A message bigger than maxBytes gets a batch of its own.
func sizedBatches(uids []imap.UID, sizes map[imap.UID]int64, maxCount int, maxBytes int64) [][]imap.UID {
	var out [][]imap.UID
	var cur []imap.UID
	var size int64
	for _, u := range uids {
		if len(cur) > 0 && (len(cur) == maxCount || size+sizes[u] > maxBytes) {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, u)
		size += sizes[u]
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// skipFolder leaves out folders that can't be opened, and Gmail views that only repeat mail filed elsewhere.
// All Mail stays: on Gmail, archived mail lives nowhere else.
func skipFolder(box *imap.ListData) bool {
	for _, a := range box.Attrs {
		switch a {
		case imap.MailboxAttrNoSelect, imap.MailboxAttrNonExistent, imap.MailboxAttrFlagged, imap.MailboxAttrImportant:
			return true
		}
	}
	return false
}

// Gmail's All Mail (\All) is its archive, as Apple Mail treats it.
var specialUse = map[imap.MailboxAttr]string{
	imap.MailboxAttrSent: "sent", imap.MailboxAttrDrafts: "drafts", imap.MailboxAttrTrash: "trash",
	imap.MailboxAttrJunk: "spam", imap.MailboxAttrArchive: "archive", imap.MailboxAttrAll: "archive",
}

var roleNames = map[string]string{
	"sent": "sent", "sent items": "sent", "sent mail": "sent", "sent messages": "sent",
	"drafts": "drafts", "trash": "trash", "deleted items": "trash", "deleted messages": "trash", "bin": "trash",
	"junk": "spam", "spam": "spam", "junk e-mail": "spam", "junk email": "spam", "archive": "archive", "archives": "archive",
}

// mailboxRole maps a server folder onto Inboxed's roles: by its SPECIAL-USE attribute when the server sends one,
// else by a well-known top-level name. Nil means a user folder.
func mailboxRole(box *imap.ListData) *string {
	role := ""
	if strings.EqualFold(box.Mailbox, "INBOX") {
		role = "inbox"
	}
	for _, a := range box.Attrs {
		if r, ok := specialUse[a]; ok {
			role = r
		}
	}
	if role == "" && parentPath(box) == "" {
		role = roleNames[strings.ToLower(box.Mailbox)]
	}
	if role == "" {
		return nil
	}
	return &role
}

func folderName(box *imap.ListData) string {
	if strings.EqualFold(box.Mailbox, "INBOX") {
		return "Inbox"
	}
	if box.Delim == 0 {
		return box.Mailbox
	}
	return box.Mailbox[strings.LastIndex(box.Mailbox, string(box.Delim))+1:]
}

func parentPath(box *imap.ListData) string {
	if box.Delim == 0 {
		return ""
	}
	if i := strings.LastIndex(box.Mailbox, string(box.Delim)); i > 0 {
		return box.Mailbox[:i]
	}
	return ""
}

// Account identity colors, never the accent's orange hue.
var accountColors = []string{
	"oklch(0.5 0.11 160)", "oklch(0.5 0.12 250)", "oklch(0.5 0.12 300)",
	"oklch(0.5 0.1 200)", "oklch(0.5 0.12 120)", "oklch(0.5 0.12 330)",
}

// AddAccount signs in to both servers before saving anything, then stores the account, keeps the password in the
// keychain and starts syncing.
func (sy *Syncer) AddAccount(set AccountSettings) (Account, error) {
	set.Email = strings.TrimSpace(set.Email)
	set.Username = cmp.Or(strings.TrimSpace(set.Username), set.Email)
	if set.Email == "" || set.Password == "" || set.IMAP.Host == "" || set.SMTP.Host == "" || set.IMAP.Port == 0 || set.SMTP.Port == 0 {
		return Account{}, problem("Fill in the email address, the password and both servers.")
	}
	c, err := loginIMAP(set.IMAP, set.Username, set.Password, nil)
	if err != nil {
		return Account{}, err
	}
	c.Logout().Wait()
	c.Close()
	if err := checkSMTP(set.SMTP, set.Username, set.Password); err != nil {
		return Account{}, err
	}

	existing, err := sy.store.Accounts()
	if err != nil {
		return Account{}, err
	}
	a := Account{
		ID:       "acc-" + strings.ToLower(rand.Text()[:12]),
		Name:     cmp.Or(strings.TrimSpace(set.Name), set.Email),
		Email:    set.Email,
		Provider: "imap",
		Color:    accountColors[len(existing)%len(accountColors)],
	}
	if err := keyring.Set(keyringService, a.ID, set.Password); err != nil {
		return Account{}, fmt.Errorf("saving the password to the keychain: %w", err)
	}
	if err := sy.store.CreateAccount(a, set); err != nil {
		keyring.Delete(keyringService, a.ID)
		return Account{}, err
	}
	sy.Start(a.ID)
	return a, nil
}

// RemoveAccount stops its sync and deletes the account, its mail on this computer and its keychain entry.
// Nothing changes on the server.
func (sy *Syncer) RemoveAccount(id string) error {
	sy.Stop(id)
	if err := sy.store.DeleteAccount(id); err != nil {
		return err
	}
	if err := keyring.Delete(keyringService, id); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}
