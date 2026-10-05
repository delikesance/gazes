"use client";

import React from "react";
import Link from "next/link";
import { Chip } from "../ui/Chip";

export interface AlertProps {
  /** Error code shown as a chip. */
  code?: string;
  text?: string;
  time?: string;
  actionLabel?: string;
  /** With a label: link if href is set, button otherwise. */
  href?: string;
  onClick?: () => void;
  /** role="alert" (announced immediately) instead of role="note". */
  live?: boolean;
}

const ACTION_STYLE: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  minHeight: 44,
  boxSizing: "border-box",
  padding: "0 16px",
  border: 0,
  borderRadius: 999,
  background: "transparent",
  color: "#fecaca",
  boxShadow: "inset 0 0 0 1px rgba(254,202,202,0.6)",
  fontFamily: "inherit",
  fontSize: 13,
  fontWeight: 600,
  textDecoration: "none",
  cursor: "pointer",
};

const FOCUS = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#fafafa]";

export const Alert: React.FC<AlertProps> = ({ code, text, time, actionLabel, href, onClick, live = false }) => (
  <div
    role={live ? "alert" : "note"}
    style={{
      width: "100%",
      minWidth: 0,
      boxSizing: "border-box",
      display: "flex",
      flexWrap: "wrap",
      alignItems: "center",
      gap: "8px 12px",
      padding: "12px 14px",
      borderRadius: 20,
      background: "#7f1d1d",
      color: "#fecaca",
      fontFamily: "'DM Sans', system-ui, sans-serif",
      fontSize: 13,
      lineHeight: 1.4,
    }}
  >
    <svg width="12" height="12" viewBox="0 0 10 10" aria-hidden="true" style={{ flex: "none" }}>
      <path d="M5 0L10 10H0Z" fill="currentColor"></path>
    </svg>
    {code && <Chip tone="code" label={code} />}
    <span style={{ flex: "1 1 160px", minWidth: 0 }}>{text}</span>
    {time && <span style={{ fontSize: 12 }}>{time}</span>}
    {actionLabel && href && (
      <Link href={href} className={FOCUS} style={ACTION_STYLE}>
        {actionLabel}
      </Link>
    )}
    {actionLabel && !href && (
      <button type="button" className={FOCUS} onClick={() => onClick?.()} style={ACTION_STYLE}>
        {actionLabel}
      </button>
    )}
  </div>
);

Alert.displayName = "Alert";
