"use client";

import { cn } from "@/lib/utils";
import { InputHTMLAttributes, forwardRef } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, label, error, hint, id, ...props }, ref) => {
    return (
      <div className="space-y-1.5">
        {label && (
          <label
            htmlFor={id}
            className="block text-sm text-zinc-600"
          >
            {label}
          </label>
        )}
        <input
          id={id}
          ref={ref}
          className={cn(
            "w-full px-3 py-2 text-sm bg-white/50 border border-zinc-200/60 rounded-lg",
            "text-zinc-800 placeholder:text-zinc-300",
            "focus:outline-none focus:ring-2 focus:ring-emerald-500/30 focus:border-emerald-400",
            "focus:bg-white/70 transition-all",
            "disabled:opacity-50 disabled:cursor-not-allowed",
            error && "border-red-400 focus:ring-red-500/30 focus:border-red-400",
            className
          )}
          {...props}
        />
        {hint && !error && (
          <p className="text-xs text-zinc-300">{hint}</p>
        )}
        {error && (
          <p className="text-sm text-red-500">{error}</p>
        )}
      </div>
    );
  }
);

Input.displayName = "Input";
