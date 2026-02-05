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
      color: "text-emerald-600",
      bg: "bg-emerald-50",
    },
    {
      name: "Actors",
      value: actorCount,
      color: "text-blue-600",
      bg: "bg-blue-50",
    },
    {
      name: "Lexicons",
      value: lexiconCount,
      color: "text-purple-600",
      bg: "bg-purple-50",
    },
  ];

  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
      {stats.map((stat, index) => (
        <div key={stat.name} className="flex items-center gap-2">
          {isLoading ? (
            <div className="h-5 w-16 animate-pulse rounded bg-zinc-100" />
          ) : (
            <>
              <span className={`font-medium tabular-nums ${stat.color}`}>
                {formatNumber(stat.value)}
              </span>
              <span className="text-zinc-400">{stat.name}</span>
            </>
          )}
          {index < stats.length - 1 && (
            <span className="text-zinc-200 ml-4">&middot;</span>
          )}
        </div>
      ))}
    </div>
  );
}
