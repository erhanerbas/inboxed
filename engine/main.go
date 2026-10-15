// Command engine is the Inboxed mail engine. Tauri launches it as a sidecar.
//
// Handshake: it listens on a random localhost port and prints one JSON line,
// {"port":N,"token":"..."}, to stdout. Every request must include
// "Authorization: Bearer <token>". It exits when stdin closes, which happens
// when the Tauri app exits.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	token := rand.Text()

	go func() {
		io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}()

	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"port":  ln.Addr().(*net.TCPAddr).Port,
		"token": token,
	})

	log.Fatal(http.Serve(ln, withAuth(token, routes())))
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

// withAuth adds CORS headers so the webview origin can call the engine, and
// rejects any request that doesn't carry the session token.
func withAuth(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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
