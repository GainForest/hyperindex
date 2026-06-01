"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { graphqlClient } from "@/lib/graphql/client";
import {
  GET_STATISTICS,
  GET_ACTIVITY_BUCKETS,
  GET_RECENT_ACTIVITY,
  GET_SETTINGS,
} from "@/lib/graphql/queries";
import { POPULATE_ACTIVITY } from "@/lib/graphql/mutations";
import { StatsCards } from "@/components/dashboard/StatsCards";
import { ActivityChart } from "@/components/dashboard/ActivityChart";
import { RecentActivity } from "@/components/dashboard/RecentActivity";
import { Alert } from "@/components/ui";
import { useAdminSession } from "@/lib/auth";
import Link from "next/link";
import type {
  StatisticsResponse,
  ActivityBucketsResponse,
  RecentActivityResponse,
  SettingsResponse,
  TimeRange,
} from "@/types";

export default function Dashboard() {
  const [timeRange, setTimeRange] = useState<TimeRange>("ONE_DAY");
  const queryClient = useQueryClient();
  const { isAdmin: hasAdminAccess, isLoading: isAdminLoading } = useAdminSession();

  // Fetch statistics
  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ["statistics"],
    queryFn: () => graphqlClient.request<StatisticsResponse>(GET_STATISTICS),
  });

  // Fetch settings for configuration alerts
  const { data: settingsData } = useQuery({
    queryKey: ["settings"],
    queryFn: () => graphqlClient.request<SettingsResponse>(GET_SETTINGS),
    enabled: hasAdminAccess,
  });

  // Fetch activity buckets
  const { data: activityData, isLoading: activityLoading } = useQuery({
    queryKey: ["activityBuckets", timeRange],
    queryFn: () =>
      graphqlClient.request<ActivityBucketsResponse>(GET_ACTIVITY_BUCKETS, {
        range: timeRange,
      }),
  });

  // Fetch recent activity
  const { data: recentData, isLoading: recentLoading } = useQuery({
    queryKey: ["recentActivity"],
    queryFn: () =>
      graphqlClient.request<RecentActivityResponse>(GET_RECENT_ACTIVITY, {
        hours: 24,
      }),
  });

  // Populate activity mutation
  const populateActivity = useMutation({
    mutationFn: () => graphqlClient.request<{ populateActivity: number }>(POPULATE_ACTIVITY),
    onSuccess: (data) => {
      // Refetch activity data
      queryClient.invalidateQueries({ queryKey: ["recentActivity"] });
      queryClient.invalidateQueries({ queryKey: ["activityBuckets"] });
      alert(`Populated ${data.populateActivity} activity entries`);
    },
    onError: (error: Error) => {
      alert(`Error: ${error.message}`);
    },
  });

  const stats = statsData?.statistics ?? { recordCount: 0, actorCount: 0, lexiconCount: 0 };
  const settings = settingsData?.settings;

  return (
    <div className="pt-8 sm:pt-12 space-y-10">
      {/* Hero Section */}
      <div className="max-w-md">
        <h2
          className="font-[family-name:var(--font-syne)] text-3xl sm:text-4xl leading-tight"
          style={{ color: "var(--foreground)" }}
        >
          Dashboard
        </h2>
        <p className="mt-3 leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
          Monitor your AppView server. Track records, activity, and manage your AT Protocol deployment.
        </p>
      </div>

      {/* Configuration Alerts */}
      <div className="space-y-3">
        {hasAdminAccess && settings && !settings.domainAuthority && (
          <Alert variant="warning">
            No domain authority configured.{" "}
            <Link href="/settings" className="font-medium underline hover:no-underline">
              Configure in Settings
            </Link>
          </Alert>
        )}
        {stats.lexiconCount === 0 && (
          <Alert variant="info">
            No lexicons loaded.{" "}
            <Link href="/lexicons" className="font-medium underline hover:no-underline">
              Upload lexicons
            </Link>
          </Alert>
        )}
      </div>

      {/* Quick Actions */}
      <div className="flex flex-wrap items-center gap-2">
        {isAdminLoading && (
          <div className="h-10 w-36 animate-pulse rounded-lg" style={{ backgroundColor: "var(--muted)" }} />
        )}
        {hasAdminAccess && (
          <Link
            href="/settings"
            className="group flex items-center gap-2 py-2 px-3 -mx-3 rounded-lg
                       hover:opacity-80 transition-opacity"
          >
            <svg
              className="w-5 h-5 transition-colors"
              style={{ color: "var(--muted-foreground)" }}
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
            </svg>
            <span
              className="text-sm font-medium transition-colors"
              style={{ color: "var(--foreground)" }}
            >
              Settings
            </span>
          </Link>
        )}
        <Link
          href="/backfill"
          className="group flex items-center gap-2 py-2 px-3 rounded-lg
                     hover:opacity-80 transition-opacity"
        >
          <svg
            className="w-5 h-5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={1.5}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99" />
          </svg>
          <span
            className="text-sm font-medium transition-colors"
            style={{ color: "var(--foreground)" }}
          >
            Backfill
          </span>
        </Link>
        <Link
          href="/query-builder"
          className="group flex items-center gap-2 py-2 px-3 rounded-lg
                     hover:opacity-80 transition-opacity"
        >
          <svg
            className="w-5 h-5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={1.5}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 0 1 6 3.75h12A2.25 2.25 0 0 1 20.25 6v12A2.25 2.25 0 0 1 18 20.25H6A2.25 2.25 0 0 1 3.75 18V6Zm4.5 3.75h7.5M8.25 12h4.5m-4.5 2.25h6" />
          </svg>
          <span
            className="text-sm font-medium transition-colors"
            style={{ color: "var(--foreground)" }}
          >
            Query Builder
          </span>
        </Link>
        <a
          href="/graphiql"
          target="_blank"
          rel="noopener noreferrer"
          className="group flex items-center gap-2 py-2 px-3 rounded-lg
                     hover:opacity-80 transition-opacity"
        >
          <svg
            className="w-5 h-5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
            fill="none"
            viewBox="0 0 24 24"
            strokeWidth={1.5}
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" />
          </svg>
          <span
            className="text-sm font-medium transition-colors"
            style={{ color: "var(--foreground)" }}
          >
            GraphiQL
          </span>
        </a>
      </div>

      {/* Stats */}
      <StatsCards
        recordCount={stats.recordCount}
        actorCount={stats.actorCount}
        lexiconCount={stats.lexiconCount}
        isLoading={statsLoading}
      />

      {/* Activity Chart */}
      <ActivityChart
        data={activityData?.activityBuckets ?? []}
        timeRange={timeRange}
        onTimeRangeChange={setTimeRange}
        isLoading={activityLoading}
      />

      {/* Recent Activity */}
      <RecentActivity
        entries={recentData?.recentActivity ?? []}
        isLoading={recentLoading}
      />

      {/* Rebuild Activity Button */}
      <div className="flex justify-center">
        <button
          onClick={() => populateActivity.mutate()}
          disabled={populateActivity.isPending}
          className="text-xs transition-colors disabled:opacity-50"
          style={{ color: "var(--muted-foreground)" }}
        >
          {populateActivity.isPending ? "Rebuilding..." : "Rebuild activity from records"}
        </button>
      </div>
    </div>
  );
}
