package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPI(t *testing.T) {
	srv := httptest.NewServer(routes(newTestStore(t)))
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
