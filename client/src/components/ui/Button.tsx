"use client";

import { cn } from "@/lib/utils";
import { ButtonHTMLAttributes, forwardRef, ElementType, ComponentPropsWithoutRef, CSSProperties } from "react";

type ButtonBaseProps = {
  variant?: "default" | "outline" | "ghost" | "destructive" | "primary";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  as?: ElementType;
};

type ButtonProps<T extends ElementType = "button"> = ButtonBaseProps &
  Omit<ComponentPropsWithoutRef<T>, keyof ButtonBaseProps>;

const buttonVariants = {
  default: "",
  primary: "",
  outline: "border bg-transparent",
  ghost: "bg-transparent",
  destructive: "",
};

const variantStyles: Record<string, CSSProperties> = {
  default: { backgroundColor: "var(--primary)", color: "var(--primary-foreground)" },
  primary: { backgroundColor: "var(--primary)", color: "var(--primary-foreground)" },
  outline: { borderColor: "var(--border)", color: "var(--foreground)" },
  ghost: { color: "var(--foreground)" },
  destructive: { backgroundColor: "var(--destructive)", color: "#fff" },
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
      style,
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
          "inline-flex items-center justify-center gap-2 rounded-lg font-[family-name:var(--font-outfit)] font-medium transition-colors hover:opacity-90",
          "focus-visible:outline-none focus-visible:ring-2",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          buttonVariants[variant],
          buttonSizes[size],
          className
        )}
        style={{ ...variantStyles[variant], outline: "var(--ring)", ...style }}
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
