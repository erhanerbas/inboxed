package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func routes(s *Store, sy *Syncer, ev *Events) http.Handler {
	// A local change on an IMAP account is queued; waking its syncer sends it now rather than at the next check.
	// Thread ids start with their account id ("<account>/…"); "all" is the unified view.
	sent := func(account string, err error) error {
		account, _, _ = strings.Cut(account, "/")
		switch {
		case err != nil:
		case account == "all":
			sy.WakeAll()
		default:
			sy.Wake(account)
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
	mux.HandleFunc("PATCH /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Name string }
		if decode(w, r, &body) {
			err := s.RenameAccount(r.PathValue("id"), body.Name)
			if err == nil {
				ev.Publish() // other windows' lists and the rail show the new name
			}
			respond(w, nil, err)
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
	mux.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
		var d Draft
		if decode(w, r, &d) {
			respond(w, nil, sy.Send(d))
		}
	})
	mux.HandleFunc("PUT /drafts", func(w http.ResponseWriter, r *http.Request) {
		var d Draft
		if decode(w, r, &d) {
			id, err := sy.SaveDraft(d)
			if err != nil {
				respond(w, nil, err)
				return
			}
			respond(w, map[string]string{"id": id}, nil)
		}
	})
	mux.HandleFunc("DELETE /drafts/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, nil, sy.DeleteDraft(r.PathValue("id")))
	})
	// An upload for a message being written: stored like received attachments, named by its hash in Draft.Attachments.
	mux.HandleFunc("POST /files", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUpload))
		if err != nil {
			respond(w, nil, problem("Attachments can add up to 25 MB."))
			return
		}
		hash, err := s.saveFile(data)
		respond(w, map[string]string{"hash": hash}, err)
	})
	// An attachment's content, for the preview. "attachment" keeps anything from rendering it by accident.
	mux.HandleFunc("GET /messages/{id}/attachments/{index}", func(w http.ResponseWriter, r *http.Request) {
		index, _ := strconv.Atoi(r.PathValue("index"))
		list, err := s.attachments(r.PathValue("id"))
		if err == nil && (index < 0 || index >= len(list)) {
			err = ErrNotFound
		}
		var data []byte
		if err == nil {
			data, err = s.File(list[index].Hash)
		}
		if errors.Is(err, os.ErrNotExist) {
			err = ErrNotFound
		}
		if err != nil {
			respond(w, nil, err)
			return
		}
		w.Header().Set("Content-Type", cmp.Or(list[index].Type, "application/octet-stream"))
		w.Header().Set("Content-Disposition", "attachment")
		w.Write(data)
	})
	mux.HandleFunc("POST /messages/{id}/attachments/{index}/open", func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil {
			respond(w, nil, ErrNotFound)
			return
		}
		respond(w, nil, openAttachment(s, r.PathValue("id"), index))
	})
	mux.HandleFunc("GET /contacts", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Contacts(time.Now())
		respond(w, v, err)
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
			respond(w, nil, sent(r.PathValue("id"), s.SetFlags(r.PathValue("id"), body.Unread, body.Starred)))
		}
	})
	mux.HandleFunc("POST /threads/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ To string }
		if decode(w, r, &body) {
			respond(w, nil, sent(r.PathValue("id"), s.Move(r.PathValue("id"), body.To)))
		}
	})
	mux.HandleFunc("POST /read-all", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Account, Key string }
		if decode(w, r, &body) {
			respond(w, nil, sent(body.Account, s.MarkAllRead(body.Account, body.Key)))
		}
	})
	return mux
}

const maxUpload = 25<<20 + 1<<10 // 25 MB, the most providers take in one message

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
