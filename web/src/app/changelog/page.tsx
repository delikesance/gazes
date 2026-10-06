"use client";
import { useEffect, useMemo } from "react";
import { useI18n } from "@/lib/i18n";
import { CHANGELOG } from "@/lib/changelog";
import { markChangelogSeen } from "@/components/ChangelogBanner";

export default function ChangelogPage() {
  const { t, locale } = useI18n();
  const lang = locale === "en" ? "en" : "fr";
  const date = useMemo(() => new Intl.DateTimeFormat(lang, { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" }), [lang]);
  useEffect(markChangelogSeen, []);
  return (
    <main className="history-page">
      <div className="history-inner page-inset legal-page changelog-page">
        <span className="eyebrow">{t("Journal de développement")}</span>
        <h1 className="serif">{t("Nouveautés")}</h1>
        {CHANGELOG.map((entry) => (
          <section key={entry.date} aria-labelledby={`changelog-${entry.date}`}>
            <time dateTime={entry.date} className="eyebrow changelog-date">{date.format(new Date(`${entry.date}T00:00:00Z`))}</time>
            <h2 id={`changelog-${entry.date}`}>{entry[lang].title}</h2>
            <ul>{entry[lang].items.map((item) => <li key={item}>{item}</li>)}</ul>
          </section>
        ))}
      </div>
    </main>
  );
}
