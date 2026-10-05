"use client";

import React from "react";

interface ThumbProps {
  title: string;
  size?: "sm" | "lg";
  tint?: number | string;
  decorative?: boolean;
}

const TINTS = ["#2a2650", "#3d2a2a", "#1f3a3a", "#3a3320", "#2a3a2a", "#3a2a3a", "#202c44"];

export const Thumb: React.FC<ThumbProps> = ({
  title,
  size = "sm",
  tint,
  decorative = true,
}) => {
  const titleStr = String(title || "");
  const words = titleStr.split(/[\s\-:]+/).filter((w) => /^[A-Za-zÀ-ɏ0-9]/.test(w));
  const letters = words.filter((w) => /^[A-Za-zÀ-ɏ]/.test(w));
  const src = letters.length ? letters : words;
  const ini = src.slice(0, 2).map((w) => w.charAt(0)).join("").toUpperCase() || "?";

  let idx = tint !== undefined && tint !== null ? Number(tint) : null;
  if (idx === null) {
    let h = 0;
    for (let i = 0; i < titleStr.length; i++) {
      h = (h * 31 + titleStr.charCodeAt(i)) % 7919;
    }
    idx = h;
  }
  idx = ((Math.floor(idx) % 7) + 7) % 7;

  const lg = size === "lg";
  const w = lg ? 56 : 36;
  const h = lg ? 80 : 52;
  const r = lg ? 10 : 8;
  const fs = lg ? 14 : 11;

  return (
    <span
      role={decorative ? "presentation" : "img"}
      aria-hidden={decorative ? "true" : "false"}
      aria-label={decorative ? "" : "Vignette de " + titleStr}
      style={{
        flex: "none",
        boxSizing: "border-box",
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        width: w + "px",
        height: h + "px",
        borderRadius: r + "px",
        background: TINTS[idx],
        color: "#fafafa",
        fontFamily: "'Geist Mono', monospace",
        fontSize: fs + "px",
        lineHeight: "1",
      }}
    >
      {ini}
    </span>
  );
};

Thumb.displayName = "Thumb";
