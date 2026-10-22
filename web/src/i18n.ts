import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import et from "./locales/et.json";

export const LANGUAGES = [
  { code: "et", label: "Eesti" },
  { code: "en", label: "English" },
] as const;

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, et: { translation: et } },
    fallbackLng: "en",
    supportedLngs: ["en", "et"],
    nonExplicitSupportedLngs: true,
    interpolation: { escapeValue: false },
    detection: { order: ["localStorage", "navigator"], lookupLocalStorage: "tram.lang", caches: ["localStorage"] },
  });

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng;
});

export default i18n;
