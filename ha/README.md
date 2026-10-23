# Bus for Home Assistant

Two pieces:

- **[`custom_components/bus`](../custom_components/bus)** (at the repository root, where HACS expects it) — an
  integration that turns your saved lines into sensors.
- **[`card/`](card)** — `bus-card`, a Lovelace card that shows them like the web app does (bundled with the
  integration, nothing to install separately).

## 1. Create an API token

In the Bus web app open **Settings → API tokens**, name the token (e.g. "Home Assistant") and copy it. It is
shown once. Tokens are read-only: they can read your lines and departures but cannot change anything, and
they never have admin rights. Revoke a token there (or as an admin under **Administration → API tokens**).

## 2. Install the integration

Requires Home Assistant 2025.2 or newer.

**HACS:** **HACS → ⋮ → Custom repositories**, add `https://github.com/unmaykr-a/transit-tracker` with type
*Integration*, then install **Bus (Tallinn transit)** and restart Home Assistant. **Manually:** download `bus.zip` from the latest release and unpack it into
`config/custom_components/bus/`, then restart.

Then **Settings → Devices & services → Add integration → Bus** and enter:

| Field | Example |
| --- | --- |
| Server URL | `https://bus.example.com` |
| API token | `bus_…` |
| Verify SSL certificate | on (turn off only for a self-signed certificate on your LAN) |

### Options

Open the integration's **Configure** dialog:

- **Saved lines** — which lines become devices. With everything selected, the integration follows your saved
  lines (new ones appear after the next reload).
- **Update interval** — 30 s by default, 15 s minimum. All lines come from one request per interval.
- **Walk from** — where walking time is measured from: `zone.home` by default, or a person or device tracker to
  follow you around. Its position is sent with each request (and never stored by the server), which enables walking
  time, the verdict and *leave in*.

### Walking time and "leave in" (location)

*Leave in*, *catchable* and the verdict need a starting point. In order, the integration uses:

1. The **Walk from** entity's GPS position (a person or tracker fed by the Companion app, or a zone).
2. If that entity has no coordinates — a person tracked only by the router, say — the **zone it is in**
   (`home` → `zone.home`, `Work` → your Work zone). Away from every zone there is no position.
3. Otherwise, each line's own **usual starting point**, set in the Bus web app (edit a trip → *Usual starting
   point*, from the places under **Settings → Walking**).

If walking time stays empty, check under **Developer tools → States** that the Walk-from entity has
`latitude`/`longitude` attributes, or pick `zone.home`.

### Entities

One device per saved line, e.g. a line labelled *Work*:

| Entity | State | Notes |
| --- | --- | --- |
| `sensor.bus_work_next` | timestamp of the next departure | the next one you can still catch when *Walk from* is set |
| `sensor.bus_work_leave_in` | minutes until you must leave (a duration) | needs *Walk from*; unknown otherwise and when nothing is catchable |
| `binary_sensor.bus_work_catchable` | on while a listed departure can be caught | needs *Walk from* |

`sensor.bus_*_next` attributes: `line`, `mode`, `label`, `kind` (`line` or `trip`), `lines` (a trip's lines),
`headsign`, `destination`, `stop_name`, `platform`, `minutes`, `delay_s`, `realtime`, `next_departures` (up to 5,
each with `time`, `minutes`, `delay_s`, `realtime`, `headsign`, `line`, `mode`, `verdict`, `leave_in_min`),
`walk_min`, `walk_approx`, `walk_from` (the line's own starting place, when used), `verdict`, `leave_in_min`,
`catchable`, `order`, `url`, `error`, and for trips with changes `via` (the stops it changes at), `changes` (the
ride caught at each change: `at`, `line`, `mode`, `time`, `wait_min`, `walk_min`) and `arrive_at` (estimated arrival;
also set for direct trips and lines saved with a stop to get off at).

If the token is revoked, Home Assistant asks for a new one (re-authentication) instead of failing silently. To move
to another server URL or token yourself, use the integration's **Reconfigure** menu entry. When the server cannot be
reached for a line, the `error` attribute says why.

### Automation: "time to leave"

```yaml
alias: Leave for the tram
triggers:
  - trigger: numeric_state
    entity_id: sensor.bus_work_leave_in
    below: 2.5
    above: 1.5 # fires once when it reaches 2
conditions:
  - condition: state
    entity_id: person.me
    state: home
actions:
  - action: notify.mobile_app_my_phone
    data:
      title: "{{ state_attr('sensor.bus_work_next', 'line') }} → {{ state_attr('sensor.bus_work_next', 'headsign') }}"
      message: >-
        Leave in 2 min · departs {{ state_attr('sensor.bus_work_next', 'minutes') }} min from
        {{ state_attr('sensor.bus_work_next', 'stop_name') }}
```

## 3. The card

The integration ships the `bus-card` and loads it on every dashboard: there is nothing to download and no resource
to add. Edit a dashboard → **Add card** → search **Bus**. With no lines picked it shows all of your saved lines, in
your Bus order, laid out like the web app: the line and where you board, a big countdown with the clock time, the
verdict chip and the next departures. Trips show their lines and the line of each departure.

(If you added `bus-card.js` as a resource by hand earlier, remove that resource under **Settings → Dashboards → ⋮ →
Resources**.)

Options in the visual editor, or in YAML:

```yaml
type: custom:bus-card
# entities: [sensor.bus_work_next, sensor.bus_gym_next]   # leave out for every saved line
departures: 3 # 1–5
show_walk: true
show_verdict: true
layout: full # or compact (one row per line)
title: Departures # optional
```

The card uses your Home Assistant theme, counts down between updates, skips departures you can no longer catch,
and opens the line in the Bus web app when tapped.

## 4. On the phone's home screen

Home screen widgets in the Companion apps cannot show dashboard cards, only entity states and text templates.
Two good options:

- **The Bus web app itself** — open your Bus URL in the phone's browser → *Add to Home Screen*. It installs as an
  app (PWA) with the full My lines view, live countdowns and the map.
- **A Companion app widget** with this template. On **Android**: long-press the home screen → Widgets → Home
  Assistant → *Template*. On **iOS**: add the Home Assistant widget → *Details* (three lines; use one line per row).

  ```jinja
  {%- for s in integration_entities('bus') | select('match', 'sensor\..*_next$') %}
  {%- set m = ((as_timestamp(states(s), 0) - now().timestamp()) / 60) | round(0) | int %}
  {%- set leave = state_attr(s, 'leave_in_min') %}
  <b>{{ state_attr(s, 'line') }}</b> {{ state_attr(s, 'label') }}: <big>{{ m ~ ' min' if m > 0 else 'now' }}</big>
  {%- if leave is number %} · leave {{ 'now' if leave <= 0 else 'in ' ~ leave ~ ' min' }}{% endif %}<br>
  {%- endfor %}
  ```

  On Android you can also open any `sensor.bus_*_next` entity → ⋮ → *Add to* → *Widget* for a one-line widget.
  Widgets refresh when the sensors change; allow the Companion app's notification access for real-time updates.

## Develop

```sh
# integration (Python 3.13; run from ha/, tests import ../custom_components)
python -m venv .venv && .venv/bin/pip install -r requirements_test.txt
.venv/bin/python -m pytest
.venv/bin/ruff check --config pyproject.toml . ../custom_components

# card
cd card && npm ci && npm test && npm run build   # → card/dist/bus-card.js
```
