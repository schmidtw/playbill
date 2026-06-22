package artselect_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelect_SingleCandidateIsChosen(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Poster, URL: "p1.jpg"},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "p1.jpg", got[0].URL)
	assert.Equal(t, artselect.Poster, got[0].Kind)
}
