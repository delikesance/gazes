"use client";

import React from "react";

export type ButtonVariant = "primary" | "secondary" | "outline" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg" | "icon";

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  children: React.ReactNode;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = "secondary", size = "md", className = "", children, ...props }, ref) => {
    const baseStyles =
      "inline-flex items-center justify-center font-medium transition-all select-none disabled:opacity-50 disabled:pointer-events-none cursor-pointer";

    const variantStyles: Record<ButtonVariant, string> = {
      primary:
        "bg-white text-zinc-950 hover:bg-zinc-200 active:scale-[0.98] shadow-sm",
      secondary:
        "bg-zinc-900 text-zinc-100 border border-zinc-800 hover:bg-zinc-850 hover:border-zinc-700 active:scale-[0.98]",
      outline:
        "bg-transparent text-zinc-300 border border-zinc-800 hover:text-white hover:border-zinc-600 active:scale-[0.98]",
      ghost:
        "bg-transparent text-zinc-400 hover:text-zinc-100 hover:bg-zinc-900 active:scale-[0.98]",
      danger:
        "bg-zinc-900 text-red-400 border border-zinc-800 hover:bg-red-950/30 hover:border-red-900/50 active:scale-[0.98]",
    };

    const sizeStyles: Record<ButtonSize, string> = {
      sm: "h-8 px-3 text-xs rounded-full gap-1.5",
      md: "h-9 px-4 text-xs sm:text-sm rounded-full gap-2",
      lg: "h-11 px-5 text-sm sm:text-base rounded-full gap-2.5",
      icon: "h-8 w-8 p-0 rounded-full",
    };

    return (
      <button
        ref={ref}
        className={`${baseStyles} ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
        {...props}
      >
        {children}
      </button>
    );
  }
);

Button.displayName = "Button";
