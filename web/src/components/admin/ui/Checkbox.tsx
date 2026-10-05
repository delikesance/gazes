"use client";

import React, { useId, useState } from "react";

interface CheckboxProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label: string;
  detail?: string;
  detailMono?: boolean;
}

export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  (
    {
      label,
      detail,
      detailMono = true,
      checked: controlledChecked,
      onChange,
      className = "",
      ...props
    },
    ref
  ) => {
    const id = useId();
    const [internalChecked, setInternalChecked] = useState(controlledChecked ?? false);
    const checked = controlledChecked !== undefined ? controlledChecked : internalChecked;
    const disabled = props.disabled || false;

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const newChecked = e.target.checked;
      if (controlledChecked === undefined) {
        setInternalChecked(newChecked);
      }
      onChange?.(e);
    };

    const detailClass = detailMono
      ? "font-mono text-xs text-[#a1a1aa]"
      : "text-xs text-[#a1a1aa]";

    return (
      <label
        className={`flex items-center gap-3 w-full min-h-[44px] px-3.5 rounded-[14px] bg-[#17171a] text-[#fafafa] text-sm cursor-pointer ${disabled ? "opacity-50 cursor-not-allowed" : ""} focus-within:outline-2 focus-within:outline-[#fafafa] focus-within:outline-offset-1 ${className}`}
      >
        <input
          ref={ref}
          type="checkbox"
          id={id}
          checked={checked}
          disabled={disabled}
          onChange={handleChange}
          className="w-5 h-5 m-0 accent-[#9b8afb] cursor-pointer"
          {...props}
        />
        <span className="flex-1 min-w-0">{label}</span>
        {detail && <span className={detailClass}>{detail}</span>}
      </label>
    );
  }
);

Checkbox.displayName = "Checkbox";
