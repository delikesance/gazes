"use client";
import { useRef } from "react";
import { Minus, Plus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { GENRES } from "@/lib/genres";

/**
 * Genre chips with three states. Click includes a genre, right click or long press excludes it,
 * and acting again on the same side clears it. Long press covers touch screens without a right click.
 */
export function GenreFilter({ include, exclude, onChange }: { include: string[]; exclude: string[]; onChange: (include: string[], exclude: string[]) => void }) {
  const { t } = useI18n();
  const pressTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const longPressed = useRef(false);
  const cancelPress = () => { if (pressTimer.current) { clearTimeout(pressTimer.current); pressTimer.current = undefined; } };
  const choose = (value: string, side: "include" | "exclude") => {
    const without = (list: string[]) => list.filter((item) => item !== value);
    const already = (side === "include" ? include : exclude).includes(value);
    onChange(
      side === "include" && !already ? [...without(include), value] : without(include),
      side === "exclude" && !already ? [...without(exclude), value] : without(exclude),
    );
  };
  const startPress = (value: string) => {
    longPressed.current = false;
    cancelPress();
    pressTimer.current = setTimeout(() => { longPressed.current = true; pressTimer.current = undefined; choose(value, "exclude"); if (typeof navigator !== "undefined") navigator.vibrate?.(15); }, 450);
  };
  return <div className="genre-filter" role="group" aria-label={t("Genres")}>
    {GENRES.map(([value, label]) => {
      const state = include.includes(value) ? "include" : exclude.includes(value) ? "exclude" : "none";
      const name = t(label);
      return <button key={value} type="button" data-state={state} aria-pressed={state !== "none"}
        aria-label={state === "include" ? t("{genre}, inclus", { genre: name }) : state === "exclude" ? t("{genre}, exclu", { genre: name }) : name}
        onPointerDown={(event) => { if (event.button === 0) startPress(value); }} onPointerUp={cancelPress} onPointerLeave={cancelPress} onPointerCancel={cancelPress}
        onClick={() => { if (longPressed.current) { longPressed.current = false; return; } choose(value, "include"); }}
        onContextMenu={(event) => { event.preventDefault(); if (!longPressed.current) choose(value, "exclude"); }}>
        {state === "include" && <Plus size={13} aria-hidden="true" />}{state === "exclude" && <Minus size={13} aria-hidden="true" />}{name}
      </button>;
    })}
  </div>;
}
