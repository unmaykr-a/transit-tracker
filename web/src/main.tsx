import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./i18n";
import "./index.css";
import { initTheme } from "./lib/theme";
import { queryClient } from "./lib/queryClient";
import { router } from "./router";

initTheme();

// Installable PWA; skipped in dev so Vite's module reloading is not cached.
if (import.meta.env.PROD && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js").catch(() => {});
  });
}

// After a deploy, a tab opened earlier asks for page chunks that no longer
// exist: reload once to pick up the new build instead of showing an error.
window.addEventListener("vite:preloadError", (e) => {
  try {
    if (sessionStorage.getItem("bus-reloaded") === location.pathname) return;
    sessionStorage.setItem("bus-reloaded", location.pathname);
  } catch {
    return; // storage blocked: without the guard a reload could loop
  }
  e.preventDefault();
  location.reload();
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
