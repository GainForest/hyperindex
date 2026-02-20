"use client";

import type { ActivityBucket, TimeRange } from "@/types";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { format } from "date-fns";

interface ActivityChartProps {
  data: ActivityBucket[];
  timeRange: TimeRange;
  onTimeRangeChange: (range: TimeRange) => void;
  isLoading?: boolean;
}

const timeRanges: { value: TimeRange; label: string }[] = [
  { value: "ONE_HOUR", label: "1h" },
  { value: "THREE_HOURS", label: "3h" },
  { value: "SIX_HOURS", label: "6h" },
  { value: "ONE_DAY", label: "24h" },
  { value: "SEVEN_DAYS", label: "7d" },
];

export function ActivityChart({
  data,
  timeRange,
  onTimeRangeChange,
  isLoading,
}: ActivityChartProps) {
  const chartData = data.map((bucket) => ({
    timestamp: bucket.timestamp,
    creates: bucket.creates,
    updates: bucket.updates,
    deletes: bucket.deletes,
    total: bucket.creates + bucket.updates + bucket.deletes,
  }));

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3
          className="font-[family-name:var(--font-syne)] text-xl"
          style={{ color: "var(--foreground)" }}
        >
          Activity
        </h3>
        <div className="flex items-center gap-1">
          {timeRanges.map((range) => (
            <button
              key={range.value}
              onClick={() => onTimeRangeChange(range.value)}
              className="px-2 py-0.5 rounded text-xs transition-colors cursor-pointer"
              style={
                timeRange === range.value
                  ? { backgroundColor: "var(--accent)", color: "var(--primary)", fontWeight: 500 }
                  : { color: "var(--muted-foreground)" }
              }
            >
              {range.label}
            </button>
          ))}
        </div>
      </div>

      <div
        className="rounded-xl border p-4"
        style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}
      >
        {isLoading ? (
          <div
            className="h-48 animate-pulse rounded-lg"
            style={{ backgroundColor: "var(--muted)" }}
          />
        ) : data.length === 0 ? (
          <div
            className="flex h-48 items-center justify-center text-sm"
            style={{ color: "var(--muted-foreground)" }}
          >
            No activity data available
          </div>
        ) : (
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis
                  dataKey="timestamp"
                  tickFormatter={(value) =>
                    format(new Date(value), timeRange === "SEVEN_DAYS" ? "MMM d" : "HH:mm")
                  }
                  fontSize={11}
                  stroke="var(--muted-foreground)"
                  tickLine={false}
                  axisLine={false}
                />
                <YAxis
                  fontSize={11}
                  stroke="var(--muted-foreground)"
                  tickLine={false}
                  axisLine={false}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "var(--card)",
                    border: "1px solid var(--border)",
                    borderRadius: "0.75rem",
                    fontSize: "12px",
                    color: "var(--foreground)",
                  }}
                  labelFormatter={(value) =>
                    format(new Date(value), "MMM d, yyyy HH:mm")
                  }
                />
                <Area
                  type="monotone"
                  dataKey="creates"
                  stackId="1"
                  stroke="#10b981"
                  fill="#10b981"
                  fillOpacity={0.4}
                  name="Creates"
                />
                <Area
                  type="monotone"
                  dataKey="updates"
                  stackId="1"
                  stroke="#3b82f6"
                  fill="#3b82f6"
                  fillOpacity={0.4}
                  name="Updates"
                />
                <Area
                  type="monotone"
                  dataKey="deletes"
                  stackId="1"
                  stroke="#f59e0b"
                  fill="#f59e0b"
                  fillOpacity={0.4}
                  name="Deletes"
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </div>
  );
}
