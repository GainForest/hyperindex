"use client";

import Image from "next/image";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";

import useDebounce from "@/lib/use-debounce";
import { Badge } from "@/components/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Button } from "@/components/ui/Button";

type SuggestedActor = {
  did: string;
  handle: string;
  displayName?: string;
  avatar?: string;
};

type SuggestedActorsResponse = {
  actors: SuggestedActor[];
};

type BlueskyProfilesResponse = {
  profiles?: SuggestedActor[];
};

interface AdminDidBatchPickerProps {
  existingAdminDids: string[];
  pendingDids: string[];
  disabled?: boolean;
  onAddDid: (did: string) => void;
  onRemoveDid: (did: string) => void;
}

const EMPTY_ACTORS: SuggestedActor[] = [];
const DID_PATTERN = /^did:[a-z0-9]+:[A-Za-z0-9._:%-]+(?:[:][A-Za-z0-9._:%-]+)*$/;
const BLUESKY_TYPEAHEAD_ENDPOINT = "https://public.api.bsky.app/xrpc/app.bsky.actor.searchActorsTypeahead";
const BLUESKY_PROFILES_ENDPOINT = "https://public.api.bsky.app/xrpc/app.bsky.actor.getProfiles";
const PROFILES_CHUNK_SIZE = 25;

export function AdminDidBatchPicker({
  existingAdminDids,
  pendingDids,
  disabled = false,
  onAddDid,
  onRemoveDid,
}: AdminDidBatchPickerProps) {
  const [search, setSearch] = useState("");
  const [manualDid, setManualDid] = useState("");
  const [manualError, setManualError] = useState<string | null>(null);
  const debouncedSearch = useDebounce(search, 300);
  const normalizedSearch = debouncedSearch.trim();

  const blockedDids = useMemo(
    () => new Set([...existingAdminDids, ...pendingDids]),
    [existingAdminDids, pendingDids],
  );

  const { data: suggestedActors = EMPTY_ACTORS, isFetching } = useQuery({
    queryKey: ["bluesky-typeahead", normalizedSearch],
    queryFn: async () => {
      const params = new URLSearchParams({
        q: normalizedSearch,
        limit: "10",
      });

      const response = await fetch(`${BLUESKY_TYPEAHEAD_ENDPOINT}?${params.toString()}`, {
        method: "GET",
        headers: {
          Accept: "application/json",
        },
      });

      if (!response.ok) {
        throw new Error("Failed to load suggestions");
      }

      const data = (await response.json()) as SuggestedActorsResponse;
      return data.actors ?? EMPTY_ACTORS;
    },
    enabled: !!normalizedSearch && !disabled,
  });

  const { data: pendingProfiles = EMPTY_ACTORS } = useQuery({
    queryKey: ["pending-admin-profiles", pendingDids],
    queryFn: async () => {
      const chunks: string[][] = [];
      for (let i = 0; i < pendingDids.length; i += PROFILES_CHUNK_SIZE) {
        chunks.push(pendingDids.slice(i, i + PROFILES_CHUNK_SIZE));
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
            return [] as SuggestedActor[];
          }

          const data = (await response.json()) as BlueskyProfilesResponse;
          return data.profiles ?? [];
        }),
      );

      return responses
        .filter((result): result is PromiseFulfilledResult<SuggestedActor[]> => result.status === "fulfilled")
        .flatMap((result) => result.value);
    },
    enabled: pendingDids.length > 0,
  });

  const pendingProfilesByDid = useMemo(
    () => new Map(pendingProfiles.map((profile) => [profile.did, profile])),
    [pendingProfiles],
  );

  const addDid = (did: string) => {
    const normalized = did.trim();
    if (!normalized || blockedDids.has(normalized)) {
      return;
    }

    onAddDid(normalized);
  };

  const handleManualAdd = () => {
    const did = manualDid.trim();
    if (!did) {
      return;
    }

    if (!DID_PATTERN.test(did)) {
      setManualError("Invalid DID format");
      return;
    }

    if (blockedDids.has(did)) {
      setManualError("This DID is already added");
      return;
    }

    setManualError(null);
    addDid(did);
    setManualDid("");
  };

  return (
    <div className="space-y-4">
      <div className="rounded-lg border" style={{ borderColor: "var(--border)", backgroundColor: "var(--card)" }}>
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search Bluesky users by handle or DID..."
            value={search}
            onValueChange={setSearch}
            disabled={disabled}
          />
          <CommandList>
            {isFetching ? <CommandEmpty>Loading suggestions...</CommandEmpty> : null}
            {!isFetching && !!normalizedSearch && suggestedActors.length === 0 ? (
              <CommandEmpty>No suggestions found.</CommandEmpty>
            ) : null}
            {!isFetching && suggestedActors.length > 0 ? (
              <CommandGroup heading="Suggestions">
                {suggestedActors.map((actor) => {
                  const isDisabled = blockedDids.has(actor.did);
                  return (
                    <CommandItem
                      key={actor.did}
                      disabled={isDisabled || disabled}
                      onSelect={() => {
                        addDid(actor.did);
                        setSearch("");
                      }}
                      className="py-2"
                    >
                      {actor.avatar ? (
                        <Image
                          src={actor.avatar}
                          alt={actor.handle ? `Avatar for @${actor.handle}` : actor.displayName ? `Avatar for ${actor.displayName}` : `Avatar for ${actor.did}`}
                          width={24}
                          height={24}
                          className="rounded-full"
                        />
                      ) : (
                        <div
                          className="flex size-6 items-center justify-center rounded-full text-[10px] font-semibold"
                          style={{ backgroundColor: "var(--muted)", color: "var(--muted-foreground)" }}
                        >
                          {(actor.displayName || actor.handle || actor.did).slice(0, 1).toUpperCase()}
                        </div>
                      )}
                      <div className="min-w-0">
                        <p className="truncate text-sm" style={{ color: "var(--foreground)" }}>
                          {actor.displayName || actor.handle}
                        </p>
                        <p className="truncate text-xs" style={{ color: "var(--muted-foreground)" }}>
                          @{actor.handle} · {actor.did}
                        </p>
                      </div>
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            ) : null}
          </CommandList>
        </Command>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
        <input
          type="text"
          value={manualDid}
          onChange={(e) => {
            setManualDid(e.target.value);
            if (manualError) {
              setManualError(null);
            }
          }}
          onKeyDown={(e) => {
            if (e.key !== "Enter") {
              return;
            }

            e.preventDefault();
            handleManualAdd();
          }}
          disabled={disabled}
          placeholder="Or paste DID manually (did:plc:...)"
          className="w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-2"
          style={{
            borderColor: manualError ? "var(--destructive)" : "var(--input)",
            backgroundColor: "var(--card)",
            color: "var(--foreground)",
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={disabled || !manualDid.trim()}
          onClick={handleManualAdd}
          className="w-full sm:w-auto"
        >
          Add to batch
        </Button>
      </div>

      {manualError ? (
        <p className="text-sm" style={{ color: "var(--destructive)" }}>
          {manualError}
        </p>
      ) : null}

      <div className="space-y-2">
        <p className="text-sm font-medium" style={{ color: "var(--foreground)" }}>
          Pending additions
        </p>
        {pendingDids.length === 0 ? (
          <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
            No DIDs selected yet.
          </p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {pendingDids.map((did) => {
              const profile = pendingProfilesByDid.get(did);
              const displayName = profile?.displayName || profile?.handle || did;

              return (
                <Badge
                  key={did}
                  className="gap-2 border py-1"
                  style={{
                    borderColor: "var(--border)",
                    backgroundColor: "var(--muted)",
                    color: "var(--foreground)",
                  }}
                >
                  {profile?.avatar ? (
                    <Image
                      src={profile.avatar}
                      alt={profile.handle ? `Avatar for @${profile.handle}` : profile.displayName ? `Avatar for ${profile.displayName}` : `Avatar for ${did}`}
                      width={18}
                      height={18}
                      className="rounded-full"
                    />
                  ) : (
                    <div
                      className="flex size-[18px] shrink-0 items-center justify-center rounded-full text-[9px] font-semibold"
                      style={{ backgroundColor: "var(--card)", color: "var(--muted-foreground)" }}
                    >
                      {displayName.slice(0, 1).toUpperCase()}
                    </div>
                  )}
                  <div className="flex max-w-[260px] min-w-0 flex-col leading-tight">
                    <span className="truncate text-[11px] font-medium">
                      {profile?.handle ? `@${profile.handle}` : displayName}
                    </span>
                    <span className="truncate font-mono text-[10px]" style={{ color: "var(--muted-foreground)" }}>
                      {did}
                    </span>
                  </div>
                  <button
                    type="button"
                    disabled={disabled}
                    onClick={() => onRemoveDid(did)}
                    className="rounded-full p-0.5 transition-opacity hover:opacity-80 disabled:opacity-40"
                    aria-label={`Remove ${did} from pending admins`}
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
