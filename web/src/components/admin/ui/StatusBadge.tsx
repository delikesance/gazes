"use client";

import React, { useState } from "react";

export type StatusType = "Nouveau" | "En cours" | "Résolu";

interface StatusBadgeProps {
  status?: StatusType;
  onClick?: (nextStatus: StatusType) => void;
}

const STATUS_CONFIG: Record<StatusType, { color: string; ring: string; inner: string; tick: string }> = {
  Nouveau: {
    color: "#9b8afb",
    ring: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z",
    inner: "",
    tick: "",
  },
  "En cours": {
    color: "#fafafa",
    ring: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z",
    inner: "M12 3a9 9 0 0 1 0 18z",
    tick: "",
  },
  Résolu: {
    color: "#a1a1aa",
    ring: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z",
    inner: "",
    tick: "M8 12.5l3 3 5-6",
  },
};

const NEXT_STATUS: Record<StatusType, StatusType> = {
  Nouveau: "En cours",
  "En cours": "Résolu",
  Résolu: "Nouveau",
};

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status: initialStatus = "Nouveau", onClick }) => {
  const [status, setStatus] = useState<StatusType>(initialStatus);
  const config = STATUS_CONFIG[status];

  const handleClick = () => {
    const nextStatus = NEXT_STATUS[status];
    setStatus(nextStatus);
    onClick?.(nextStatus);
  };

  const baseStyles =
    "inline-flex items-center gap-2 box-border border-0 rounded-full font-semibold whitespace-nowrap transition-colors focus-visible:outline-2 focus-visible:outline-[#fafafa] focus-visible:outline-offset-1";

  if (onClick) {
    return (
      <button
        type="button"
        className={`${baseStyles} h-11 px-3.5 bg-[#151517] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)] cursor-pointer hover:bg-[#1c1c1f] text-sm`}
        style={{ color: config.color }}
        aria-label={`Statut : ${status}. Passer à ${NEXT_STATUS[status]}`}
        onClick={handleClick}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: "none" }}>
          <path d={config.ring} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
          {config.inner && <path d={config.inner} fill="currentColor" stroke="none" />}
          {config.tick && <path d={config.tick} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />}
        </svg>
        <span>{status}</span>
        <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: "none" }}>
          <path d="M6 9l6 6 6-6" fill="none" stroke="#a1a1aa" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
    );
  }

  return (
    <span
      className={`${baseStyles} h-7 px-3 bg-[#151517] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)] text-sm`}
      style={{ color: config.color }}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true" style={{ flex: "none" }}>
        <path d={config.ring} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
        {config.inner && <path d={config.inner} fill="currentColor" stroke="none" />}
        {config.tick && <path d={config.tick} fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />}
      </svg>
      <span>{status}</span>
    </span>
  );
};

StatusBadge.displayName = "StatusBadge";
