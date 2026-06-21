# TMDB required, Fanart.tv optional with graceful degradation

`playbill` uses two providers. TMDB is required — it supplies movie identity,
metadata, and the baseline art (poster, fanart, logo); with no `TMDB_API_KEY`
the tool refuses to run. Fanart.tv is optional — with a `FANARTTV_API_KEY` it
supplies the extended art types (clearlogo, banner, discart, landscape,
clearart); without one, those types are skipped and the run still succeeds.

We chose graceful degradation over requiring both keys so the tool is useful on
day one with a single free key, and the curated extended art is an opt-in
upgrade rather than a hard gate. For art types both providers offer (poster,
fanart), Fanart.tv is preferred for its community-ranked quality, falling back to
TMDB.

## Consequences

- A run with only TMDB produces a complete, valid library — just without the
  fancy extended art.
- Reports must distinguish "art type skipped because no Fanart.tv key" from "art
  type unavailable for this movie," so absence isn't mistaken for failure.
