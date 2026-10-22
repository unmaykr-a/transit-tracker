/** Card strings in English and Estonian, matching the web app's wording. */
const en = {
  min: "{n} min",
  now: "now",
  leaveIn: "Leave in {n} min",
  leaveNow: "Leave now",
  tooTight: "Too tight",
  walk: "{n} min walk",
  walkApprox: "~{n} min walk",
  scheduled: "scheduled",
  noDepartures: "No departures",
  feedError: "Departures cannot be fetched right now",
  unavailable: "Unavailable",
  notFound: "Entity not found: {id}",
  departures: "Departures shown",
  showWalk: "Show walking time",
  showVerdict: "Show verdict",
  layout: "Layout",
  full: "Full",
  compact: "Compact",
  title: "Title (optional)",
  from: "from {stop}",
  walkFrom: "from {place}",
  then: "then",
  allLines: "Saved lines (leave empty for all)",
  noLines: "No saved lines yet. Add lines in Bus, then reload the integration.",
};

const et: typeof en = {
  min: "{n} min",
  now: "kohe",
  leaveIn: "Mine {n} min pärast",
  leaveNow: "Mine kohe",
  tooTight: "Liiga napp",
  walk: "{n} min jalgsi",
  walkApprox: "~{n} min jalgsi",
  scheduled: "plaaniline",
  noDepartures: "Väljumisi pole",
  feedError: "Väljumisi ei saa praegu kätte",
  unavailable: "Pole saadaval",
  notFound: "Olemit ei leitud: {id}",
  departures: "Näidatavaid väljumisi",
  showWalk: "Näita kõnniaega",
  showVerdict: "Näita hinnangut",
  layout: "Paigutus",
  full: "Täielik",
  compact: "Kompaktne",
  title: "Pealkiri (valikuline)",
  from: "peatusest {stop}",
  walkFrom: "kohast {place}",
  then: "siis",
  allLines: "Salvestatud liinid (tühi: kõik)",
  noLines: "Salvestatud liine veel pole. Lisa liine Busis ja laadi integratsioon uuesti.",
};

export type Key = keyof typeof en;

export function translator(lang: string | undefined): (key: Key, vars?: Record<string, string | number>) => string {
  const table = lang?.toLowerCase().startsWith("et") ? et : en;
  return (key, vars) => table[key].replace(/\{(\w+)\}/g, (_, k: string) => String(vars?.[k] ?? ""));
}
