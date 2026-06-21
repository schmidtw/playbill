# PRD: playbill — headless Kodi movie-library enricher

## Problem Statement

I run a Kodi instance backed by a movie library on my NAS, organized one movie
per folder as `Title (Year)/Title (Year).ext`. I want each movie to have a
complete local data set — a rich `.nfo` plus the full artwork set (poster,
fanart, banner, clearlogo, discart, landscape) — so Kodi shows the best possible
experience and never has to scrape online.

Today I get that quality from MediaElch, but MediaElch is a GUI I have to sit in
front of and babysit (manual scrape-and-confirm per movie). I want to "just run
a program across a directory and the right files show up" — unattended, on a
schedule, on the NAS. Tools I've tried (tinyMediaManager) are too needy, and
daemon-style managers (Radarr) are heavier than I want. I'd rather have a small,
self-contained CLI I can cron.

## Solution

A single, dependency-free Go binary, `playbill`, that walks a movie library
and enriches each movie folder **in place**: it writes a MediaElch-style rich NFO
and downloads the artwork set, using the folder name as the source of truth for
title and year. It runs fully non-interactively — confident matches get
enriched, uncertain ones are skipped and reported — so it's safe to schedule.
Metadata and baseline art come from TMDB (required); extended art comes from
Fanart.tv (optional). Stream-detail info in the NFO is read directly from the
video file using pure-Go parsers, so there's no ffmpeg/ffprobe dependency and the
whole thing ships as one static binary.

It is "good-enough automation" — modeled on MediaElch's output richness, but not a
byte-for-byte clone.

## User Stories

1. As a Kodi user, I want to run one command against my movie root, so that every
   movie folder gets its NFO and artwork without per-movie interaction.
2. As a NAS operator, I want a single static binary with no external runtime
   dependencies, so that I can drop it in or containerize it without installing
   ffmpeg, Java, or a desktop app.
3. As a cron user, I want the tool to run fully unattended and never block on a
   prompt, so that scheduled runs always complete.
4. As a library owner, I want the folder name (`Title (Year)`) treated as ground
   truth, so that the tool never renames, moves, or deletes my video files.
5. As a careful user, I want enrichment to be additive only, so that the tool can
   never damage my existing library.
6. As a returning user, I want re-runs to skip files that already exist, so that
   re-running is cheap and idempotent.
7. As a user who hand-tunes art/metadata, I want my manual edits preserved on
   re-run, so that the tool doesn't clobber my choices.
8. As a user refreshing stale data, I want a `--force` flag, so that I can
   deliberately re-fetch and overwrite everything when I choose to.
9. As a user with a clean library, I want a confident match from the folder's
   title and year, so that the correct movie's data is fetched.
10. As a user who has corrected a match, I want an existing `<uniqueid type="tmdb">`
    in the NFO to be trusted over re-searching, so that my correction sticks and
    re-runs are faster.
11. As a user with an ambiguous or unfindable folder, I want it skipped and
    reported rather than guessed, so that wrong data is never silently written.
12. As a user, I want an end-of-run summary of what was enriched, skipped,
    unmatched, and errored, so that I know exactly what to fix and re-run.
13. As a TMDB user, I want metadata, identity, and baseline art (poster, fanart,
    logo) from TMDB, so that the tool works with a single free API key.
14. As a user without a Fanart.tv key, I want the run to still succeed, so that
    extended art is an optional upgrade, not a hard gate.
15. As a user with a Fanart.tv key, I want the extended art types (clearlogo,
    banner, discart, landscape, clearart) downloaded, so that my library has the
    full art set.
16. As a security-conscious user, I want API keys supplied via environment
    variables, so that secrets don't leak into shell history or `ps` output.
17. As a CLI user, I want run behavior controlled by flags (`--dir`, `--force`,
    `--dry-run`, `--art`, `--concurrency`, `--json`), so that I can tune a run
    without editing config.
18. As a user, I want the default artwork set to be poster, fanart, banner,
    clearlogo, discart, and landscape, so that I reproduce the layout I already
    like.
19. As a user with different needs, I want to override the art set with `--art`,
    so that I can add or remove types without a code change.
20. As a user, I want exactly one best image chosen per art type, so that my
    folders aren't cluttered with duplicates.
21. As a user, I want art chosen by language, then popularity, then resolution,
    so that I get the right-language, highest-quality image.
22. As a user, I want Fanart.tv preferred over TMDB for art types both offer, so
    that I get the community-curated version when available.
23. As a Kodi user, I want a rich NFO (cast with thumb URLs, ratings,
    certification, genres, studios, country, director/writers, collection,
    trailer), so that Kodi can run on local information only.
24. As a Kodi user, I want the NFO to embed the full art candidate catalog, so
    that Kodi's offline "Choose art" UI can switch images without re-scraping.
25. As a Kodi user, I want `<fileinfo><streamdetails>` populated from the actual
    video file, so that resolution/codec/audio flags show correctly before
    playback.
26. As a NAS user with MP4/M4V and MKV files, I want both probed for stream
    details, so that the common formats in my library are covered.
27. As a user with an odd container, I want stream details skipped and reported
    rather than the run failing, so that one unusual file doesn't break the run.
28. As a Kodi user, I want both TMDB and IMDB unique IDs written (plus a legacy
    `<id>`), so that Kodi and older skins identify the movie reliably.
29. As a user, I want Kodi-owned runtime fields (playcount, resume, etc.) omitted,
    so that the tool doesn't fight Kodi over playback state.
30. As a cautious user, I want a `--dry-run` mode that reports what would be
    written without writing, so that I can preview a run before committing.
31. As a performance-minded user, I want folders processed concurrently with a
    bounded worker pool, so that a large library enriches quickly.
32. As an API citizen, I want provider rate limits respected, so that the tool
    doesn't get throttled or banned.
33. As an operator, I want one failed folder to never abort the whole run, so
    that a single bad movie doesn't stop the rest.
34. As a scripting user, I want an optional `--json` report and meaningful exit
    codes, so that I can drive the tool from other automation.
35. As a Kodi user, I want a reminder that the extended art types require the Kodi
    artwork whitelist to display, so that I'm not surprised when banner/clearlogo
    don't show.

## Implementation Decisions

**Architecture & scope (see ADR-0001):**
- Additive, in-place enrichment only. Folder name is ground truth for title/year.
  No rename/move/delete. Fully non-interactive: confident match → enrich,
  otherwise skip + report. Idempotent; skip-if-present by default, `--force`
  overwrites.

**Providers (see ADR-0003):**
- TMDB required (identity, metadata, baseline art). Fanart.tv optional with
  graceful degradation. For shared art types, Fanart.tv preferred over TMDB.
- Keys via env (`TMDB_API_KEY`, `FANARTTV_API_KEY`); behavior via flags. No config
  file in v1.

**Stream details (see ADR-0002):**
- Pure-Go probe of MP4/M4V and MKV; other containers skip `<fileinfo>` and are
  reported. No ffprobe/ffmpeg dependency; single static binary (`CGO_ENABLED=0`).

**Modules:**
- `nameparse` (deep, pure) — folder name → title + year.
- `tmdb` / `fanart` clients — REST/JSON behind a small `ArtProvider` interface;
  `tmdb` also yields a canonical `Movie` model.
- `artselect` (deep, pure) — candidate art + policy → chosen image per type + full
  catalog. Language → popularity → resolution; provider precedence.
- `probe` (deep) — `Prober` interface with MP4 and MKV implementations →
  `StreamDetails`.
- `nfo` (deep, pure) — canonical `Movie` + chosen art + catalog + stream details →
  NFO XML bytes. MediaElch-style richness; Kodi-21 oriented; runtime fields
  omitted; embeds full art catalog; writes tmdb (default) + imdb uniqueids + legacy
  `<id>`; includes top YouTube trailer.
- `library` (thin I/O) — walks root, yields `MovieFolder` with existing-file
  inventory.
- `writer` (thin I/O) — downloads chosen art and writes NFO/art with Kodi naming
  (`Title (Year)-<arttype>.<ext>`); enforces skip-if-present / `--force` /
  `--dry-run`.
- `enricher` — per-folder orchestration + run-level concurrency (bounded pool,
  `--concurrency` default 4), per-provider rate limiting, per-folder error
  isolation.
- `report` — aggregates outcomes into a summary + optional `--json`.
- `cmd/playbill` — flag/env wiring only.

**Selection & output specifics:**
- One best file per art type written to disk; full candidate catalog embedded in
  NFO.
- Default art set: poster, fanart, banner, clearlogo, discart, landscape;
  overridable via `--art`.
- Cast thumbnails referenced as URLs in the NFO (not downloaded to `.actors/`).

## Testing Decisions

**What makes a good test here:** assert externally observable behavior, not
internals. For the pure modules that means input → output: a folder name in, a
parsed title/year out; candidate lists + policy in, chosen art + catalog out; a
`Movie` model in, NFO bytes out (golden file); a fixture video in, a
`StreamDetails` struct out. Tests should not assert private call sequences or
struct shapes that aren't part of the contract.

**Modules to be tested in v1 (committed):** the four deep, pure modules —
- `nameparse` — table-driven over folder-name edge cases (year in title, no year,
  extra tokens, unicode).
- `artselect` — table-driven over selection policy (language preference,
  popularity tiebreak, resolution tiebreak, provider precedence, empty
  candidates).
- `nfo` — golden-file tests: a fixed `Movie` model marshals to an expected NFO,
  including the embedded catalog, uniqueids, stream details, and omission of
  runtime fields.
- `probe` — fixture-file tests: a small real `.m4v` and `.mkv` produce the
  expected `StreamDetails`; an unsupported container returns the
  skip-and-report signal rather than an error that aborts.

**Prior art:** none yet — greenfield repo. These establish the patterns:
table-driven tests with `testify`, `fstest.MapFS` for any filesystem edges,
`httptest` for client contract tests if/when added, and golden files for `nfo`.

**Deferred (not committed in v1):** provider client contract tests (`tmdb`,
`fanart`), `library`/`writer` filesystem-edge tests, and `enricher` end-to-end
integration tests. The architecture keeps these seams clean so they can be added
later without refactoring.

## Out of Scope

- Renaming, moving, organizing, or deleting any existing files (enrichment is
  additive only).
- Fuzzy matching / interactive disambiguation of unmatched folders — those are
  skipped and reported for the user to fix.
- TV shows, episodes, music videos, extras, stacked/multi-disc movies.
- A config file (env + flags only in v1).
- Downloading actor images into a `.actors/` folder (cast thumbs are URLs in the
  NFO).
- ffprobe/ffmpeg or any external runtime dependency.
- Byte-for-byte MediaElch output parity (`<generator>`, exact field ordering,
  Kodi runtime fields).
- Probing containers beyond MP4/M4V and MKV (those skip stream details).
- Managing Kodi itself (library scans, the artwork whitelist) — surfaced as a
  user reminder, not automated.

## Further Notes

- **Tracer bullet:** the first vertical slice should prove the riskiest piece —
  the pure-Go `probe` against one real `.m4v` and one `.mkv` from the library,
  emitting a correct `<streamdetails>` block. Network/XML/file-walking are
  straightforward Go once probing is proven.
- **Kodi reminder:** the extended art types (banner, clearlogo, discart,
  landscape) won't display until they're added to Kodi's artwork whitelist
  (Settings → Media → Videos, or `advancedsettings.xml`). The tool writes the
  files; Kodi must be told to use them.
- **Likely libraries:** `abema/go-mp4` for MP4/M4V; an EBML/Matroska parser for
  MKV. Both must be pure-Go to preserve the single-binary property.
- Decisions are recorded in `docs/adr/0001`–`0003`; domain terms in `CONTEXT.md`.
