package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// saveFile keeps attachment content under its SHA-256, so a message's copies (and the same file sent twice) share it.
func (s *Store) saveFile(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	path := filepath.Join(s.files, hash)
	if _, err := os.Stat(path); err == nil {
		now := time.Now() // fresh again, so a startup clean-up that began before this doesn't take it
		return hash, os.Chtimes(path, now, now)
	}
	if err := os.MkdirAll(s.files, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.files, "tmp-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // gone once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return hash, os.Rename(tmp.Name(), path)
}

// File is an attachment's content.
func (s *Store) File(hash string) ([]byte, error) {
	if !isHash(hash) { // a SHA-256 in hex, never a path (drafts name files too)
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(s.files, hash))
}

func isHash(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}

// cleanGrace: files this recent may belong to a message still being stored (tests shorten it).
var cleanGrace = time.Minute

// CleanFiles removes attachment files no message refers to any more. main runs it in the background at startup;
// files written since it began are left alone (their message may not be committed yet).
func (s *Store) CleanFiles() error {
	started := time.Now()
	entries, err := os.ReadDir(s.files)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	used := map[string]bool{}
	rows, err := s.db.Query(`SELECT attachments FROM messages WHERE attachments != '[]'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var raw string
		var list []Attachment
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		json.Unmarshal([]byte(raw), &list)
		for _, a := range list {
			used[a.Hash] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, e := range entries {
		if info, err := e.Info(); err != nil || info.ModTime().After(started.Add(-cleanGrace)) {
			continue
		}
		if !used[e.Name()] {
			os.Remove(filepath.Join(s.files, e.Name()))
		}
	}
	return nil
}
