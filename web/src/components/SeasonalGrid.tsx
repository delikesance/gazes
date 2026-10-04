"use client";
import { useI18n } from "@/lib/i18n";

import { useEffect, useRef, type ReactNode } from "react";

// Pick a divisor so every row contains the same number of cards.
export function SeasonalGrid({ count, children, evenRows = true }: { count: number; children: ReactNode; evenRows?: boolean }) {
  const { t } = useI18n();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const grid = ref.current;
    if (!grid) return;
    const update = () => {
      const capacity = Number(getComputedStyle(grid).getPropertyValue("--grid-capacity")) || 2;
      let columns = Math.min(capacity, Math.max(1, count));
      // A feed of any length (suggestions) keeps the full width; only fixed-size lists need even rows.
      while (evenRows && columns > 1 && count % columns !== 0) columns--;
      grid.style.setProperty("--season-columns", String(columns));
      grid.dataset.columns = String(columns);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(grid);
    return () => observer.disconnect();
  }, [count, evenRows]);
  return <div ref={ref} className="poster-grid seasonal-grid page-inset" aria-label={t("Animes à découvrir")}>{children}</div>;
}
