package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestComposeAttachesFiles(t *testing.T) {
	s, inbox := trailStore(t)
	add(t, s, inbox, 1, pdfMessage("menu@x"))
	files, _ := s.attachments("acc/menu@x") // a forward sends the original's files as they're stored
	note, _ := s.saveFile([]byte("hello"))     // and an upload is stored the same way
	out, err := s.Compose(Draft{AccountID: "acc", To: "sam@x", Subject: "Fwd: Menu", Body: "See below",
		Attachments: []Attachment{files[0], {Name: "a.txt", Type: "text/plain", Hash: note}}}, seedNow, false)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseMessage(out.Raw, seedNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(p.Text) != "See below" || len(p.Attachments) != 2 || p.Attachments[0].Name != "menu.pdf" ||
		!bytes.Equal(p.Files[0], []byte("%PDF-1.4 tiny")) || p.Attachments[1].Name != "a.txt" || string(p.Files[1]) != "hello" {
		t.Errorf("text %q, attachments %+v", p.Text, p.Attachments)
	}
}

func TestComposeRefusesBadHashes(t *testing.T) {
	s, _ := trailStore(t)
	for _, hash := range []string{strings.Repeat("../", 21) + "a", strings.Repeat("A", 64), "abc"} {
		if _, err := s.File(hash); !errors.Is(err, ErrNotFound) {
			t.Errorf("File(%q): %v", hash, err)
		}
	}
	_, err := s.Compose(Draft{AccountID: "acc", To: "sam@x", Attachments: []Attachment{{Name: "gone.pdf", Hash: strings.Repeat("0", 64)}}}, seedNow, false)
	var p problem
	if !errors.As(err, &p) || !strings.Contains(err.Error(), "gone.pdf") {
		t.Errorf("a missing file: %v", err)
	}
}

func TestAttachmentBytesAPI(t *testing.T) {
	s, inbox := trailStore(t)
	add(t, s, inbox, 1, pdfMessage("menu@x"))
	h := routes(s, testSyncer(t, s), NewEvents())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/messages/acc%2Fmenu%40x/attachments/0", nil))
	if rec.Code != 200 || rec.Body.String() != "%PDF-1.4 tiny" || rec.Header().Get("Content-Type") != "application/pdf" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("GET bytes: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/messages/acc%2Fmenu%40x/attachments/1", nil))
	if rec.Code != 404 {
		t.Errorf("out of range: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/files", strings.NewReader("hello")))
	var got struct{ Hash string }
	json.NewDecoder(rec.Body).Decode(&got)
	if data, err := s.File(got.Hash); rec.Code != 200 || string(data) != "hello" {
		t.Errorf("POST /files: %d %q (%v)", rec.Code, data, err)
	}
}

func TestSafeNameStripsTricksAndFits(t *testing.T) {
	if got := safeName("invoice‮fdp.command"); strings.ContainsRune(got, '‮') {
		t.Errorf("right-to-left override kept: %q", got)
	}
	if got := safeName("a​b.pdf"); got != "ab.pdf" {
		t.Errorf("zero-width space kept: %q", got)
	}
	long := safeName(strings.Repeat("é", 200) + ".pdf")
	if len(long) > 200 || !strings.HasSuffix(long, ".pdf") || !utf8.ValidString(long) {
		t.Errorf("long name: %d bytes, %q…", len(long), long[:10])
	}
}
