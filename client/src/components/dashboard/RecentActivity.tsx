"use client";

import { formatTimestamp } from "@/lib/utils";
import type { ActivityEntry } from "@/types";

interface RecentActivityProps {
  entries: ActivityEntry[];
  isLoading?: boolean;
}

/**
 * Build the URL to view a record on impactindexer.org
 */
function getRecordUrl(entry: ActivityEntry): string | null {
  // Only link to create/update operations that have an rkey
  if (!entry.rkey || entry.operation === "delete") {
    return null;
  }
  const params = new URLSearchParams({
    did: entry.did,
    collection: entry.collection,
    rkey: entry.rkey,
  });
  return `https://impactindexer.org/data?${params.toString()}`;
}

export function RecentActivity({ entries, isLoading }: RecentActivityProps) {
  return (
    <div className="space-y-4">
      <h3
        className="font-[family-name:var(--font-syne)] text-xl"
        style={{ color: "var(--foreground)" }}
      >
        Recent Activity
      </h3>

      <div
        className="rounded-xl border"
        style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}
      >
        {isLoading ? (
          <div className="p-4 space-y-3">
            {[...Array(5)].map((_, i) => (
              <div
                key={i}
                className="h-10 animate-pulse rounded-lg"
                style={{ backgroundColor: "var(--muted)" }}
              />
            ))}
          </div>
        ) : entries.length === 0 ? (
          <div
            className="py-12 text-center text-sm"
            style={{ color: "var(--muted-foreground)" }}
          >
            No recent activity
          </div>
        ) : (
          <div>
            {entries.slice(0, 10).map((entry, idx) => {
              const recordUrl = getRecordUrl(entry);
              const isLast = idx === Math.min(entries.length, 10) - 1;
              const content = (
                <>
                  <div className="flex items-center gap-3 min-w-0">
                    <StatusDot status={entry.status} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span
                          className="px-1.5 py-0.5 rounded text-[10px] font-medium uppercase tracking-wide"
                          style={
                            entry.operation === "create"
                              ? {
                                  backgroundColor: "oklch(0.65 0.15 155 / 0.1)",
                                  color: "oklch(0.55 0.15 155)",
                                }
                              : entry.operation === "update"
                              ? {
                                  backgroundColor: "oklch(0.60 0.15 250 / 0.1)",
                                  color: "oklch(0.50 0.15 250)",
                                }
                              : {
                                  backgroundColor: "oklch(0.75 0.15 75 / 0.1)",
                                  color: "oklch(0.60 0.15 75)",
                                }
                          }
                        >
                          {entry.operation}
                        </span>
                        <span
                          className="text-sm font-medium truncate"
                          style={{ color: "var(--foreground)" }}
                        >
                          {entry.collection}
                        </span>
                      </div>
                      <p
                        className="text-xs truncate font-mono"
                        style={{ color: "var(--muted-foreground)" }}
                      >
                        {entry.did.slice(0, 32)}...
                      </p>
                    </div>
                  </div>
                  <div className="text-right shrink-0 ml-4">
                    <p
                      className="text-xs font-mono"
                      style={{ color: "var(--muted-foreground)" }}
                    >
                      {formatTimestamp(entry.timestamp)}
                    </p>
                    {entry.errorMessage && (
                      <p className="text-xs text-red-500 truncate max-w-[150px]">
                        {entry.errorMessage}
                      </p>
                    )}
                  </div>
                </>
              );

              const rowStyle: React.CSSProperties = {
                borderBottom: isLast ? undefined : "1px solid var(--border)",
              };

              if (recordUrl) {
                return (
                  <a
                    key={entry.id}
                    href={recordUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center justify-between px-4 py-3 hover:opacity-90 transition-opacity cursor-pointer"
                    style={rowStyle}
                  >
                    {content}
                  </a>
                );
              }

              return (
                <div
                  key={entry.id}
                  className="flex items-center justify-between px-4 py-3"
                  style={rowStyle}
                >
                  {content}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  const dotStyle: React.CSSProperties =
    status === "success"
      ? { backgroundColor: "oklch(0.65 0.15 155)" }
      : status === "error"
      ? { backgroundColor: "oklch(0.60 0.20 25)" }
      : { backgroundColor: "oklch(0.75 0.15 75)" };

  return (
    <span
      className="w-2 h-2 rounded-full shrink-0"
      style={dotStyle}
    />
  );
}
