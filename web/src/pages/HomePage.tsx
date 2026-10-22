import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { StaleBanner } from "@/components/StaleBanner";
import { Button } from "@/components/ui/button";

export default function HomePage() {
  const { t } = useTranslation();
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("home.title")}</h1>
        <p className="text-muted-foreground">{t("app.tagline")}</p>
      </header>
      <StaleBanner className="self-start" />
      <section className="flex flex-col items-start gap-4 rounded-xl border bg-card p-4">
        <p className="text-muted-foreground">{t("home.empty")}</p>
        <Button asChild>
          <Link to="/map">{t("home.openMap")}</Link>
        </Button>
      </section>
    </div>
  );
}
