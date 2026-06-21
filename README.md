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

## Status

Design phase. See the design docs:

- [`CONTEXT.md`](CONTEXT.md) — domain glossary
- [`docs/PRD.md`](docs/PRD.md) — product requirements
- [`docs/adr/`](docs/adr/) — architecture decision records

## Configuration

| | |
|---|---|
| `TMDB_API_KEY` | required — metadata, identity, baseline art |
| `FANARTTV_API_KEY` | optional — extended art types |

Run behavior is controlled by flags (`--dir`, `--force`, `--dry-run`, `--art`,
`--concurrency`, `--json`).

> **Kodi note:** the extended art types (banner, clearlogo, discart, landscape)
> won't display until they're added to Kodi's artwork whitelist
> (Settings → Media → Videos, or `advancedsettings.xml`). `playbill` writes the
> files; Kodi must be told to use them.
