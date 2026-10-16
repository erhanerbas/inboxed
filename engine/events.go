package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

var keepAlive = 25 * time.Second // a comment line now and then keeps idle connections from being cut

// Events tells every connected UI that something changed. Events carry no data: the UI reloads what it shows.
type Events struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func NewEvents() *Events { return &Events{subs: map[chan struct{}]bool{}} }

// Publish never blocks: each subscriber holds at most one unsent event, so a burst becomes one reload.
func (e *Events) Publish() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// ServeHTTP streams Server-Sent Events until the client goes away.
func (e *Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan struct{}, 1)
	e.mu.Lock()
	e.subs[ch] = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.subs, ch)
		e.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: change\ndata: {}\n\n")
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		flusher.Flush()
	}
}
