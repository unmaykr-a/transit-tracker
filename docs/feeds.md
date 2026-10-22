# Upstream feeds — confirmed formats

Verified live on 2026-10-22 between 09:20 and 09:25 Europe/Tallinn (Thursday morning peak).
All feeds answered without keys. Real samples live in `internal/feeds/testdata/`.
Requests used `User-Agent: Bus/0.1 (+https://github.com/unmaykr-a/transit-tracker)`.

Status legend: **OK** = matches PLAN.md, **DIFF** = differs from PLAN.md (see "Differences from the plan").

| Source | Status | Fixture |
| --- | --- | --- |
| Tallinn `gps.txt` | DIFF (minor) | `tallinn_gps.txt` |
| Tallinn `siri-stop-departures.php` | DIFF (minor) | `tallinn_siri_1300.txt`, `tallinn_siri_empty.txt` |
| Tallinn `data/stops.txt` | DIFF | `tallinn_stops.txt` |
| Tallinn `data/routes.txt` | DIFF | `tallinn_routes.txt` |
| Elron `map_data.json` | DIFF (URL) | `elron_map_data.json` |
| Elron `stops_data.json` | DIFF (URL) | `elron_stops_data.json` |
| Elron GTFS `eu-gtfs.remix.com/elron.zip` | OK, with surprises | `elron_gtfs/` (trimmed: R12 + R14, 8 trips) |
| peatus.ee OTP GraphQL | OK, with surprises | `otp_stops_*.json`, `otp_plan_walk.json` |
| Transitous (MOTIS) | OK | `transitous_geocode.json`, `transitous_plan_walk.json` |
| Ridango WebSocket | not probed (stretch goal) | — |

---

## Tallinn live GPS — `https://transport.tallinn.ee/gps.txt`

- `200 text/plain`, ~23 KB, 481 vehicles at peak (402 bus, 46 tram, 33 trolley).
- Sends `Last-Modified` and a weak `ETag`; served through Cloudflare (`cf-cache-status: HIT`). Conditional GET works.
- No header row. Comma-separated, always 10 fields:

| # | Field | Example | Notes |
| --- | --- | --- | --- |
| 1 | type | `3` | 1 = trolleybus, 2 = bus, 3 = tram |
| 2 | line | `T6`, `17A` | **Trams are prefixed `T`** (`T1`…`T6`). Buses/trolleys are plain |
| 3 | lon × 1e6 | `24762450` | integer |
| 4 | lat × 1e6 | `59433480` | integer |
| 5 | speed | *(empty)* | always empty |
| 6 | heading | `137` | degrees |
| 7 | vehicle id | `98` | unique within type, not globally — key on `type+id` |
| 8 | low floor | `Z` / `false` | `Z` = low floor |
| 9 | unknown | `2`…`240` | integer, meaning unknown (not speed); ignored |
| 10 | destination | `Suur-Paala` | UTF-8 |

## Tallinn SIRI departures — `https://transport.tallinn.ee/siri-stop-departures.php?stopid=<SiriID>`

- `200 text/plain; charset=UTF-8`. `stopid` is the `SiriID` column of `stops.txt` (not `ID`).
- Line 1 is a header carrying the **server time as seconds since local midnight** in field 5:
  `Transport,RouteNum,ExpectedTimeInSeconds,ScheduleTimeInSeconds,33788,version20201024`
- Line 2 is `stop,<SiriID>`; the stop line is omitted when there are no departures.
- Then one line per departure, 7 fields:
  `bus,55,34573,34573,P. Pinna,785,Z`
  = `type(name),line,expectedSecOfDay,scheduledSecOfDay,destination,secondsUntil,lowFloor`.
  `type` is a word here (`bus`, `tram`, `trol`), unlike the number in `gps.txt`.
- Many stops return only the header even at peak (e.g. `Hobujaama` 1298, temporary `ajutine` stops).
  The UI must treat "header only" as "no live data", not "no service", and fall back to OTP schedule.

## Tallinn static data — `https://transport.tallinn.ee/data/stops.txt`, `routes.txt`

Not plain CSV. Both are the **stops.lt / "Pikas" format** used by the site's own `ti.js`:

- Semicolon-separated, UTF-8 with BOM, first row is a header (last header cell is a version/date stamp).
- **Field inheritance**: an empty field means "same as the previous row" (e.g. `Name`, `Street`, `Transport`).
- Coordinates are integers × 1e5 (`5943750;2475700` → 59.43750, 24.75700).

`stops.txt` (3 747 rows): `ID;SiriID;Lat;Lng;Stops;Name;Info;Street;Area;City`

- Covers **all of Harju county**, not just the city. Rows starting with `a…` are stop areas (no SiriID).
- `ID` (e.g. `21105-1`) is the national stop code — **identical to OTP `code` and Elron GTFS `stop_code`**.
  This is the join key between sources; name+30 m matching is only a fallback.
- `Stops` lists neighbouring stop IDs (transfer graph), not platforms.
- Platform letters (A, B…) are **not** in this file.

`routes.txt` (427 rows): `RouteNum;Authority;City;Transport;Operator;ValidityPeriods;SpecialDates;RouteTag;RouteType;Commercial;RouteName;Weekdays;Streets;RouteStops;RouteStopsPlatforms`

- Each route variant row (`RouteType` `a-b`, `b-a`, …) has an ordered `RouteStops` list of stop IDs.
- The **row after each variant is a delta-encoded timetable** (the `+1070,+155,-0195,…` lines). Decoding it
  means porting the `ti.js` algorithm; we take schedules from OTP instead (see below) and only use
  `routes.txt` for ordered stop lists and route names.
- `Transport`: `bus`, `tram`, `trol` (inherited). Tram route numbers are `T1`…`T6`, matching `gps.txt`.
- No shapes. Shapes come from OTP pattern geometry (city) and Elron GTFS `shapes.txt` (rail).

## Elron live trains — `https://elron.ee/map_data.json`

- **`www.elron.ee` 301-redirects to `elron.ee`**; use `https://elron.ee/…` directly.
- Sends `Cache-Control: max-age=1209600` (14 days) — wrong for a live feed; ignore it and poll.
- `{"status":1,"data":[…]}`, 17 trains at 09:21. All values are **strings**, Estonian keys:

| Key | Meaning | Example |
| --- | --- | --- |
| `reis` | trip number | `"101"` |
| `liin` | route name (not a line code like `R12`) | `"Tartu-Tallinn"` |
| `reisi_algus_aeg`, `reisi_lopp_aeg` | scheduled start / end, `HH:MM` | `"06:52"` |
| `kiirus` | speed km/h | `"118"` |
| `latitude`, `longitude` | decimal degrees | `"59.393153"` |
| `rongi_suund` | heading degrees | `"293"` |
| `erinevus_plaanist` | delay in minutes, may be `""` | `"1"` |
| `lisateade`, `pohjus_teade` | free-text notice / reason | `"liiklushäire tõttu"` |
| `avalda_kodulehel` | publish-notice flag `"1"`/`""` | |
| `asukoha_uuendus` | position timestamp, local time | `"2026-10-22 09:21:01"` |
| `reisi_staatus` | trip status | `"plaaniline"` |
| `viimane_peatus` | last stop passed | `"Tapa"` |

Mapping `reis` → GTFS trip: `trips.txt` `trip_short_name` is a name (`Tallinn -Turba`), not the number;
match on route + scheduled start time instead.

## Elron stations — `https://elron.ee/stops_data.json`

- Same redirect as above. `{"status":1,"data":[{"peatus","latitude","longitude","teade"}]}`, **141 stations**
  (includes Latvian stations such as Cesis). No ids — name is the key.

## Elron GTFS — `https://eu-gtfs.remix.com/elron.zip`

- 1.3 MB zip, 12 files, generated 2026-10-01. Standard GTFS plus `calendar_attributes`, `directions`,
  `route_attributes`.
- 28 routes, all `route_type=2`, short names `R11`…`E35`, colour `ff711d`.
- **`stops.txt` is the national stop register (18 054 stops)**, not just 141 stations. It carries
  `stop_code` (= Tallinn `ID`), `alias` (e.g. `Tallinn; Balti Jaam`), `stop_area`, `authority`.
- The rail station at Balti jaam is named **`Tallinn`** (`10138`, code `21107-1`), with `Balti Jaam` only in `alias`.

## peatus.ee OTP — `POST https://api.peatus.ee/routing/v1/routers/estonia/index/graphql`

- Works keyless, `Content-Type: application/json`, OTP1 GraphQL schema (`stops(name:)`, `stop(id:)`,
  `stoptimesWithoutPatterns`, `plan`).
- Stop ids are `estonia:<gtfs stop_id>`; `code` = Tallinn `ID`, and for city stops the **numeric gtfs id equals the
  Tallinn `SiriID`** (`estonia:1298` = Hobujaama, SiriID 1298). Three-way join with no fuzzy matching.
- `platformCode` is `null` for city stops.
- `stops(name:"Balti jaam")` returns tram/bus platforms only; rail platforms are under `stops(name:"Tallinn")`
  (`estonia:10137` code `21106-1`, `estonia:10138` code `21107-1`). Search must also match GTFS `alias`.
- Departures returned for trams, buses and trains, but **`realtime: false` everywhere** at the time of testing,
  including trains. Train delays therefore come from Elron `map_data.json`, not OTP.
- `plan(... transportModes:[{mode:WALK}])` returns duration, distance and an encoded polyline
  (1.2 km walk → 1 042 s). Good as the primary walking provider.

## Transitous — `https://api.transitous.org`

- `GET /api/v1/geocode?text=…` → JSON array of matches (stops carry ids like `ee-tallinn_1234`).
- `GET /api/v5/plan?fromPlace=lat,lon&toPlace=lat,lon&directModes=WALK&…` → JSON with `direct` itineraries.
  One request was reset by peer, the retry succeeded — treat as best-effort fallback only.

## Differences from the plan

1. Elron JSON URLs moved to `https://elron.ee/…` (301 from `www`).
2. `stops.txt` / `routes.txt` are semicolon-separated with field inheritance and ×1e5 coordinates, cover Harju
   county, and `routes.txt` embeds encoded timetables. No platform letters anywhere in the Tallinn files.
3. `gps.txt` tram lines carry a `T` prefix; field 9 is an unknown integer; field 8 is `Z`/`false`.
4. SIRI output has a header row (with server time) and a `stop,<id>` row before departures; many stops return no rows.
5. OTP has no realtime at the moment; rail stops are named `Tallinn`, not `Balti jaam`.
6. Elron GTFS `stops.txt` is the national register — useful: one source of stop codes for both city and rail.
7. No source has platform letters (A, B…). They must be derived (e.g. ordered by stop code suffix/bearing, or
   labelled by the destinations each platform serves).
8. Elron live trains identify routes by name (`liin`) and trip number (`reis`), not GTFS ids.

## Notes from M2 (static data)

- **City routes come from OTP, not `routes.txt`.** `agency(id:"estonia:tallinn_10312960") { routes { patterns … } }`
  returns all 80 city routes with 197 patterns (stop codes, headsigns, encoded geometry) in one ~1.3 s request.
  Route ids encode the real mode (`tallinna-lin_trol_72`), which OTP's `mode` field does not (trolleybuses are `BUS`).
  `routes.txt` is therefore not parsed; its encoded timetables are not needed.
- OTP stop names carry English annotations ("Balti jaam (train station)", "Bussijaam (bus station)"); so does the
  operator's own `stops.txt` for a few stops. The importer strips them.
- SIRI reports tram lines as `T1`…`T6`, like `gps.txt`.
- SIRI and OTP both list vehicles terminating at a stop; the API drops rows whose headsign is the stop itself.
- GTFS `alias` is loose: it sometimes holds the district ("Suur-Paala" has alias "Ülemiste"). It is only used to
  join a rail station to a nearby city stop area (Tallinn ↔ Balti jaam, Ülemiste ↔ Ülemiste jaam) and as a
  low-ranked search term.
- Rail departures come from OTP stoptimes and join to Elron GTFS by route id; `headsign` is sometimes null, in which
  case the terminus of the route's long name is used.

## Notes from M5 (ETAs)

- `gps.txt` has no per-vehicle timestamps; a fix is dated by the file's `Last-Modified` (the file changes about
  every 10 s). SIRI's header clock matched ours to the second on 2026-10-22, so there is no clock offset to correct.
- `bus eta-check -record` captured 25 minutes of the Thursday-morning peak (30 busiest city platforms, one SIRI
  sample per minute; ~3 700 SIRI live times paired with our predictions). `-replay` re-runs the model on it and
  also grades both our predictions and SIRI's against **arrivals observed in the GPS feed** (the moment the
  predicted vehicle reaches the stop), which is closer to ground truth than SIRI.
- Result with the shipped model (shared per-stop-pair paces, per-stop dwell, layover excluded), scored after 12 min
  of learning:

  | | within ±1 min | median \|err\| | bias |
  | --- | --- | --- | --- |
  | ours vs SIRI, all leads (2–20 min) | 59 % | 48 s | +48 s |
  | ours vs SIRI, lead ≤ 5 min | 92 % | 22 s | +16 s |
  | ours vs observed arrivals | 79 % | 35 s | +13 s |
  | SIRI vs observed arrivals | 89 % | 28 s | −6 s |

  **The PLAN.md M5 target (80 % within ±1 min of SIRI over all leads) is not met.** Short-range predictions are
  on par with SIRI; beyond 5 minutes SIRI is better, presumably because it knows the timetable. Tuning the pace
  decay, dwell radius and minimum observed distance moved the score by under 3 points; the step changes came from
  model structure (excluding terminus layover: route speed had collapsed to 0; separating dwell from travel;
  sharing paces across lines on the same street).
- **Self-calibration (added after 1.0.0).** A second, 75-minute afternoon recording showed the remaining error was a
  bias that grows with the horizon: the model was ~5 % slow. The tracker now checks its own predictions against the
  arrivals it later sees in the GPS feed (one probe per vehicle, minute and horizon band; only once every segment and
  stop on the way has measured data) and learns a log-scale factor per mode and band (< 5, 5–10, ≥ 10 min), applied at
  half strength. Results, scored after warm-up (`eta-check -replay`):

  | recording | metric | before | after |
  | --- | --- | --- | --- |
  | morning (25 min) | within ±1 min of SIRI | 58.6 % | 66.1 % |
  | morning | within ±1 min of observed arrivals | 78.9 % | 81.5 % (SIRI 88.9 %) |
  | afternoon (75 min, ~12 500 predictions) | within ±1 min of SIRI | 59.4 % | 63.9 % |
  | afternoon | within ±1 min of observed arrivals | 65.7 % | 67.7 % (SIRI 66.9 %) |

  (The half-strength choice was made on the first 40 minutes of the afternoon recording; the table shows all 75.)
  On the larger afternoon sample our predictions match real arrivals slightly more often than SIRI's do; the gap to the 80 %
  "vs SIRI" target is mostly the two predictors disagreeing beyond 10 minutes. The learned factors are exported as
  `bus_eta_scale{mode,band}`.
- Departure boards and My lines use SIRI times, not these ETAs. Tracker ETAs drive the vehicle sheet's
  "next stops" list and the map animation only.

## Notes on Harju county buses

- OTP lists the county operators as agencies `estonia:harjumaa_<registry code>` (2026-10-22: Hansabuss 125 routes,
  SEBE 53, GoBus 43, Ekspressbussiliinid 1, Alukvik 1). All are mode `BUS`; short names (100–277, `H1`…, `J1`…,
  `Anija1`…) do not clash with city lines.
- No live data: `gps.txt` and SIRI cover Tallinn city vehicles only (types 1–3), and OTP marks county stoptimes
  `SCHEDULED` (`realtime: false`). The Ridango system the operators use has no public feed we know of.
- County terminals in Tallinn use numbered platform names (`Balti jaam 1`, `Balti jaam 6`), so they form their own stop
  areas next to the city's `Balti jaam`.
- Importing them grows the static data from ~1 360 to ~3 500 stops and ~270 to ~880 patterns (import ~5 s); idle
  memory with county routes is ~56 MB RSS.
