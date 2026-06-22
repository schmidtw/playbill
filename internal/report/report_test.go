package report_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/schmidtw/playbill/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestReport_JSON_AggregatesOutcomes(t *testing.T) {
	var r report.Report
	r.Enriched("The Matrix (1999)")
	r.Enriched("Amélie (2001)")
	r.Skipped("Inception (2010)")
	r.Unmatched("Random Folder")
	r.Planned("Dune (2021)")
	r.Errored("Broken (2000)", "permission denied")

	data, err := r.JSON()
	require.NoError(t, err)

	var got struct {
		Counts struct {
			Enriched  int `json:"enriched"`
			Skipped   int `json:"skipped"`
			Unmatched int `json:"unmatched"`
			Planned   int `json:"planned"`
			Errored   int `json:"errored"`
		} `json:"counts"`
		Enriched  []string `json:"enriched"`
		Skipped   []string `json:"skipped"`
		Unmatched []string `json:"unmatched"`
		Planned   []string `json:"planned"`
		Errored   []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"errored"`
	}
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, 2, got.Counts.Enriched)
	assert.Equal(t, 1, got.Counts.Skipped)
	assert.Equal(t, 1, got.Counts.Unmatched)
	assert.Equal(t, 1, got.Counts.Planned)
	assert.Equal(t, 1, got.Counts.Errored)

	assert.Equal(t, []string{"The Matrix (1999)", "Amélie (2001)"}, got.Enriched)
	assert.Equal(t, []string{"Random Folder"}, got.Unmatched)
	require.Len(t, got.Errored, 1)
	assert.Equal(t, "Broken (2000)", got.Errored[0].Name)
	assert.Equal(t, "permission denied", got.Errored[0].Error)
}

func TestReport_ConcurrentRecordingIsSafe(t *testing.T) {
	var r report.Report
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			r.Enriched("movie")
			r.Errored("bad", "boom")
		})
	}
	wg.Wait()

	data, err := r.JSON()
	require.NoError(t, err)

	var got struct {
		Counts struct {
			Enriched int `json:"enriched"`
			Errored  int `json:"errored"`
		} `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, 50, got.Counts.Enriched)
	assert.Equal(t, 50, got.Counts.Errored)
}
