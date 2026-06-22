package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/schmidtw/playbill/internal/tmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResolver is an injectable resolver. It records the last request it saw and
// returns a canned movie (or error) so run() can be tested without a network.
type fakeResolver struct {
	movie     nfo.Movie
	err       error
	lastReq   tmdb.ResolveRequest
	images    []artselect.Image
	imagesErr error
	lastID    string
}

func (f *fakeResolver) Resolve(req tmdb.ResolveRequest) (nfo.Movie, error) {
	f.lastReq = req
	return f.movie, f.err
}

func (f *fakeResolver) Images(id string) ([]artselect.Image, error) {
	f.lastID = id
	return f.images, f.imagesErr
}

// matrixResolver returns a resolver yielding a minimal rich movie for tests that
// don't care about the metadata body.
func matrixResolver() *fakeResolver {
	return &fakeResolver{movie: nfo.Movie{
		Title: "The Matrix",
		Year:  1999,
		UniqueIDs: []nfo.UniqueID{
			{Type: "tmdb", Default: true, Value: "603"},
		},
	}}
}

func mkVideo(t *testing.T, root, folder string) {
	t.Helper()
	dir := filepath.Join(root, folder)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, folder+".mkv"), []byte("v"), 0o644))
}

func TestRun_WritesRichNFOForMatchedFolder(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	var out bytes.Buffer
	err := run(config{dir: root, out: &out, resolver: matrixResolver()})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	got, err := os.ReadFile(nfoPath)
	require.NoError(t, err)

	body := string(got)
	assert.Contains(t, body, "<title>The Matrix</title>")
	assert.Contains(t, body, `<uniqueid type="tmdb" default="true">603</uniqueid>`)
	assert.Contains(t, body, "<id>603</id>")
	assert.Contains(t, out.String(), "enriched: 1")
}

func TestRun_PassesParsedTitleAndYearToResolver(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	fr := matrixResolver()
	require.NoError(t, run(config{dir: root, out: &bytes.Buffer{}, resolver: fr}))

	assert.Equal(t, "The Matrix", fr.lastReq.Title)
	assert.Equal(t, 1999, fr.lastReq.Year)
	assert.Empty(t, fr.lastReq.KnownID, "no existing NFO means no known id")
}

func TestRun_ExistingTMDBIDShortCircuitsSearch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).mkv"), []byte("v"), 0o644))
	// A prior NFO already carries a (possibly hand-corrected) tmdb id.
	prior := `<movie><title>The Matrix</title><uniqueid type="tmdb">99999</uniqueid></movie>`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).nfo"), []byte(prior), 0o644))

	fr := matrixResolver()
	require.NoError(t, run(config{dir: root, out: &bytes.Buffer{}, resolver: fr}))

	assert.Equal(t, "99999", fr.lastReq.KnownID, "existing tmdb id is passed to short-circuit the search")
}

func TestRun_IntegratesStreamDetailsFromVideo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "probe", "testdata", "sample.m4v"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).m4v"), fixture, 0o644))

	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: matrixResolver()}))

	got, err := os.ReadFile(filepath.Join(dir, "The Matrix (1999).nfo"))
	require.NoError(t, err)

	body := string(got)
	assert.Contains(t, body, "<fileinfo>")
	assert.Contains(t, body, "<codec>h264</codec>")
	assert.Contains(t, body, "<width>320</width>")
	assert.Contains(t, body, "<language>eng</language>")
}

func TestRun_UnprobeableVideoOmitsStreamDetails(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)") // writes a dummy .mkv we cannot probe

	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: matrixResolver()}))

	got, err := os.ReadFile(filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "<fileinfo>")
	assert.Contains(t, out.String(), "enriched: 1")
}

func TestRun_LeavesExistingNFOUntouched(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")
	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	require.NoError(t, os.WriteFile(nfoPath, []byte("hand-tuned"), 0o644))

	var out bytes.Buffer
	err := run(config{dir: root, out: &out, resolver: matrixResolver()})
	require.NoError(t, err)

	got, err := os.ReadFile(nfoPath)
	require.NoError(t, err)
	assert.Equal(t, "hand-tuned", string(got))
	assert.Contains(t, out.String(), "skipped: 1")
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	var out bytes.Buffer
	err := run(config{dir: root, dryRun: true, out: &out, resolver: matrixResolver()})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	_, statErr := os.Stat(nfoPath)
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write the NFO")
	assert.Contains(t, out.String(), "planned: 1")
}

func TestRun_UnparseableFolderReported(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "Random Folder")

	var out bytes.Buffer
	err := run(config{dir: root, out: &out, resolver: matrixResolver()})
	require.NoError(t, err)

	nfoPath := filepath.Join(root, "Random Folder", "Random Folder.nfo")
	_, statErr := os.Stat(nfoPath)
	assert.True(t, os.IsNotExist(statErr), "unparseable folder must not be enriched")
	assert.Contains(t, out.String(), "unmatched: 1")
	assert.Contains(t, out.String(), "Random Folder")
}

func TestRun_NoTMDBMatchIsSkippedAndReported(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	fr := &fakeResolver{err: tmdb.ErrNoMatch}
	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: fr}))

	nfoPath := filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo")
	_, statErr := os.Stat(nfoPath)
	assert.True(t, os.IsNotExist(statErr), "no-match folder must not be enriched")
	assert.Contains(t, out.String(), "unmatched: 1")
}

func TestRun_AmbiguousTMDBMatchIsSkippedAndReported(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	fr := &fakeResolver{err: tmdb.ErrAmbiguousMatch}
	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: fr}))

	assert.Contains(t, out.String(), "unmatched: 1")
}

func TestRun_ResolverErrorIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	fr := &fakeResolver{err: assertAnError{}}
	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: fr}))
	assert.Contains(t, out.String(), "errored: 1")
}

// assertAnError is a non-sentinel error used to exercise the errored path.
type assertAnError struct{}

func (assertAnError) Error() string { return "boom" }

func TestRun_MissingDirIsError(t *testing.T) {
	var out bytes.Buffer
	err := run(config{dir: filepath.Join(t.TempDir(), "nope"), out: &out, resolver: matrixResolver()})
	assert.Error(t, err)
}

// concurrencyResolver is a thread-safe resolver that gates Resolve on a barrier
// and tracks the peak number of folders resolved at once, so a test can assert
// the worker pool both parallelizes and stays bounded.
type concurrencyResolver struct {
	release  chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
}

func (c *concurrencyResolver) Resolve(tmdb.ResolveRequest) (nfo.Movie, error) {
	n := c.inFlight.Add(1)
	for {
		old := c.peak.Load()
		if n <= old || c.peak.CompareAndSwap(old, n) {
			break
		}
	}
	<-c.release // block until the test lets workers proceed
	c.inFlight.Add(-1)
	return nfo.Movie{Title: "M", Year: 2000}, nil
}

func (c *concurrencyResolver) Images(string) ([]artselect.Image, error) { return nil, nil }

func TestRun_ProcessesFoldersConcurrentlyUpToLimit(t *testing.T) {
	const folders, limit = 12, 3
	root := t.TempDir()
	for i := range folders {
		mkVideo(t, root, fmt.Sprintf("Movie %02d (2000)", i))
	}

	cr := &concurrencyResolver{release: make(chan struct{})}

	var out bytes.Buffer
	var wg sync.WaitGroup
	wg.Go(func() {
		require.NoError(t, run(config{dir: root, concurrency: limit, out: &out, resolver: cr}))
	})

	// Once the pool has saturated, peak must equal the limit — never exceed it.
	assert.Eventually(t, func() bool { return cr.peak.Load() == limit }, time.Second, time.Millisecond,
		"expected exactly %d folders resolving at once", limit)

	close(cr.release) // let every worker finish
	wg.Wait()

	assert.LessOrEqual(t, cr.peak.Load(), int32(limit), "worker pool must not exceed --concurrency")
	assert.Contains(t, out.String(), fmt.Sprintf("enriched: %d", folders), "every folder is still processed")
}

func TestParseArgs_DefaultsAndDir(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies"}, &errOut)
	require.NoError(t, err)
	assert.Equal(t, "/movies", cfg.dir)
	assert.False(t, cfg.dryRun)
}

func TestParseArgs_Concurrency(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies"}, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 4, cfg.concurrency, "default concurrency is 4")

	cfg, err = parseArgs([]string{"--dir", "/movies", "--concurrency", "8"}, &errOut)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.concurrency)
}

func TestParseArgs_DryRun(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies", "--dry-run"}, &errOut)
	require.NoError(t, err)
	assert.True(t, cfg.dryRun)
}

func TestParseArgs_Force(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseArgs([]string{"--dir", "/movies"}, &errOut)
	require.NoError(t, err)
	assert.False(t, cfg.force, "force defaults to off")

	cfg, err = parseArgs([]string{"--dir", "/movies", "--force"}, &errOut)
	require.NoError(t, err)
	assert.True(t, cfg.force)
}

// imageServer serves a one-pixel body for any request and records the paths it
// was asked for, so a test can assert which art URLs were downloaded.
func imageServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte("img"))
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func TestRun_DownloadsSelectedArtWithKodiNaming(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	srv, paths := imageServer(t)
	fr := matrixResolver()
	fr.images = []artselect.Image{
		{Kind: artselect.Poster, Provider: artselect.ProviderTMDB, URL: srv.URL + "/poster.jpg", Language: "en", Popularity: 9},
		{Kind: artselect.Poster, Provider: artselect.ProviderTMDB, URL: srv.URL + "/poster-lo.jpg", Language: "en", Popularity: 1},
		{Kind: artselect.Fanart, Provider: artselect.ProviderTMDB, URL: srv.URL + "/fanart.jpg", Language: "", Popularity: 5},
		{Kind: artselect.Clearlogo, Provider: artselect.ProviderTMDB, URL: srv.URL + "/logo.png", Language: "en", Popularity: 5},
	}

	var out bytes.Buffer
	require.NoError(t, run(config{dir: root, out: &out, resolver: fr, client: srv.Client()}))

	dir := filepath.Join(root, "The Matrix (1999)")
	for _, name := range []string{
		"The Matrix (1999)-poster.jpg",
		"The Matrix (1999)-fanart.jpg",
		"The Matrix (1999)-clearlogo.png",
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, err, "expected art file %s", name)
	}

	// Exactly one image per art type is downloaded: the low-popularity poster
	// is never fetched.
	assert.ElementsMatch(t, []string{"/poster.jpg", "/fanart.jpg", "/logo.png"}, *paths)
	assert.Equal(t, "603", fr.lastID, "art is fetched for the resolved tmdb id")
}

func TestRun_ExistingArtSkippedUnlessForce(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")
	dir := filepath.Join(root, "The Matrix (1999)")
	posterPath := filepath.Join(dir, "The Matrix (1999)-poster.jpg")
	require.NoError(t, os.WriteFile(posterPath, []byte("hand-tuned"), 0o644))

	srv, _ := imageServer(t)
	fr := matrixResolver()
	fr.images = []artselect.Image{
		{Kind: artselect.Poster, Provider: artselect.ProviderTMDB, URL: srv.URL + "/poster.jpg", Language: "en", Popularity: 9},
	}

	// Default run: the existing poster is preserved.
	require.NoError(t, run(config{dir: root, out: &bytes.Buffer{}, resolver: fr, client: srv.Client()}))
	got, err := os.ReadFile(posterPath)
	require.NoError(t, err)
	assert.Equal(t, "hand-tuned", string(got), "existing art is left untouched without --force")

	// Force run: the poster is re-downloaded and overwritten.
	require.NoError(t, run(config{dir: root, force: true, out: &bytes.Buffer{}, resolver: fr, client: srv.Client()}))
	got, err = os.ReadFile(posterPath)
	require.NoError(t, err)
	assert.Equal(t, "img", string(got), "--force re-fetches and overwrites existing art")
}

func TestRun_DryRunDownloadsNoArt(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	srv, paths := imageServer(t)
	fr := matrixResolver()
	fr.images = []artselect.Image{
		{Kind: artselect.Poster, Provider: artselect.ProviderTMDB, URL: srv.URL + "/poster.jpg", Language: "en", Popularity: 9},
	}

	require.NoError(t, run(config{dir: root, dryRun: true, out: &bytes.Buffer{}, resolver: fr, client: srv.Client()}))

	_, statErr := os.Stat(filepath.Join(root, "The Matrix (1999)", "The Matrix (1999)-poster.jpg"))
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write art")
	assert.Empty(t, *paths, "dry-run must not download art")
}

func TestParseArgs_MissingDir(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseArgs([]string{}, &errOut)
	assert.ErrorIs(t, err, errMissingDir)
	assert.Contains(t, errOut.String(), "--dir is required")
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseArgs([]string{"--bogus"}, &errOut)
	assert.Error(t, err)
}

// stubTMDB serves a one-result search and a small details body so realMain can
// be exercised end to end against TMDB_BASE_URL.
func stubTMDB(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/movie" {
			_, _ = w.Write([]byte(`{"results":[{"id":603,"title":"The Matrix","release_date":"1999-03-30"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":603,"title":"The Matrix","release_date":"1999-03-30","imdb_id":"tt0133093"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRealMain_SuccessReturnsZero(t *testing.T) {
	root := t.TempDir()
	mkVideo(t, root, "The Matrix (1999)")

	t.Setenv("TMDB_API_KEY", "key")
	t.Setenv("TMDB_BASE_URL", stubTMDB(t))

	var out, errOut bytes.Buffer
	code := realMain([]string{"--dir", root}, &out, &errOut)
	assert.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), "enriched: 1")

	got, err := os.ReadFile(filepath.Join(root, "The Matrix (1999)", "The Matrix (1999).nfo"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `<uniqueid type="imdb">tt0133093</uniqueid>`)
}

func TestRealMain_MissingAPIKeyReturnsOne(t *testing.T) {
	t.Setenv("TMDB_API_KEY", "")
	var out, errOut bytes.Buffer
	code := realMain([]string{"--dir", t.TempDir()}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "TMDB_API_KEY")
}

func TestRealMain_BadUsageReturnsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := realMain([]string{}, &out, &errOut)
	assert.Equal(t, 2, code)
}

func TestRealMain_ScanFailureReturnsOne(t *testing.T) {
	t.Setenv("TMDB_API_KEY", "key")
	var out, errOut bytes.Buffer
	code := realMain([]string{"--dir", filepath.Join(t.TempDir(), "nope")}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "error:")
}

func TestRun_WriteErrorIsReportedNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("write-permission errors do not occur when running as root")
	}

	root := t.TempDir()
	dir := filepath.Join(root, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "The Matrix (1999).mkv"), []byte("v"), 0o644))
	// Make the folder read-only so the NFO cannot be created.
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	var out bytes.Buffer
	err := run(config{dir: root, out: &out, resolver: matrixResolver()})
	require.NoError(t, err) // one bad folder never aborts the run
	assert.Contains(t, out.String(), "errored: 1")
}
