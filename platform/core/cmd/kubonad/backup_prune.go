package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// kubonad backup-prune --dir /abs/backups [--keep-last 7] [--keep-daily 7]
//
//	[--keep-weekly 4] [--now RFC3339] [--apply]
//
// Dry-run by default. It only considers regular files whose names end in
// ".backup-YYYYMMDDTHHMMSSZ" (the exact suffix `kubonad backup` writes).
// A confirmed backup is always kept if it is in the newest-N, newest-per-day,
// or newest-per-ISO-week set. Directories, symlinks, and unparseable names
// are never touched.
func runBackupPrune(args []string) int {
	fs := flag.NewFlagSet("backup-prune", flag.ContinueOnError)
	dir := fs.String("dir", "", "absolute backup directory")
	keepLast := fs.Int("keep-last", 7, "always keep the newest N backups")
	keepDaily := fs.Int("keep-daily", 7, "keep the newest backup per UTC day for N days")
	keepWeekly := fs.Int("keep-weekly", 4, "keep the newest backup per ISO week for N weeks")
	nowRaw := fs.String("now", "", "RFC3339 reference time (default: now; for drills/replays)")
	apply := fs.Bool("apply", false, "delete the pruned files (default: dry run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || !filepath.IsAbs(*dir) {
		fmt.Fprintln(os.Stderr, "error: --dir must be an absolute path")
		return 1
	}
	if *keepLast < 0 || *keepDaily < 0 || *keepWeekly < 0 {
		fmt.Fprintln(os.Stderr, "error: retention counts must be non-negative")
		return 2
	}
	if *keepLast+*keepDaily+*keepWeekly == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one retention window must be non-zero (refusing delete-all)")
		return 2
	}
	now := time.Now().UTC()
	if *nowRaw != "" {
		parsed, err := time.Parse(time.RFC3339, *nowRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: --now must be RFC3339:", err)
			return 2
		}
		now = parsed.UTC()
	}
	info, err := os.Stat(*dir)
	if err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "error: backup directory not readable:", err)
		return 1
	}

	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: read backup directory:", err)
		return 1
	}
	var backups []backupFile
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		ts, ok := parseBackupTimestamp(entry.Name())
		if !ok {
			continue
		}
		backups = append(backups, backupFile{name: entry.Name(), at: ts})
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].at.Equal(backups[j].at) {
			return backups[i].name > backups[j].name
		}
		return backups[i].at.After(backups[j].at)
	})

	keep := map[string]string{}
	days := map[string]bool{}
	weeks := map[string]bool{}
	dailyCutoff := now.AddDate(0, 0, -*keepDaily)
	weeklyCutoff := now.AddDate(0, 0, -7**keepWeekly)
	for i, b := range backups {
		if i < *keepLast {
			keep[b.name] = "newest"
			continue
		}
		day := b.at.Format("2006-01-02")
		if b.at.After(dailyCutoff) && !days[day] {
			days[day] = true
			keep[b.name] = "daily"
			continue
		}
		year, week := b.at.ISOWeek()
		weekKey := fmt.Sprintf("%04d-W%02d", year, week)
		if b.at.After(weeklyCutoff) && !weeks[weekKey] {
			weeks[weekKey] = true
			keep[b.name] = "weekly"
		}
	}

	deleted := 0
	for _, b := range backups {
		if _, ok := keep[b.name]; ok {
			continue
		}
		if *apply {
			if err := os.Remove(filepath.Join(*dir, b.name)); err != nil {
				fmt.Fprintf(os.Stderr, "error: delete %s: %v\n", b.name, err)
				return 1
			}
			deleted++
			fmt.Printf("deleted %s\n", b.name)
		} else {
			fmt.Printf("would delete %s\n", b.name)
		}
	}
	mode := "dry run"
	if *apply {
		mode = "applied"
	}
	fmt.Printf("backup-prune: %s; total=%d keep=%d prune=%d\n", mode, len(backups), len(keep), len(backups)-len(keep))
	return 0
}

type backupFile struct {
	name string
	at   time.Time
}

var backupNameRE = regexp.MustCompile(`\.backup-(\d{8}T\d{6}Z)$`)

func parseBackupTimestamp(name string) (time.Time, bool) {
	m := backupNameRE.FindStringSubmatch(name)
	if len(m) != 2 {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102T150405Z", m[1])
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
