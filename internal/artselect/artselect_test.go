package artselect_test

import (
	"testing"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelect_Policy is the table-driven exercise of the selection policy: each
// case offers several candidates of one art type and names the URL that must
// win. The policy keys, in order, are preferred language, provider precedence,
// popularity, resolution, and finally the lower URL.
func TestSelect_Policy(t *testing.T) {
	tests := []struct {
		name      string
		preferred string
		want      string // winning URL, or "" to expect no selection
		cands     []artselect.Image
	}{
		{
			name: "lone candidate is chosen",
			want: "p1.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "p1.jpg"},
			},
		},
		{
			name:      "preferred language beats a more popular, higher-res foreign image",
			preferred: "en",
			want:      "en.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "de.jpg", Language: "de", Popularity: 99, Width: 4000, Height: 6000},
				{Kind: artselect.Poster, URL: "en.jpg", Language: "en", Popularity: 1, Width: 100, Height: 150},
			},
		},
		{
			name:      "language-neutral image beats a foreign-language one",
			preferred: "en",
			want:      "neutral.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "de.jpg", Language: "de", Popularity: 99},
				{Kind: artselect.Poster, URL: "neutral.jpg", Language: "", Popularity: 1},
			},
		},
		{
			name:      "popularity breaks a language tie over resolution",
			preferred: "en",
			want:      "high.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "low.jpg", Language: "en", Popularity: 5, Width: 4000, Height: 6000},
				{Kind: artselect.Poster, URL: "high.jpg", Language: "en", Popularity: 50, Width: 100, Height: 150},
			},
		},
		{
			name:      "resolution breaks a popularity tie",
			preferred: "en",
			want:      "big.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "small.jpg", Language: "en", Popularity: 10, Width: 1000, Height: 1500},
				{Kind: artselect.Poster, URL: "big.jpg", Language: "en", Popularity: 10, Width: 2000, Height: 3000},
			},
		},
		{
			name:      "Fanart.tv outranks TMDB for a shared art type",
			preferred: "en",
			want:      "fanart.png",
			cands: []artselect.Image{
				{Kind: artselect.Clearlogo, URL: "tmdb.png", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 99, Width: 4000, Height: 2000},
				{Kind: artselect.Clearlogo, URL: "fanart.png", Provider: artselect.ProviderFanart, Language: "en", Popularity: 1, Width: 400, Height: 200},
			},
		},
		{
			name:      "TMDB is chosen when Fanart.tv offers nothing for the type",
			preferred: "en",
			want:      "tmdb-only.png",
			cands: []artselect.Image{
				{Kind: artselect.Clearlogo, URL: "tmdb-only.png", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 3},
			},
		},
		{
			name:      "Fanart.tv supplies a clearart candidate",
			preferred: "en",
			want:      "clearart.png",
			cands: []artselect.Image{
				{Kind: artselect.Clearart, URL: "clearart.png", Provider: artselect.ProviderFanart, Language: "en", Popularity: 7},
			},
		},
		{
			name:      "an otherwise-equal tie breaks on the lower URL",
			preferred: "en",
			want:      "a.jpg",
			cands: []artselect.Image{
				{Kind: artselect.Poster, URL: "b.jpg", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 5, Width: 100, Height: 150},
				{Kind: artselect.Poster, URL: "a.jpg", Provider: artselect.ProviderTMDB, Language: "en", Popularity: 5, Width: 100, Height: 150},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artselect.Select(tt.cands, tt.preferred)
			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0].URL)
		})
	}
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

func TestSelect_ClearartOrdersAfterLandscape(t *testing.T) {
	candidates := []artselect.Image{
		{Kind: artselect.Clearart, URL: "clearart.png", Language: "en", Popularity: 1},
		{Kind: artselect.Landscape, URL: "landscape.jpg", Language: "en", Popularity: 1},
	}

	got := artselect.Select(candidates, "en")
	require.Len(t, got, 2)
	assert.Equal(t, []artselect.Kind{artselect.Landscape, artselect.Clearart},
		[]artselect.Kind{got[0].Kind, got[1].Kind}, "clearart sorts after the default art types")
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
