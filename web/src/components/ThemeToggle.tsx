"use client";
import { useI18n } from "@/lib/i18n";

import { Moon, Sun } from "lucide-react";

export function ThemeToggle() {
  const { t } = useI18n();
  function toggleTheme() {
    const root = document.documentElement;
    const light = !root.classList.contains("light");
    root.classList.toggle("light", light);
    root.classList.toggle("dark", !light);
    try { localStorage.setItem("gazes-theme", light ? "light" : "dark"); } catch { /* The theme still works when storage is unavailable. */ }
  }
  return <button type="button" className="theme-toggle" onClick={toggleTheme} aria-label={t("Changer de thème clair/sombre")} title={t("Changer de thème clair/sombre")}>
    <Sun className="theme-sun" size={17} aria-hidden="true" />
    <Moon className="theme-moon" size={17} aria-hidden="true" />
  </button>;
}
