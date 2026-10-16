package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

func routes(s *Store, sy *Syncer, ev *Events) http.Handler {
	// A local change on an IMAP account is queued; waking the syncer sends it now rather than at the next check.
	sent := func(err error) error {
		if err == nil {
			sy.WakeAll()
		}
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /events", ev)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]string{"status": "ok"}, nil)
	})
	mux.HandleFunc("GET /accounts", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Accounts()
		for i := range v {
			v[i].Sync = sy.Progress(v[i].ID)
		}
		respond(w, v, err)
	})
	mux.HandleFunc("POST /accounts", func(w http.ResponseWriter, r *http.Request) {
		var body AccountSettings
		if decode(w, r, &body) {
			v, err := sy.AddAccount(body)
			respond(w, v, err)
		}
	})
	mux.HandleFunc("DELETE /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, sy.RemoveAccount(r.PathValue("id")))
	})
	mux.HandleFunc("POST /accounts/{id}/sync", func(w http.ResponseWriter, r *http.Request) {
		set, err := s.AccountSettings(r.PathValue("id"))
		if err == nil && set.IMAP.Host == "" {
			err = problem("This account has no mail server to sync with.")
		}
		if err == nil {
			sy.Start(r.PathValue("id"))
		}
		respond(w, nil, err)
	})
	mux.HandleFunc("GET /mailboxes", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Mailboxes()
		respond(w, v, err)
	})
	mux.HandleFunc("GET /threads", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		v, err := s.Threads(q.Get("account"), q.Get("key"), q.Get("q"))
		respond(w, v, err)
	})
	mux.HandleFunc("GET /threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Thread(r.PathValue("id"))
		respond(w, v, err)
	})
	mux.HandleFunc("PATCH /threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Unread, Starred *bool }
		if decode(w, r, &body) {
			respond(w, nil, sent(s.SetFlags(r.PathValue("id"), body.Unread, body.Starred)))
		}
	})
	mux.HandleFunc("POST /threads/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ To string }
		if decode(w, r, &body) {
			respond(w, nil, sent(s.Move(r.PathValue("id"), body.To)))
		}
	})
	mux.HandleFunc("POST /read-all", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Account, Key string }
		if decode(w, r, &body) {
			respond(w, nil, sent(s.MarkAllRead(body.Account, body.Key)))
		}
	})
	return mux
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		respond(w, nil, fmt.Errorf("%w: %v", ErrInvalid, err))
		return false
	}
	return true
}

// respond writes v as JSON, an empty 204 when v is nil, or {"error": …} with the status the error calls for.
func respond(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case errors.Is(err, ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		w.WriteHeader(http.StatusBadRequest)
	case err != nil:
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
	case v == nil:
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		json.NewEncoder(w).Encode(v)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
