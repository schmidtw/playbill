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

func TestSelect_FanartProviderPreferredOverTMDB(t *testing.T) {
	candidates := []artselect.Image{
		// Same language: provider precedence outranks popularity/resolution,
		// because scores are not comparable across providers.
		{Kind: artselect.Clearlogo, URL: "tmdb.png", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 99, Width: 4000, Height: 2000},
		{Kind: artselect.Clearlogo, URL: "fanart.png", Provider: artselect.ProviderFanart, Language: "en", Popularity: 1, Width: 400, Height: 200},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 1)
	assert.Equal(t, "fanart.png", got[0].URL, "Fanart.tv is preferred over TMDB for a shared art type")
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

func TestSelect_EmptyCandidates(t *testing.T) {
	assert.Empty(t, artselect.Select(nil, "en"))
	assert.Empty(t, artselect.Select([]artselect.Image{}, "en"))
}

func TestSelect_OneBestPerKindSortedByKind(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Fanart, URL: "f-lo.jpg", Language: "en", Popularity: 1},
		{Kind: artselect.Poster, URL: "p-lo.jpg", Language: "en", Popularity: 1},
		{Kind: artselect.Fanart, URL: "f-hi.jpg", Language: "en", Popularity: 9},
		{Kind: artselect.Clearlogo, URL: "c.png", Language: "en", Popularity: 1},
		{Kind: artselect.Poster, URL: "p-hi.jpg", Language: "en", Popularity: 9},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 3, "exactly one image per art type")

	// Output is sorted by Kind so writes are deterministic across runs.
	kinds := []artselect.Kind{got[0].Kind, got[1].Kind, got[2].Kind}
	assert.Equal(t, []artselect.Kind{artselect.Poster, artselect.Fanart, artselect.Clearlogo}, kinds)

	byKind := map[artselect.Kind]string{}
	for _, img := range got {
		byKind[img.Kind] = img.URL
	}
	assert.Equal(t, "p-hi.jpg", byKind[artselect.Poster])
	assert.Equal(t, "f-hi.jpg", byKind[artselect.Fanart])
	assert.Equal(t, "c.png", byKind[artselect.Clearlogo])
}

func TestSelect_FullTieIsDeterministicAcrossInputOrder(t *testing.T) {
	a := artselect.Image{Kind: artselect.Poster, URL: "a.jpg", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 5, Width: 100, Height: 150}
	b := artselect.Image{Kind: artselect.Poster, URL: "b.jpg", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 5, Width: 100, Height: 150}

	forward := artselect.Select([]artselect.Image{a, b}, "en")
	reverse := artselect.Select([]artselect.Image{b, a}, "en")

	require.Len(t, forward, 1)
	require.Len(t, reverse, 1)
	assert.Equal(t, "a.jpg", forward[0].URL, "an otherwise-equal tie breaks on the lower URL")
	assert.Equal(t, forward[0].URL, reverse[0].URL, "selection is independent of input order")
}
