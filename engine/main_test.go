package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth(t *testing.T) {
	h := withAuth("secret", routes())
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
