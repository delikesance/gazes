"use client";
import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { usePathname, useRouter } from "next/navigation";

/** The page one level up: episode → season → series → catalogue. */
export function parentPath(pathname: string): string | null {
  const episode = pathname.match(/^(\/anime\/\d+\/seasons\/\d+)\/episodes\/\d+$/);
  if (episode) return episode[1];
  const season = pathname.match(/^(\/anime\/\d+)\/seasons\/\d+$/);
  if (season) return season[1];
  return pathname === "/" ? null : "/";
}

const CHORDS: Record<string, string> = { c: "/", p: "/for-you", b: "/history", n: "/changelog" };

/** Desktop shortcuts outside text fields: "g" then c/p/b/n jumps to a page, "u" goes up one level ("/" for search lives in the header). */
export function KeyboardNav() {
  const router = useRouter();
  const pathname = usePathname();
  const { t } = useI18n();
  const [help, setHelp] = useState(false);
  useEffect(() => {
    let armedAt = 0;
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))) return;
      if (event.ctrlKey || event.metaKey || event.altKey) return;
      if (event.key === "Escape") { setHelp(false); return; }
      if (event.key === "?") { event.preventDefault(); setHelp((open) => !open); return; }
      const key = event.key.toLowerCase();
      if (key === "g") { armedAt = Date.now(); return; }
      if (Date.now() - armedAt < 900 && CHORDS[key]) { armedAt = 0; event.preventDefault(); router.push(CHORDS[key]); return; }
      if (key === "u") { const up = parentPath(pathname); if (up) { event.preventDefault(); router.push(up); } }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router, pathname]);
  if (!help) return null;
  const rows: [string, string][] = [["/", t("Rechercher")], ["g c", t("Catalogue")], ["g p", t("Pour vous")], ["g b", t("Bibliothèque")], ["g n", t("Nouveautés")], ["u", t("Remonter d’un niveau")], ["?", t("Afficher cette aide")]];
  return <div className="shortcuts-backdrop" onClick={() => setHelp(false)}>
    <div className="shortcuts-card" role="dialog" aria-modal="true" aria-label={t("Raccourcis clavier")} onClick={(event) => event.stopPropagation()}>
      <h2 className="serif">{t("Raccourcis clavier")}</h2>
      <dl>{rows.map(([keys, label]) => <div key={keys}><dt>{keys.split(" ").map((key) => <kbd key={key}>{key}</kbd>)}</dt><dd>{label}</dd></div>)}</dl>
    </div>
  </div>;
}
