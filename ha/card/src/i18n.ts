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
  late: "+{n} min",
  noDepartures: "No departures",
  unavailable: "Unavailable",
  notFound: "Entity not found: {id}",
  pickEntities: "Saved lines",
  departures: "Departures shown",
  showWalk: "Show walking time",
  showVerdict: "Show verdict",
  layout: "Layout",
  full: "Full",
  compact: "Compact",
  title: "Title (optional)",
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
  late: "+{n} min",
  noDepartures: "Väljumisi pole",
  unavailable: "Pole saadaval",
  notFound: "Olemit ei leitud: {id}",
  pickEntities: "Salvestatud liinid",
  departures: "Näidatavaid väljumisi",
  showWalk: "Näita kõnniaega",
  showVerdict: "Näita hinnangut",
  layout: "Paigutus",
  full: "Täielik",
  compact: "Kompaktne",
  title: "Pealkiri (valikuline)",
};

export type Key = keyof typeof en;

export function translator(lang: string | undefined): (key: Key, vars?: Record<string, string | number>) => string {
  const table = lang?.toLowerCase().startsWith("et") ? et : en;
  return (key, vars) => table[key].replace(/\{(\w+)\}/g, (_, k: string) => String(vars?.[k] ?? ""));
}
