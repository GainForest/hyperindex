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

// JSON Syntax Highlighter Component
function JsonHighlight({ json }: { json: string }) {
  const highlighted = useMemo(() => {
    try {
      const parsed = JSON.parse(json);
      const formatted = JSON.stringify(parsed, null, 2);

      return formatted
        .replace(/"([^"]+)":/g, '<span class="text-purple-600">"$1"</span>:')
        .replace(/: "([^"]+)"/g, ': <span class="text-emerald-600">"$1"</span>')
        .replace(/: (\d+)/g, ': <span class="text-amber-600">$1</span>')
        .replace(/: (true|false)/g, ': <span class="text-blue-600">$1</span>')
        .replace(/: (null)/g, ': <span class="text-zinc-400">$1</span>');
    } catch {
      return json;
    }
  }, [json]);

  return (
    <pre
      className="text-xs overflow-x-auto bg-zinc-50 p-4 rounded-lg text-zinc-700 font-mono border border-zinc-200/60"
      dangerouslySetInnerHTML={{ __html: highlighted }}
    />
  );
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

// Tree Branch Component
function TreeBranch({
  node,
  isLast = false,
  prefix = "",
  isRoot = false,
  onDelete,
  deletingNsid,
}: {
  node: TreeNode;
  isLast?: boolean;
  prefix?: string;
  isRoot?: boolean;
  onDelete: (nsid: string) => void;
  deletingNsid: string | null;
}) {
  const [expanded, setExpanded] = useState(true);
  const children = Array.from(node.children.entries()).sort(([a], [b]) => a.localeCompare(b));
  const hasChildren = children.length > 0;
  const branch = isLast ? "\u2514\u2500\u2500 " : "\u251C\u2500\u2500 ";
  const childPrefix = prefix + (isLast ? "    " : "\u2502   ");

  const description = useMemo(() => {
    if (!node.lexicon) return null;
    try {
      const parsed = JSON.parse(node.lexicon.json);
      return parsed?.defs?.main?.description || parsed?.description || null;
    } catch {
      return null;
    }
  }, [node.lexicon]);

  if (isRoot) {
    return (
      <div className="mb-4 last:mb-0">
        <div className="flex items-center gap-2 group py-1">
          <button
            onClick={() => setExpanded(!expanded)}
            className="flex items-center gap-2 cursor-pointer"
          >
            <span className={`text-zinc-400 text-xs transition-transform duration-200 ${expanded ? '' : '-rotate-90'}`}>
              {'\u25BE'}
            </span>
            <span className="font-mono text-sm font-medium text-zinc-800">{node.name}</span>
            <span className="text-zinc-400 text-xs">({countLeaves(node)})</span>
          </button>
        </div>
        {hasChildren && (
          <div
            className="grid transition-[grid-template-rows] duration-200 ease-out"
            style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
          >
            <div className="overflow-hidden">
              <div className="mt-1">
                {children.map(([key, child], i) => (
                  <TreeBranch
                    key={key}
                    node={child}
                    isLast={i === children.length - 1}
                    prefix="    "
                    onDelete={onDelete}
                    deletingNsid={deletingNsid}
                  />
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    );
  }

  if (node.lexicon) {
    const isDeleting = deletingNsid === node.lexicon.id;
    return (
      <LexiconLeafNode
        node={node}
        prefix={prefix}
        branch={branch}
        description={description}
        onDelete={onDelete}
        isDeleting={isDeleting}
      />
    );
  }

  return (
    <div>
      <div className="flex items-start py-1 hover:bg-zinc-50 -mx-1 px-1 rounded transition-colors group">
        <span className="font-mono text-xs text-zinc-300 whitespace-pre select-none leading-5 shrink-0">
          {prefix}{branch}
        </span>
        <button onClick={() => setExpanded(!expanded)} className="flex items-center cursor-pointer">
          <span className="font-mono text-sm text-zinc-500 leading-5">{node.name}</span>
          <span
            className={`text-zinc-300 text-[10px] ml-1.5 leading-5 transition-transform duration-200 ${expanded ? '' : '-rotate-90'}`}
          >
            {'\u25BE'}
          </span>
        </button>
      </div>
      {hasChildren && (
        <div
          className="grid transition-[grid-template-rows] duration-200 ease-out"
          style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
        >
          <div className="overflow-hidden">
            {children.map(([key, child], i) => (
              <TreeBranch
                key={key}
                node={child}
                isLast={i === children.length - 1}
                prefix={childPrefix}
                onDelete={onDelete}
                deletingNsid={deletingNsid}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Expandable Leaf Node Component
function LexiconLeafNode({
  node,
  prefix,
  branch,
  description,
  onDelete,
  isDeleting,
}: {
  node: TreeNode;
  prefix: string;
  branch: string;
  description: string | null;
  onDelete: (nsid: string) => void;
  isDeleting: boolean;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div>
      <div className="group flex items-start py-1 hover:bg-zinc-50 -mx-1 px-1 rounded transition-colors min-w-0">
        <span className="font-mono text-xs text-zinc-300 whitespace-pre select-none leading-5 shrink-0">
          {prefix}{branch}
        </span>
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-1 cursor-pointer min-w-0"
        >
          <svg className={`h-3 w-3 text-zinc-400 shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
          </svg>
          <svg className="h-3.5 w-3.5 text-emerald-500 shrink-0" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
          </svg>
          <span className="font-mono text-sm text-emerald-600 hover:text-emerald-700 leading-5">
            {node.name}
          </span>
        </button>
        {description && (
          <span className="text-xs text-zinc-400 ml-2 truncate leading-5 hidden sm:inline">
            {description}
          </span>
        )}
        <button
          onClick={(e) => {
            e.stopPropagation();
            if (node.lexicon && !isDeleting) {
              onDelete(node.lexicon.id);
            }
          }}
          disabled={isDeleting}
          className="opacity-0 group-hover:opacity-100 p-1 ml-auto text-zinc-400 hover:text-red-500 transition-all shrink-0 disabled:opacity-50"
          title={`Delete ${node.lexicon?.id}`}
        >
          {isDeleting ? (
            <div className="w-3.5 h-3.5 rounded-full border-2 border-zinc-300 border-t-zinc-500 animate-spin" />
          ) : (
            <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0" />
            </svg>
          )}
        </button>
      </div>
      {expanded && node.lexicon && (
        <div className="ml-8 mt-1 mb-2">
          <JsonHighlight json={node.lexicon.json} />
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

  const { data, isLoading, error: fetchError } = useQuery({
    queryKey: ["lexicons"],
    queryFn: () => graphqlClient.request<LexiconsResponse>(GET_LEXICONS),
  });

  const registerMutation = useMutation({
    mutationFn: (nsid: string) =>
      graphqlClient.request(REGISTER_LEXICON, { nsid }),
    onSuccess: (_, nsid) => {
      setSuccess(`Registered ${nsid}`);
      setError(null);
      setNsidInput("");
      queryClient.invalidateQueries({ queryKey: ["lexicons"] });
      setTimeout(() => setSuccess(null), 3000);
    },
    onError: (err: Error) => {
      setError(err.message);
      setSuccess(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (nsid: string) =>
      graphqlClient.request(DELETE_LEXICON, { nsid }),
    onMutate: (nsid) => {
      setDeletingNsid(nsid);
    },
    onSuccess: (_, nsid) => {
      setSuccess(`Deleted ${nsid}`);
      setError(null);
      queryClient.invalidateQueries({ queryKey: ["lexicons"] });
      setTimeout(() => setSuccess(null), 3000);
    },
    onError: (err: Error) => {
      setError(err.message);
      setSuccess(null);
    },
    onSettled: () => {
      setDeletingNsid(null);
    },
  });

  const handleRegister = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = nsidInput.trim();
    if (!trimmed) return;

    if (!isValidNsid(trimmed)) {
      setError("Invalid NSID format. Expected something like org.hypercerts.claim.activity");
      return;
    }

    setError(null);
    registerMutation.mutate(trimmed);
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

  if (fetchError) {
    return (
      <div className="pt-8 sm:pt-12">
        <Alert variant="error">Failed to load lexicons: {(fetchError as Error).message}</Alert>
      </div>
    );
  }

  return (
    <div className="pt-8 sm:pt-12 space-y-10">
      {/* Hero Section */}
      <div className="max-w-md">
        <h2 className="font-[family-name:var(--font-garamond)] text-3xl sm:text-4xl text-zinc-900 leading-tight">
          Lexicons
        </h2>
        <p className="text-zinc-500 mt-3 leading-relaxed">
          Register and manage AT Protocol lexicon schemas for your AppView
        </p>
      </div>

      {/* Alerts */}
      {error && (
        <Alert variant="error" onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {success && (
        <Alert variant="success">{success}</Alert>
      )}

      {/* Register Lexicon */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-garamond)] text-xl text-zinc-900">
          Register Lexicon
        </h3>
        <div className="rounded-xl border border-zinc-200/60 bg-white p-6">
          <form onSubmit={handleRegister} className="flex gap-3">
            <div className="flex-1">
              <Input
                placeholder="Enter NSID (e.g., org.hypercerts.claim.activity)"
                value={nsidInput}
                onChange={(e) => {
                  setNsidInput(e.target.value);
                  setError(null);
                }}
                className="font-mono"
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={registerMutation.isPending || !nsidInput.trim()}
              loading={registerMutation.isPending}
            >
              {registerMutation.isPending ? "Resolving..." : "Register"}
            </Button>
          </form>
          <p className="text-xs text-zinc-400 mt-2">
            The lexicon will be resolved via DNS and fetched from the authoritative PDS.
          </p>
        </div>
      </div>

      {/* Search */}
      <div className="relative">
        <svg className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-zinc-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
        </svg>
        <input
          type="text"
          placeholder="Search lexicons..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full pl-10 px-3 py-2 text-sm bg-white/50 border border-zinc-200/60 rounded-lg
                     text-zinc-800 placeholder:text-zinc-300
                     focus:outline-none focus:ring-2 focus:ring-emerald-500/30 focus:border-emerald-400
                     transition-all"
        />
      </div>

      {/* Lexicon Tree */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <h3 className="font-[family-name:var(--font-garamond)] text-xl text-zinc-900">
            Registered Lexicons
          </h3>
          {data?.lexicons && (
            <span className="text-sm text-zinc-400">
              ({filteredLexicons.length} of {data.lexicons.length})
            </span>
          )}
        </div>

        <div className="rounded-xl border border-zinc-200/60 bg-white p-6">
          {isLoading ? (
            <div className="flex items-center justify-center py-8 gap-2">
              <div className="w-4 h-4 rounded-full border-2 border-zinc-200 border-t-zinc-400 animate-spin" />
              <span className="text-zinc-400 text-sm">Loading lexicons...</span>
            </div>
          ) : roots.length === 0 ? (
            <div className="text-center py-8 text-zinc-400 text-sm">
              {searchQuery
                ? "No lexicons match your search"
                : "No lexicons registered. Enter an NSID above to get started."}
            </div>
          ) : (
            <div className="font-mono">
              {roots.map(([key, node]) => (
                <TreeBranch
                  key={key}
                  node={node}
                  isRoot
                  onDelete={(nsid) => deleteMutation.mutate(nsid)}
                  deletingNsid={deletingNsid}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
