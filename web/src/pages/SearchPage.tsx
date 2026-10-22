import { useTranslation } from "react-i18next";

export default function SearchPage() {
  const { t } = useTranslation();
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4 px-4 py-8">
      <h1 className="text-2xl font-semibold tracking-tight">{t("search.title")}</h1>
      <p className="text-muted-foreground">{t("search.soon")}</p>
    </div>
  );
}
