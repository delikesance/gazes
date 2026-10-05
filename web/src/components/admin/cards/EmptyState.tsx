"use client";

import React from "react";
import Link from "next/link";

export interface EmptyStateProps {
  title: string;
  text?: string;
  actionLabel?: string;
  /** With a label: link if href is set, button otherwise. */
  href?: string;
  onClick?: () => void;
}

const ACTION_STYLE: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  minHeight: 44,
  boxSizing: "border-box",
  marginTop: 4,
  padding: "0 20px",
  border: 0,
  borderRadius: 999,
  background: "#151517",
  color: "#fafafa",
  boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
  fontFamily: "inherit",
  fontSize: 14,
  fontWeight: 600,
  textDecoration: "none",
  cursor: "pointer",
};

const FOCUS = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#fafafa]";

export const EmptyState: React.FC<EmptyStateProps> = ({ title, text, actionLabel, href, onClick }) => (
  <div
    role="status"
    style={{
      width: "100%",
      minWidth: 0,
      boxSizing: "border-box",
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: 8,
      padding: "20px 24px",
      borderRadius: 20,
      background: "#17171a",
      fontFamily: "'DM Sans', system-ui, sans-serif",
      color: "#fafafa",
      fontSize: 14,
      lineHeight: 1.4,
    }}
  >
    <span style={{ fontWeight: 500, fontSize: 15 }}>{title}</span>
    {text && <span style={{ fontSize: 13, color: "#a1a1aa" }}>{text}</span>}
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

EmptyState.displayName = "EmptyState";
