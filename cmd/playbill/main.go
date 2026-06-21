// Command playbill walks a Kodi movie library and enriches each Movie Folder
// in place. This walking skeleton scans for "Title (Year)" folders with a video
// file and writes a minimal NFO; it runs fully non-interactively.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/schmidtw/playbill/internal/library"
	"github.com/schmidtw/playbill/internal/nameparse"
	"github.com/schmidtw/playbill/internal/nfo"
	"github.com/schmidtw/playbill/internal/probe"
	"github.com/schmidtw/playbill/internal/report"
	"github.com/schmidtw/playbill/internal/writer"
)

// config holds the resolved run options.
type config struct {
	dir    string
	dryRun bool
	out    io.Writer
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

	if err := run(cfg); err != nil {
		_, _ = fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	return 0
}

// parseArgs parses command-line flags into a config. Usage and parse errors are
// written to errOut. The returned config has a nil out; the caller wires the
// destination writer.
func parseArgs(args []string, errOut io.Writer) (config, error) {
	fs := flag.NewFlagSet("playbill", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", "", "movie library root to enrich (required)")
	dryRun := fs.Bool("dry-run", false, "report intended writes without modifying the filesystem")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *dir == "" {
		_, _ = fmt.Fprintln(errOut, "error: --dir is required")
		return config{}, errMissingDir
	}

	return config{dir: *dir, dryRun: *dryRun}, nil
}

// run scans the library, writes a minimal NFO per matched folder, and prints an
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

// processFolder parses one folder's name and writes its NFO, recording the
// outcome in rep.
func processFolder(cfg config, f library.MovieFolder, rep *report.Report) {
	title, year, ok := nameparse.Parse(f.Name)
	if !ok {
		rep.Unmatched(f.Name)
		return
	}

	sd := streamDetails(filepath.Join(f.Path, f.VideoFile))
	data, err := nfo.Marshal(nfo.Movie{Title: title, Year: year, StreamDetails: sd})
	if err != nil {
		rep.Errored(f.Name, err.Error())
		return
	}

	nfoPath := filepath.Join(f.Path, f.Name+".nfo")
	res, err := writer.WriteNFO(nfoPath, data, cfg.dryRun)
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
