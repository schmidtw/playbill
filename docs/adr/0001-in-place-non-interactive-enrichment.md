# Additive, in-place, non-interactive enrichment

`playbill` treats each Movie Folder's name as the ground-truth source of title
and year, and only ever *adds* files (NFO + artwork) alongside the existing
video — it never renames, moves, deletes, or reorganizes. When it cannot
confidently match a folder it skips and reports rather than prompting, so a full
run is unattended and cron-safe. Re-runs are idempotent: existing files are left
untouched unless `--force` is given.

We chose this over building a full media manager (à la tinyMediaManager /
MediaElch) because the library is already cleanly organized as `Title (Year)`,
which makes the genuinely hard parts of those tools — fuzzy matching and
destructive renaming — unnecessary. The cost is that messy/unmatched folders are
the user's job to fix (rename or drop in a `uniqueid`), not the tool's.

## Consequences

- The tool is safe to re-run and to point at a live Kodi library.
- Manual edits to NFO/artwork survive re-runs (skip-if-present).
- Folders that don't already match `Title (Year)` get no enrichment until the
  user fixes them — surfaced in the end-of-run report.
