"use client";

import React, { useId, useState } from "react";

interface TextFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label: string;
  hint?: string;
  error?: string;
  prefix?: string;
  suffix?: string;
  shape?: "field" | "pill";
}

export const TextField = React.forwardRef<HTMLInputElement, TextFieldProps>(
  (
    {
      label,
      hint,
      error,
      prefix,
      suffix,
      shape = "field",
      value: controlledValue,
      onChange,
      ...props
    },
    ref
  ) => {
    const id = useId();
    const errId = id + "-err";
    const hintId = id + "-hint";

    const [internalValue, setInternalValue] = useState(controlledValue ?? "");
    const val = controlledValue !== undefined ? controlledValue : internalValue;

    const radius = shape === "pill" ? "rounded-full" : "rounded-[14px]";
    const ring = error ? "shadow-[inset_0_0_0_1px_#f87171]" : "shadow-[inset_0_0_0_1px_rgba(255,255,255,0.07)]";

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      if (controlledValue === undefined) {
        setInternalValue(newValue);
      }
      onChange?.(e);
    };

    return (
      <div className="flex flex-col gap-1.5">
        <label className="text-xs text-[#a1a1aa] font-medium">
          <span>{label}</span>
          <div className={`flex items-center gap-2 min-h-[44px] px-3.5 bg-[#17171a] ${radius} ${ring} focus-within:outline-2 focus-within:outline-[#fafafa] focus-within:outline-offset-1`}>
            {prefix && <span className="font-mono text-xs text-[#a1a1aa]">{prefix}</span>}
            <input
              ref={ref}
              type={props.type || "text"}
              value={val}
              onChange={handleChange}
              aria-invalid={error ? "true" : "false"}
              aria-describedby={error ? errId : hint ? hintId : undefined}
              className="flex-1 bg-transparent border-0 outline-0 text-[#fafafa] placeholder:text-[#a1a1aa]"
              {...props}
            />
            {suffix && <span className="font-mono text-xs text-[#a1a1aa]">{suffix}</span>}
          </div>
        </label>
        {error && (
          <span id={errId} role="alert" className="flex items-center gap-1.5 text-xs text-[#fecaca]">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 3L22 21H2z M12 10v5 M12 18h.01"></path>
            </svg>
            <span>Erreur : {error}</span>
          </span>
        )}
        {hint && !error && (
          <span id={hintId} className="text-xs text-[#a1a1aa]">{hint}</span>
        )}
      </div>
    );
  }
);

TextField.displayName = "TextField";
