package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	idleTimeout   = 2 * time.Minute  // an import gives up when the server has answered nothing for this long
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
	// PLAIN where offered; many Exchange servers only offer LOGIN.
	auth := sasl.NewPlainClient("", username, password)
	if !c.SupportsAuth(sasl.Plain) && c.SupportsAuth(sasl.Login) {
		auth = sasl.NewLoginClient(username, password)
	}
	if err := c.Auth(auth); err != nil {
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

// Stop cancels an account's import (closing its connection, so even a stalled command ends) and forgets its progress.
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
	rank            int        // 0 inbox, 1 other folders, 2 All Mail: the mail people look at first comes first
	uids            []imap.UID // newest first
}

var (
	idHeader  = &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: []string{"Message-ID"}, Peek: true}
	wholeBody = &imap.FetchItemBodySection{Peek: true} // PEEK: importing never marks mail as read on the server
)

func (im *Importer) importAccount(ctx context.Context, accountID string) (err error) {
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
	// Closing the connection is the only way to end a command the server never answers: on cancel (the account was
	// removed) and when the server has been silent for idleTimeout.
	var stalled atomic.Bool
	watchdog := time.AfterFunc(idleTimeout, func() { stalled.Store(true); c.Close() })
	alive := func() { watchdog.Reset(idleTimeout) }
	stopOnCancel := context.AfterFunc(ctx, func() { c.Close() })
	defer func() {
		alive()
		c.Logout().Wait()
		c.Close()
		watchdog.Stop()
		stopOnCancel()
		if stalled.Load() {
			err = stoppedAnswering(set.IMAP)
		}
	}()

	alive()
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
		alive()
		sel, err := c.Select(box.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
		if commandFailed(err) {
			log.Printf("import %s: skipping %s: %v", accountID, box.Mailbox, err)
			continue
		} else if err != nil {
			return err
		}
		role := mailboxRole(box)
		id, err := im.store.UpsertMailbox(accountID, box.Mailbox, folderName(box), role, parentPath(box), sel.UIDValidity)
		if err != nil {
			return err
		}
		if sel.NumMessages == 0 {
			continue
		}
		alive()
		found, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
		if commandFailed(err) {
			log.Printf("import %s: skipping %s: %v", accountID, box.Mailbox, err)
			continue
		} else if err != nil {
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
		rank := 1
		switch {
		case role != nil && *role == "inbox":
			rank = 0
		case slices.Contains(box.Attrs, imap.MailboxAttrAll):
			rank = 2 // All Mail repeats most of the other folders, so by then it's mostly headers-only
		}
		work = append(work, folderWork{id, box.Mailbox, rank, uids})
	}
	slices.SortStableFunc(work, func(a, b folderWork) int { return cmp.Compare(a.rank, b.rank) })

	done, before := 0, 0 // before: UIDs in the folders already finished
	im.report(accountID, done, total)
	for _, w := range work {
		if len(w.uids) == 0 {
			continue
		}
		alive()
		if _, err := c.Select(w.path, &imap.SelectOptions{ReadOnly: true}).Wait(); commandFailed(err) {
			log.Printf("import %s: skipping %s: %v", accountID, w.path, err)
			before += len(w.uids)
			done = before
			continue
		} else if err != nil {
			return err
		}
		for start := 0; start < len(w.uids); start += headerBatch {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := min(start+headerBatch, len(w.uids))
			if err := im.importChunk(c, alive, accountID, w, w.uids[start:end], func() {
				done++
				im.report(accountID, done, total)
			}); err != nil {
				return err
			}
			done = before + end // exact even when the server dropped UIDs between the search and the fetch
			im.report(accountID, done, total)
		}
		before += len(w.uids)
	}
	return nil
}

// importChunk imports a run of UIDs from the selected folder in two passes. Headers first: a message already stored
// (a copy in another folder, a Gmail label, All Mail) only gains a location. Then whole messages, for the rest only.
func (im *Importer) importChunk(c *imapclient.Client, alive func(), accountID string, w folderWork, uids []imap.UID, step func()) error {
	alive()
	heads, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Flags: true, RFC822Size: true, BodySection: []*imap.FetchItemBodySection{idHeader},
	}).Collect()
	if err != nil && !commandFailed(err) {
		return err
	}
	sizes := map[imap.UID]int64{}
	var need []imap.UID
	if err != nil {
		need = uids // the server refused the header pass: fetch everything whole instead
	} else {
		for _, h := range heads {
			sizes[h.UID] = h.RFC822Size
			if id := messageID(h.FindBodySection(idHeader)); id != "" {
				ok, err := im.store.AddLocation(accountID, w.mailboxID, uint32(h.UID), id,
					slices.Contains(h.Flags, imap.FlagSeen), slices.Contains(h.Flags, imap.FlagFlagged))
				if err != nil {
					return err
				}
				if ok {
					step()
					continue
				}
			}
			need = append(need, h.UID)
		}
	}
	slices.SortFunc(need, func(a, b imap.UID) int { return cmp.Compare(b, a) })
	for _, batch := range sizedBatches(need, sizes, bodyBatch, bodyBatchBytes) {
		if err := im.fetchBodies(c, alive, accountID, w, batch, step); err != nil {
			return err
		}
	}
	return nil
}

// fetchBodies downloads and stores whole messages. When the server refuses a batch (one corrupt or oversized
// message), it retries one message at a time and skips only the ones it can't get.
func (im *Importer) fetchBodies(c *imapclient.Client, alive func(), accountID string, w folderWork, uids []imap.UID, step func()) error {
	alive()
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Flags: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{wholeBody},
	}).Collect()
	switch {
	case commandFailed(err) && len(uids) > 1:
		for _, u := range uids {
			if err := im.fetchBodies(c, alive, accountID, w, []imap.UID{u}, step); err != nil {
				return err
			}
		}
		return nil
	case commandFailed(err):
		log.Printf("import %s: skipping UID %d in %s: %v", accountID, uids[0], w.path, err)
		step()
		return nil
	case err != nil:
		return err
	}
	slices.SortFunc(msgs, func(a, b *imapclient.FetchMessageBuffer) int { return cmp.Compare(b.UID, a.UID) })
	for _, m := range msgs {
		step()
		p, err := parseMessage(m.FindBodySection(wholeBody), m.InternalDate)
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
	return nil
}

// messageID reads the Message-ID out of a header-only fetch, as parseMessage would (no angle brackets).
func messageID(header []byte) string {
	msg, err := netmail.ReadMessage(bytes.NewReader(append(header, "\r\n"...)))
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(msg.Header.Get("Message-Id")), "<>")
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
