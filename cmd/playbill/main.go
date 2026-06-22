// Command playbill walks a Kodi movie library and enriches each Movie Folder
// in place. It scans for "Title (Year)" folders with a video file, matches each
// against TMDB, and writes a rich, MediaElch-style NFO; it runs fully
// non-interactively. The TMDB API key is read from TMDB_API_KEY.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/schmidtw/playbill/internal/fanart"
	"github.com/schmidtw/playbill/internal/library"
	"github.com/schmidtw/playbill/internal/nameparse"
	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/schmidtw/playbill/internal/probe"
	"github.com/schmidtw/playbill/internal/report"
	"github.com/schmidtw/playbill/internal/tmdb"
	"github.com/schmidtw/playbill/internal/writer"
)

// version is the build version, stamped at link time via
// -ldflags "-X main.version=...". It defaults to "dev" for un-stamped builds
// (e.g. plain `go build` or `go run`).
var version = "dev"

// preferredLang is the language used to rank artwork. Folder names and metadata
// in this library are English; a configurable preference is a later concern.
const preferredLang = "en"

// defaultConcurrency is the number of folders processed in parallel when
// --concurrency is not given (see the PRD: bounded worker pool, default 4).
const defaultConcurrency = 4

// defaultArtSet is the art set written when --art is not given: the layout the
// PRD reproduces (user story 18). Clearart is available via --art but not part
// of the default set.
var defaultArtSet = []artselect.Kind{
	artselect.Poster, artselect.Fanart, artselect.Banner,
	artselect.Clearlogo, artselect.Discart, artselect.Landscape,
}

// knownArtKinds is every art type --art accepts, so an unknown type is rejected
// up front rather than silently fetching nothing.
var knownArtKinds = map[artselect.Kind]bool{
	artselect.Poster: true, artselect.Fanart: true, artselect.Banner: true,
	artselect.Clearlogo: true, artselect.Discart: true, artselect.Landscape: true,
	artselect.Clearart: true,
}

// fanartOnlyKinds are art types only Fanart.tv supplies. Without a Fanart.tv
// key they can never be fetched, so a missing one is reported as skipped-no-key
// rather than unavailable.
var fanartOnlyKinds = map[artselect.Kind]bool{
	artselect.Banner: true, artselect.Discart: true,
	artselect.Landscape: true, artselect.Clearart: true,
}

// resolver turns a parsed folder name into the canonical, rich movie metadata
// and supplies its baseline artwork candidates. *tmdb.Client satisfies it;
// tests inject fakes.
type resolver interface {
	Resolve(tmdb.ResolveRequest) (nfo.Movie, error)
	Images(id string) ([]artselect.Image, error)
}

// artProvider supplies artwork candidates for a movie id. Both *tmdb.Client and
// *fanart.Client satisfy it; the Fanart.tv provider is optional and left nil
// when no key is configured.
type artProvider interface {
	Images(id string) ([]artselect.Image, error)
}

// config holds the resolved run options.
type config struct {
	dir         string
	dryRun      bool
	force       bool
	json        bool
	// showVersion is set by --version; realMain prints the build version and
	// exits without scanning a library.
	showVersion bool
	concurrency int
	art         []artselect.Kind
	out         io.Writer
	client      *http.Client
	resolver    resolver
	// fanart is the optional Fanart.tv provider, nil when FANARTTV_API_KEY is
	// unset so extended art degrades gracefully (user story 14).
	fanart artProvider
	// report, when non-nil, is where run accumulates per-folder outcomes so the
	// caller can inspect the run for its exit code. run creates its own when nil.
	report *report.Report
}

// Process exit codes (user story 34: meaningful exit codes for automation).
const (
	exitOK      = 0 // every folder processed cleanly
	exitFatal   = 1 // the run could not start or complete (e.g. unreadable root)
	exitUsage   = 2 // bad command-line usage
	exitErrored = 3 // run completed but one or more folders errored
)

// errMissingDir is returned by parseArgs when --dir is not supplied.
var errMissingDir = errors.New("--dir is required")

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

// realMain is the testable entry point. It parses args, runs the enrichment,
// and returns a process exit code reflecting the run outcome: exitOK when every
// folder processed cleanly, exitErrored when some folder errored, exitFatal on a
// fatal run error, and exitUsage on bad usage.
func realMain(args []string, out, errOut io.Writer) int {
	cfg, err := parseArgs(args, errOut)
	if err != nil {
		return exitUsage
	}
	if cfg.showVersion {
		_, _ = fmt.Fprintln(out, "playbill", version)
		return exitOK
	}
	cfg.out = out
	cfg.client = &http.Client{Timeout: 30 * time.Second}
	cfg.report = &report.Report{}

	cfg.resolver, err = newResolver()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return exitFatal
	}
	cfg.fanart = newFanart()

	if err := run(cfg); err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return exitFatal
	}
	if cfg.report.HasErrors() {
		return exitErrored
	}
	return exitOK
}

// newResolver builds the TMDB client from the environment. The API key comes
// from TMDB_API_KEY (a missing key is a clear fatal error); TMDB_BASE_URL
// optionally overrides the API root.
func newResolver() (resolver, error) {
	var opts []tmdb.Option
	if base := os.Getenv("TMDB_BASE_URL"); base != "" {
		opts = append(opts, tmdb.WithBaseURL(base))
	}
	return tmdb.New(os.Getenv("TMDB_API_KEY"), opts...)
}

// newFanart builds the optional Fanart.tv provider from the environment. With
// no FANARTTV_API_KEY it returns a nil provider so the extended art types
// degrade gracefully and the run still succeeds (user story 14). The key comes
// from FANARTTV_API_KEY; FANARTTV_BASE_URL optionally overrides the API root.
func newFanart() artProvider {
	key := os.Getenv("FANARTTV_API_KEY")
	if key == "" {
		return nil
	}

	var opts []fanart.Option
	if base := os.Getenv("FANARTTV_BASE_URL"); base != "" {
		opts = append(opts, fanart.WithBaseURL(base))
	}
	c, err := fanart.New(key, opts...)
	if err != nil {
		return nil // New only fails on an empty key, already handled above
	}
	return c
}

// parseArgs parses command-line flags into a config. Usage and parse errors are
// written to errOut. The returned config has a nil out; the caller wires the
// destination writer.
func parseArgs(args []string, errOut io.Writer) (config, error) {
	fs := flag.NewFlagSet("playbill", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", "", "movie library root to enrich (required)")
	dryRun := fs.Bool("dry-run", false, "report intended writes without modifying the filesystem")
	force := fs.Bool("force", false, "re-fetch and overwrite existing NFO and artwork files")
	concurrency := fs.Int("concurrency", defaultConcurrency, "number of folders to process in parallel")
	jsonOut := fs.Bool("json", false, "emit a machine-readable JSON report instead of the text summary")
	art := fs.String("art", "", "comma-separated art types to fetch (default: poster,fanart,banner,clearlogo,discart,landscape)")
	showVersion := fs.Bool("version", false, "print the build version and exit")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *showVersion {
		return config{showVersion: true}, nil
	}
	if *dir == "" {
		_, _ = fmt.Fprintln(errOut, "error: --dir is required")
		return config{}, errMissingDir
	}

	artSet, err := parseArtSet(*art)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return config{}, err
	}

	return config{dir: *dir, dryRun: *dryRun, force: *force, concurrency: *concurrency, json: *jsonOut, art: artSet}, nil
}

// parseArtSet turns a --art value into the ordered list of art types to fetch.
// An empty value yields the default set; an unknown type is an error so a typo
// fails fast instead of silently fetching nothing (user story 19).
func parseArtSet(raw string) ([]artselect.Kind, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultArtSet, nil
	}

	var kinds []artselect.Kind
	for tok := range strings.SplitSeq(raw, ",") {
		kind := artselect.Kind(strings.TrimSpace(tok))
		if kind == "" {
			continue
		}
		if !knownArtKinds[kind] {
			return nil, fmt.Errorf("unknown art type %q", kind)
		}
		kinds = append(kinds, kind)
	}
	return kinds, nil
}

// run scans the library, writes a rich NFO per matched folder, and prints an
// end-of-run summary to cfg.out. Folders are processed concurrently by a
// bounded worker pool (cfg.concurrency); a single bad folder is recorded and
// never aborts the run; a failure to scan the root is fatal and returned.
func run(cfg config) error {
	folders, err := library.Scan(cfg.dir)
	if err != nil {
		return err
	}

	rep := cfg.report
	if rep == nil {
		rep = &report.Report{}
	}
	processFolders(cfg, folders, rep)

	return render(cfg, rep)
}

// render writes the end-of-run report to cfg.out: an indented JSON document
// when cfg.json is set (user story 34), otherwise the human-readable summary.
func render(cfg config, rep *report.Report) error {
	if cfg.json {
		data, err := rep.JSON()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cfg.out, string(data))
		return nil
	}
	_, _ = fmt.Fprint(cfg.out, rep.Summary())
	return nil
}

// processFolders runs processFolder over every folder, bounding the number in
// flight to cfg.concurrency (a value below 1 means sequential). The report is
// the only shared state and is safe for concurrent recording.
func processFolders(cfg config, folders []library.MovieFolder, rep *report.Report) {
	limit := max(cfg.concurrency, 1)

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, f := range folders {
		sem <- struct{}{} // block once `limit` folders are in flight
		wg.Go(func() {
			defer func() { <-sem }()
			processFolder(cfg, f, rep)
		})
	}
	wg.Wait()
}

// processFolder parses one folder's name, resolves it against TMDB, and writes
// its rich NFO, recording the outcome in rep. A folder whose name does not parse
// or that has no confident TMDB match is skipped and reported, never guessed.
func processFolder(cfg config, f library.MovieFolder, rep *report.Report) {
	title, year, ok := nameparse.Parse(f.Name)
	if !ok {
		rep.Unmatched(f.Name)
		return
	}

	nfoPath := filepath.Join(f.Path, f.Name+".nfo")
	req := tmdb.ResolveRequest{Title: title, Year: year, KnownID: existingTMDBID(nfoPath)}

	movie, err := cfg.resolver.Resolve(req)
	if errors.Is(err, tmdb.ErrNoMatch) || errors.Is(err, tmdb.ErrAmbiguousMatch) {
		rep.Unmatched(f.Name)
		return
	}
	if err != nil {
		rep.Errored(f.Name, err.Error())
		return
	}

	movie.StreamDetails = streamDetails(filepath.Join(f.Path, f.VideoFile))

	// Gather art candidates before marshaling so the full poster/fanart catalog
	// can be embedded in the NFO and the chosen images downloaded. A gather
	// failure is recorded but never costs the folder its NFO.
	id := defaultUniqueID(movie.UniqueIDs, "tmdb")
	var candidates []artselect.Image
	gathered := false
	if id != "" {
		candidates, gathered = gatherArt(cfg, f, id, rep)
	}
	movie.Posters, movie.Fanarts = artCatalog(candidates)

	data, err := nfo.Marshal(movie)
	if err != nil {
		rep.Errored(f.Name, err.Error())
		return
	}

	res, err := writer.WriteNFO(nfoPath, data, cfg.force, cfg.dryRun)
	if err != nil {
		rep.Errored(f.Name, err.Error())
		return
	}

	switch res {
	case writer.Created:
		rep.Enriched(f.Name)
	case writer.Skipped:
		rep.Skipped(f.Name)
	case writer.Planned:
		rep.Planned(f.Name)
	}

	if gathered {
		downloadArt(cfg, f, candidates, rep)
	}
}

// artCatalog splits the gathered candidates into the poster and fanart URL
// lists embedded in the NFO. Every candidate from every provider is included so
// Kodi's offline "Choose art" UI has the full set of options, independent of
// which single image is downloaded to disk.
func artCatalog(candidates []artselect.Image) (posters, fanarts []string) {
	for _, img := range candidates {
		switch img.Kind {
		case artselect.Poster:
			posters = append(posters, img.URL)
		case artselect.Fanart:
			fanarts = append(fanarts, img.URL)
		}
	}
	return posters, fanarts
}

// downloadArt selects the best image for each wanted art type from the gathered
// candidates and downloads it into the folder with Kodi naming. It is
// best-effort: a fetch failure is recorded and never aborts the run, so missing
// artwork does not cost the folder its NFO. Wanted types that no provider had
// are reported, distinguishing art skipped for lack of a Fanart.tv key from art
// genuinely unavailable for the movie.
func downloadArt(cfg config, f library.MovieFolder, candidates []artselect.Image, rep *report.Report) {
	wanted := cfg.art
	if len(wanted) == 0 {
		wanted = defaultArtSet
	}

	have := map[artselect.Kind]bool{}
	for _, img := range artselect.Select(wantedOnly(candidates, wanted), preferredLang) {
		have[img.Kind] = true
		name := f.Name + "-" + string(img.Kind) + imageExt(img.URL)
		path := filepath.Join(f.Path, name)
		if _, err := writer.WriteArt(cfg.client, path, img.URL, cfg.force, cfg.dryRun); err != nil {
			rep.Errored(f.Name, "artwork "+string(img.Kind)+": "+err.Error())
		}
	}

	reportMissingArt(cfg, f.Name, wanted, have, rep)
}

// gatherArt collects artwork candidates from TMDB and, when configured, the
// optional Fanart.tv provider. It returns ok=false when the required TMDB lookup
// fails (the error is recorded); a Fanart.tv failure is recorded but does not
// stop the run from using TMDB's art.
func gatherArt(cfg config, f library.MovieFolder, id string, rep *report.Report) ([]artselect.Image, bool) {
	candidates, err := cfg.resolver.Images(id)
	if err != nil {
		rep.Errored(f.Name, "artwork: "+err.Error())
		return nil, false
	}

	if cfg.fanart != nil {
		extra, err := cfg.fanart.Images(id)
		if err != nil {
			rep.Errored(f.Name, "fanart artwork: "+err.Error())
		} else {
			candidates = append(candidates, extra...)
		}
	}
	return candidates, true
}

// wantedOnly keeps just the candidates whose art type is in the wanted set, so
// --art controls which types are even considered for selection.
func wantedOnly(candidates []artselect.Image, wanted []artselect.Kind) []artselect.Image {
	want := map[artselect.Kind]bool{}
	for _, k := range wanted {
		want[k] = true
	}

	var out []artselect.Image
	for _, img := range candidates {
		if want[img.Kind] {
			out = append(out, img)
		}
	}
	return out
}

// reportMissingArt records every wanted art type for which no provider had a
// candidate, distinguishing a type skipped for lack of a Fanart.tv key (only
// Fanart.tv supplies it and none is configured) from one unavailable for the
// movie.
func reportMissingArt(cfg config, folder string, wanted []artselect.Kind, have map[artselect.Kind]bool, rep *report.Report) {
	for _, k := range wanted {
		if have[k] {
			continue
		}
		if cfg.fanart == nil && fanartOnlyKinds[k] {
			rep.ArtSkippedNoKey(folder, string(k))
		} else {
			rep.ArtUnavailable(folder, string(k))
		}
	}
}

// defaultUniqueID returns the value of the unique id of the given type, or "".
func defaultUniqueID(ids []nfo.UniqueID, typ string) string {
	for _, id := range ids {
		if id.Type == typ {
			return id.Value
		}
	}
	return ""
}

// imageExt returns the file extension of an image URL (including the dot), used
// to name the art file. It defaults to ".jpg" when the URL carries none.
func imageExt(url string) string {
	if ext := filepath.Ext(url); ext != "" {
		return ext
	}
	return ".jpg"
}

// existingTMDBID returns the TMDB id recorded in an existing NFO at nfoPath, or
// "" when there is no readable NFO with a tmdb unique id. A known id lets the
// resolver trust a prior (possibly hand-corrected) match instead of searching.
func existingTMDBID(nfoPath string) string {
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		return ""
	}
	if id, ok := nfo.TMDBID(data); ok {
		return id
	}
	return ""
}

// streamDetails probes the video for Stream Details, mapping them into the NFO
// model. Probing is best-effort: a container we cannot read (or any probe
// failure) yields no <fileinfo> rather than aborting the folder, so one odd
// file never breaks the run.
func streamDetails(videoPath string) *nfo.StreamDetails {
	sd, err := probe.Probe(videoPath)
	if err != nil {
		return nil
	}

	out := &nfo.StreamDetails{}
	if sd.Video.Codec != "" {
		out.Video = &nfo.VideoStream{
			Codec:             sd.Video.Codec,
			Aspect:            sd.Video.Aspect,
			Width:             sd.Video.Width,
			Height:            sd.Video.Height,
			DurationInSeconds: sd.Video.DurationInSeconds,
			ScanType:          sd.Video.ScanType,
		}
	}
	for _, a := range sd.Audio {
		out.Audio = append(out.Audio, nfo.AudioStream(a))
	}
	return out
}
