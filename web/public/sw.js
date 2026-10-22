// Service worker: makes the app installable and keeps the shell usable offline.
// - Hashed build assets: cache-first (they never change).
// - Page navigations: network-first, falling back to the cached shell.
// - /api/v1/stops (all platforms, changes nightly): stale-while-revalidate.
// - Everything else (live data, auth): network only.
const VERSION = "tram-v1";
const SHELL = ["/", "/manifest.webmanifest", "/icon.svg", "/icon-192.png"];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(VERSION).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

async function cacheFirst(req) {
  const hit = await caches.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) (await caches.open(VERSION)).put(req, res.clone());
  return res;
}

async function networkFirstShell(req) {
  try {
    const res = await fetch(req);
    if (res.ok) (await caches.open(VERSION)).put("/", res.clone());
    return res;
  } catch {
    return (await caches.match("/")) ?? Response.error();
  }
}

async function staleWhileRevalidate(req) {
  const cache = await caches.open(VERSION);
  const hit = await cache.match(req);
  const refresh = fetch(req)
    .then((res) => {
      if (res.ok) cache.put(req, res.clone());
      return res;
    })
    .catch(() => hit);
  return hit ?? refresh;
}

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/assets/")) event.respondWith(cacheFirst(req));
  else if (req.mode === "navigate") event.respondWith(networkFirstShell(req));
  else if (url.pathname === "/api/v1/stops") event.respondWith(staleWhileRevalidate(req));
});
