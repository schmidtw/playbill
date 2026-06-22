package nfo_test

import (
	"os"
	"testing"

	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshal_Minimal(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/minimal.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}

func TestTMDBID(t *testing.T) {
	tests := []struct {
		name string
		nfo  string
		want string
		ok   bool
	}{
		{
			name: "present with default attr",
			nfo:  `<movie><uniqueid type="tmdb" default="true">603</uniqueid></movie>`,
			want: "603",
			ok:   true,
		},
		{
			name: "present among others",
			nfo:  `<movie><uniqueid type="imdb">tt0133093</uniqueid><uniqueid type="tmdb">603</uniqueid></movie>`,
			want: "603",
			ok:   true,
		},
		{
			name: "absent",
			nfo:  `<movie><uniqueid type="imdb">tt0133093</uniqueid></movie>`,
			want: "",
			ok:   false,
		},
		{
			name: "empty tmdb value is not a match",
			nfo:  `<movie><uniqueid type="tmdb"></uniqueid></movie>`,
			want: "",
			ok:   false,
		},
		{
			name: "not xml",
			nfo:  "hand-tuned plain text",
			want: "",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nfo.TMDBID([]byte(tt.nfo))
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMarshal_Rich(t *testing.T) {
	m := nfo.Movie{
		Title:         "The Matrix",
		OriginalTitle: "The Matrix",
		SortTitle:     "Matrix",
		Year:          1999,
		Premiered:     "1999-03-30",
		Runtime:       136,
		Plot:          "A computer hacker learns the true nature of reality.",
		Tagline:       "Welcome to the Real World.",
		MPAA:          "R",
		Genres:        []string{"Action", "Science Fiction"},
		Countries:     []string{"United States of America"},
		Studios:       []string{"Warner Bros. Pictures", "Village Roadshow Pictures"},
		Set:           "The Matrix Collection",
		Directors:     []string{"Lana Wachowski", "Lilly Wachowski"},
		Writers:       []string{"Lana Wachowski", "Lilly Wachowski"},
		Ratings: []nfo.Rating{
			{Name: "themoviedb", Max: 10, Default: true, Value: 8.2, Votes: 24149},
		},
		Actors: []nfo.Actor{
			{Name: "Keanu Reeves", Role: "Neo", Order: 0, Thumb: "https://image.tmdb.org/t/p/original/a.jpg"},
			{Name: "Laurence Fishburne", Role: "Morpheus", Order: 1, Thumb: "https://image.tmdb.org/t/p/original/b.jpg"},
		},
		Trailer: "plugin://plugin.video.youtube/?action=play_video&videoid=vKQi3bBA1y8",
		UniqueIDs: []nfo.UniqueID{
			{Type: "tmdb", Default: true, Value: "603"},
			{Type: "imdb", Value: "tt0133093"},
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/rich.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}

func TestMarshal_ArtCatalog(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		// The full poster/fanart catalog from both providers, independent of
		// which single image is downloaded to disk.
		Posters: []string{
			"https://image.tmdb.org/t/p/original/poster1.jpg",
			"https://image.tmdb.org/t/p/original/poster2.jpg",
			"https://assets.fanart.tv/movies/603/movieposter/poster3.jpg",
		},
		Fanarts: []string{
			"https://image.tmdb.org/t/p/original/fanart1.jpg",
			"https://assets.fanart.tv/movies/603/moviebackground/fanart2.jpg",
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/catalog.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}

func TestMarshal_UniqueIDsAndLegacyID(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		UniqueIDs: []nfo.UniqueID{
			{Type: "tmdb", Default: true, Value: "603"},
			{Type: "imdb", Value: "tt0133093"},
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	body := string(got)
	assert.Contains(t, body, `<uniqueid type="tmdb" default="true">603</uniqueid>`)
	assert.Contains(t, body, `<uniqueid type="imdb">tt0133093</uniqueid>`)
	// Legacy <id> mirrors the default unique id for older skins.
	assert.Contains(t, body, "<id>603</id>")
}

func TestMarshal_WithStreamDetails(t *testing.T) {
	m := nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		StreamDetails: &nfo.StreamDetails{
			Video: &nfo.VideoStream{
				Codec:             "h264",
				Aspect:            1.78,
				Width:             1920,
				Height:            1080,
				DurationInSeconds: 8160,
				ScanType:          "progressive",
			},
			Audio: []nfo.AudioStream{
				{Codec: "aac", Language: "eng", Channels: 2},
				{Codec: "ac3", Language: "fra", Channels: 6},
			},
		},
	}

	got, err := nfo.Marshal(m)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/streamdetails.nfo")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got))
}
