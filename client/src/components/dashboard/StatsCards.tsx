"use client";

import { formatNumber } from "@/lib/utils";

interface StatsCardsProps {
  recordCount: number;
  actorCount: number;
  lexiconCount: number;
  isLoading?: boolean;
}

export function StatsCards({
  recordCount,
  actorCount,
  lexiconCount,
  isLoading,
}: StatsCardsProps) {
  const stats = [
    {
      name: "Records",
      value: recordCount,
      colorStyle: { color: "oklch(0.65 0.15 155)" },
    },
    {
      name: "Actors",
      value: actorCount,
      colorStyle: { color: "oklch(0.55 0.15 250)" },
    },
    {
      name: "Lexicons",
      value: lexiconCount,
      colorStyle: { color: "oklch(0.55 0.15 310)" },
    },
  ];

  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
      {stats.map((stat, index) => (
        <div key={stat.name} className="flex items-center gap-2">
          {isLoading ? (
            <div
              className="h-5 w-16 animate-pulse rounded"
              style={{ backgroundColor: "var(--muted)" }}
            />
          ) : (
            <>
              <span className="font-medium tabular-nums" style={stat.colorStyle}>
                {formatNumber(stat.value)}
              </span>
              <span style={{ color: "var(--muted-foreground)" }}>{stat.name}</span>
            </>
          )}
          {index < stats.length - 1 && (
            <span className="ml-4" style={{ color: "var(--border)" }}>&middot;</span>
          )}
        </div>
      ))}
    </div>
  );
}
