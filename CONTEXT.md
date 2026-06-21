# playbill

A one-shot CLI that walks a Kodi movie library and writes metadata (NFO) and
artwork files into each movie's folder, so Kodi displays a complete local
data/artwork set. It enriches in place and never reorganizes files.

## Language

**Movie Folder**:
A directory holding exactly one movie's video file, named `Title (Year)`. Its
name is the ground-truth source of the movie's title and year — the tool never
renames it.
_Avoid_: directory, path

**Enrich**:
To write NFO and artwork files into a Movie Folder. Enrichment is additive only:
it never renames, moves, or deletes the existing video file or folder.
_Avoid_: scrape (scraping is one step of enrichment), import, organize

**Artwork**:
An image file Kodi associates with a movie, written as `Title (Year)-<arttype>.<ext>`
(e.g. `-poster.jpg`, `-fanart.jpg`, `-clearlogo.png`, `-discart.png`).
_Avoid_: image, art, thumb (thumb is one specific art type)

**NFO**:
The `Title (Year).nfo` XML file holding the movie's metadata and the unique
provider IDs (TMDB/IMDB) Kodi uses to identify it. Modeled on a
MediaElch-generated, Kodi-21 NFO for richness (full art catalog, cast, ratings,
stream details) — but not byte-for-byte parity; Kodi-owned runtime fields
(playcount, lastplayed, resume, userrating) are omitted.
_Avoid_: metadata file, sidecar

**Stream Details**:
The technical media facts (video codec/resolution/scan type, audio
tracks/languages/channels, duration) written into the NFO's `<fileinfo>`. Read
by probing the local video file itself, not from any online provider.
_Avoid_: mediainfo, ffprobe output, technical metadata
