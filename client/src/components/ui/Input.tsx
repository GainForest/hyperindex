"use client";

import { cn } from "@/lib/utils";
import { InputHTMLAttributes, forwardRef } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, label, error, hint, id, style, ...props }, ref) => {
    return (
      <div className="space-y-1.5">
        {label && (
          <label
            htmlFor={id}
            className="block text-sm"
            style={{ color: "var(--foreground)" }}
          >
            {label}
          </label>
        )}
        <input
          id={id}
          ref={ref}
          className={cn(
            "w-full px-3 py-2 text-sm border rounded-lg focus:outline-none focus:ring-2 transition-all",
            "disabled:opacity-50 disabled:cursor-not-allowed",
            className
          )}
          style={{
            backgroundColor: "var(--card)",
            borderColor: error ? "var(--destructive)" : "var(--input)",
            color: "var(--foreground)",
            ...style,
          }}
          {...props}
        />
        {hint && !error && (
          <p className="text-xs" style={{ color: "var(--muted-foreground)" }}>{hint}</p>
        )}
        {error && (
          <p className="text-sm text-red-500">{error}</p>
        )}
      </div>
    );
  }
);

Input.displayName = "Input";
