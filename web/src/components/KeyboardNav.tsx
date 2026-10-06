"use client";
import { useEffect } from "react";
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
  useEffect(() => {
    let armedAt = 0;
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))) return;
      if (event.ctrlKey || event.metaKey || event.altKey) return;
      const key = event.key.toLowerCase();
      if (key === "g") { armedAt = Date.now(); return; }
      if (Date.now() - armedAt < 900 && CHORDS[key]) { armedAt = 0; event.preventDefault(); router.push(CHORDS[key]); return; }
      if (key === "u") { const up = parentPath(pathname); if (up) { event.preventDefault(); router.push(up); } }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router, pathname]);
  return null;
}
