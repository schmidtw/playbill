// Package writer writes enrichment files into a Movie Folder.
//
// Writes are additive and idempotent: an existing file is skipped (skip-if-
// present) unless force is set, in which case it is overwritten. Dry-run reports
// the intended write without touching disk. See ADR-0001 and user story 8.
package writer

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
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

// WriteNFO writes data to path. An existing file is skipped unless force is set;
// when dryRun is true nothing is written and the intended outcome is reported
// (Skipped for a present file that force would not touch, Planned otherwise).
func WriteNFO(path string, data []byte, force, dryRun bool) (Result, error) {
	res, write, err := plan(path, force, dryRun)
	if err != nil || !write {
		return res, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return Created, nil
}

// WriteArt downloads url and writes it to path with the same skip/force/dry-run
// rules as WriteNFO. The image is fetched only when it will actually be written,
// so a skip or dry-run performs no network request.
func WriteArt(client *http.Client, path, url string, force, dryRun bool) (Result, error) {
	res, write, err := plan(path, force, dryRun)
	if err != nil || !write {
		return res, err
	}

	data, err := download(client, url)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return Created, nil
}

// plan applies the skip-if-present / force / dry-run policy. It returns the
// Result to report and whether the caller should proceed to write: write is
// true only when a real write should happen now.
func plan(path string, force, dryRun bool) (res Result, write bool, err error) {
	exists, err := fileExists(path)
	if err != nil {
		return "", false, err
	}
	if exists && !force {
		return Skipped, false, nil
	}
	if dryRun {
		return Planned, false, nil
	}
	return "", true, nil
}

// download fetches url and returns its body, turning a non-2xx status into an
// error so a missing image fails the write rather than saving an error page.
func download(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("writer: download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
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
