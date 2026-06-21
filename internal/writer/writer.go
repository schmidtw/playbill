// Package writer writes enrichment files into a Movie Folder.
//
// Writes are additive and idempotent: an existing file is never overwritten
// (skip-if-present), and dry-run reports the intended write without touching
// disk. See ADR-0001.
package writer

import (
	"errors"
	"io/fs"
	"os"
)

// Result reports what happened (or would happen) for a single write.
type Result string

const (
	// Created means the file was written.
	Created Result = "created"
	// Skipped means an existing file was left untouched.
	Skipped Result = "skipped"
	// Planned means a dry-run would have written the file.
	Planned Result = "planned"
)

// WriteNFO writes data to path unless a file already exists there. When dryRun
// is true the file is never modified: a present file reports Skipped and an
// absent one reports Planned.
func WriteNFO(path string, data []byte, dryRun bool) (Result, error) {
	exists, err := fileExists(path)
	if err != nil {
		return "", err
	}
	if exists {
		return Skipped, nil
	}
	if dryRun {
		return Planned, nil
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return Created, nil
}

// fileExists reports whether path exists, distinguishing a genuine stat error
// from absence.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
