// Package report aggregates per-folder outcomes into an end-of-run summary.
//
// One failed folder never aborts a run; its error is recorded here instead (see
// user stories 12 and 33).
package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// erroredFolder pairs a folder name with the error that befell it.
type erroredFolder struct {
	name string
	err  string
}

// Report accumulates the outcome of each Movie Folder processed in a run. The
// zero value is ready to use, and every recording method is safe for concurrent
// use by the bounded worker pool that drives a run.
type Report struct {
	mu        sync.Mutex
	enriched  []string
	skipped   []string
	unmatched []string
	planned   []string
	errored   []erroredFolder
}

// Enriched records a folder that had its NFO written.
func (r *Report) Enriched(name string) { r.add(&r.enriched, name) }

// Skipped records a folder left untouched because its NFO already existed.
func (r *Report) Skipped(name string) { r.add(&r.skipped, name) }

// Unmatched records a folder whose name did not parse as "Title (Year)".
func (r *Report) Unmatched(name string) { r.add(&r.unmatched, name) }

// Planned records a folder whose NFO would be written (dry-run).
func (r *Report) Planned(name string) { r.add(&r.planned, name) }

// add appends name to one of the outcome slices under the lock.
func (r *Report) add(bucket *[]string, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*bucket = append(*bucket, name)
}

// Errored records a folder that failed to process, with the error message.
func (r *Report) Errored(name, err string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errored = append(r.errored, erroredFolder{name: name, err: err})
}

// HasErrors reports whether any folder errored.
func (r *Report) HasErrors() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.errored) > 0
}

// Summary renders a human-readable end-of-run summary: a counts line followed
// by the names of unmatched and errored folders so the user knows what to fix.
func (r *Report) Summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder

	fmt.Fprintf(&b, "enriched: %d  skipped: %d  unmatched: %d  planned: %d  errored: %d\n",
		len(r.enriched), len(r.skipped), len(r.unmatched), len(r.planned), len(r.errored))

	for _, name := range r.unmatched {
		fmt.Fprintf(&b, "  unmatched: %s\n", name)
	}
	for _, e := range r.errored {
		fmt.Fprintf(&b, "  errored:   %s: %s\n", e.name, e.err)
	}

	return b.String()
}

// jsonError is the machine-readable shape of an errored folder.
type jsonError struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// jsonReport is the machine-readable shape emitted by JSON: per-outcome counts
// plus the folder names behind each outcome, so other automation can drive the
// tool off a single document.
type jsonReport struct {
	Counts struct {
		Enriched  int `json:"enriched"`
		Skipped   int `json:"skipped"`
		Unmatched int `json:"unmatched"`
		Planned   int `json:"planned"`
		Errored   int `json:"errored"`
	} `json:"counts"`
	Enriched  []string    `json:"enriched"`
	Skipped   []string    `json:"skipped"`
	Unmatched []string    `json:"unmatched"`
	Planned   []string    `json:"planned"`
	Errored   []jsonError `json:"errored"`
}

// JSON renders the run outcome as an indented, machine-readable report (see user
// story 34). The shape is stable: a counts object plus the folder names behind
// each outcome.
func (r *Report) JSON() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var jr jsonReport
	jr.Counts.Enriched = len(r.enriched)
	jr.Counts.Skipped = len(r.skipped)
	jr.Counts.Unmatched = len(r.unmatched)
	jr.Counts.Planned = len(r.planned)
	jr.Counts.Errored = len(r.errored)

	jr.Enriched = nonNil(r.enriched)
	jr.Skipped = nonNil(r.skipped)
	jr.Unmatched = nonNil(r.unmatched)
	jr.Planned = nonNil(r.planned)

	jr.Errored = make([]jsonError, 0, len(r.errored))
	for _, e := range r.errored {
		jr.Errored = append(jr.Errored, jsonError{Name: e.name, Error: e.err})
	}

	return json.MarshalIndent(jr, "", "  ")
}

// nonNil returns s, or an empty (non-nil) slice when s is nil, so the JSON
// encodes an empty array [] rather than null for an outcome that never occurred.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
