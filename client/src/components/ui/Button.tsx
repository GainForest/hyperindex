"use client";

import { cn } from "@/lib/utils";
import { ButtonHTMLAttributes, forwardRef, ElementType, ComponentPropsWithoutRef } from "react";

type ButtonBaseProps = {
  variant?: "default" | "outline" | "ghost" | "destructive" | "primary";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  as?: ElementType;
};

type ButtonProps<T extends ElementType = "button"> = ButtonBaseProps &
  Omit<ComponentPropsWithoutRef<T>, keyof ButtonBaseProps>;

const buttonVariants = {
  default: "bg-zinc-900 text-white hover:bg-zinc-800",
  primary: "bg-emerald-600 text-white hover:bg-emerald-700",
  outline: "border border-zinc-200/60 bg-transparent text-zinc-600 hover:bg-zinc-50",
  ghost: "bg-transparent text-zinc-600 hover:bg-zinc-50",
  destructive: "bg-red-600 text-white hover:bg-red-700",
};

const buttonSizes = {
  sm: "h-8 px-3 text-sm",
  md: "h-10 px-4 text-sm",
  lg: "h-12 px-6",
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant = "default",
      size = "md",
      loading,
      disabled,
      children,
      as,
      ...props
    },
    ref
  ) => {
    const Component = as || "button";
    const isButton = Component === "button";

    return (
      <Component
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-colors",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/30 focus-visible:border-emerald-400",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          buttonVariants[variant],
          buttonSizes[size],
          className
        )}
        {...(isButton ? { disabled: disabled || loading } : {})}
        {...props}
      >
        {loading && (
          <div className="w-4 h-4 rounded-full border-2 border-current border-t-transparent animate-spin" />
        )}
        {children}
      </Component>
    );
  }
);

Button.displayName = "Button";
