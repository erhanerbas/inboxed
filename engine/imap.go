package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	fetchBatch     = 50
)

// problem is an error meant for a person (a form, a status line). The API returns its text as is, with status 400.
type problem string

func (p problem) Error() string      { return string(p) }
func (problem) Is(target error) bool { return target == ErrInvalid }

func address(s Server) string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

func cantConnect(s Server, err error) error {
	return problem(fmt.Sprintf("Couldn't connect to %s. Check the server name, port and security. (%v)", address(s), err))
}

func refused(s Server, err error) error {
	return problem(fmt.Sprintf("%s didn't accept the username or password. (%v)", s.Host, err))
}

// Unencrypted connections are only for servers on this computer (tests, local bridges); anything else needs TLS.
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
func loginIMAP(s Server, username, password string) (*imapclient.Client, error) {
	if err := checkSecurity(s); err != nil {
		return nil, err
	}
	opts := &imapclient.Options{Dialer: &net.Dialer{Timeout: dialTimeout}}
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
	if err := c.Login(username, password).Wait(); err != nil {
		c.Close()
		return nil, refused(s, err)
	}
	return c, nil
}

// checkSMTP signs in to the outgoing server, so a typo shows up now rather than at the first send.
func checkSMTP(s Server, username, password string) error {
	if err := checkSecurity(s); err != nil {
		return err
	}
	conn, err := (&net.Dialer{Timeout: dialTimeout}).Dial("tcp", address(s))
	if err != nil {
		return cantConnect(s, err)
	}
	conn.SetDeadline(time.Now().Add(2 * dialTimeout)) // a server that accepts but never answers can't hang the form
	var c *smtp.Client
	switch s.Security {
	case "tls":
		tc := tls.Client(conn, &tls.Config{ServerName: s.Host})
		if err := tc.Handshake(); err != nil {
			conn.Close()
			return cantConnect(s, err)
		}
		c = smtp.NewClient(tc)
	case "starttls":
		if c, err = smtp.NewClientStartTLS(conn, &tls.Config{ServerName: s.Host}); err != nil {
			conn.Close()
			return cantConnect(s, err)
		}
	default:
		c = smtp.NewClient(conn)
	}
	defer c.Close()
	if err := c.Auth(sasl.NewPlainClient("", username, password)); err != nil {
		return refused(s, err)
	}
	return nil
}

// Importer copies accounts' mail from their IMAP servers into the store: one background job per account.
type Importer struct {
	store *Store
	mu    sync.Mutex
	jobs  map[string]*job
}

type job struct {
	progress Progress
	cancel   context.CancelFunc
}

func NewImporter(s *Store) *Importer { return &Importer{store: s, jobs: map[string]*job{}} }

// Progress is an account's import as of now, or nil when none has run since the engine started.
func (im *Importer) Progress(accountID string) *Progress {
	im.mu.Lock()
	defer im.mu.Unlock()
	if j, ok := im.jobs[accountID]; ok {
		p := j.progress
		return &p
	}
	return nil
}

func (im *Importer) report(accountID string, done, total int) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if j, ok := im.jobs[accountID]; ok {
		j.progress.Done, j.progress.Total = done, total
	}
}

// Start imports everything of an account it hasn't seen yet, in the background: inbox first, newest first.
// It does nothing while an import of the account is already running.
func (im *Importer) Start(accountID string) {
	im.mu.Lock()
	if j, ok := im.jobs[accountID]; ok && j.progress.State == "importing" {
		im.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{progress: Progress{State: "importing"}, cancel: cancel}
	im.jobs[accountID] = j
	im.mu.Unlock()

	go func() {
		err := im.importAccount(ctx, accountID)
		im.mu.Lock()
		defer im.mu.Unlock()
		if im.jobs[accountID] != j || ctx.Err() != nil {
			return // stopped: the account was removed
		}
		if err != nil {
			log.Printf("import %s: %v", accountID, err)
			j.progress.State, j.progress.Error = "error", err.Error()
		} else {
			j.progress.State = "done"
		}
	}()
}

// Stop cancels an account's import and forgets its progress.
func (im *Importer) Stop(accountID string) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if j, ok := im.jobs[accountID]; ok {
		j.cancel()
		delete(im.jobs, accountID)
	}
}

type folderWork struct {
	mailboxID, path string
	inbox           bool
	uids            []imap.UID // newest first
}

func (im *Importer) importAccount(ctx context.Context, accountID string) error {
	set, err := im.store.AccountSettings(accountID)
	if err != nil {
		return err
	}
	password, err := keyring.Get(keyringService, accountID)
	if err != nil {
		return problem("Inboxed couldn't read this account's password from the keychain. Remove the account and add it again.")
	}
	c, err := loginIMAP(set.IMAP, set.Username, password)
	if err != nil {
		return err
	}
	// A closure: `defer c.Logout().Wait()` would send LOGOUT right away and defer only the Wait.
	defer func() {
		c.Logout().Wait()
		c.Close()
	}()

	listed, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true}).Collect()
	if err != nil {
		return err
	}
	slices.SortFunc(listed, func(a, b *imap.ListData) int { return strings.Compare(a.Mailbox, b.Mailbox) }) // parents first
	var work []folderWork
	total := 0
	for _, box := range listed {
		if skipFolder(box) {
			continue
		}
		sel, err := c.Select(box.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return fmt.Errorf("opening %s: %w", box.Mailbox, err)
		}
		role := mailboxRole(box)
		id, err := im.store.UpsertMailbox(accountID, box.Mailbox, folderName(box), role, parentPath(box), sel.UIDValidity)
		if err != nil {
			return err
		}
		if sel.NumMessages == 0 {
			continue
		}
		found, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
		if err != nil {
			return err
		}
		known, err := im.store.KnownUIDs(id)
		if err != nil {
			return err
		}
		var uids []imap.UID
		for _, u := range found.AllUIDs() {
			if !known[uint32(u)] {
				uids = append(uids, u)
			}
		}
		slices.SortFunc(uids, func(a, b imap.UID) int { return cmp.Compare(b, a) })
		total += len(uids)
		work = append(work, folderWork{id, box.Mailbox, role != nil && *role == "inbox", uids})
	}
	// The inbox first: it's the mail people look at first.
	slices.SortStableFunc(work, func(a, b folderWork) int {
		if a.inbox == b.inbox {
			return 0
		}
		if a.inbox {
			return -1
		}
		return 1
	})

	done := 0
	im.report(accountID, done, total)
	body := &imap.FetchItemBodySection{Peek: true} // PEEK: importing never marks mail as read on the server
	for _, w := range work {
		if len(w.uids) == 0 {
			continue
		}
		if _, err := c.Select(w.path, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
			return fmt.Errorf("opening %s: %w", w.path, err)
		}
		for start := 0; start < len(w.uids); start += fetchBatch {
			if err := ctx.Err(); err != nil {
				return err
			}
			batch := w.uids[start:min(start+fetchBatch, len(w.uids))]
			msgs, err := c.Fetch(imap.UIDSetNum(batch...), &imap.FetchOptions{
				UID: true, Flags: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{body},
			}).Collect()
			if err != nil {
				return err
			}
			slices.SortFunc(msgs, func(a, b *imapclient.FetchMessageBuffer) int { return cmp.Compare(b.UID, a.UID) })
			for _, m := range msgs {
				done++
				p, err := parseMessage(m.FindBodySection(body), m.InternalDate)
				if err != nil {
					log.Printf("import %s: skipping UID %d in %s: %v", accountID, m.UID, w.path, err)
					continue
				}
				if err := im.store.AddMessage(Incoming{
					AccountID: accountID, MailboxID: w.mailboxID, UID: uint32(m.UID),
					Seen: slices.Contains(m.Flags, imap.FlagSeen), Flagged: slices.Contains(m.Flags, imap.FlagFlagged),
					parsed: p,
				}); err != nil {
					return err
				}
			}
			im.report(accountID, done, total)
		}
	}
	return nil
}

// skipFolder leaves out folders that can't be opened, and Gmail's views that only repeat mail filed elsewhere.
func skipFolder(box *imap.ListData) bool {
	for _, a := range box.Attrs {
		switch a {
		case imap.MailboxAttrNoSelect, imap.MailboxAttrNonExistent, imap.MailboxAttrAll, imap.MailboxAttrFlagged, imap.MailboxAttrImportant:
			return true
		}
	}
	return false
}

var specialUse = map[imap.MailboxAttr]string{
	imap.MailboxAttrSent: "sent", imap.MailboxAttrDrafts: "drafts", imap.MailboxAttrTrash: "trash",
	imap.MailboxAttrJunk: "spam", imap.MailboxAttrArchive: "archive",
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
// keychain and starts importing.
func (im *Importer) AddAccount(set AccountSettings) (Account, error) {
	set.Email = strings.TrimSpace(set.Email)
	set.Username = cmp.Or(strings.TrimSpace(set.Username), set.Email)
	if set.Email == "" || set.Password == "" || set.IMAP.Host == "" || set.SMTP.Host == "" || set.IMAP.Port == 0 || set.SMTP.Port == 0 {
		return Account{}, problem("Fill in the email address, the password and both servers.")
	}
	c, err := loginIMAP(set.IMAP, set.Username, set.Password)
	if err != nil {
		return Account{}, err
	}
	c.Logout().Wait()
	c.Close()
	if err := checkSMTP(set.SMTP, set.Username, set.Password); err != nil {
		return Account{}, err
	}

	existing, err := im.store.Accounts()
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
	if err := im.store.CreateAccount(a, set); err != nil {
		keyring.Delete(keyringService, a.ID)
		return Account{}, err
	}
	im.Start(a.ID)
	return a, nil
}

// RemoveAccount stops its import and deletes the account, its mail on this computer and its keychain entry.
// Nothing changes on the server.
func (im *Importer) RemoveAccount(id string) error {
	im.Stop(id)
	if err := im.store.DeleteAccount(id); err != nil {
		return err
	}
	if err := keyring.Delete(keyringService, id); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}
