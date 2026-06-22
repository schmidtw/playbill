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
	"time"

	"github.com/schmidtw/playbill/internal/artselect"
	"github.com/schmidtw/playbill/internal/library"
	"github.com/schmidtw/playbill/internal/nameparse"
	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/schmidtw/playbill/internal/probe"
	"github.com/schmidtw/playbill/internal/report"
	"github.com/schmidtw/playbill/internal/tmdb"
	"github.com/schmidtw/playbill/internal/writer"
)

// preferredLang is the language used to rank artwork. Folder names and metadata
// in this library are English; a configurable preference is a later concern.
const preferredLang = "en"

// resolver turns a parsed folder name into the canonical, rich movie metadata
// and supplies its baseline artwork candidates. *tmdb.Client satisfies it;
// tests inject fakes.
type resolver interface {
	Resolve(tmdb.ResolveRequest) (nfo.Movie, error)
	Images(id string) ([]artselect.Image, error)
}

// config holds the resolved run options.
type config struct {
	dir      string
	dryRun   bool
	force    bool
	out      io.Writer
	client   *http.Client
	resolver resolver
}

// errMissingDir is returned by parseArgs when --dir is not supplied.
var errMissingDir = errors.New("--dir is required")

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

// realMain is the testable entry point. It parses args, runs the enrichment,
// and returns a process exit code: 0 on success, 1 on a fatal run error, 2 on
// bad usage.
func realMain(args []string, out, errOut io.Writer) int {
	cfg, err := parseArgs(args, errOut)
	if err != nil {
		return 2
	}
	cfg.out = out
	cfg.client = &http.Client{Timeout: 30 * time.Second}

	cfg.resolver, err = newResolver()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return 1
	}

	if err := run(cfg); err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	return 0
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

// parseArgs parses command-line flags into a config. Usage and parse errors are
// written to errOut. The returned config has a nil out; the caller wires the
// destination writer.
func parseArgs(args []string, errOut io.Writer) (config, error) {
	fs := flag.NewFlagSet("playbill", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", "", "movie library root to enrich (required)")
	dryRun := fs.Bool("dry-run", false, "report intended writes without modifying the filesystem")
	force := fs.Bool("force", false, "re-fetch and overwrite existing NFO and artwork files")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *dir == "" {
		_, _ = fmt.Fprintln(errOut, "error: --dir is required")
		return config{}, errMissingDir
	}

	return config{dir: *dir, dryRun: *dryRun, force: *force}, nil
}

// run scans the library, writes a rich NFO per matched folder, and prints an
// end-of-run summary to cfg.out. A single bad folder is recorded and never
// aborts the run; a failure to scan the root is fatal and returned.
func run(cfg config) error {
	folders, err := library.Scan(cfg.dir)
	if err != nil {
		return err
	}

	var rep report.Report
	for _, f := range folders {
		processFolder(cfg, f, &rep)
	}

	_, _ = fmt.Fprint(cfg.out, rep.Summary())
	return nil
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

	downloadArt(cfg, f, movie, rep)
}

// downloadArt selects the baseline artwork for a resolved movie and downloads
// one best image per art type into the folder with Kodi naming. It is best-
// effort: a failure to list or fetch art is recorded and never aborts the run,
// so missing artwork does not cost the folder its NFO.
func downloadArt(cfg config, f library.MovieFolder, movie nfo.Movie, rep *report.Report) {
	id := defaultUniqueID(movie.UniqueIDs, "tmdb")
	if id == "" {
		return
	}

	candidates, err := cfg.resolver.Images(id)
	if err != nil {
		rep.Errored(f.Name, "artwork: "+err.Error())
		return
	}

	for _, img := range artselect.Select(candidates, preferredLang) {
		name := f.Name + "-" + string(img.Kind) + imageExt(img.URL)
		path := filepath.Join(f.Path, name)
		if _, err := writer.WriteArt(cfg.client, path, img.URL, cfg.force, cfg.dryRun); err != nil {
			rep.Errored(f.Name, "artwork "+string(img.Kind)+": "+err.Error())
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
