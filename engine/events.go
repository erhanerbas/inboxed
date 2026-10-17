package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var keepAlive = 25 * time.Second // a comment line now and then keeps idle connections from being cut

// Events tells every connected UI that something changed, and what new mail arrived. Change events carry no data
// (the UI reloads what it shows) and coalesce; mail events carry their messages and are queued.
type Events struct {
	mu      sync.Mutex
	subs    map[*subscriber]bool
	waiting [][]byte // mail found while no UI was listening (launch): the first one to connect gets it
}

type subscriber struct {
	change chan struct{} // at most one unsent change: a burst is one reload
	mail   chan []byte   // a few batches of new mail; the oldest gives way when a UI falls behind
}

func NewEvents() *Events { return &Events{subs: map[*subscriber]bool{}} }

// Publish never blocks.
func (e *Events) Publish() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for sub := range e.subs {
		select {
		case sub.change <- struct{}{}:
		default:
		}
	}
}

// Mail sends a batch of new mail to every UI. It never blocks.
func (e *Events) Mail(batch []NewMail) {
	data, err := json.Marshal(batch)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.subs) == 0 {
		e.waiting = append(e.waiting, data)
		if len(e.waiting) > 16 {
			e.waiting = e.waiting[1:]
		}
		return
	}
	for sub := range e.subs {
		for {
			select {
			case sub.mail <- data:
			default:
				select {
				case <-sub.mail: // full: drop the oldest batch and try again
				default:
				}
				continue
			}
			break
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
	sub := &subscriber{change: make(chan struct{}, 1), mail: make(chan []byte, 16)}
	e.mu.Lock()
	e.subs[sub] = true
	for _, data := range e.waiting {
		sub.mail <- data // fits: at most 16 wait, and the queue is new
	}
	e.waiting = nil
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.subs, sub)
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
		case <-sub.change:
			fmt.Fprint(w, "event: change\ndata: {}\n\n")
		case data := <-sub.mail:
			fmt.Fprintf(w, "event: mail\ndata: %s\n\n", data)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		flusher.Flush()
	}
}
