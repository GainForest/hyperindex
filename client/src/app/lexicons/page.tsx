"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { graphqlClient } from "@/lib/graphql/client";
import { GET_LEXICONS } from "@/lib/graphql/queries";
import { REGISTER_LEXICON, DELETE_LEXICON } from "@/lib/graphql/mutations";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Alert } from "@/components/ui/Alert";
import type { LexiconsResponse, Lexicon } from "@/types";

// NSID validation
function isValidNsid(nsid: string): boolean {
  const parts = nsid.split(".");
  if (parts.length < 3) return false;
  return parts.every((p) => /^[a-z][a-z0-9-]*$/i.test(p));
}

// Tree node structure
interface TreeNode {
  name: string;
  fullPath: string;
  lexicon?: Lexicon;
  children: Map<string, TreeNode>;
}

// Build hierarchical tree from flat lexicon list
function buildTree(lexicons: Lexicon[]): Map<string, TreeNode> {
  const root = new Map<string, TreeNode>();

  for (const lexicon of lexicons) {
    const parts = lexicon.id.split(".");
    const rootKey = parts.slice(0, 2).join(".");
    const remaining = parts.slice(2);

    if (!root.has(rootKey)) {
      root.set(rootKey, { name: rootKey, fullPath: rootKey, children: new Map() });
    }

    let current = root.get(rootKey)!.children;
    let path = rootKey;

    for (let i = 0; i < remaining.length; i++) {
      const part = remaining[i];
      path = `${path}.${part}`;

      if (!current.has(part)) {
        current.set(part, { name: part, fullPath: path, children: new Map() });
      }

      const node = current.get(part)!;
      if (i === remaining.length - 1) {
        node.lexicon = lexicon;
      }
      current = node.children;
    }
  }

  return root;
}

// Count leaf nodes
function countLeaves(node: TreeNode): number {
  let count = node.lexicon ? 1 : 0;
  for (const child of node.children.values()) {
    count += countLeaves(child);
  }
  return count;
}

// Get description from lexicon JSON
function getDescription(lexicon: Lexicon): string | null {
  try {
    const parsed = JSON.parse(lexicon.json);
    return parsed?.defs?.main?.description || parsed?.description || null;
  } catch {
    return null;
  }
}

// Tree Branch Component
function TreeBranch({
  node,
  isLast = false,
  prefix = "",
  isRoot = false,
  onDelete,
  deletingNsid,
  expandedId,
  onToggleExpand,
}: {
  node: TreeNode;
  isLast?: boolean;
  prefix?: string;
  isRoot?: boolean;
  onDelete: (nsid: string) => void;
  deletingNsid: string | null;
  expandedId: string | null;
  onToggleExpand: (id: string) => void;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const children = Array.from(node.children.entries()).sort(([a], [b]) => a.localeCompare(b));
  const hasChildren = children.length > 0;
  const branch = isLast ? "└── " : "├── ";
  const childPrefix = prefix + (isLast ? "    " : "│   ");

  // Root authority node (e.g., "org.impactindexer")
  if (isRoot) {
    return (
      <div className="mb-3 last:mb-0">
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="flex items-center gap-2 group py-0.5"
        >
          <span
            className="text-xs transition-transform duration-200"
            style={{ transform: collapsed ? "rotate(-90deg)" : "rotate(0deg)", color: "var(--border)" }}
          >
            ▾
          </span>
          <span className="font-mono text-sm font-medium" style={{ color: "var(--foreground)" }}>{node.name}</span>
          <span className="text-xs" style={{ color: "var(--border)" }}>{countLeaves(node)}</span>
        </button>
        {!collapsed && hasChildren && (
          <div className="mt-1">
            {children.map(([key, child], i) => (
              <TreeBranch
                key={key}
                node={child}
                isLast={i === children.length - 1}
                prefix="    "
                onDelete={onDelete}
                deletingNsid={deletingNsid}
                expandedId={expandedId}
                onToggleExpand={onToggleExpand}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  // Leaf node with lexicon
  if (node.lexicon) {
    const isExpanded = expandedId === node.lexicon.id;
    const isDeleting = deletingNsid === node.lexicon.id;
    const description = getDescription(node.lexicon);

    return (
      <div>
        <div className="group flex items-center py-0.5 -mx-1 px-1 rounded transition-colors hover:opacity-90">
          <span className="font-mono text-xs whitespace-pre select-none shrink-0 hidden sm:inline" style={{ color: "var(--border)" }}>
            {prefix}{branch}
          </span>
          <button
            onClick={() => onToggleExpand(node.lexicon!.id)}
            className="font-mono text-sm transition-colors text-left"
            style={{ color: "var(--primary)" }}
          >
            {node.name}
          </button>
          {description && (
            <span className="text-xs ml-2 truncate hidden sm:inline" style={{ color: "var(--border)" }}>
              {description}
            </span>
          )}
          <button
            onClick={(e) => {
              e.stopPropagation();
              if (!isDeleting) onDelete(node.lexicon!.id);
            }}
            disabled={isDeleting}
            className="opacity-0 group-hover:opacity-100 ml-auto p-1 text-red-500 hover:text-red-400 transition-all disabled:opacity-50"
            title={`Delete ${node.lexicon.id}`}
          >
            {isDeleting ? (
              <div className="w-3 h-3 rounded-full border-2 border-t-transparent animate-spin" style={{ borderColor: "var(--border)", borderTopColor: "var(--muted-foreground)" }} />
            ) : (
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
              </svg>
            )}
          </button>
        </div>
        {isExpanded && (
          <div className="ml-4 sm:ml-8 mt-2 mb-3">
            <pre className="text-xs p-3 rounded-lg overflow-x-auto border" style={{ backgroundColor: "var(--muted)", color: "var(--secondary-foreground)", borderColor: "var(--border)" }}>
              {JSON.stringify(JSON.parse(node.lexicon.json), null, 2)}
            </pre>
          </div>
        )}
      </div>
    );
  }

  // Intermediate directory node
  return (
    <div>
      <div className="flex items-center py-0.5 -mx-1 px-1 rounded transition-colors hover:opacity-90">
        <span className="font-mono text-xs whitespace-pre select-none shrink-0 hidden sm:inline" style={{ color: "var(--border)" }}>
          {prefix}{branch}
        </span>
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="flex items-center"
        >
          <span className="font-mono text-sm" style={{ color: "var(--muted-foreground)" }}>{node.name}</span>
          <span
            className="text-[10px] ml-1 transition-transform duration-200"
            style={{ transform: collapsed ? "rotate(-90deg)" : "rotate(0deg)", color: "var(--border)" }}
          >
            ▾
          </span>
        </button>
      </div>
      {!collapsed && hasChildren && (
        <div>
          {children.map(([key, child], i) => (
            <TreeBranch
              key={key}
              node={child}
              isLast={i === children.length - 1}
              prefix={childPrefix}
              onDelete={onDelete}
              deletingNsid={deletingNsid}
              expandedId={expandedId}
              onToggleExpand={onToggleExpand}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export default function LexiconsPage() {
  const queryClient = useQueryClient();
  const [searchQuery, setSearchQuery] = useState("");
  const [nsidInput, setNsidInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [deletingNsid, setDeletingNsid] = useState<string | null>(null);
  const [confirmNsid, setConfirmNsid] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [batchPending, setBatchPending] = useState(false);

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: ["lexicons"],
    queryFn: () => graphqlClient.request<LexiconsResponse>(GET_LEXICONS),
  });

  const registerMutation = useMutation({
    mutationFn: (nsid: string) =>
      graphqlClient.request(REGISTER_LEXICON, { nsid }),
  });

  const deleteMutation = useMutation({
    mutationFn: (nsid: string) =>
      graphqlClient.request(DELETE_LEXICON, { nsid }),
    onMutate: (nsid) => setDeletingNsid(nsid),
    onSuccess: (_, nsid) => {
      setSuccess(`Deleted ${nsid}`);
      setError(null);
      if (expandedId === nsid) setExpandedId(null);
      queryClient.invalidateQueries({ queryKey: ["lexicons"] });
      setTimeout(() => setSuccess(null), 3000);
    },
    onError: (err: Error) => {
      setError(err.message);
      setSuccess(null);
    },
    onSettled: () => setDeletingNsid(null),
  });

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!nsidInput.trim()) return;

    // Split by commas, newlines, or whitespace; trim and filter empty
    const nsids = [...new Set(
      nsidInput.split(/[,\n\s]+/).map((s) => s.trim()).filter(Boolean)
    )];

    // Validate all NSIDs before submitting any
    const invalidNsids = nsids.filter((nsid) => !isValidNsid(nsid));
    if (invalidNsids.length > 0) {
      setError(`Invalid NSID format: ${invalidNsids.join(", ")}`);
      return;
    }

    setError(null);
    setBatchPending(true);

    let completed = 0;
    let firstError: string | null = null;

    for (const nsid of nsids) {
      try {
        await registerMutation.mutateAsync(nsid);
        completed++;
        if (completed < nsids.length) {
          setSuccess(`Registered ${completed}/${nsids.length} lexicons...`);
        }
      } catch (err) {
        firstError = (err as Error).message;
        setError(`Failed to register ${nsid}: ${firstError}`);
        break;
      }
    }

    setBatchPending(false);

    if (completed > 0) {
      queryClient.invalidateQueries({ queryKey: ["lexicons"] });
      if (!firstError) {
        setNsidInput("");
        setSuccess(`Registered ${completed} lexicon${completed !== 1 ? "s" : ""}`);
        setTimeout(() => setSuccess(null), 3000);
      } else {
        setSuccess(`Registered ${completed}/${nsids.length} lexicons (stopped on error)`);
      }
    }
  };

  const filteredLexicons = useMemo(() => {
    if (!data?.lexicons) return [];
    if (!searchQuery) return data.lexicons;

    const query = searchQuery.toLowerCase();
    return data.lexicons.filter(
      (lex) =>
        lex.id.toLowerCase().includes(query) ||
        lex.json.toLowerCase().includes(query)
    );
  }, [data?.lexicons, searchQuery]);

  const tree = useMemo(() => buildTree(filteredLexicons), [filteredLexicons]);
  const roots = Array.from(tree.entries()).sort(([a], [b]) => a.localeCompare(b));
  const isConfirmDeleting = confirmNsid !== null && confirmNsid === deletingNsid;

  if (fetchError) {
    return (
      <div className="py-8">
        <Alert variant="error">Failed to load lexicons: {(fetchError as Error).message}</Alert>
      </div>
    );
  }

  return (
    <div className="py-8 space-y-6">
      {/* Header */}
      <div>
        <h2 className="font-[family-name:var(--font-syne)] text-2xl" style={{ color: "var(--foreground)" }}>
          Lexicons
        </h2>
        <p className="text-sm mt-1" style={{ color: "var(--muted-foreground)" }}>
          Register AT Protocol lexicon schemas for your AppView
        </p>
      </div>

      {/* Alerts */}
      {error && (
        <Alert variant="error" onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {success && <Alert variant="success">{success}</Alert>}

      {/* Register */}
      <form onSubmit={handleRegister} className="flex gap-2 items-start">
        <textarea
          value={nsidInput}
          onChange={(e) => {
            setNsidInput(e.target.value);
            setError(null);
          }}
          placeholder="Enter NSIDs (comma or newline separated)..."
          rows={1}
          className="flex-1 px-3 py-1.5 rounded-lg text-sm font-mono focus:outline-none focus:ring-1 transition-all resize-y"
          style={{
            backgroundColor: "var(--card)",
            borderColor: "var(--border)",
            color: "var(--foreground)",
            border: "1px solid var(--border)",
            minHeight: "2.25rem",
          }}
        />
        <Button
          type="submit"
          variant="primary"
          disabled={batchPending || !nsidInput.trim()}
          loading={batchPending}
        >
          Register
        </Button>
      </form>

      {/* Search */}
      <div className="relative">
        <svg className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5" style={{ color: "var(--border)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
        </svg>
        <input
          type="text"
          placeholder="Search..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full pl-9 pr-3 py-1.5 text-sm rounded-lg focus:outline-none focus:ring-1 transition-all"
          style={{
            backgroundColor: "var(--card)",
            borderColor: "var(--border)",
            color: "var(--foreground)",
            border: "1px solid var(--border)",
          }}
        />
      </div>

      {/* Tree */}
      {isLoading ? (
        <div className="flex items-center gap-2 py-8 justify-center">
          <div className="w-3 h-3 border-2 rounded-full animate-spin" style={{ borderColor: "var(--border)", borderTopColor: "var(--primary)" }} />
          <span className="text-xs" style={{ color: "var(--muted-foreground)" }}>Loading...</span>
        </div>
      ) : roots.length === 0 ? (
        <p className="text-sm py-4 text-center" style={{ color: "var(--muted-foreground)" }}>
          {searchQuery ? `No lexicons match "${searchQuery}"` : "No lexicons registered yet."}
        </p>
      ) : (
        <div className="font-mono">
          {roots.map(([key, node]) => (
            <TreeBranch
              key={key}
              node={node}
              isRoot
              onDelete={(nsid) => setConfirmNsid(nsid)}
              deletingNsid={deletingNsid}
              expandedId={expandedId}
              onToggleExpand={(id) => setExpandedId(expandedId === id ? null : id)}
            />
          ))}
        </div>
      )}

      {confirmNsid && (
        <div className="fixed inset-0 z-50 flex items-center justify-center px-4">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setConfirmNsid(null)}
          />
          <div
            className="relative w-full max-w-md rounded-xl border p-5 shadow-xl"
            style={{ backgroundColor: "var(--card)", borderColor: "var(--border)", color: "var(--foreground)" }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-lexicon-title"
          >
            <h3 id="delete-lexicon-title" className="text-base font-semibold">
              Delete lexicon
            </h3>
            <p className="mt-2 text-sm" style={{ color: "var(--muted-foreground)" }}>
              You are about to delete <span className="font-mono" style={{ color: "var(--foreground)" }}>{confirmNsid}</span>.
              This action is destructive and cannot be undone.
            </p>
            <div className="mt-4 flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setConfirmNsid(null)}
                disabled={isConfirmDeleting}
              >
                Cancel
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                loading={isConfirmDeleting}
                onClick={() => {
                  if (confirmNsid) {
                    deleteMutation.mutate(confirmNsid);
                    setConfirmNsid(null);
                  }
                }}
              >
                Delete
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
