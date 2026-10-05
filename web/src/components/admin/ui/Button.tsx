"use client";

import React from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger" | "glass";
export type ButtonSize = "sm" | "md" | "lg";

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: "arrow-up" | "arrow-down" | "copy" | "download" | "check" | "play" | "plus" | "minus" | "x" | "chevron-down" | "search";
  iconOnly?: boolean;
  fullWidth?: boolean;
  href?: string;
  pressed?: boolean;
  children?: React.ReactNode;
}

const ICONS: Record<string, string> = {
  "arrow-up": "M12 19V5 M5 12l7-7 7 7",
  "arrow-down": "M12 5v14 M19 12l-7 7-7-7",
  "copy": "M10 8h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2z M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2",
  "download": "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4 M7 10l5 5 5-5 M12 15V3",
  "check": "M20 6L9 17l-5-5",
  "play": "M6 3l14 9-14 9z",
  "plus": "M5 12h14 M12 5v14",
  "minus": "M5 12h14",
  "x": "M18 6L6 18 M6 6l12 12",
  "chevron-down": "M6 9l6 6 6-6",
  "search": "M21 21l-4.3-4.3 M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z",
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      variant = "primary",
      size = "md",
      icon,
      iconOnly = false,
      fullWidth = false,
      href,
      pressed,
      className = "",
      children,
      ...props
    },
    ref
  ) => {
    const baseStyles =
      "inline-flex items-center justify-center gap-2 box-border border-0 font-medium cursor-pointer text-decoration-none whitespace-nowrap transition-colors select-none disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-[#fafafa] focus-visible:outline-offset-1";

    const variantStyles: Record<ButtonVariant, string> = {
      primary: "bg-[#fafafa] text-[#111111] font-semibold hover:bg-[#e4e4e7]",
      secondary:
        "bg-[#151517] text-[#fafafa] font-medium shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)] hover:bg-[#1c1c1f]",
      ghost:
        "bg-transparent text-[#a1a1aa] font-medium hover:bg-[#1c1c1f] hover:text-[#fafafa]",
      danger:
        "bg-[#7f1d1d] text-[#fecaca] font-semibold hover:bg-[#991b1b]",
      glass:
        "bg-[rgba(255,255,255,0.14)] text-[#fafafa] font-medium shadow-[inset_0_0_0_1px_rgba(255,255,255,0.22)] backdrop-blur-[8px] hover:bg-[rgba(255,255,255,0.24)]",
    };

    const sizeStyles: Record<ButtonSize, string> = {
      sm: "h-9 px-4 text-xs rounded-full",
      md: "h-11 px-5 text-sm rounded-full",
      lg: "h-13 px-7 text-base rounded-full",
    };

    const iconPath = icon ? ICONS[icon] : "";
    const hasIcon = !!iconPath;
    const showLabel = !iconOnly || !hasIcon;

    const classNames = `${baseStyles} ${variantStyles[variant]} ${sizeStyles[size]} ${
      pressed ? "bg-[#fafafa] text-[#111111] font-semibold" : ""
    } ${fullWidth ? "w-full flex" : ""} ${className}`;

    const iconSize = size === "sm" ? 14 : 16;

    const iconEl = hasIcon ? (
      <svg
        width={iconSize}
        height={iconSize}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        style={{ flex: "none" }}
      >
        <path d={iconPath}></path>
      </svg>
    ) : null;

    if (href && !props.disabled) {
      return (
        <a
          ref={ref as never}
          href={href}
          className={classNames}
          aria-label={props["aria-label"]}
        >
          {iconEl}
          {showLabel && children}
        </a>
      );
    }

    return (
      <button
        ref={ref}
        className={classNames}
        aria-pressed={pressed !== undefined ? (pressed ? "true" : "false") : undefined}
        {...props}
      >
        {iconEl}
        {showLabel && children}
      </button>
    );
  }
);

Button.displayName = "Button";
