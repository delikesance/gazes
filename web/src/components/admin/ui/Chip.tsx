"use client";

import React from "react";

export type ChipTone = "neutral" | "accent" | "danger" | "code";

interface ChipProps extends React.HTMLAttributes<HTMLSpanElement> {
  label: string;
  tone?: ChipTone;
}

export const Chip: React.FC<ChipProps> = ({ label, tone = "neutral", className = "", ...props }) => {
  const toneStyles: Record<ChipTone, string> = {
    neutral:
      "bg-[#151517] text-[#a1a1aa] text-xs uppercase tracking-[0.08em] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)]",
    accent:
      "bg-[#151517] text-[#9b8afb] text-xs uppercase tracking-[0.08em] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)]",
    danger: "bg-[#7f1d1d] text-[#fecaca] text-xs uppercase tracking-[0.08em]",
    code: "bg-[#17171a] text-[#fafafa] text-sm shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)]",
  };

  const baseStyles = "inline-flex items-center box-border h-[26px] px-2.5 rounded-full font-mono whitespace-nowrap";

  return (
    <span className={`${baseStyles} ${toneStyles[tone]} ${className}`} {...props}>
      {label}
    </span>
  );
};

Chip.displayName = "Chip";
