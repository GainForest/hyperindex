"use client";

import { formatRelative } from "@/lib/utils";
import type { ActivityEntry } from "@/types";

interface RecentActivityProps {
  entries: ActivityEntry[];
  isLoading?: boolean;
}

export function RecentActivity({ entries, isLoading }: RecentActivityProps) {
  return (
    <div className="space-y-4">
      <h3 className="font-[family-name:var(--font-garamond)] text-xl text-zinc-900">
        Recent Activity
      </h3>

      <div className="rounded-xl border border-zinc-200/60 bg-white">
        {isLoading ? (
          <div className="p-4 space-y-3">
            {[...Array(5)].map((_, i) => (
              <div
                key={i}
                className="h-10 animate-pulse rounded-lg bg-zinc-50"
              />
            ))}
          </div>
        ) : entries.length === 0 ? (
          <div className="py-12 text-center text-sm text-zinc-400">
            No recent activity
          </div>
        ) : (
          <div className="divide-y divide-zinc-100">
            {entries.slice(0, 10).map((entry) => (
              <div
                key={entry.id}
                className="flex items-center justify-between px-4 py-3"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <StatusDot status={entry.status} />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[10px] font-medium uppercase tracking-wide ${
                          entry.operation === "create"
                            ? "bg-emerald-50 text-emerald-600"
                            : entry.operation === "update"
                            ? "bg-blue-50 text-blue-600"
                            : "bg-amber-50 text-amber-600"
                        }`}
                      >
                        {entry.operation}
                      </span>
                      <span className="text-sm font-medium text-zinc-800 truncate">
                        {entry.collection}
                      </span>
                    </div>
                    <p className="text-xs text-zinc-400 truncate font-mono">
                      {entry.did.slice(0, 32)}...
                    </p>
                  </div>
                </div>
                <div className="text-right shrink-0 ml-4">
                  <p className="text-xs text-zinc-400">
                    {formatRelative(entry.timestamp)}
                  </p>
                  {entry.errorMessage && (
                    <p className="text-xs text-red-500 truncate max-w-[150px]">
                      {entry.errorMessage}
                    </p>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  return (
    <span
      className={`w-2 h-2 rounded-full shrink-0 ${
        status === "success"
          ? "bg-emerald-400"
          : status === "error"
          ? "bg-red-400"
          : "bg-amber-400"
      }`}
    />
  );
}
