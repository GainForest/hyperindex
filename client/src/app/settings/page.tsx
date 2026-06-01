"use client";

import Image from "next/image";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { graphqlClient } from "@/lib/graphql/client";
import { GET_SETTINGS, GET_OAUTH_CLIENTS, GET_PURGE_ACTOR_PREVIEW } from "@/lib/graphql/queries";
import { REMOVE_LABELER_SUBSCRIBE_URL, UPDATE_SETTINGS, RESET_ALL, PURGE_ACTOR } from "@/lib/graphql/mutations";
import { useAuth, useAdminSession } from "@/lib/auth";
import {
  Button,
  Input,
  Alert,
} from "@/components/ui";
import type { SettingsResponse, OAuthClientsResponse, PurgeActorPreviewResponse } from "@/types";

type BlueskyProfile = {
  did: string;
  handle: string;
  displayName?: string;
  avatar?: string;
};

type BlueskyProfilesResponse = {
  profiles?: BlueskyProfile[];
};

const BLUESKY_PROFILES_ENDPOINT = "https://public.api.bsky.app/xrpc/app.bsky.actor.getProfiles";
const PROFILES_CHUNK_SIZE = 25;

export default function SettingsPage() {
  const router = useRouter();
  const { isLoading: isAuthLoading } = useAuth();
  const { isAdmin: hasAdminAccess, isLoading: isAdminLoading } = useAdminSession();
  const queryClient = useQueryClient();

  useEffect(() => {
    if (isAuthLoading || isAdminLoading) {
      return;
    }

    if (!hasAdminAccess) {
      router.replace("/");
    }
  }, [hasAdminAccess, isAdminLoading, isAuthLoading, router]);

  // Fetch settings
  const { data: settingsData, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: () => graphqlClient.request<SettingsResponse>(GET_SETTINGS),
    enabled: hasAdminAccess,
  });

  // Fetch OAuth clients
  const { data: oauthData } = useQuery({
    queryKey: ["oauthClients"],
    queryFn: () => graphqlClient.request<OAuthClientsResponse>(GET_OAUTH_CLIENTS),
    enabled: hasAdminAccess,
  });

  const settings = settingsData?.settings;
  const oauthClients = oauthData?.oauthClients ?? [];
  const adminDids = settings?.adminDids ?? [];
  const labelerSubscribeUrls = settings?.labelerSubscribeUrls ?? [];

  const { data: adminProfiles = [], isFetching: isFetchingAdminProfiles } = useQuery({
    queryKey: ["admin-profiles", adminDids],
    queryFn: async () => {
      const chunks: string[][] = [];
      for (let i = 0; i < adminDids.length; i += PROFILES_CHUNK_SIZE) {
        chunks.push(adminDids.slice(i, i + PROFILES_CHUNK_SIZE));
      }

      const responses = await Promise.allSettled(
        chunks.map(async (chunk) => {
          const params = new URLSearchParams();
          for (const did of chunk) {
            params.append("actors", did);
          }

          const response = await fetch(`${BLUESKY_PROFILES_ENDPOINT}?${params.toString()}`, {
            method: "GET",
            headers: {
              Accept: "application/json",
            },
          });

          if (!response.ok) {
            return [] as BlueskyProfile[];
          }

          const data = (await response.json()) as BlueskyProfilesResponse;
          return data.profiles ?? [];
        }),
      );

      return responses
        .filter((result): result is PromiseFulfilledResult<BlueskyProfile[]> => result.status === "fulfilled")
        .flatMap((result) => result.value);
    },
    enabled: hasAdminAccess && adminDids.length > 0,
  });

  const adminProfilesByDid = useMemo(
    () => new Map(adminProfiles.map((profile) => [profile.did, profile])),
    [adminProfiles],
  );

  // Form state
  const [domainAuthority, setDomainAuthority] = useState<string | null>(null);
  const [relayUrl, setRelayUrl] = useState<string | null>(null);
  const [plcDirectoryUrl, setPlcDirectoryUrl] = useState<string | null>(null);
  const [jetstreamUrl, setJetstreamUrl] = useState<string | null>(null);
  const [oauthScopes, setOauthScopes] = useState<string | null>(null);
  const [purgeDid, setPurgeDid] = useState("");
  const [purgeConfirm, setPurgeConfirm] = useState("");
  const [resetConfirmation, setResetConfirmation] = useState("");
  const [removingLabelerUrl, setRemovingLabelerUrl] = useState<string | null>(null);
  const [alert, setAlert] = useState<{ type: "success" | "error"; message: string } | null>(null);

  const normalizedPurgeDid = purgeDid.trim();
  const hasPurgeDid = normalizedPurgeDid.length > 0;

  const { data: purgePreviewData, isFetching: isFetchingPurgePreview } = useQuery({
    queryKey: ["purge-actor-preview", normalizedPurgeDid],
    queryFn: () =>
      graphqlClient.request<PurgeActorPreviewResponse>(GET_PURGE_ACTOR_PREVIEW, {
        did: normalizedPurgeDid,
      }),
    enabled: hasAdminAccess && hasPurgeDid,
  });

  const purgePreview = purgePreviewData?.purgeActorPreview;

  // Update settings mutation
  const updateMutation = useMutation({
    mutationFn: (variables: Record<string, unknown>) =>
      graphqlClient.request(UPDATE_SETTINGS, variables),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      setAlert({ type: "success", message: "Settings updated successfully" });
    },
    onError: (error: Error) => {
      setAlert({ type: "error", message: error.message });
    },
  });

  // Remove labeler subscription mutation
  const removeLabelerMutation = useMutation({
    mutationFn: (url: string) =>
      graphqlClient.request(REMOVE_LABELER_SUBSCRIBE_URL, { url }),
    onMutate: (url: string) => {
      setRemovingLabelerUrl(url);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      setAlert({ type: "success", message: "Labeler URL removed. Restart Hyperindex to stop any already-running subscription." });
    },
    onError: (error: Error) => {
      setAlert({ type: "error", message: error.message });
    },
    onSettled: () => {
      setRemovingLabelerUrl(null);
    },
  });

  // Reset mutation
  const resetMutation = useMutation({
    mutationFn: (confirm: string) =>
      graphqlClient.request(RESET_ALL, { confirm }),
    onSuccess: () => {
      queryClient.invalidateQueries();
      setResetConfirmation("");
      setAlert({ type: "success", message: "All data has been reset" });
    },
    onError: (error: Error) => {
      setAlert({ type: "error", message: error.message });
    },
  });

  // Purge actor mutation
  const purgeActorMutation = useMutation({
    mutationFn: (variables: { did: string; confirm: string }) =>
      graphqlClient.request<{ purgeActor: boolean }>(PURGE_ACTOR, variables),
    onSuccess: () => {
      queryClient.invalidateQueries();
      setPurgeDid("");
      setPurgeConfirm("");
      setAlert({ type: "success", message: "Actor data purged successfully" });
    },
    onError: (error: Error) => {
      setAlert({ type: "error", message: error.message });
    },
  });

  const handleSaveSettings = () => {
    const nextDomainAuthority = domainAuthority ?? settings?.domainAuthority ?? "";
    const nextRelayURL = relayUrl ?? settings?.relayUrl ?? "";
    const nextPLCDirectoryURL = plcDirectoryUrl ?? settings?.plcDirectoryUrl ?? "";
    const nextJetstreamURL = jetstreamUrl ?? settings?.jetstreamUrl ?? "";
    const nextOAuthScopes = oauthScopes ?? settings?.oauthSupportedScopes ?? "";

    updateMutation.mutate({
      domainAuthority: nextDomainAuthority || undefined,
      relayUrl: nextRelayURL || undefined,
      plcDirectoryUrl: nextPLCDirectoryURL || undefined,
      jetstreamUrl: nextJetstreamURL || undefined,
      oauthSupportedScopes: nextOAuthScopes || undefined,
    });
  };

  const handleReset = () => {
    if (resetConfirmation === "RESET") {
      resetMutation.mutate("RESET");
    }
  };

  const handleRemoveLabelerUrl = (url: string) => {
    removeLabelerMutation.mutate(url);
  };

  const handlePurgeActor = () => {
    purgeActorMutation.mutate({
      did: normalizedPurgeDid,
      confirm: purgeConfirm,
    });
  };

  if (isAuthLoading || isAdminLoading || !hasAdminAccess || isLoading) {
    return (
      <div className="pt-8 sm:pt-12 space-y-6">
        {[...Array(3)].map((_, i) => (
          <div key={i} className="h-48 animate-pulse rounded-xl" style={{ backgroundColor: "var(--muted)" }} />
        ))}
      </div>
    );
  }

  return (
    <div className="pt-8 sm:pt-12 space-y-10">
      {/* Hero Section */}
      <div className="max-w-md">
        <h2 className="font-[family-name:var(--font-syne)] text-3xl sm:text-4xl leading-tight" style={{ color: "var(--foreground)" }}>
          Settings
        </h2>
        <p className="mt-3 leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
          Configure your Hyperindex AppView instance
        </p>
      </div>

      {alert && (
        <Alert variant={alert.type === "success" ? "success" : "error"}>
          {alert.message}
        </Alert>
      )}

      {/* Basic Settings */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Basic Settings
        </h3>
        <div className="rounded-xl border p-6 space-y-4" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
            <Input
              label="Domain Authority"
              placeholder="your-domain.com"
              value={domainAuthority ?? settings?.domainAuthority ?? ""}
              onChange={(e) => setDomainAuthority(e.target.value)}
              hint="The domain that owns this AppView instance"
            />
          <div className="flex justify-end pt-2">
            <Button
              variant="primary"
              onClick={handleSaveSettings}
              loading={updateMutation.isPending}
            >
              Save Settings
            </Button>
          </div>
        </div>
      </div>

      {/* External Services */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          External Services
        </h3>
        <div className="rounded-xl border p-6 space-y-4" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
            <Input
              label="Relay URL"
              placeholder="https://relay1.us-west.bsky.network"
              value={relayUrl ?? settings?.relayUrl ?? ""}
              onChange={(e) => setRelayUrl(e.target.value)}
            />
            <Input
              label="PLC Directory URL"
              placeholder="https://plc.directory"
              value={plcDirectoryUrl ?? settings?.plcDirectoryUrl ?? ""}
              onChange={(e) => setPlcDirectoryUrl(e.target.value)}
            />
            <Input
              label="Jetstream URL"
              placeholder="wss://jetstream2.us-west.bsky.network/subscribe"
              value={jetstreamUrl ?? settings?.jetstreamUrl ?? ""}
              onChange={(e) => setJetstreamUrl(e.target.value)}
            />
            <Input
              label="OAuth Supported Scopes"
              placeholder="atproto transition:generic"
              value={oauthScopes ?? settings?.oauthSupportedScopes ?? ""}
              onChange={(e) => setOauthScopes(e.target.value)}
            />
          <div className="flex justify-end pt-2">
            <Button
              variant="primary"
              onClick={handleSaveSettings}
              loading={updateMutation.isPending}
            >
              Save Settings
            </Button>
          </div>
        </div>
      </div>

      {/* External Labelers */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          External Labelers
        </h3>
        <div className="rounded-xl border p-6 space-y-4" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <div>
            <p className="text-sm font-medium" style={{ color: "var(--foreground)" }}>
              Configured labeler subscription URLs
            </p>
            <p className="mt-1 text-xs" style={{ color: "var(--muted-foreground)" }}>
              Removing a URL updates the persisted configuration. Restart Hyperindex to stop any subscription that is already running.
            </p>
          </div>

          {!settings?.labelerSubscribeEnabled ? (
            <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
              External labeler subscriptions are disabled for this instance.
            </p>
          ) : labelerSubscribeUrls.length === 0 ? (
            <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
              No external labeler URLs configured.
            </p>
          ) : (
            <ul className="divide-y" style={{ borderColor: "var(--border)" }}>
              {labelerSubscribeUrls.map((url) => (
                <li key={url} className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between">
                  <code className="break-all text-xs font-mono" style={{ color: "var(--muted-foreground)" }}>
                    {url}
                  </code>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => handleRemoveLabelerUrl(url)}
                    loading={removingLabelerUrl === url}
                    disabled={removingLabelerUrl !== null && removingLabelerUrl !== url}
                  >
                    Remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Admin DIDs */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Administrators
        </h3>
        <div className="rounded-xl border p-6 space-y-6" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <div className="space-y-3">
            <p className="text-sm font-medium" style={{ color: "var(--foreground)" }}>
              Current administrators
            </p>
            {adminDids.length === 0 ? (
              <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                No administrators configured
              </p>
            ) : (
              <div className="space-y-2">
                {isFetchingAdminProfiles ? (
                  <p className="text-xs" style={{ color: "var(--muted-foreground)" }}>
                    Resolving Bluesky handles...
                  </p>
                ) : null}
                <ul className="divide-y" style={{ borderColor: "var(--border)" }}>
                  {adminDids.map((did) => {
                    const profile = adminProfilesByDid.get(did);
                    const displayName = profile?.displayName || profile?.handle || did;
                    const handle = profile?.handle;

                    return (
                      <li key={did} className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
                        <div className="min-w-0 flex items-center gap-3">
                          {profile?.avatar ? (
                            <Image
                              src={profile.avatar}
                              alt={handle ? `Avatar for @${handle}` : profile?.displayName ? `Avatar for ${profile.displayName}` : `Avatar for ${did}`}
                              width={28}
                              height={28}
                              className="rounded-full"
                            />
                          ) : (
                            <div
                              className="flex size-7 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold"
                              style={{ backgroundColor: "var(--muted)", color: "var(--muted-foreground)" }}
                            >
                              {(displayName || did).slice(0, 1).toUpperCase()}
                            </div>
                          )}
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium" style={{ color: "var(--foreground)" }}>
                              {displayName}
                            </p>
                            {handle ? (
                              <p className="truncate text-xs" style={{ color: "var(--muted-foreground)" }}>
                                @{handle}
                              </p>
                            ) : null}
                            <code className="truncate text-xs font-mono" style={{ color: "var(--muted-foreground)" }}>
                              {did}
                            </code>
                          </div>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </div>
            )}
          </div>

          <div className="space-y-4">
            <p className="text-sm font-medium" style={{ color: "var(--foreground)" }}>
              Managed via environment variables
            </p>
            <div className="rounded-lg border p-4 text-sm" style={{ borderColor: "var(--border)", backgroundColor: "var(--muted)" }}>
              <p style={{ color: "var(--foreground)" }}>
                Administrator access is read-only in the app.
              </p>
              <p className="mt-2" style={{ color: "var(--muted-foreground)" }}>
                Update <code className="font-mono">ADMIN_DIDS</code> on the Hyperindex backend to change this list.
              </p>
            </div>
            <p className="text-xs" style={{ color: "var(--muted-foreground)" }}>
              The backend <code className="font-mono">ADMIN_DIDS</code> value is the source of truth for authorization.
            </p>
          </div>
        </div>
      </div>

      {/* OAuth Clients */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          OAuth Clients
        </h3>
        <div className="rounded-xl border p-6" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          {oauthClients.length === 0 ? (
            <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
              No OAuth clients registered
            </p>
          ) : (
            <ul className="divide-y" style={{ borderColor: "var(--border)" }}>
              {oauthClients.map((client) => (
                <li key={client.clientId} className="py-3 first:pt-0 last:pb-0">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="font-medium" style={{ color: "var(--foreground)" }}>
                        {client.clientName}
                      </p>
                      <code className="text-xs font-mono" style={{ color: "var(--muted-foreground)" }}>{client.clientId}</code>
                    </div>
                    <span className="rounded-full px-2 py-1 text-xs" style={{ backgroundColor: "var(--muted)", color: "var(--secondary-foreground)" }}>
                      {client.clientType}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Danger Zone */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl text-red-600">
          Danger Zone
        </h3>
        <div className="rounded-xl border border-red-200/60 bg-red-50/30 p-6 space-y-4">
          <div className="space-y-4 pb-4 border-b border-red-200/60">
            <p className="text-sm" style={{ color: "var(--secondary-foreground)" }}>
              Purge a single DID from the index. This removes all indexed records and the actor row.
            </p>
            <Input
              label="Actor DID"
              placeholder="did:plc:..."
              value={purgeDid}
              onChange={(e) => setPurgeDid(e.target.value)}
            />
            <Input
              label="Type PURGE to confirm"
              placeholder="PURGE"
              value={purgeConfirm}
              onChange={(e) => setPurgeConfirm(e.target.value)}
            />
            <div className="flex justify-end">
              <Button
                variant="destructive"
              onClick={handlePurgeActor}
                disabled={!hasPurgeDid || purgeConfirm !== "PURGE"}
                loading={purgeActorMutation.isPending}
              >
                Purge Actor Data
              </Button>
            </div>
            {hasPurgeDid ? (
              <div className="rounded-lg border px-3 py-2 text-xs" style={{ borderColor: "var(--border)", color: "var(--secondary-foreground)", backgroundColor: "var(--card)" }}>
                {isFetchingPurgePreview ? (
                  <p>Checking current index impact…</p>
                ) : purgePreview ? (
                  <>
                    <p><strong>DID:</strong> <code>{purgePreview.did}</code></p>
                    <p><strong>Valid DID:</strong> {purgePreview.isValidDid ? "yes" : "no"}</p>
                    <p><strong>Actor exists:</strong> {purgePreview.actorExists ? "yes" : "no"}</p>
                    <p><strong>Indexed records to delete:</strong> {purgePreview.recordCount}</p>
                  </>
                ) : (
                  <p>Unable to load purge preview.</p>
                )}
              </div>
            ) : null}
          </div>

          <p className="text-sm" style={{ color: "var(--secondary-foreground)" }}>
            Reset all data including records, actors, and activity. This action cannot be undone.
          </p>
          <div className="flex flex-col sm:flex-row items-start sm:items-end gap-4">
            <div className="w-full sm:w-auto">
              <Input
                label="Type RESET to confirm"
                placeholder="RESET"
                value={resetConfirmation}
                onChange={(e) => setResetConfirmation(e.target.value)}
              />
            </div>
            <Button
              variant="destructive"
              onClick={handleReset}
              disabled={resetConfirmation !== "RESET"}
              loading={resetMutation.isPending}
            >
              Reset All Data
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
