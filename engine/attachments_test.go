package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pdfNamed(id, name string) []byte {
	return crlf("From: Maya <maya@x>\nSubject: Menu\nMessage-ID: <" + id + ">\nMIME-Version: 1.0\n" +
		"Content-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain\n\nMenu attached.\n" +
		"--b\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=\"" + name + "\"\n\n%PDF-1.4 tiny\n--b--\n")
}

func TestSaveAttachmentStaysInFolder(t *testing.T) {
	s, inbox := trailStore(t)
	dir := t.TempDir()
	for i, name := range []string{"../../evil.pdf", `a/b\c.pdf`, "..", "\x00x.pdf", ".hidden"} {
		id := strings.Repeat("n", i+1) + "@x"
		add(t, s, inbox, uint32(i+1), pdfNamed(id, name))
		path, err := s.SaveAttachment("acc/"+id, 0, dir)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if filepath.Dir(path) != dir || filepath.Base(path) == "" || strings.HasPrefix(filepath.Base(path), ".") {
			t.Errorf("%q saved as %q", name, path)
		}
	}
	add(t, s, inbox, 9, pdfMessage("menu@x"))
	first, _ := s.SaveAttachment("acc/menu@x", 0, dir)
	second, _ := s.SaveAttachment("acc/menu@x", 0, dir)
	if filepath.Base(first) != "menu.pdf" || filepath.Base(second) != "menu (2).pdf" {
		t.Errorf("saved as %q then %q", first, second)
	}
	if got, _ := os.ReadFile(second); string(got) != "%PDF-1.4 tiny" {
		t.Errorf("content %q", got)
	}
}

func TestOpenAttachmentRoute(t *testing.T) {
	h, s, _ := testRoutes(t)
	addTestAccount(t, s)
	inbox, _ := s.UpsertMailbox("acc", "INBOX", "Inbox", ptr("inbox"), "", 1)
	add(t, s, inbox, 1, pdfMessage("menu@x"))
	t.Setenv("INBOXED_DOWNLOADS", t.TempDir())
	var revealed, quarantined string
	oldReveal, oldQuarantine := reveal, quarantine
	reveal = func(p string) error { revealed = p; return nil }
	quarantine = func(p string) error { quarantined = p; return nil }
	t.Cleanup(func() { reveal, quarantine = oldReveal, oldQuarantine })
	call := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
		return rec.Code
	}
	if code := call("/messages/acc%2Fmenu%40x/attachments/0/open"); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	if revealed == "" || revealed != quarantined || filepath.Base(revealed) != "menu.pdf" {
		t.Errorf("revealed %q, quarantined %q", revealed, quarantined)
	}
	if code := call("/messages/acc%2Fmenu%40x/attachments/1/open"); code != http.StatusNotFound {
		t.Errorf("index out of range: %d", code)
	}
}

func TestForwardCarriesAttachments(t *testing.T) {
	s, inbox := trailStore(t)
	add(t, s, inbox, 1, pdfMessage("menu@x"))
	out, err := s.Compose(Draft{AccountID: "acc", To: "sam@x", Subject: "Fwd: Menu", Body: "See below", ForwardOf: "acc/menu@x"}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseMessage(out.Raw, seedNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(p.Text) != "See below" || len(p.Attachments) != 1 || p.Attachments[0].Name != "menu.pdf" ||
		!bytes.Equal(p.Files[0], []byte("%PDF-1.4 tiny")) {
		t.Errorf("forward: text %q, attachments %+v", p.Text, p.Attachments)
	}
	if _, err := s.Compose(Draft{AccountID: "acc", To: "sam@x", ForwardOf: "a1/x"}, seedNow, false); err == nil {
		t.Error("forwarding another account's mail")
	}
}
