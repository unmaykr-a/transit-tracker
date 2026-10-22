import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import et from "./locales/et.json";

export const LANGUAGES = [
  { code: "et", label: "Eesti" },
  { code: "en", label: "English" },
] as const;

const LANG_KEY = "bus.lang";
// The detector reads the key as a bare string, so it is not stored through lib/storage (JSON).
function chosenLanguage(): string | null {
  try {
    return globalThis.localStorage?.getItem(LANG_KEY) ?? null;
  } catch {
    return null;
  }
}

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, et: { translation: et } },
    fallbackLng: "en",
    supportedLngs: ["en", "et"],
    nonExplicitSupportedLngs: true,
    interpolation: { escapeValue: false },
    detection: { order: ["localStorage", "navigator"], lookupLocalStorage: LANG_KEY, caches: [] },
  });

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng;
});

/** A language the person picked: kept across visits. */
export function setLanguage(lng: string) {
  try {
    globalThis.localStorage?.setItem(LANG_KEY, lng);
  } catch {
    // Just won't persist.
  }
  void i18n.changeLanguage(lng);
}

/** The instance default, for visitors who never picked a language. */
export function applyDefaultLanguage(lng: string) {
  if (!chosenLanguage() && i18n.resolvedLanguage !== lng) void i18n.changeLanguage(lng);
}

export default i18n;
