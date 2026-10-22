# Bus for Home Assistant

Two pieces:

- **[`custom_components/bus`](../custom_components/bus)** (at the repository root, where HACS expects it) — an
  integration that turns your saved lines into sensors.
- **[`card/`](card)** — `bus-card`, a Lovelace card that shows them like the web app does.

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
- **Walk from** — a zone (`zone.home`), person or device tracker. Its position is sent with each request
  (and never stored by the server), which enables walking time, the verdict and *leave in*.

### Entities

One device per saved line, e.g. a line labelled *Work*:

| Entity | State | Notes |
| --- | --- | --- |
| `sensor.bus_work_next` | timestamp of the next departure | the next one you can still catch when *Walk from* is set |
| `sensor.bus_work_leave_in` | minutes until you must leave | needs *Walk from*; unknown otherwise |
| `binary_sensor.bus_work_catchable` | on while a listed departure can be caught | needs *Walk from* |

`sensor.bus_*_next` attributes: `line`, `mode`, `label`, `headsign`, `stop_name`, `platform`, `minutes`,
`delay_s`, `realtime`, `next_departures` (up to 5, each with `time`, `minutes`, `delay_s`, `realtime`,
`headsign`, `verdict`, `leave_in_min`), `walk_min`, `walk_approx`, `verdict`, `leave_in_min`, `catchable`,
`url`.

If the token is revoked, Home Assistant asks for a new one (re-authentication) instead of failing silently.

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

## 3. Install the card

**Manually:** download `bus-card.js` from the latest release into `config/www/`, then
**Settings → Dashboards → ⋮ → Resources → Add resource**: URL `/local/bus-card.js`, type *JavaScript module*.
(HACS installs dashboard cards from separate repositories only, so the card is not offered there yet.)

Add the card from the card picker (**Bus**) and choose lines in the visual editor, or in YAML:

```yaml
type: custom:bus-card
entities:
  - sensor.bus_work_next
  - sensor.bus_gym_next
departures: 3 # 1–5
show_walk: true
show_verdict: true
layout: full # or compact
title: Departures # optional
```

The card uses your Home Assistant theme, counts down between updates, skips departures you can no longer
catch, and opens the line in the Bus web app when tapped.

## Develop

```sh
# integration (Python 3.13; run from ha/, tests import ../custom_components)
python -m venv .venv && .venv/bin/pip install -r requirements_test.txt
.venv/bin/python -m pytest
.venv/bin/ruff check --config pyproject.toml . ../custom_components

# card
cd card && npm ci && npm test && npm run build   # → card/dist/bus-card.js
```
