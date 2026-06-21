// Package report aggregates per-folder outcomes into an end-of-run summary.
//
// One failed folder never aborts a run; its error is recorded here instead (see
// user stories 12 and 33).
package report

import (
	"fmt"
	"strings"
)

// erroredFolder pairs a folder name with the error that befell it.
type erroredFolder struct {
	name string
	err  string
}

// Report accumulates the outcome of each Movie Folder processed in a run. The
// zero value is ready to use.
type Report struct {
	enriched  []string
	skipped   []string
	unmatched []string
	planned   []string
	errored   []erroredFolder
}

// Enriched records a folder that had its NFO written.
func (r *Report) Enriched(name string) { r.enriched = append(r.enriched, name) }

// Skipped records a folder left untouched because its NFO already existed.
func (r *Report) Skipped(name string) { r.skipped = append(r.skipped, name) }

// Unmatched records a folder whose name did not parse as "Title (Year)".
func (r *Report) Unmatched(name string) { r.unmatched = append(r.unmatched, name) }

// Planned records a folder whose NFO would be written (dry-run).
func (r *Report) Planned(name string) { r.planned = append(r.planned, name) }

// Errored records a folder that failed to process, with the error message.
func (r *Report) Errored(name, err string) {
	r.errored = append(r.errored, erroredFolder{name: name, err: err})
}

// HasErrors reports whether any folder errored.
func (r *Report) HasErrors() bool { return len(r.errored) > 0 }

// Summary renders a human-readable end-of-run summary: a counts line followed
// by the names of unmatched and errored folders so the user knows what to fix.
func (r *Report) Summary() string {
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
