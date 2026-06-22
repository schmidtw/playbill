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

func TestSelect_PrefersPreferredLanguage(t *testing.T) {
	candidates := []artselect.Image{
		// A more popular, higher-res image in the wrong language must still
		// lose to the preferred-language one: language is the first key.
		{Kind: artselect.Poster, URL: "de.jpg", Language: "de", Popularity: 99, Width: 4000, Height: 6000},
		{Kind: artselect.Poster, URL: "en.jpg", Language: "en", Popularity: 1, Width: 100, Height: 150},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "en.jpg", got[0].URL)
}

func TestSelect_PopularityBreaksLanguageTie(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Poster, URL: "low.jpg", Language: "en", Popularity: 5, Width: 4000, Height: 6000},
		{Kind: artselect.Poster, URL: "high.jpg", Language: "en", Popularity: 50, Width: 100, Height: 150},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "high.jpg", got[0].URL, "same language: higher popularity wins over higher resolution")
}

func TestSelect_ResolutionBreaksPopularityTie(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Poster, URL: "small.jpg", Language: "en", Popularity: 10, Width: 1000, Height: 1500},
		{Kind: artselect.Poster, URL: "big.jpg", Language: "en", Popularity: 10, Width: 2000, Height: 3000},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "big.jpg", got[0].URL, "same language and popularity: higher resolution wins")
}

func TestSelect_PrefersNeutralOverForeignLanguage(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Poster, URL: "de.jpg", Language: "de", Popularity: 99},
		{Kind: artselect.Poster, URL: "neutral.jpg", Language: "", Popularity: 1},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "neutral.jpg", got[0].URL, "a language-neutral image beats a wrong-language one")
}
