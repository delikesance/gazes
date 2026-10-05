"use client";

import React, { useState } from "react";

interface ToggleProps {
  label: string;
  checked?: boolean;
  showState?: boolean;
  tone?: "accent" | "danger";
  ariaLabel?: string;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
}

export const Toggle = React.forwardRef<HTMLButtonElement, ToggleProps>(
  (
    {
      label,
      checked: controlledChecked,
      showState = false,
      tone = "accent",
      ariaLabel = "",
      onChange,
      disabled = false,
    },
    ref
  ) => {
    const [internalChecked, setInternalChecked] = useState(controlledChecked ?? false);
    const checked = controlledChecked !== undefined ? controlledChecked : internalChecked;
    const danger = tone === "danger";

    const checkColor = danger ? "#f87171" : "#9b8afb";
    const stateText = checked ? "Activé" : "Désactivé";

    const trackBg = checked
      ? danger
        ? "bg-[#f87171]"
        : "bg-[#9b8afb]"
      : "bg-[#17171a] shadow-[inset_0_0_0_1px_#71717a]";

    const trackStyle = `relative w-11 h-6 rounded-full box-sizing-border flex items-center justify-center ${trackBg}`;

    const thumbLeft = checked ? "left-[23px]" : "left-[3px]";
    const thumbBg = checked ? "bg-[#111111]" : "bg-[#a1a1aa]";
    const thumbStyle = `absolute top-[3px] ${thumbLeft} w-[18px] h-[18px] rounded-full ${thumbBg} transition-all duration-150`;

    const handleClick = () => {
      if (disabled) return;
      const newChecked = !checked;
      if (controlledChecked === undefined) {
        setInternalChecked(newChecked);
      }
      onChange?.(newChecked);
    };

    return (
      <button
        ref={ref}
        type="button"
        role="switch"
        aria-checked={checked ? "true" : "false"}
        aria-label={ariaLabel}
        className={`flex items-center justify-between gap-3 w-full min-h-[44px] p-0 border-0 bg-transparent text-[#fafafa] text-sm font-medium text-left cursor-pointer rounded-full ${disabled ? "cursor-not-allowed opacity-50" : ""} focus-visible:outline-2 focus-visible:outline-[#fafafa] focus-visible:outline-offset-1`}
        onClick={handleClick}
        disabled={disabled}
      >
        <span>{label}</span>
        <span className="flex items-center gap-2.5">
          {showState && <span className="font-mono text-xs text-[#a1a1aa]">{stateText}</span>}
          <div className={trackStyle}>
            <div className={thumbStyle}>
              {checked && (
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke={checkColor}
                  strokeWidth="3.2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M20 6L9 17l-5-5"></path>
                </svg>
              )}
            </div>
          </div>
        </span>
      </button>
    );
  }
);

Toggle.displayName = "Toggle";
