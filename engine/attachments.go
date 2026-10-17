package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// Opening an attachment saves it to Downloads, marks it as downloaded from the internet and shows it in the file
// manager. Nothing is run: a double-click there goes through the OS's own checks (Gatekeeper reads the quarantine).
var (
	reveal = func(path string) error {
		switch runtime.GOOS {
		case "darwin":
			return exec.Command("open", "-R", path).Run()
		case "windows":
			return exec.Command("explorer", "/select,", path).Run()
		default:
			return exec.Command("xdg-open", filepath.Dir(path)).Run()
		}
	}
	quarantine = func(path string) error {
		if runtime.GOOS != "darwin" {
			return nil
		}
		return exec.Command("xattr", "-w", "com.apple.quarantine", fmt.Sprintf("0081;%x;Inboxed;", time.Now().Unix()), path).Run()
	}
)

// downloads is where opened attachments go: INBOXED_DOWNLOADS (headless checks), else ~/Downloads, else the temp dir.
func downloads() string {
	if dir := os.Getenv("INBOXED_DOWNLOADS"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		if dir := filepath.Join(home, "Downloads"); isDir(dir) {
			return dir
		}
	}
	return os.TempDir()
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// attachments is a message's attachment list, or ErrNotFound.
func (s *Store) attachments(messageID string) ([]Attachment, error) {
	var raw string
	err := s.db.QueryRow(`SELECT attachments FROM messages WHERE id = ?`, messageID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	var list []Attachment
	return list, json.Unmarshal([]byte(raw), &list)
}

// safeName is a sender-chosen file name made harmless: no directories, no control or invisible formatting characters
// (a right-to-left override makes "invoice\u202efdp.command" look like a PDF), not hidden, and short enough for any
// file system (cut on a character boundary, keeping the extension).
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	name = cmp.Or(strings.TrimLeft(strings.TrimSpace(name), ". "), "attachment")
	const limit = 200 // bytes; leaves room for " (99)"
	if len(name) > limit {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		base := []rune(strings.TrimSuffix(name, filepath.Ext(name)))
		for len(string(base))+len(ext) > limit {
			base = base[:len(base)-1]
		}
		name = string(base) + ext
	}
	return name
}

// SaveAttachment writes one attachment into dir under its own (cleaned) name, adding " (2)", " (3)" … rather than
// overwriting anything, and returns the path.
func (s *Store) SaveAttachment(messageID string, index int, dir string) (string, error) {
	list, err := s.attachments(messageID)
	if err != nil {
		return "", err
	}
	if index < 0 || index >= len(list) || list[index].Hash == "" {
		return "", ErrNotFound
	}
	data, err := s.File(list[index].Hash)
	if err != nil {
		return "", err
	}
	name := safeName(list[index].Name)
	ext := filepath.Ext(name)
	for n := 1; ; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		return path, f.Close()
	}
}

// openAttachment saves an attachment to Downloads, quarantines it and reveals it.
func openAttachment(s *Store, messageID string, index int) error {
	path, err := s.SaveAttachment(messageID, index, downloads())
	if err != nil {
		return err
	}
	if err := quarantine(path); err != nil {
		log.Printf("quarantine %s: %v", path, err)
	}
	return reveal(path)
}
