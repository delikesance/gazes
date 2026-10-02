"use client";

import React from "react";

export type BadgeVariant = "default" | "outline" | "contrast" | "subtle" | "mono";

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  children: React.ReactNode;
}

export const Badge: React.FC<BadgeProps> = ({
  variant = "default",
  className = "",
  children,
  ...props
}) => {
  const baseStyles =
    "inline-flex items-center gap-1 px-2 py-0.5 text-[10px] sm:text-xs font-medium rounded transition-colors select-none";

  const variantStyles: Record<BadgeVariant, string> = {
    default: "bg-zinc-900 text-zinc-300 border border-zinc-800",
    outline: "bg-transparent text-zinc-400 border border-zinc-800",
    contrast: "bg-white text-zinc-950 font-semibold",
    subtle: "bg-zinc-900/40 text-zinc-500 border border-zinc-800/40",
    mono: "bg-zinc-900 text-zinc-300 border border-zinc-800 font-mono tracking-tight",
  };

  return (
    <span className={`${baseStyles} ${variantStyles[variant]} ${className}`} {...props}>
      {children}
    </span>
  );
};
