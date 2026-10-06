"use client";
import { useEffect, useId, useRef, type ReactNode } from "react";

const FOCUSABLE = "a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])";

/** Modal window: dimmed backdrop, Escape and backdrop click close it, Tab stays inside, focus returns to the opener. */
export function Dialog({ title, onClose, role = "dialog", children }: { title: string; onClose: () => void; role?: "dialog" | "alertdialog"; children: ReactNode }) {
  const card = useRef<HTMLDivElement>(null);
  const titleId = useId();

  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    const node = card.current;
    (node?.querySelector<HTMLElement>("[data-autofocus]") ?? node?.querySelector<HTMLElement>(FOCUSABLE))?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.stopPropagation(); onClose(); return; }
      if (event.key !== "Tab" || !node) return;
      const items = Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (items.length === 0) return;
      const first = items[0], last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    window.addEventListener("keydown", onKey, true);
    return () => { window.removeEventListener("keydown", onKey, true); opener?.focus?.(); };
  }, [onClose]);

  return (
    <div className="dialog-backdrop" onClick={onClose}>
      <div ref={card} role={role} aria-modal="true" aria-labelledby={titleId} className="dialog-card" onClick={(event) => event.stopPropagation()}>
        <h2 id={titleId} className="serif dialog-title">{title}</h2>
        {children}
      </div>
    </div>
  );
}
