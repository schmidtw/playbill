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

// artOutcome pairs a folder with one art type that was not written, used to
// distinguish art missing because there is no Fanart.tv key from art genuinely
// unavailable for the movie (user stories 14 and 15).
type artOutcome struct {
	folder string
	kind   string
}

// Report accumulates the outcome of each Movie Folder processed in a run. The
// zero value is ready to use, and every recording method is safe for concurrent
// use by the bounded worker pool that drives a run.
type Report struct {
	mu         sync.Mutex
	enriched   []string
	skipped    []string
	unmatched  []string
	planned    []string
	errored    []erroredFolder
	artNoKey   []artOutcome
	artUnavail []artOutcome
}

// Enriched records a folder that had its NFO written.
func (r *Report) Enriched(name string) { r.add(&r.enriched, name) }

// Skipped records a folder left untouched because its NFO already existed.
func (r *Report) Skipped(name string) { r.add(&r.skipped, name) }

// Unmatched records a folder whose name did not parse as "Title (Year)".
func (r *Report) Unmatched(name string) { r.add(&r.unmatched, name) }

// Planned records a folder whose NFO would be written (dry-run).
func (r *Report) Planned(name string) { r.add(&r.planned, name) }

// ArtSkippedNoKey records an art type that could not be fetched because there
// is no Fanart.tv key (only Fanart.tv supplies it). It is the optional-provider
// path: the run still succeeds.
func (r *Report) ArtSkippedNoKey(folder, kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artNoKey = append(r.artNoKey, artOutcome{folder: folder, kind: kind})
}

// ArtUnavailable records a wanted art type that no queried provider had for the
// movie, as opposed to one skipped for lack of a Fanart.tv key.
func (r *Report) ArtUnavailable(folder, kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artUnavail = append(r.artUnavail, artOutcome{folder: folder, kind: kind})
}

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

	if len(r.artNoKey) > 0 || len(r.artUnavail) > 0 {
		fmt.Fprintf(&b, "artwork — skipped (no Fanart.tv key): %d  unavailable: %d\n",
			len(r.artNoKey), len(r.artUnavail))
	}

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

// jsonArt is the machine-readable shape of one missing art type.
type jsonArt struct {
	Folder string `json:"folder"`
	Kind   string `json:"kind"`
}

// jsonArtwork groups the art types that were not written by reason, so
// automation can tell "no Fanart.tv key" apart from "unavailable for the movie".
type jsonArtwork struct {
	SkippedNoKey []jsonArt `json:"skipped_no_key"`
	Unavailable  []jsonArt `json:"unavailable"`
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
	Artwork   jsonArtwork `json:"artwork"`
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

	jr.Artwork.SkippedNoKey = jsonArtOutcomes(r.artNoKey)
	jr.Artwork.Unavailable = jsonArtOutcomes(r.artUnavail)

	return json.MarshalIndent(jr, "", "  ")
}

// jsonArtOutcomes maps recorded art outcomes to their machine-readable shape,
// always returning a non-nil slice so the JSON encodes [] rather than null.
func jsonArtOutcomes(outcomes []artOutcome) []jsonArt {
	out := make([]jsonArt, 0, len(outcomes))
	for _, o := range outcomes {
		out = append(out, jsonArt{Folder: o.folder, Kind: o.kind})
	}
	return out
}

// nonNil returns s, or an empty (non-nil) slice when s is nil, so the JSON
// encodes an empty array [] rather than null for an outcome that never occurred.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
