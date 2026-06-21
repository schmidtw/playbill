# Pure-Go stream-detail probing (no ffprobe)

To populate the NFO's `<fileinfo><streamdetails>` (video codec/resolution/scan
type, audio tracks/languages/channels, duration), `playbill` reads the video
file with pure-Go container parsers — MP4/M4V and MKV — rather than shelling out
to `ffprobe`/`mediainfo`.

The obvious path is ffprobe: ubiquitous, accurate, broad format coverage. We
rejected it to keep the deliverable a single self-contained static binary
(`CGO_ENABLED=0`) with no external runtime dependency to install on the NAS or in
the Docker image. The trade-off is weaker format coverage: only MP4/M4V and MKV
are probed; any other container skips the `<fileinfo>` block and is noted in the
report.

## Consequences

- One binary, no ffmpeg install, trivial Docker image and cron deployment.
- A reader seeing a hand-rolled MP4/MKV parser should not "fix" it by swapping in
  ffprobe — the no-external-dependency property is deliberate.
- Adding a new container format means adding/finding a pure-Go parser, not just a
  CLI flag.
