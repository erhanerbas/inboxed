// Command engine is the Inboxed mail engine. Tauri launches it as a sidecar.
//
// Handshake: it listens on a random localhost port and prints one JSON line,
// {"port":N,"token":"..."}, to stdout. Every request must include
// "Authorization: Bearer <token>". It exits when stdin closes, which happens
// when the Tauri app exits.
//
// Data lives in INBOXED_DATA_DIR/inboxed.db; INBOXED_SEED=1 fills an empty database with demo mail.
package main

import (
	"cmp"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	dir := os.Getenv("INBOXED_DATA_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(base, "dev.inboxed.app")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(dir, "inboxed.db"))
	if err != nil {
		log.Fatal(err)
	}
	if os.Getenv("INBOXED_SEED") == "1" {
		if err := store.Seed(time.Now()); err != nil {
			log.Fatal(err)
		}
	}

	// INBOXED_PORT and INBOXED_TOKEN pin the address for browser development (npm run dev:engine). Rust clears both for
	// the app, so a stray shell variable can never fix the token of a real install.
	ln, err := net.Listen("tcp", "127.0.0.1:"+cmp.Or(os.Getenv("INBOXED_PORT"), "0"))
	if err != nil {
		log.Fatal(err)
	}
	token := cmp.Or(os.Getenv("INBOXED_TOKEN"), rand.Text())
	origins := strings.FieldsFunc(os.Getenv("INBOXED_ORIGINS"), func(r rune) bool { return r == ',' })

	// Tauri talks to the engine over a pipe and closes it on exit; a terminal or /dev/null (npm run dev:engine) never does.
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeNamedPipe != 0 {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		}()
	}

	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"port":  ln.Addr().(*net.TCPAddr).Port,
		"token": token,
	})

	log.Fatal(http.Serve(ln, withAuth(token, origins, routes(store))))
}

// withAuth lets only the app's own webview origins call the engine from a page (a site on the open web can't, even
// with the token), and rejects any request without the session token. extraOrigins comes from INBOXED_ORIGINS,
// which Rust sets to the Vite dev server in debug builds only.
func withAuth(token string, extraOrigins []string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	allowed := map[string]bool{"tauri://localhost": true, "http://tauri.localhost": true}
	for _, o := range extraOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			if !allowed[origin] {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
