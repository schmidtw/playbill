package tmdb_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/schmidtw/playbill/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_EmptyKeyIsError(t *testing.T) {
	_, err := tmdb.New("")
	assert.ErrorIs(t, err, tmdb.ErrNoAPIKey)
}

func TestNew_WithKeySucceeds(t *testing.T) {
	c, err := tmdb.New("deadbeef")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// searchBody is a one-result search response for The Matrix (1999).
const searchBody = `{
  "results": [
    {"id": 603, "title": "The Matrix", "original_title": "The Matrix", "release_date": "1999-03-30"}
  ]
}`

// detailsBody is a full movie-details response with credits, external IDs,
// release-date certification, and a YouTube trailer.
const detailsBody = `{
  "id": 603,
  "imdb_id": "tt0133093",
  "title": "The Matrix",
  "original_title": "The Matrix",
  "overview": "A computer hacker learns the true nature of reality.",
  "tagline": "Welcome to the Real World.",
  "release_date": "1999-03-30",
  "runtime": 136,
  "vote_average": 8.2,
  "vote_count": 24149,
  "genres": [{"id": 28, "name": "Action"}, {"id": 878, "name": "Science Fiction"}],
  "production_companies": [{"name": "Warner Bros. Pictures"}, {"name": "Village Roadshow Pictures"}],
  "production_countries": [{"name": "United States of America"}],
  "belongs_to_collection": {"name": "The Matrix Collection"},
  "credits": {
    "cast": [
      {"name": "Keanu Reeves", "character": "Neo", "order": 0, "profile_path": "/a.jpg"},
      {"name": "Laurence Fishburne", "character": "Morpheus", "order": 1, "profile_path": "/b.jpg"}
    ],
    "crew": [
      {"name": "Lana Wachowski", "job": "Director", "department": "Directing"},
      {"name": "Lilly Wachowski", "job": "Director", "department": "Directing"},
      {"name": "Lana Wachowski", "job": "Writer", "department": "Writing"},
      {"name": "Joel Silver", "job": "Producer", "department": "Production"}
    ]
  },
  "external_ids": {"imdb_id": "tt0133093"},
  "release_dates": {
    "results": [
      {"iso_3166_1": "US", "release_dates": [{"certification": "R", "type": 3}]},
      {"iso_3166_1": "GB", "release_dates": [{"certification": "15", "type": 3}]}
    ]
  },
  "videos": {
    "results": [
      {"site": "YouTube", "type": "Teaser", "key": "teaserkey"},
      {"site": "YouTube", "type": "Trailer", "key": "vKQi3bBA1y8"}
    ]
  }
}`

// fakeTMDB returns an httptest server that serves the search and details bodies
// and records which paths were requested.
func fakeTMDB(t *testing.T, search, details string) (*httptest.Server, *[]string) {
	t.Helper()
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(search))
		case "/movie/603":
			_, _ = w.Write([]byte(details))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestResolve_SearchAndMap(t *testing.T) {
	srv, hits := fakeTMDB(t, searchBody, detailsBody)

	c, err := tmdb.New("key", tmdb.WithBaseURL(srv.URL))
	require.NoError(t, err)

	m, err := c.Resolve(tmdb.ResolveRequest{Title: "The Matrix", Year: 1999})
	require.NoError(t, err)

	assert.Equal(t, "The Matrix", m.Title)
	assert.Equal(t, "The Matrix", m.OriginalTitle)
	assert.Equal(t, 1999, m.Year)
	assert.Equal(t, "1999-03-30", m.Premiered)
	assert.Equal(t, 136, m.Runtime)
	assert.Equal(t, "A computer hacker learns the true nature of reality.", m.Plot)
	assert.Equal(t, "Welcome to the Real World.", m.Tagline)
	assert.Equal(t, "R", m.MPAA)
	assert.Equal(t, []string{"Action", "Science Fiction"}, m.Genres)
	assert.Equal(t, []string{"United States of America"}, m.Countries)
	assert.Equal(t, []string{"Warner Bros. Pictures", "Village Roadshow Pictures"}, m.Studios)
	assert.Equal(t, "The Matrix Collection", m.Set)
	assert.Equal(t, []string{"Lana Wachowski", "Lilly Wachowski"}, m.Directors)
	assert.Equal(t, []string{"Lana Wachowski"}, m.Writers)

	require.Len(t, m.Ratings, 1)
	assert.Equal(t, "themoviedb", m.Ratings[0].Name)
	assert.Equal(t, 10, m.Ratings[0].Max)
	assert.True(t, m.Ratings[0].Default)
	assert.InDelta(t, 8.2, m.Ratings[0].Value, 0.001)
	assert.Equal(t, 24149, m.Ratings[0].Votes)

	require.Len(t, m.Actors, 2)
	assert.Equal(t, "Keanu Reeves", m.Actors[0].Name)
	assert.Equal(t, "Neo", m.Actors[0].Role)
	assert.Equal(t, 0, m.Actors[0].Order)
	assert.Equal(t, "https://image.tmdb.org/t/p/original/a.jpg", m.Actors[0].Thumb)

	assert.Equal(t, "plugin://plugin.video.youtube/?action=play_video&videoid=vKQi3bBA1y8", m.Trailer)

	require.Len(t, m.UniqueIDs, 2)
	assert.Equal(t, nfo.UniqueID{Type: "tmdb", Default: true, Value: "603"}, m.UniqueIDs[0])
	assert.Equal(t, nfo.UniqueID{Type: "imdb", Value: "tt0133093"}, m.UniqueIDs[1])

	assert.Equal(t, []string{"/search/movie", "/movie/603"}, *hits)
}
