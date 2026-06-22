# playbill

Headless CLI that writes rich NFO + artwork into your Kodi movie library — run it
across a directory and the right files show up.

`playbill` walks a movie library organized one film per folder
(`Title (Year)/Title (Year).ext`) and enriches each folder **in place**: it writes
a MediaElch-style `.nfo` (cast, credits, ratings, collection, stream details) and
downloads the artwork set (poster, fanart, banner, clearlogo, discart, landscape).
It runs fully unattended — confident matches are enriched, uncertain ones are
skipped and reported — so it's safe to schedule on a NAS.

- **In place, additive only** — the folder name is the source of truth; nothing is
  renamed, moved, or deleted.
- **Idempotent** — existing files are left untouched unless `--force` is given.
- **Single static binary** — no ffmpeg/ffprobe, no Java, no GUI. Stream details are
  read with pure-Go MP4/M4V and MKV parsers.
- **TMDB required, Fanart.tv optional** — works with a single free key; extended art
  is an opt-in upgrade.

## Install

### Build from source

Requires Go (see `go.mod` for the version). The build is `CGO_ENABLED=0`, so the
result is a single statically linked binary with no external runtime
dependencies.

```sh
make build          # -> ./dist/playbill
./dist/playbill --version
```

`make help` lists the other targets (`test`, `cover`, `lint`, `docker`, …).

### Docker

A minimal `scratch` image (just the binary plus CA certificates) is provided for
NAS/cron use:

```sh
make docker         # builds the image (auto-detects docker or podman)

docker run --rm \
  -e TMDB_API_KEY \
  -e FANARTTV_API_KEY \
  -v /path/to/movies:/library \
  playbill --dir /library
```

The library is mounted at `/library` and API keys are passed through the
environment, so secrets never appear on the command line. `make docker-run
LIBRARY=/path/to/movies` wraps the same invocation.

## Configuration

| | |
|---|---|
| `TMDB_API_KEY` | required — metadata, identity, baseline art |
| `FANARTTV_API_KEY` | optional — extended art types |

API keys are read from the environment so they never leak into shell history or
`ps` output. Run behavior is controlled by flags:

```sh
playbill --dir /path/to/movies                 # enrich everything new
playbill --dir /path/to/movies --dry-run       # preview without writing
playbill --dir /path/to/movies --force         # re-fetch and overwrite
playbill --dir /path/to/movies --json          # machine-readable report
```

| Flag | Default | Meaning |
|---|---|---|
| `--dir` | _(required)_ | movie library root to enrich |
| `--dry-run` | `false` | report intended writes without modifying anything |
| `--force` | `false` | re-fetch and overwrite existing NFO and artwork |
| `--art` | `poster,fanart,banner,clearlogo,discart,landscape` | art types to fetch |
| `--concurrency` | `4` | folders processed in parallel |
| `--json` | `false` | emit a JSON report instead of the text summary |
| `--version` | | print the build version and exit |

Exit codes: `0` all folders processed cleanly, `1` fatal run error, `2` bad
usage, `3` run completed but one or more folders errored.

## Design docs

- [`CONTEXT.md`](CONTEXT.md) — domain glossary
- [`docs/PRD.md`](docs/PRD.md) — product requirements
- [`docs/adr/`](docs/adr/) — architecture decision records

> **Kodi note:** the extended art types (banner, clearlogo, discart, landscape)
> won't display until they're added to Kodi's artwork whitelist
> (Settings → Media → Videos, or `advancedsettings.xml`). `playbill` writes the
> files; Kodi must be told to use them.
