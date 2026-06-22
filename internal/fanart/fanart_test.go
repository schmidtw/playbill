package fanart_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/schmidtw/playbill/internal/fanart"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matrixArt is a trimmed Fanart.tv /movies response carrying one image of each
// extended art type plus the baseline types Fanart.tv also offers.
const matrixArt = `{
  "name": "The Matrix",
  "tmdb_id": "603",
  "imdb_id": "tt0133093",
  "movieposter":     [{"url":"https://art/poster.jpg","lang":"en","likes":"3"}],
  "moviebackground": [{"url":"https://art/fanart.jpg","lang":"","likes":"4"}],
  "moviebanner":     [{"url":"https://art/banner.jpg","lang":"en","likes":"5"}],
  "hdmovielogo":     [{"url":"https://art/logo.png","lang":"en","likes":"6"}],
  "moviedisc":       [{"url":"https://art/disc.png","lang":"en","likes":"7","disc":"1","disc_type":"bluray"}],
  "moviethumb":      [{"url":"https://art/thumb.jpg","lang":"en","likes":"8"}],
  "hdmovieclearart": [{"url":"https://art/clearart.png","lang":"en","likes":"9"}]
}`

// artServer serves body for any /movies/{id} request and records the path and
// query it was asked for, so a test can assert the id and api_key wiring.
func artServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func newClient(t *testing.T, srv *httptest.Server) *fanart.Client {
	t.Helper()
	c, err := fanart.New("key", fanart.WithBaseURL(srv.URL), fanart.WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	return c
}

func TestImages_MapsExtendedArtTypes(t *testing.T) {
	srv, reqs := artServer(t, http.StatusOK, matrixArt)
	imgs, err := newClient(t, srv).Images("603")
	require.NoError(t, err)

	// Every candidate is tagged as the Fanart.tv provider.
	byKind := map[artselect.Kind]artselect.Image{}
	for _, img := range imgs {
		assert.Equal(t, artselect.ProviderFanart, img.Provider, "fanart candidates carry the fanart provider")
		byKind[img.Kind] = img
	}

	assert.Equal(t, "https://art/banner.jpg", byKind[artselect.Banner].URL)
	assert.Equal(t, "https://art/logo.png", byKind[artselect.Clearlogo].URL)
	assert.Equal(t, "https://art/disc.png", byKind[artselect.Discart].URL)
	assert.Equal(t, "https://art/thumb.jpg", byKind[artselect.Landscape].URL)
	assert.Equal(t, "https://art/clearart.png", byKind[artselect.Clearart].URL)
	assert.Equal(t, "https://art/poster.jpg", byKind[artselect.Poster].URL)
	assert.Equal(t, "https://art/fanart.jpg", byKind[artselect.Fanart].URL)

	// "likes" becomes the popularity score and "lang" the language.
	assert.Equal(t, float64(5), byKind[artselect.Banner].Popularity)
	assert.Equal(t, "en", byKind[artselect.Banner].Language)
	assert.Equal(t, "", byKind[artselect.Fanart].Language, "a neutral image has no language")

	require.Len(t, *reqs, 1)
	assert.Contains(t, (*reqs)[0], "/movies/603")
	assert.Contains(t, (*reqs)[0], "api_key=key")
}

func TestImages_UnknownMovieDegradesToNoArt(t *testing.T) {
	srv, _ := artServer(t, http.StatusNotFound, `{"status":"error","error message":"Not found"}`)
	imgs, err := newClient(t, srv).Images("000")
	require.NoError(t, err, "a movie Fanart.tv has never seen is not an error")
	assert.Empty(t, imgs)
}

func TestImages_ServerErrorIsReported(t *testing.T) {
	srv, _ := artServer(t, http.StatusInternalServerError, "boom")
	_, err := newClient(t, srv).Images("603")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestImages_SkipsEntriesWithoutURL(t *testing.T) {
	srv, _ := artServer(t, http.StatusOK, `{"moviebanner":[{"url":"","lang":"en","likes":"5"}]}`)
	imgs, err := newClient(t, srv).Images("603")
	require.NoError(t, err)
	assert.Empty(t, imgs, "an entry with no download URL is not a candidate")
}

func TestNew_RequiresAPIKey(t *testing.T) {
	_, err := fanart.New("")
	assert.ErrorIs(t, err, fanart.ErrNoAPIKey)
}
