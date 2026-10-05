"use client";

import React from "react";

export type PillSize = "sm" | "md";

interface PillProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  selected?: boolean;
  count?: number;
  size?: PillSize;
  mode?: "button" | "tab";
}

export const Pill = React.forwardRef<HTMLButtonElement, PillProps>(
  ({ label, selected = false, count, size = "sm", mode = "button", className = "", ...props }, ref) => {
    const baseStyles =
      "inline-flex items-center gap-2 box-border border-0 rounded-full font-medium cursor-pointer text-decoration-none whitespace-nowrap transition-colors select-none focus-visible:outline-2 focus-visible:outline-[#fafafa] focus-visible:outline-offset-1";

    const sizeStyles: Record<PillSize, string> = {
      sm: "h-9 px-4 text-sm",
      md: "h-11 px-4 text-sm",
    };

    const variantStyles = selected
      ? "bg-[#fafafa] text-[#111111]"
      : "bg-[#151517] text-[#a1a1aa] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)] hover:bg-[#1c1c1f]";

    const classNames = `${baseStyles} ${sizeStyles[size]} ${variantStyles} ${className}`;

    return (
      <button
        ref={ref}
        type="button"
        role={mode === "tab" ? "tab" : "button"}
        aria-selected={mode === "tab" ? (selected ? "true" : "false") : undefined}
        aria-pressed={mode === "button" ? (selected ? "true" : "false") : undefined}
        className={classNames}
        {...props}
      >
        <span>{label}</span>
        {count !== undefined && count !== null && (
          <span className="font-mono text-xs text-[#a1a1aa]" style={selected ? { color: "#3f3f46" } : {}}>
            {typeof count === "number" ? count.toLocaleString("fr-FR") : count}
          </span>
        )}
      </button>
    );
  }
);

Pill.displayName = "Pill";
