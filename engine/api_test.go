package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAPI(t *testing.T) {
	srv := httptest.NewServer(func() http.Handler { h, _, _ := testRoutes(t); return h }())
	defer srv.Close()
	call := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	decode := func(res *http.Response, v any) {
		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			t.Fatal(err)
		}
	}

	var list []Thread
	decode(call("GET", "/threads?account=all&key=inbox", ""), &list)
	if len(list) != 6 || list[0].ID != "t1" || list[0].Messages != nil {
		t.Fatalf("inbox list: %d rows, first %+v", len(list), list[0])
	}
	if res := call("PATCH", "/accounts/a1", `{"name":"Home"}`); res.StatusCode != 204 {
		t.Errorf("rename: %d", res.StatusCode)
	}
	var accounts []Account
	decode(call("GET", "/accounts", ""), &accounts)
	if accounts[0].ID != "a1" || accounts[0].Name != "Home" {
		t.Errorf("after rename: %+v", accounts[0])
	}
	if res := call("PATCH", "/accounts/nope", `{"name":"X"}`); res.StatusCode != 404 {
		t.Errorf("rename a missing account: %d", res.StatusCode)
	}
	var thread Thread
	decode(call("GET", "/threads/t1", ""), &thread)
	if len(thread.Messages) != 4 {
		t.Errorf("thread has %d messages", len(thread.Messages))
	}

	statuses := []struct {
		method, path, body string
		want               int
	}{
		{"PATCH", "/threads/t1", `{"unread":false}`, 204},
		{"POST", "/threads/t2/move", `{"to":"archive"}`, 204},
		{"POST", "/read-all", `{"account":"all","key":"inbox"}`, 204},
		{"POST", "/threads/t1/move", `{"to":"nowhere"}`, 400},
		{"PATCH", "/threads/t1", `not json`, 400},
		{"GET", "/threads/nope", "", 404},
		{"GET", "/threads?account=a1&key=missing", "", 404},
	}
	for _, c := range statuses {
		if res := call(c.method, c.path, c.body); res.StatusCode != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.path, res.StatusCode, c.want)
		}
	}
	var boxes []Mailbox
	decode(call("GET", "/mailboxes", ""), &boxes)
	for _, m := range boxes {
		if m.Unread != 0 && m.Role != nil && *m.Role == "inbox" {
			t.Errorf("%s still has %d unread after read-all", m.ID, m.Unread)
		}
	}
}

// testRoutes is the API over a seeded store, as main wires it.
func testRoutes(t *testing.T) (http.Handler, *Store, *Syncer) {
	s := newTestStore(t)
	im := testSyncer(t, s)
	return routes(s, im, NewEvents()), s, im
}

func TestAccountsAPI(t *testing.T) {
	imapSrv, smtpSrv, _ := testServers(t)
	h, _, im := testRoutes(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	call := func(method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}

	res := call("POST", "/accounts", `{"email":"you@example.com","password":"nope"}`)
	var refused map[string]string
	json.NewDecoder(res.Body).Decode(&refused)
	if res.StatusCode != 400 || !strings.HasPrefix(refused["error"], "Fill in") {
		t.Errorf("incomplete form: %d %v", res.StatusCode, refused)
	}

	set, _ := json.Marshal(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	var a Account
	json.NewDecoder(call("POST", "/accounts", string(set)).Body).Decode(&a)
	if a.Provider != "imap" || a.ID == "" {
		t.Fatalf("added %+v", a)
	}
	waitImport(t, im, a.ID)
	var accounts []Account
	json.NewDecoder(call("GET", "/accounts", "").Body).Decode(&accounts)
	if last := accounts[len(accounts)-1]; last.Sync == nil || last.Sync.State != "done" || last.Sync.Done != 4 {
		t.Errorf("GET /accounts sync: %+v", last.Sync)
	}
	if res := call("POST", "/accounts/a1/sync", ""); res.StatusCode != 400 {
		t.Errorf("importing a demo account: %d", res.StatusCode)
	}
	if res := call("DELETE", "/accounts/"+a.ID, ""); res.StatusCode != 204 {
		t.Errorf("DELETE: %d", res.StatusCode)
	}
	if res := call("DELETE", "/accounts/"+a.ID, ""); res.StatusCode != 404 {
		t.Errorf("DELETE twice: %d", res.StatusCode)
	}
}

func TestEscapedThreadID(t *testing.T) {
	imapSrv, smtpSrv, _ := testServers(t)
	h, _, im := testRoutes(t)
	a, err := im.AddAccount(AccountSettings{Email: "you@example.com", Password: "secret", IMAP: imapSrv, SMTP: smtpSrv})
	if err != nil {
		t.Fatal(err)
	}
	waitImport(t, im, a.ID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/threads/"+url.PathEscape(a.ID+"/d1@example.com"), nil))
	var th Thread
	json.NewDecoder(rec.Body).Decode(&th)
	if rec.Code != 200 || len(th.Messages) != 3 {
		t.Errorf("GET an escaped thread id: %d, %d messages", rec.Code, len(th.Messages))
	}
}
