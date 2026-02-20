import { cn } from "@/lib/utils";
import { CSSProperties, HTMLAttributes, forwardRef } from "react";

interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  variant?: "info" | "success" | "warning" | "error";
  onClose?: () => void;
}

const variants = {
  info: "border",
  success: "border",
  warning: "border",
  error: "border",
};

const variantStyles: Record<string, CSSProperties> = {
  info: { backgroundColor: "oklch(0.60 0.15 250 / 0.08)", color: "oklch(0.45 0.15 250)", borderColor: "oklch(0.60 0.15 250 / 0.2)" },
  success: { backgroundColor: "oklch(0.65 0.15 155 / 0.08)", color: "oklch(0.45 0.15 155)", borderColor: "oklch(0.65 0.15 155 / 0.2)" },
  warning: { backgroundColor: "oklch(0.75 0.15 75 / 0.08)", color: "oklch(0.55 0.15 75)", borderColor: "oklch(0.75 0.15 75 / 0.2)" },
  error: { backgroundColor: "oklch(0.60 0.20 25 / 0.08)", color: "oklch(0.50 0.20 25)", borderColor: "oklch(0.60 0.20 25 / 0.2)" },
};

export const Alert = forwardRef<HTMLDivElement, AlertProps>(
  ({ className, variant = "info", children, onClose, style, ...props }, ref) => {
    return (
      <div
        ref={ref}
        role="alert"
        className={cn(
          "flex items-start gap-3 rounded-lg border p-4",
          variants[variant],
          className
        )}
        style={{ ...variantStyles[variant], ...style }}
        {...props}
      >
        <svg
          className="h-5 w-5 flex-shrink-0"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={1.5}
          stroke="currentColor"
        >
          {variant === "info" && (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="m11.25 11.25.041-.02a.75.75 0 0 1 1.063.852l-.708 2.836a.75.75 0 0 0 1.063.853l.041-.021M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9-3.75h.008v.008H12V8.25Z"
            />
          )}
          {variant === "success" && (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
            />
          )}
          {variant === "warning" && (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
            />
          )}
          {variant === "error" && (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="m9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
            />
          )}
        </svg>
        <div className="flex-1 text-sm">{children}</div>
        {onClose && (
          <button
            onClick={onClose}
            className="flex-shrink-0 opacity-70 hover:opacity-100 transition-opacity"
          >
            <svg
              className="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6 18 18 6M6 6l12 12"
              />
            </svg>
          </button>
        )}
      </div>
    );
  }
);

Alert.displayName = "Alert";
