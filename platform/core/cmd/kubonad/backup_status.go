package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var backupStatusNameRE = regexp.MustCompile(`\.backup-(\d{8}T\d{6}Z)(?:\.age)?$`)

type backupStatus struct {
	Dir         string `json:"dir"`
	Path        string `json:"path,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
	AgeSeconds  int64  `json:"age_seconds,omitempty"`
	MaxAge      string `json:"max_age"`
	Encrypted   bool   `json:"encrypted"`
	Fresh       bool   `json:"fresh"`
	BackupCount int    `json:"backup_count"`
	LastSuccess string `json:"last_success,omitempty"`
	LastFailure string `json:"last_failure,omitempty"`
	Error       string `json:"error,omitempty"`
}

// kubonad backup-status --dir /abs/backups [--max-age 26h] [--json]
//
// Reports the newest verified backup age. It does not re-verify every file
// (that would read the whole ledger on every health probe); `backup` already
// verified the copy when it wrote it. Exits 1 when no backup exists or the
// newest is older than --max-age.
func runBackupStatus(args []string) int {
	fs := flag.NewFlagSet("backup-status", flag.ContinueOnError)
	dir := fs.String("dir", "", "absolute backup directory")
	maxAge := fs.Duration("max-age", 26*time.Hour, "maximum acceptable age of the newest backup")
	jsonOut := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || !filepath.IsAbs(*dir) {
		fmt.Fprintln(os.Stderr, "error: --dir must be an absolute path")
		return 1
	}
	if *maxAge <= 0 {
		fmt.Fprintln(os.Stderr, "error: --max-age must be positive")
		return 1
	}
	st := inspectBackups(*dir, *maxAge)
	if *jsonOut {
		out, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(out))
	} else if st.Error != "" {
		fmt.Fprintln(os.Stderr, "backup-status:", st.Error)
	} else {
		enc := "plaintext"
		if st.Encrypted {
			enc = "age-encrypted"
		}
		fmt.Printf("backup-status: %s\nnewest: %s\nage: %s\nmax-age: %s\nbackups: %d\n", enc, st.Path, time.Duration(st.AgeSeconds)*time.Second, st.MaxAge, st.BackupCount)
	}
	if !st.Fresh {
		return 1
	}
	return 0
}

func inspectBackups(dir string, maxAge time.Duration) backupStatus {
	st := backupStatus{Dir: dir, MaxAge: maxAge.String(), Fresh: false}
	entries, err := os.ReadDir(dir)
	if err != nil {
		st.Error = fmt.Sprintf("read backup directory: %v", err)
		return st
	}
	type candidate struct {
		path string
		at   time.Time
		enc  bool
	}
	var found []candidate
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		m := backupStatusNameRE.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		at, err := time.Parse("20060102T150405Z", m[1])
		if err != nil {
			continue
		}
		found = append(found, candidate{
			path: filepath.Join(dir, entry.Name()),
			at:   at,
			enc:  strings.HasSuffix(entry.Name(), ".age"),
		})
	}
	st.BackupCount = len(found)
	st.LastSuccess = readMarker(filepath.Join(dir, ".last-success"))
	st.LastFailure = readMarker(filepath.Join(dir, ".last-failure"))
	if len(found) == 0 {
		st.Error = "no backup files found"
		return st
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	newest := found[0]
	st.Path = newest.path
	st.Timestamp = newest.at.UTC().Format(time.RFC3339)
	st.Encrypted = newest.enc
	st.AgeSeconds = int64(time.Since(newest.at).Seconds())
	if st.AgeSeconds < 0 {
		st.AgeSeconds = 0
	}
	st.Fresh = time.Since(newest.at) <= maxAge
	if st.LastFailure != "" && st.LastFailure > st.LastSuccess {
		st.Fresh = false
		st.Error = "last backup attempt failed at " + st.LastFailure
	}
	return st
}

func readMarker(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
