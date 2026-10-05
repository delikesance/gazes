"use client";

import React, { useId, useState } from "react";

export interface SelectOption {
  value: string | number;
  label: string;
}

interface SelectFieldProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  options: SelectOption[];
  hideLabel?: boolean;
}

export const SelectField = React.forwardRef<HTMLSelectElement, SelectFieldProps>(
  ({ label, options, hideLabel = false, value: controlledValue, onChange, ...props }, ref) => {
    const id = useId();
    const [internalValue, setInternalValue] = useState(controlledValue ?? "");
    const val = controlledValue !== undefined ? controlledValue : internalValue;

    const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
      const newValue = e.target.value;
      if (controlledValue === undefined) {
        setInternalValue(newValue);
      }
      onChange?.(e);
    };

    return (
      <div className="flex flex-col gap-1.5">
        <label htmlFor={id} className={`text-xs ${hideLabel ? "absolute w-1 h-1 m-[-1px] p-0 overflow-hidden clip-rect-0 whitespace-nowrap border-0" : "text-[#a1a1aa] font-medium"}`}>
          {label}
        </label>
        <div className="relative flex items-center">
          <select
            ref={ref}
            id={id}
            value={val}
            onChange={handleChange}
            className="appearance-none w-full min-h-[44px] px-3.5 pr-10 border-0 rounded-[14px] bg-[#17171a] text-[#fafafa] text-sm font-medium shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)] cursor-pointer focus-visible:outline-2 focus-visible:outline-[#fafafa] focus-visible:outline-offset-1"
            {...props}
          >
            {options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="#a1a1aa"
            strokeWidth="2.2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
            className="absolute right-3.5 pointer-events-none"
          >
            <path d="M6 9l6 6 6-6"></path>
          </svg>
        </div>
      </div>
    );
  }
);

SelectField.displayName = "SelectField";
