package report_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/report"
	"github.com/stretchr/testify/assert"
)

func TestReport_Summary_Counts(t *testing.T) {
	var r report.Report
	r.Enriched("The Matrix (1999)")
	r.Enriched("Amélie (2001)")
	r.Skipped("Inception (2010)")
	r.Unmatched("Random Folder")

	s := r.Summary()

	assert.Contains(t, s, "enriched: 2")
	assert.Contains(t, s, "skipped: 1")
	assert.Contains(t, s, "unmatched: 1")
}

func TestReport_Summary_ListsUnmatched(t *testing.T) {
	var r report.Report
	r.Unmatched("Random Folder")
	r.Unmatched("Another Bad One")

	s := r.Summary()

	// Unmatched folders are listed by name so the user knows what to fix.
	assert.Contains(t, s, "Random Folder")
	assert.Contains(t, s, "Another Bad One")
}

func TestReport_Summary_IncludesPlannedAndErrored(t *testing.T) {
	var r report.Report
	r.Planned("The Matrix (1999)")
	r.Errored("Broken (2000)", "permission denied")

	s := r.Summary()

	assert.Contains(t, s, "planned: 1")
	assert.Contains(t, s, "errored: 1")
	assert.Contains(t, s, "Broken (2000)")
	assert.Contains(t, s, "permission denied")
}

func TestReport_HasErrors(t *testing.T) {
	var r report.Report
	assert.False(t, r.HasErrors())

	r.Errored("Broken (2000)", "boom")
	assert.True(t, r.HasErrors())
}
