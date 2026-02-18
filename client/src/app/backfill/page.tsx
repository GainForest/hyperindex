"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { graphqlClient } from "@/lib/graphql/client";
import { IS_BACKFILLING } from "@/lib/graphql/queries";
import { TRIGGER_BACKFILL, BACKFILL_ACTOR } from "@/lib/graphql/mutations";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Alert } from "@/components/ui/Alert";
import type { IsBackfillingResponse } from "@/types";

export default function BackfillPage() {
  const queryClient = useQueryClient();
  const [actorDid, setActorDid] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Check if backfill is running
  const { data: backfillStatus, isLoading: statusLoading } = useQuery({
    queryKey: ["backfillStatus"],
    queryFn: () => graphqlClient.request<IsBackfillingResponse>(IS_BACKFILLING),
    refetchInterval: 5000,
  });

  // Trigger full backfill mutation
  const triggerBackfillMutation = useMutation({
    mutationFn: () => graphqlClient.request(TRIGGER_BACKFILL),
    onSuccess: () => {
      setSuccess("Full backfill started successfully");
      setError(null);
      queryClient.invalidateQueries({ queryKey: ["backfillStatus"] });
      setTimeout(() => setSuccess(null), 5000);
    },
    onError: (err: Error) => {
      setError(err.message);
      setSuccess(null);
    },
  });

  // Backfill single actor mutation
  const backfillActorMutation = useMutation({
    mutationFn: (did: string) => graphqlClient.request(BACKFILL_ACTOR, { did }),
    onSuccess: () => {
      setSuccess(`Backfill started for ${actorDid}`);
      setError(null);
      setActorDid("");
      queryClient.invalidateQueries({ queryKey: ["backfillStatus"] });
      setTimeout(() => setSuccess(null), 5000);
    },
    onError: (err: Error) => {
      setError(err.message);
      setSuccess(null);
    },
  });

  const handleActorBackfill = (e: React.FormEvent) => {
    e.preventDefault();
    if (!actorDid.trim()) {
      setError("Please enter a DID");
      return;
    }
    if (!actorDid.startsWith("did:")) {
      setError("Invalid DID format. DIDs should start with 'did:'");
      return;
    }
    backfillActorMutation.mutate(actorDid.trim());
  };

  const isBackfilling = backfillStatus?.isBackfilling ?? false;

  return (
    <div className="pt-8 sm:pt-12 space-y-10">
      {/* Hero Section */}
      <div className="max-w-md">
        <h2 className="font-[family-name:var(--font-syne)] text-3xl sm:text-4xl leading-tight" style={{ color: "var(--foreground)" }}>
          Backfill
        </h2>
        <p className="mt-3 leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
          Sync historical data from the AT Protocol relay
        </p>
      </div>

      {/* Alerts */}
      {error && (
        <Alert variant="error" onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {success && (
        <Alert variant="success" onClose={() => setSuccess(null)}>
          {success}
        </Alert>
      )}

      {/* Status */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Status
        </h3>
        <div className="rounded-xl border p-6" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          {statusLoading ? (
            <div className="flex items-center gap-2" style={{ color: "var(--muted-foreground)" }}>
              <div className="w-4 h-4 rounded-full border-2 animate-spin" style={{ borderColor: "var(--border)", borderTopColor: "var(--muted-foreground)" }} />
              Checking status...
            </div>
          ) : isBackfilling ? (
            <div className="flex items-center gap-3 p-4 bg-blue-50/50 border border-blue-200/60 rounded-lg">
              <div className="w-5 h-5 rounded-full border-2 border-blue-300 border-t-blue-500 animate-spin" />
              <div>
                <div className="font-medium text-blue-700">Backfill in progress</div>
                <div className="text-sm text-blue-600/70">
                  Syncing records from the relay. This may take a while.
                </div>
              </div>
            </div>
          ) : (
            <div className="flex items-center gap-3 p-4 rounded-lg border" style={{ backgroundColor: "var(--accent)", borderColor: "var(--border)" }}>
              <svg className="h-5 w-5" style={{ color: "oklch(0.65 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
              </svg>
              <div>
                <div className="font-medium" style={{ color: "oklch(0.65 0.15 155)" }}>Idle</div>
                <div className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                  No backfill currently running
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Full Backfill */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Full Backfill
        </h3>
        <div className="rounded-xl border p-6 space-y-4" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
            Trigger a complete backfill of all known actors from the relay.
            This will fetch all historical records for actors that have been seen.
          </p>
          <div className="flex items-center gap-4">
            <Button
              variant="primary"
              onClick={() => triggerBackfillMutation.mutate()}
              disabled={triggerBackfillMutation.isPending || isBackfilling}
              loading={triggerBackfillMutation.isPending}
            >
              {triggerBackfillMutation.isPending ? (
                "Starting..."
              ) : isBackfilling ? (
                <>
                  <div className="w-4 h-4 rounded-full border-2 border-white/30 border-t-white animate-spin mr-2" />
                  Backfill Running
                </>
              ) : (
                <>
                  <svg className="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 0 1 0 1.972l-11.54 6.347a1.125 1.125 0 0 1-1.667-.986V5.653Z" />
                  </svg>
                  Start Full Backfill
                </>
              )}
            </Button>
            {isBackfilling && (
              <span className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                A backfill is already in progress
              </span>
            )}
          </div>

          <div className="p-4 bg-amber-50/50 border border-amber-200/60 rounded-lg">
            <div className="flex items-start gap-2">
              <svg className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
              </svg>
              <div className="text-sm text-amber-700">
                <strong>Note:</strong> Full backfill can take a long time depending on the number
                of actors and records. The process runs in the background and will continue
                even if you navigate away from this page.
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Actor Backfill */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Backfill Single Actor
        </h3>
        <div className="rounded-xl border p-6 space-y-4" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
            Fetch all historical records for a specific actor by their DID.
            Useful for quickly syncing a single user.
          </p>
          <form onSubmit={handleActorBackfill} className="space-y-4">
            <Input
              label="Actor DID"
              placeholder="did:plc:..."
              value={actorDid}
              onChange={(e) => setActorDid(e.target.value)}
              hint="Enter the full DID of the actor (e.g., did:plc:z72i7hdynmk6r22z27h6tvur)"
              className="font-mono"
            />
            <Button
              type="submit"
              variant="primary"
              disabled={backfillActorMutation.isPending || !actorDid.trim()}
              loading={backfillActorMutation.isPending}
            >
              {backfillActorMutation.isPending ? (
                "Starting..."
              ) : (
                <>
                  <svg className="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.347a1.125 1.125 0 0 1 0 1.972l-11.54 6.347a1.125 1.125 0 0 1-1.667-.986V5.653Z" />
                  </svg>
                  Backfill Actor
                </>
              )}
            </Button>
          </form>
        </div>
      </div>
    </div>
  );
}
