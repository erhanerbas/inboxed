package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	netmail "net/mail"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/zalando/go-keyring"
)

// Variables so tests can shorten them.
var (
	checkInterval = 5 * time.Minute // IDLE on the inbox at most this long before checking every folder
	pollInterval  = 2 * time.Minute // servers without IDLE: check this often
	retryMax      = 5 * time.Minute // longest wait between reconnects
	fullEvery     = 12              // every 12th pass (about an hour) refreshes flags even where STATUS didn't change
)

// Syncer keeps every IMAP account in sync both ways: one goroutine and one connection per account.
type Syncer struct {
	store    *Store
	onChange func()
	mu       sync.Mutex
	jobs     map[string]*job
	cycles   atomic.Int64 // completed passes, for tests
}

type job struct {
	id       string
	progress Progress
	cancel   context.CancelFunc
	wake     chan struct{}
}

func NewSyncer(s *Store, onChange func()) *Syncer {
	return &Syncer{store: s, onChange: onChange, jobs: map[string]*job{}}
}

// Progress is an account's sync as of now, or nil when it isn't running.
func (sy *Syncer) Progress(accountID string) *Progress {
	sy.mu.Lock()
	defer sy.mu.Unlock()
	if j, ok := sy.jobs[accountID]; ok {
		p := j.progress
		return &p
	}
	return nil
}

// update changes a running job's progress (ignored once the job was stopped) and tells the UI.
func (sy *Syncer) update(j *job, f func(*Progress)) {
	sy.mu.Lock()
	if sy.jobs[j.id] == j {
		f(&j.progress)
	}
	sy.mu.Unlock()
	sy.onChange()
}

func wake(j *job) {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// Start keeps an account in sync until Stop. Starting one that is already running wakes it for an immediate pass.
func (sy *Syncer) Start(accountID string) {
	sy.mu.Lock()
	defer sy.mu.Unlock()
	if j, ok := sy.jobs[accountID]; ok {
		wake(j)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{id: accountID, progress: Progress{State: "importing"}, cancel: cancel, wake: make(chan struct{}, 1)}
	sy.jobs[accountID] = j
	go sy.run(ctx, j)
}

// WakeAll asks every account for an immediate pass (a local change is waiting to be sent).
func (sy *Syncer) WakeAll() {
	sy.mu.Lock()
	defer sy.mu.Unlock()
	for _, j := range sy.jobs {
		wake(j)
	}
}

// Stop ends an account's sync (closing its connection, so even a stalled command ends) and forgets its progress.
func (sy *Syncer) Stop(accountID string) {
	sy.mu.Lock()
	defer sy.mu.Unlock()
	if j, ok := sy.jobs[accountID]; ok {
		j.cancel()
		delete(sy.jobs, accountID)
	}
}

// run reconnects after every failure, waiting 1 s, 2 s, 4 s … up to retryMax; a wake (Try again) cuts the wait short.
func (sy *Syncer) run(ctx context.Context, j *job) {
	delay := time.Second
	for {
		synced, err := sy.session(ctx, j)
		if ctx.Err() != nil {
			return
		}
		if synced {
			delay = time.Second
		}
		log.Printf("sync %s: %v", j.id, err)
		sy.update(j, func(p *Progress) { p.State, p.Error = "error", err.Error() })
		select {
		case <-time.After(delay):
		case <-j.wake:
		case <-ctx.Done():
			return
		}
		delay = min(2*delay, retryMax)
	}
}

// link watches one connection. Closing it is the only way to end a command the server never answers: on Stop, and
// when the server has been silent for idleTimeout.
type link struct {
	c          *imapclient.Client
	watchdog   *time.Timer
	stalled    atomic.Bool
	stopCancel func() bool
}

func watch(ctx context.Context, c *imapclient.Client) *link {
	l := &link{c: c}
	l.watchdog = time.AfterFunc(idleTimeout, func() { l.stalled.Store(true); c.Close() })
	l.stopCancel = context.AfterFunc(ctx, func() { c.Close() })
	return l
}

func (l *link) alive() { l.watchdog.Reset(idleTimeout) }
func (l *link) pause() { l.watchdog.Stop() } // during IDLE the server may rightly say nothing for minutes

func (l *link) end(err error, s Server) error {
	l.alive()
	l.c.Logout().Wait()
	l.c.Close()
	l.watchdog.Stop()
	l.stopCancel()
	if l.stalled.Load() {
		return stoppedAnswering(s)
	}
	return err
}

// session is one connection's life: send local changes, sync folders, wait for the server; repeat until the
// connection fails or the account is stopped. synced reports whether at least one pass completed.
func (sy *Syncer) session(ctx context.Context, j *job) (synced bool, err error) {
	set, err := sy.store.AccountSettings(j.id)
	if err != nil {
		return false, err
	}
	password, err := keyring.Get(keyringService, j.id)
	if err != nil {
		return false, problem("Inboxed couldn't read this account's password from the keychain. Remove the account and add it again.")
	}
	changed := make(chan struct{}, 1) // the handler blocks the client, so it only leaves a note
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	c, err := loginIMAP(set.IMAP, set.Username, password, &imapclient.UnilateralDataHandler{
		Expunge: func(uint32) { notify() },
		Mailbox: func(*imapclient.UnilateralDataMailbox) { notify() },
		Fetch:   func(*imapclient.FetchMessageData) { notify() },
	})
	if err != nil {
		return false, err
	}
	l := watch(ctx, c)
	defer func() { err = l.end(err, set.IMAP) }()
	idle := c.Caps().Has(imap.CapIdle)
	for pass := 0; ; pass++ {
		if err := sy.flush(c, l, j); err != nil {
			return synced, err
		}
		inbox, err := sy.syncFolders(ctx, c, l, j, pass%fullEvery == 0)
		if err != nil {
			return synced, err
		}
		synced = true
		sy.cycles.Add(1)
		sy.update(j, func(p *Progress) { p.State, p.Error = "done", "" })
		if err := wait(ctx, l, inbox, idle, changed, j.wake); err != nil {
			return synced, err
		}
	}
}

// watched is the inbox as the last folder sync left it: where it is, and how it looked.
type watched struct {
	path        string
	uidNext     imap.UID
	numMessages uint32
}

// wait returns when the server reports a change (IDLE on the inbox), a local change is waiting (wake), or it's time
// for the regular check of every folder.
func wait(ctx context.Context, l *link, inbox watched, idle bool, changed, wakeup <-chan struct{}) error {
	// Notes left by Inboxed's own commands (a MOVE's expunges) aren't news.
	select {
	case <-changed:
	default:
	}
	c, interval := l.c, pollInterval
	var cmd *imapclient.IdleCommand
	if idle && inbox.path != "" {
		interval = checkInterval
		l.alive()
		sel, err := c.Select(inbox.path, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return err
		}
		if sel.UIDNext != inbox.uidNext || sel.NumMessages != inbox.numMessages {
			return nil // it changed since the folder sync looked: go again rather than wait
		}
		if cmd, err = c.Idle(); err != nil {
			return err
		}
		l.pause()
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-changed:
	case <-wakeup:
	case <-timer.C:
	case <-c.Closed():
	case <-ctx.Done():
	}
	if cmd == nil {
		return ctx.Err()
	}
	l.alive()
	if err := cmd.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}

// flush sends local changes to the server, oldest first. A change the server refuses is retried on the next pass and
// dropped after maxAttempts (the thread then shows the server's state again).
func (sy *Syncer) flush(c *imapclient.Client, l *link, j *job) error {
	changes, err := sy.store.PendingChanges(j.id)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		targets, err := sy.store.ChangeTargets(ch)
		if err == nil {
			err = apply(c, l, sy.store, ch, targets)
		}
		switch {
		case err == nil:
			err = sy.store.FinishChange(ch.ID)
		case commandFailed(err) || errors.Is(err, ErrInvalid):
			dropped, ferr := sy.store.FailChange(ch.ID)
			log.Printf("sync %s: %s=%s on %s refused (dropped: %v): %v", j.id, ch.Kind, ch.Value, ch.ThreadID, dropped, err)
			err = ferr
		}
		if err != nil {
			return err
		}
	}
	if len(changes) > 0 {
		sy.onChange()
	}
	return nil
}

func uidSet(uids []uint32) imap.UIDSet {
	out := make([]imap.UID, len(uids))
	for i, u := range uids {
		out[i] = imap.UID(u)
	}
	return imap.UIDSetNum(out...)
}

// apply performs one change on the server and records what the server now has.
func apply(c *imapclient.Client, l *link, s *Store, ch Change, targets []Target) error {
	for _, t := range targets {
		l.alive()
		if _, err := c.Select(t.Path, nil).Wait(); err != nil {
			return err
		}
		l.alive()
		if ch.Kind == "move" {
			if _, err := c.Move(uidSet(t.UIDs), t.Dest).Wait(); err != nil {
				return err
			}
			// Gone from here; the copy in the destination (under a new UID) arrives with the next folder sync.
			if err := s.RemoveLocations(t.MailboxID, t.UIDs); err != nil {
				return err
			}
			continue
		}
		flag := map[string]imap.Flag{"seen": imap.FlagSeen, "flagged": imap.FlagFlagged}[ch.Kind]
		op := imap.StoreFlagsAdd
		if ch.Value == "0" {
			op = imap.StoreFlagsDel
		}
		if err := c.Store(uidSet(t.UIDs), &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{flag}}, nil).Close(); err != nil {
			return err
		}
		if err := s.MarkLocations(t.MailboxID, t.UIDs, ch.Kind, ch.Value == "1"); err != nil {
			return err
		}
	}
	return nil
}

type folderWork struct {
	mailboxID, path, status string
	rank                    int        // 0 inbox, 1 other folders, 2 All Mail: the mail people look at first comes first
	uids                    []imap.UID // new on the server, newest first
}

// syncFolders brings every folder in line with the server: new folders and mail imported, vanished ones forgotten,
// flags refreshed. A folder whose STATUS is unchanged is skipped unless full is set (without CONDSTORE, flag
// changes other than \Seen don't show in STATUS). Returns the inbox, for IDLE.
func (sy *Syncer) syncFolders(ctx context.Context, c *imapclient.Client, l *link, j *job, full bool) (inbox watched, err error) {
	l.alive()
	listed, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true}).Collect()
	if err != nil {
		return inbox, err
	}
	slices.SortFunc(listed, func(a, b *imap.ListData) int { return strings.Compare(a.Mailbox, b.Mailbox) }) // parents first
	stored, err := sy.store.ServerMailboxes(j.id)
	if err != nil {
		return inbox, err
	}
	listedPaths := map[string]bool{}
	var work []folderWork
	total := 0
	for _, box := range listed {
		if skipFolder(box) {
			continue
		}
		listedPaths[box.Mailbox] = true
		role := mailboxRole(box)
		l.alive()
		st, err := c.Status(box.Mailbox, &imap.StatusOptions{UIDValidity: true, NumMessages: true, UIDNext: true, NumUnseen: true}).Wait()
		if commandFailed(err) {
			log.Printf("sync %s: skipping %s: %v", j.id, box.Mailbox, err)
			continue
		} else if err != nil {
			return inbox, err
		}
		if role != nil && *role == "inbox" {
			inbox = watched{box.Mailbox, st.UIDNext, deref(st.NumMessages)}
		}
		status := fmt.Sprintf("%d/%d/%d/%d", st.UIDValidity, deref(st.NumMessages), st.UIDNext, deref(st.NumUnseen))
		if !full && sy.store.MailboxStatus(j.id+"/"+box.Mailbox) == status {
			continue
		}
		id, fresh, err := sy.reconcile(c, l, j, box, role)
		if commandFailed(err) {
			log.Printf("sync %s: skipping %s: %v", j.id, box.Mailbox, err)
			continue
		} else if err != nil {
			return inbox, err
		}
		rank := 1
		switch {
		case role != nil && *role == "inbox":
			rank = 0
		case slices.Contains(box.Attrs, imap.MailboxAttrAll):
			rank = 2 // All Mail repeats most other folders, so by then it's mostly headers only
		}
		total += len(fresh)
		work = append(work, folderWork{id, box.Mailbox, status, rank, fresh})
	}
	// Folders gone from the server, deepest first.
	var gone []string
	for path := range stored {
		if !listedPaths[path] {
			gone = append(gone, path)
		}
	}
	slices.SortFunc(gone, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	for _, path := range gone {
		if err := sy.store.DeleteMailbox(stored[path]); err != nil {
			return inbox, err
		}
	}

	slices.SortStableFunc(work, func(a, b folderWork) int { return cmp.Compare(a.rank, b.rank) })
	done, before := 0, 0 // before: UIDs in the folders already finished
	if total > 0 {
		sy.update(j, func(p *Progress) { p.State, p.Done, p.Total = "importing", 0, total })
	}
	for _, w := range work {
		if len(w.uids) > 0 {
			l.alive()
			if _, err := c.Select(w.path, &imap.SelectOptions{ReadOnly: true}).Wait(); commandFailed(err) {
				log.Printf("sync %s: skipping %s: %v", j.id, w.path, err)
				before += len(w.uids)
				continue
			} else if err != nil {
				return inbox, err
			}
			for start := 0; start < len(w.uids); start += headerBatch {
				if err := ctx.Err(); err != nil {
					return inbox, err
				}
				end := min(start+headerBatch, len(w.uids))
				if err := sy.importChunk(c, l, j, w, w.uids[start:end], func() {
					done++
					sy.update(j, func(p *Progress) { p.Done = done })
				}); err != nil {
					return inbox, err
				}
				done = before + end // exact even when the server dropped UIDs between the search and the fetch
				sy.update(j, func(p *Progress) { p.Done = done })
			}
			before += len(w.uids)
		}
		// Only now is this folder caught up; an interruption before here syncs it again next time.
		if err := sy.store.SetMailboxStatus(w.mailboxID, w.status); err != nil {
			return inbox, err
		}
	}
	return inbox, nil
}

func deref(n *uint32) uint32 {
	if n == nil {
		return 0
	}
	return *n
}

// reconcile opens one folder and compares it with what's stored: the folder is recorded, vanished UIDs are
// forgotten, flags of the rest refreshed, and the UIDs new on the server returned (newest first) for importing.
func (sy *Syncer) reconcile(c *imapclient.Client, l *link, j *job, box *imap.ListData, role *string) (string, []imap.UID, error) {
	l.alive()
	sel, err := c.Select(box.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return "", nil, err
	}
	// Recorded only once the server lets us in; a changed UIDVALIDITY forgets the old UIDs here.
	id, err := sy.store.UpsertMailbox(j.id, box.Mailbox, folderName(box), role, parentPath(box), sel.UIDValidity)
	if err != nil {
		return "", nil, err
	}
	known, err := sy.store.KnownUIDs(id)
	if err != nil {
		return "", nil, err
	}
	onServer := map[uint32]bool{}
	var fresh []imap.UID
	if sel.NumMessages > 0 {
		l.alive()
		found, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
		if err != nil {
			return "", nil, err
		}
		for _, u := range found.AllUIDs() {
			onServer[uint32(u)] = true
			if !known[uint32(u)] {
				fresh = append(fresh, u)
			}
		}
	}
	var vanished []uint32
	for u := range known {
		if !onServer[u] {
			vanished = append(vanished, u)
		}
	}
	if len(vanished) > 0 {
		if err := sy.store.RemoveLocations(id, vanished); err != nil {
			return "", nil, err
		}
	}
	if len(known) > len(vanished) {
		l.alive()
		msgs, err := c.Fetch(imap.UIDSet{imap.UIDRange{Start: 1, Stop: 0}}, &imap.FetchOptions{UID: true, Flags: true}).Collect()
		if err != nil {
			return "", nil, err
		}
		flags := map[uint32][2]bool{}
		for _, m := range msgs {
			if known[uint32(m.UID)] {
				flags[uint32(m.UID)] = [2]bool{slices.Contains(m.Flags, imap.FlagSeen), slices.Contains(m.Flags, imap.FlagFlagged)}
			}
		}
		if err := sy.store.SetLocationFlags(id, flags); err != nil {
			return "", nil, err
		}
	}
	slices.SortFunc(fresh, func(a, b imap.UID) int { return cmp.Compare(b, a) })
	return id, fresh, nil
}

var (
	idHeader  = &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: []string{"Message-ID"}, Peek: true}
	wholeBody = &imap.FetchItemBodySection{Peek: true} // PEEK: importing never marks mail as read on the server
)

// importChunk imports a run of UIDs from the selected folder in two passes. Headers first: a message already stored
// (a copy in another folder, a Gmail label, All Mail) only gains a location. Then whole messages, for the rest only.
func (sy *Syncer) importChunk(c *imapclient.Client, l *link, j *job, w folderWork, uids []imap.UID, step func()) error {
	l.alive()
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
				ok, err := sy.store.AddLocation(j.id, w.mailboxID, uint32(h.UID), id,
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
		if err := sy.fetchBodies(c, l, j, w, batch, step); err != nil {
			return err
		}
	}
	return nil
}

// fetchBodies downloads and stores whole messages. When the server refuses a batch (one corrupt or oversized
// message), it retries one message at a time and skips only the ones it can't get.
func (sy *Syncer) fetchBodies(c *imapclient.Client, l *link, j *job, w folderWork, uids []imap.UID, step func()) error {
	l.alive()
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Flags: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{wholeBody},
	}).Collect()
	switch {
	case commandFailed(err) && len(uids) > 1:
		for _, u := range uids {
			if err := sy.fetchBodies(c, l, j, w, []imap.UID{u}, step); err != nil {
				return err
			}
		}
		return nil
	case commandFailed(err):
		log.Printf("sync %s: skipping UID %d in %s: %v", j.id, uids[0], w.path, err)
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
			log.Printf("sync %s: skipping UID %d in %s: %v", j.id, m.UID, w.path, err)
			continue
		}
		if err := sy.store.AddMessage(Incoming{
			AccountID: j.id, MailboxID: w.mailboxID, UID: uint32(m.UID),
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
