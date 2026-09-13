# Catalog ranking and recommendations

The first version uses deterministic rules, without a neural model. The API owns
catalog membership and page ordering; the launcher derives interests from its
local library and play log and chooses the small recommendation shelves.

## Available evidence

- IGDB supplies genres, themes, content type, release date, `total_rating` and
  `total_rating_count`. Rating count is a proxy for external reach, not sales,
  concurrent players or a Typhon audience measurement.
- Steam supplies confirmed provider identities, classification and available
  catalog metadata. Steam review scores/counts and community tags are not
  imported by this implementation. Missing ratings remain absent in the UI.
- The local library supplies favorites, total playtime, last played, installed
  and archived state. The local play log supplies session counts. A favorite is
  explicit interest; activity requires at least two sessions and 30 minutes.
  One launch, including one long launch, does not establish interest.
- “Not interested” excludes that identity from recommendations and from the
  interest evidence. It does not infer dislike of an entire genre. The catalog
  filter can reveal dismissed games with a restore action.

Provider identities and redirects use explicit Steam/IGDB links. Similar names
never merge games. A catalog card opens the existing game detail and its release
variants. Unknown content classification stays in the default catalog; known
DLC, expansions, demos and soundtracks require an explicit content filter.
Low popularity never removes a game from catalog membership.

## Public catalog order

Let `r` be the actual rating (0–100), `n` the actual rating count, `m=20` the
prior count and `C` the average of known catalog ratings capped at 70.

```
correctedRating = (n*r + m*C) / (n+m)
popular = ln(1+n) * correctedRating/100
```

These scores are absent when the actual rating or a positive count is absent.
“Highly rated” orders by corrected rating; “Popular” orders by popular score.
“New releases” uses the provider release date; “Alphabetical” uses catalog title.
Ties include canonical UUID. No internal Typhon popularity is added: a validated
aggregate with a sufficient sample is not available.

Each meaningful game contributes `1 + min(3, playtime/2h) + 2 if favorite` to its
genres and themes. Multiple library releases of one canonical game contribute
once, using maximum counters rather than multiplying copied history. Profile
confidence is `min(1, max(meaningfulGames/3, meaningfulPlaytime/3h))`.

Automatic order initially shows “Popular”. As evidence appears, genre/theme
profile affinity gradually affects API ordering. At full confidence the label
becomes “For you”. The API combines normalized affinity times confidence (weight
0.65) and a bounded popular baseline (weight 0.35). An explicit manual sort wins
until the user selects Automatic again.

## Shelves

“Discover something new” selects up to five eligible games outside the library.
The first pass uses different primary genres; a second fills remaining slots.
Displayed games are excluded from the main catalog before pagination. Refresh
excludes the previous picks first and fills spare slots from previous eligible
picks only when insufficient alternatives are found. Candidate search is bounded
to three API pages (180 games), so a distant niche alternative may be missed.

Explanations use actual shared genres/themes, a named meaningful library game,
known rating evidence, or a game’s actual genre. No match percentage is invented.

“What to play” selects library games with an honest reason: never played,
favorite, similar interests, or return after 90 days with meaningful prior
activity/favorite/affinity. Installed games receive a small bonus. Archived,
dismissed and the visible Continue playing item are excluded.

The shelf score uses genre/theme evidence, confidence-adjusted quality and rating
count. Its purpose is choosing a small diverse set, rather than duplicating the
API sort exactly. Defaults and their bounds are in
[`internal/catalog/recommendations.go`](internal/catalog/recommendations.go).

## Configuration, persistence and failures

`TYPHON_RECOMMENDATION_CONFIG` accepts a JSON object at process startup. Defaults:

```json
{"minMeaningfulSeconds":1800,"meaningfulSessions":2,
 "personalizationMinGames":3,"personalizationMinPlaytimeSeconds":10800,
 "returnDays":90,"installedBoost":0.05,"favoriteBoost":0.15,
 "genreWeight":0.55,"themeWeight":0.15}
```

Invalid or out-of-range configuration uses the documented defaults. The API
exposes `CATALOG_RANKING_PRIOR_REVIEWS` and
`CATALOG_RANKING_AFFINITY_WEIGHT`; see the backend `CATALOG.md`.

Sort, filters and dismissals are atomically stored in `recommendation.json` in
the launcher data directory. They survive restart but are not account-synced.
Failed saves roll back; corrupt state is retained and personalization falls back
to general browsing instead of stopping catalog startup.

First-page personal signals and exclusions are pinned in a bounded in-memory
snapshot (32 sessions, 30 minutes idle lifetime). Continuations carry its token
and the API revision. If an update changes the already displayed order, or the snapshot expires, the
UI preserves its prefix and offers an explicit reload. Unrelated provider updates
can continue only after the API proves that every displayed ID keeps its position.

General browsing is retried if personalized ranking fails. Exact cached pages
remain available on network failure and are marked offline. Unavailable uncached
results show an error. Unknown rating data does not prevent browsing.

IGDB and Steam index synchronization defaults to six hours. Steam filter metadata
uses a seven-day success TTL and failure backoff. Detail/localization snapshots
use seven days for linked records and 24 hours for unlinked records. Failed
provider fetches preserve cached metadata. No provider request is made per tile
for a rating. A future Steam review import should use the official review API,
store source/count/timestamp explicitly and use the existing upstream budgets,
leases and stale-cache behavior; that import is not included here.

## Verification and present limits

Regression coverage includes small-sample rating correction, real-signal reasons,
single launches, duplicate releases, stale/corrupt persistence, dismissed/owned
exclusions, refresh, diversity, content filters and frozen pagination. The UI was
also exercised with the real Go/Wails server and local PostgreSQL-backed API,
using a separate synthetic profile and dataset: new profile, favorites appearing,
automatic personalization, dismissal/undo, filters, both languages, pagination
and process restart.

On a separate synthetic catalog of approximately 573,000 browse entries,
read-only EXPLAIN ANALYZE measured roughly 1.2 seconds for Popular and 2.6 seconds
for For you on this development machine. SQL currently scans/sorts the candidate
catalog; these are observations, not a production latency guarantee. Native game
execution and production provider ingestion require separate platform/live
checks. No production rollout is part of this change.
