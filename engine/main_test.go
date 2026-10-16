package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth(t *testing.T) {
	h := withAuth("secret", nil, func() http.Handler { h, _, _ := testRoutes(t); return h }())
	for _, tc := range []struct {
		method, auth string
		want         int
	}{
		{"GET", "", http.StatusUnauthorized},
		{"GET", "Bearer wrong", http.StatusUnauthorized},
		{"GET", "Bearer secret", http.StatusOK},
		{"OPTIONS", "", http.StatusNoContent},
	} {
		req := httptest.NewRequest(tc.method, "/health", nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s auth=%q: got %d, want %d", tc.method, tc.auth, rec.Code, tc.want)
		}
	}
}

// Only the app's own webview origins may call the engine from a browser context; a page on the open web can't,
// even if it knows the token.
func TestCORSAllowsOnlyTheApp(t *testing.T) {
	h := withAuth("secret", []string{"http://localhost:1420"}, func() http.Handler { h, _, _ := testRoutes(t); return h }())
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"tauri://localhost", true},
		{"http://tauri.localhost", true},
		{"http://localhost:1420", true},
		{"https://evil.example", false},
		{"null", false},
	} {
		for _, method := range []string{"OPTIONS", "GET"} {
			req := httptest.NewRequest(method, "/health", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", "Bearer secret")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tc.allowed && got != tc.origin {
				t.Errorf("%s %s: Access-Control-Allow-Origin %q, want the origin echoed", method, tc.origin, got)
			}
			if !tc.allowed && (got != "" || rec.Code != http.StatusForbidden) {
				t.Errorf("%s %s: got %d with Access-Control-Allow-Origin %q, want 403 and no CORS grant", method, tc.origin, rec.Code, got)
			}
		}
	}
}
