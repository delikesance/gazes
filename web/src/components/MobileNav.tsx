"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Clock, Compass, Search, Sparkles } from "lucide-react";
import { useI18n } from "@/lib/i18n";

/** Thumb-reach navigation, phones only (hidden from 768px by CSS) and never over the player. */
export function MobileNav() {
  const { t } = useI18n();
  const pathname = usePathname();
  const [hidden, setHidden] = useState(false);

  // Scrolling down gives the screen back to the content; any scroll up (or the top of the page) brings the bar back.
  useEffect(() => {
    let last = window.scrollY;
    const onScroll = () => {
      const y = window.scrollY;
      if (Math.abs(y - last) < 8) return;
      setHidden(y > last && y > 120);
      last = y;
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  if (pathname.includes("/episodes/")) return null;
  const items = [
    { href: "/", label: t("Catalogue"), icon: Compass, active: pathname === "/" },
    { href: "/for-you", label: t("Pour vous"), icon: Sparkles, active: pathname === "/for-you" },
    { href: "/history", label: t("Historique"), icon: Clock, active: pathname === "/history" },
  ];
  // Tapping the tab you are already on scrolls back to the top.
  const onTap = (active: boolean) => (event: React.MouseEvent) => {
    if (!active) return;
    event.preventDefault();
    window.scrollTo({ top: 0, behavior: "smooth" });
  };
  return <nav className="mobile-nav" data-hidden={hidden} aria-label={t("Navigation principale")}>
    <Link href={items[0].href} aria-current={items[0].active ? "page" : undefined} onClick={onTap(items[0].active)}><Compass size={20} aria-hidden="true" /><span>{items[0].label}</span></Link>
    <button type="button" onClick={() => { setHidden(false); window.dispatchEvent(new Event("gazes-open-search")); }}><Search size={20} aria-hidden="true" /><span>{t("Rechercher")}</span></button>
    {items.slice(1).map(({ href, label, icon: Icon, active }) => <Link key={href} href={href} aria-current={active ? "page" : undefined} onClick={onTap(active)}><Icon size={20} aria-hidden="true" /><span>{label}</span></Link>)}
  </nav>;
}
