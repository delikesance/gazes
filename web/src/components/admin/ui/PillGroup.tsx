"use client";

import React, { useState } from "react";
import { Pill } from "./Pill";

export interface PillOption {
  id: string | number;
  label: string;
  count?: number;
}

interface PillGroupProps {
  options: PillOption[];
  value?: string | number;
  mode?: "segmented" | "tabs";
  size?: "sm" | "md";
  ariaLabel?: string;
  onChange?: (value: string | number) => void;
}

export const PillGroup: React.FC<PillGroupProps> = ({
  options,
  value: controlledValue,
  mode = "segmented",
  size = "sm",
  ariaLabel = "Options",
  onChange,
}) => {
  const [internalValue, setInternalValue] = useState<string | number>(
    controlledValue !== undefined ? controlledValue : (options[0]?.id ?? "")
  );

  const currentValue = controlledValue !== undefined ? controlledValue : internalValue;

  const handleChange = (id: string | number) => {
    if (controlledValue === undefined) {
      setInternalValue(id);
    }
    onChange?.(id);
  };

  return (
    <div
      role={mode === "tabs" ? "tablist" : "group"}
      aria-label={ariaLabel}
      className="flex flex-wrap gap-2"
    >
      {options.map((option) => (
        <Pill
          key={option.id}
          label={option.label}
          selected={String(option.id) === String(currentValue)}
          count={option.count}
          size={size}
          mode={mode === "tabs" ? "tab" : "button"}
          onClick={() => handleChange(option.id)}
        />
      ))}
    </div>
  );
};

PillGroup.displayName = "PillGroup";
