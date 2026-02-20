"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { graphqlClient } from "@/lib/graphql/client";
import { GET_SETTINGS, GET_OAUTH_CLIENTS } from "@/lib/graphql/queries";
import { UPDATE_SETTINGS, RESET_ALL, UPLOAD_LEXICONS } from "@/lib/graphql/mutations";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Input,
  Alert,
} from "@/components/ui";
import type { SettingsResponse, OAuthClientsResponse } from "@/types";

export default function SettingsPage() {
  const queryClient = useQueryClient();

  // Fetch settings
  const { data: settingsData, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: () => graphqlClient.request<SettingsResponse>(GET_SETTINGS),
  });

  // Fetch OAuth clients
  const { data: oauthData } = useQuery({
    queryKey: ["oauthClients"],
    queryFn: () => graphqlClient.request<OAuthClientsResponse>(GET_OAUTH_CLIENTS),
  });

  const settings = settingsData?.settings;
  const oauthClients = oauthData?.oauthClients ?? [];

  // Form state
  const [domainAuthority, setDomainAuthority] = useState("");
  const [relayUrl, setRelayUrl] = useState("");
  const [plcDirectoryUrl, setPlcDirectoryUrl] = useState("");
  const [jetstreamUrl, setJetstreamUrl] = useState("");
  const [oauthScopes, setOauthScopes] = useState("");
  const [resetConfirmation, setResetConfirmation] = useState("");
  const [alert, setAlert] = useState<{ type: "success" | "error"; message: string } | null>(null);

  // Update form when settings load
  useState(() => {
    if (settings) {
      setDomainAuthority(settings.domainAuthority);
      setRelayUrl(settings.relayUrl);
      setPlcDirectoryUrl(settings.plcDirectoryUrl);
      setJetstreamUrl(settings.jetstreamUrl);
      setOauthScopes(settings.oauthSupportedScopes);
    }
  });

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

  const handleSaveSettings = () => {
    updateMutation.mutate({
      domainAuthority: domainAuthority || undefined,
      relayUrl: relayUrl || undefined,
      plcDirectoryUrl: plcDirectoryUrl || undefined,
      jetstreamUrl: jetstreamUrl || undefined,
      oauthSupportedScopes: oauthScopes || undefined,
      adminDids: settings?.adminDids,
    });
  };

  const handleReset = () => {
    if (resetConfirmation === "RESET") {
      resetMutation.mutate("RESET");
    }
  };

  if (isLoading) {
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
            value={domainAuthority}
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
            value={relayUrl}
            onChange={(e) => setRelayUrl(e.target.value)}
          />
          <Input
            label="PLC Directory URL"
            placeholder="https://plc.directory"
            value={plcDirectoryUrl}
            onChange={(e) => setPlcDirectoryUrl(e.target.value)}
          />
          <Input
            label="Jetstream URL"
            placeholder="wss://jetstream2.us-west.bsky.network/subscribe"
            value={jetstreamUrl}
            onChange={(e) => setJetstreamUrl(e.target.value)}
          />
          <Input
            label="OAuth Supported Scopes"
            placeholder="atproto transition:generic"
            value={oauthScopes}
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

      {/* Admin DIDs */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Administrators
        </h3>
        <div className="rounded-xl border p-6" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          {settings?.adminDids.length === 0 ? (
            <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
              No administrators configured
            </p>
          ) : (
            <ul className="divide-y" style={{ borderColor: "var(--border)" }}>
              {settings?.adminDids.map((did) => (
                <li key={did} className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
                  <code className="text-sm font-mono" style={{ color: "var(--secondary-foreground)" }}>{did}</code>
                </li>
              ))}
            </ul>
          )}
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
