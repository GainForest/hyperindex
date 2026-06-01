"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { publicGraphqlClient } from "@/lib/graphql/client";
import { buildConnectionQueryDocument, stableStringify } from "@/lib/graphql/query-builder";
import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";

const INTROSPECTION_QUERY = `
  query QueryBuilderIntrospection {
    __schema {
      queryType {
        fields {
          name
          description
          args {
            name
            description
            defaultValue
            type { ...TypeRef }
          }
          type { ...TypeRef }
        }
      }
      types {
        kind
        name
        description
        fields(includeDeprecated: true) {
          name
          description
          type { ...TypeRef }
        }
        inputFields {
          name
          description
          type { ...TypeRef }
        }
        enumValues(includeDeprecated: true) {
          name
          description
        }
      }
    }
  }

  fragment TypeRef on __Type {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
`;

type IntrospectionTypeRef = {
  kind: string;
  name: string | null;
  ofType?: IntrospectionTypeRef | null;
};

type IntrospectionArg = {
  name: string;
  description: string | null;
  defaultValue: string | null;
  type: IntrospectionTypeRef;
};

type IntrospectionField = {
  name: string;
  description: string | null;
  args?: IntrospectionArg[] | null;
  type: IntrospectionTypeRef;
};

type IntrospectionInputField = {
  name: string;
  description: string | null;
  type: IntrospectionTypeRef;
};

type IntrospectionEnumValue = {
  name: string;
  description: string | null;
};

type IntrospectionType = {
  kind: string;
  name: string | null;
  description: string | null;
  fields?: IntrospectionField[] | null;
  inputFields?: IntrospectionInputField[] | null;
  enumValues?: IntrospectionEnumValue[] | null;
};

type IntrospectionResponse = {
  __schema: {
    queryType: {
      fields: IntrospectionField[];
    };
    types: IntrospectionType[];
  };
};

type FilterRow = {
  id: string;
  field: string;
  operator: string;
  value: string;
};

type SelectableField = {
  name: string;
  description: string | null;
  snippet: string;
};

const KNOWN_FILTER_OPERATORS = new Set([
  "eq",
  "neq",
  "gt",
  "lt",
  "gte",
  "lte",
  "in",
  "contains",
  "startsWith",
  "isNull",
]);

function unwrapType(type: IntrospectionTypeRef): { kind: string; name: string; isList: boolean; isRequired: boolean } {
  let current: IntrospectionTypeRef | null | undefined = type;
  let isList = false;
  let isRequired = false;

  while (current) {
    if (current.kind === "NON_NULL") {
      isRequired = true;
      current = current.ofType;
      continue;
    }
    if (current.kind === "LIST") {
      isList = true;
      current = current.ofType;
      continue;
    }
    return { kind: current.kind, name: current.name ?? "", isList, isRequired };
  }

  return { kind: "", name: "", isList, isRequired };
}

function typeToString(type: IntrospectionTypeRef): string {
  if (type.kind === "NON_NULL" && type.ofType) {
    return `${typeToString(type.ofType)}!`;
  }
  if (type.kind === "LIST" && type.ofType) {
    return `[${typeToString(type.ofType)}]`;
  }
  return type.name ?? "String";
}

function isConnectionField(field: IntrospectionField, typeMap: Map<string, IntrospectionType>): boolean {
  const unwrapped = unwrapType(field.type);
  const type = typeMap.get(unwrapped.name);
  return Boolean(
    type?.kind === "OBJECT" &&
      unwrapped.name.endsWith("Connection") &&
      type.fields?.some((child) => child.name === "edges")
  );
}

function getNodeTypeName(field: IntrospectionField, typeMap: Map<string, IntrospectionType>): string | null {
  const connectionType = typeMap.get(unwrapType(field.type).name);
  const edgeField = connectionType?.fields?.find((child) => child.name === "edges");
  if (!edgeField) return null;
  const edgeType = typeMap.get(unwrapType(edgeField.type).name);
  const nodeField = edgeType?.fields?.find((child) => child.name === "node");
  return nodeField ? unwrapType(nodeField.type).name : null;
}

function buildSelectableFields(nodeType: IntrospectionType | undefined): SelectableField[] {
  return (nodeType?.fields ?? [])
    .map((field) => {
      const unwrapped = unwrapType(field.type);
      if (unwrapped.kind === "SCALAR" || unwrapped.kind === "ENUM") {
        return { name: field.name, description: field.description, snippet: field.name };
      }
      if (field.name === "externalLabels") {
        return {
          name: field.name,
          description: field.description,
          snippet: "externalLabels {\n  src\n  uri\n  cid\n  val\n  neg\n  cts\n  exp\n  ver\n}",
        };
      }
      return null;
    })
    .filter((field): field is SelectableField => field !== null);
}

function defaultFieldSelection(fields: SelectableField[]): string[] {
  const preferred = ["uri", "cid", "did", "rkey", "collection", "value"];
  const available = new Set(fields.map((field) => field.name));
  const defaults = preferred.filter((field) => available.has(field));
  return defaults.length > 0 ? defaults : fields.slice(0, 6).map((field) => field.name);
}

function parseArgumentValue(value: string, type: IntrospectionTypeRef): unknown {
  const unwrapped = unwrapType(type);
  if (unwrapped.isList) {
    return value
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean);
  }

  if (unwrapped.name === "Int") return Number.parseInt(value, 10);
  if (unwrapped.name === "Float") return Number.parseFloat(value);
  if (unwrapped.name === "Boolean") return value.toLowerCase() === "true";
  return value;
}

function parseFilterValue(value: string, operatorType: IntrospectionTypeRef): unknown {
  const unwrapped = unwrapType(operatorType);
  if (unwrapped.isList) {
    return value
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean)
      .map((part) => parseArgumentValue(part, operatorType.ofType ?? operatorType));
  }

  if (unwrapped.name === "Boolean") {
    if (!value.trim()) return true;
    return value.toLowerCase() === "true";
  }
  return parseArgumentValue(value, operatorType);
}

function formatFieldLabel(field: IntrospectionField): string {
  const description = field.description?.replace(/^Query\s+/, "").replace(/\s+records$/, "");
  return description && description !== field.name ? description : field.name;
}

function newFilterRow(field: string, operator: string): FilterRow {
  return { id: crypto.randomUUID(), field, operator, value: "" };
}

export default function QueryBuilderPage() {
  const [selectedFieldName, setSelectedFieldName] = useState("");
  const [selectedFieldNames, setSelectedFieldNames] = useState<string[]>([]);
  const [argumentValues, setArgumentValues] = useState<Record<string, string>>({ first: "20" });
  const [filters, setFilters] = useState<FilterRow[]>([]);
  const [copied, setCopied] = useState(false);
  const [result, setResult] = useState<unknown>(null);
  const [runError, setRunError] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);

  const { data, isLoading, error } = useQuery({
    queryKey: ["query-builder-introspection"],
    queryFn: () => publicGraphqlClient.request<IntrospectionResponse>(INTROSPECTION_QUERY),
  });

  const typeMap = useMemo(() => {
    return new Map((data?.__schema.types ?? []).filter((type) => type.name).map((type) => [type.name!, type]));
  }, [data]);

  const connectionFields = useMemo(() => {
    return (data?.__schema.queryType.fields ?? [])
      .filter((field) => isConnectionField(field, typeMap))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [data, typeMap]);

  useEffect(() => {
    if (!selectedFieldName && connectionFields.length > 0) {
      setSelectedFieldName(connectionFields[0].name);
    }
  }, [connectionFields, selectedFieldName]);

  const selectedField = connectionFields.find((field) => field.name === selectedFieldName) ?? connectionFields[0];
  const selectedNodeTypeName = selectedField ? getNodeTypeName(selectedField, typeMap) : null;
  const selectableFields = useMemo(
    () => buildSelectableFields(selectedNodeTypeName ? typeMap.get(selectedNodeTypeName) : undefined),
    [selectedNodeTypeName, typeMap]
  );

  useEffect(() => {
    setSelectedFieldNames(defaultFieldSelection(selectableFields));
    setArgumentValues({ first: "20" });
    setFilters([]);
    setResult(null);
    setRunError(null);
  }, [selectedFieldName, selectableFields]);

  const whereArg = selectedField?.args?.find((arg) => arg.name === "where");
  const whereInputType = whereArg ? typeMap.get(unwrapType(whereArg.type).name) : undefined;
  const filterableFields = useMemo(() => {
    return (whereInputType?.inputFields ?? []).filter((field) => {
      const filterInputType = typeMap.get(unwrapType(field.type).name);
      return Boolean(
        filterInputType?.inputFields?.some((inputField) => KNOWN_FILTER_OPERATORS.has(inputField.name))
      );
    });
  }, [typeMap, whereInputType]);

  const filterOperatorsByField = useMemo(() => {
    const operators = new Map<string, IntrospectionInputField[]>();
    for (const field of filterableFields) {
      const filterInputType = typeMap.get(unwrapType(field.type).name);
      operators.set(
        field.name,
        (filterInputType?.inputFields ?? []).filter((inputField) => KNOWN_FILTER_OPERATORS.has(inputField.name))
      );
    }
    return operators;
  }, [filterableFields, typeMap]);

  const variables = useMemo(() => {
    const nextVariables: Record<string, unknown> = {};
    for (const arg of selectedField?.args ?? []) {
      if (arg.name === "where") continue;
      const value = argumentValues[arg.name]?.trim();
      if (!value) continue;
      nextVariables[arg.name] = parseArgumentValue(value, arg.type);
    }

    const where: Record<string, Record<string, unknown>> = {};
    for (const filter of filters) {
      const operators = filterOperatorsByField.get(filter.field) ?? [];
      const operator = operators.find((candidate) => candidate.name === filter.operator);
      if (!operator) continue;
      if (filter.operator !== "isNull" && !filter.value.trim()) continue;
      where[filter.field] = {
        ...(where[filter.field] ?? {}),
        [filter.operator]: parseFilterValue(filter.value, operator.type),
      };
    }

    if (Object.keys(where).length > 0) {
      nextVariables.where = where;
    }

    return nextVariables;
  }, [argumentValues, filterOperatorsByField, filters, selectedField]);

  const activeArguments = useMemo(() => {
    return (selectedField?.args ?? [])
      .filter((arg) => Object.prototype.hasOwnProperty.call(variables, arg.name))
      .map((arg) => ({ name: arg.name, type: typeToString(arg.type) }));
  }, [selectedField, variables]);

  const selectedSnippets = useMemo(() => {
    const fieldsByName = new Map(selectableFields.map((field) => [field.name, field]));
    return selectedFieldNames.map((name) => fieldsByName.get(name)?.snippet).filter((snippet): snippet is string => Boolean(snippet));
  }, [selectableFields, selectedFieldNames]);

  const queryDocument = useMemo(() => {
    if (!selectedField) return "";
    return buildConnectionQueryDocument({
      fieldName: selectedField.name,
      arguments: activeArguments,
      selectedFields: selectedSnippets,
    });
  }, [activeArguments, selectedField, selectedSnippets]);

  const missingRequiredArgs = useMemo(() => {
    return (selectedField?.args ?? []).filter((arg) => unwrapType(arg.type).isRequired && !argumentValues[arg.name]?.trim());
  }, [argumentValues, selectedField]);

  const addFilter = () => {
    const field = filterableFields[0];
    if (!field) return;
    const operator = filterOperatorsByField.get(field.name)?.[0]?.name ?? "eq";
    setFilters((current) => [...current, newFilterRow(field.name, operator)]);
  };

  const updateFilter = (id: string, patch: Partial<FilterRow>) => {
    setFilters((current) =>
      current.map((filter) => {
        if (filter.id !== id) return filter;
        const next = { ...filter, ...patch };
        if (patch.field) {
          next.operator = filterOperatorsByField.get(patch.field)?.[0]?.name ?? "eq";
          next.value = "";
        }
        return next;
      })
    );
  };

  const copyQuery = async () => {
    await navigator.clipboard.writeText(`${queryDocument}\n\nVariables:\n${stableStringify(variables)}`);
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  };

  const runQuery = async () => {
    if (!queryDocument || missingRequiredArgs.length > 0) return;
    setIsRunning(true);
    setRunError(null);
    setResult(null);
    try {
      const response = await publicGraphqlClient.request<unknown>(queryDocument, variables);
      setResult(response);
    } catch (requestError) {
      setRunError(requestError instanceof Error ? requestError.message : "GraphQL query failed");
    } finally {
      setIsRunning(false);
    }
  };

  return (
    <div className="pt-8 sm:pt-12 space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="max-w-2xl">
          <p className="mb-3 text-xs font-semibold uppercase tracking-[0.28em]" style={{ color: "var(--muted-foreground)" }}>
            GraphQL studio
          </p>
          <h2
            className="font-[family-name:var(--font-syne)] text-3xl sm:text-4xl leading-tight"
            style={{ color: "var(--foreground)" }}
          >
            Query Builder
          </h2>
          <p className="mt-3 leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
            Discover indexed collections, compose filters and field selections, then run or copy a production-ready GraphQL query.
          </p>
        </div>
        <Link
          href="/graphiql"
          target="_blank"
          className="text-sm font-medium underline-offset-4 hover:underline"
          style={{ color: "var(--foreground)" }}
        >
          Open GraphiQL ↗
        </Link>
      </div>

      {error && (
        <Alert variant="error">
          Could not load the public GraphQL schema. Make sure Hyperindex is reachable and introspection is enabled.
        </Alert>
      )}

      <div className="grid gap-5 lg:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]">
        <div className="space-y-5">
          <Card className="overflow-hidden">
            <CardHeader>
              <CardTitle>1. Choose a collection</CardTitle>
              <CardDescription>Connection queries are discovered directly from the live public schema.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {isLoading ? (
                <div className="h-11 animate-pulse rounded-lg" style={{ backgroundColor: "var(--muted)" }} />
              ) : (
                <select
                  value={selectedField?.name ?? ""}
                  onChange={(event) => setSelectedFieldName(event.target.value)}
                  className="h-11 w-full rounded-lg border px-3 text-sm"
                  style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                >
                  {connectionFields.map((field) => (
                    <option key={field.name} value={field.name}>
                      {formatFieldLabel(field)} · {field.name}
                    </option>
                  ))}
                </select>
              )}
              {selectedField?.description && (
                <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>{selectedField.description}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>2. Arguments</CardTitle>
              <CardDescription>Set pagination, sort, and any required field arguments.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3 sm:grid-cols-2">
              {(selectedField?.args ?? [])
                .filter((arg) => arg.name !== "where")
                .map((arg) => {
                  const unwrapped = unwrapType(arg.type);
                  const enumValues = typeMap.get(unwrapped.name)?.enumValues ?? [];
                  return (
                    <label key={arg.name} className="space-y-1.5">
                      <span className="text-xs font-medium" style={{ color: "var(--muted-foreground)" }}>
                        {arg.name} {unwrapped.isRequired ? "*" : ""}
                      </span>
                      {enumValues.length > 0 ? (
                        <select
                          value={argumentValues[arg.name] ?? ""}
                          onChange={(event) => setArgumentValues((current) => ({ ...current, [arg.name]: event.target.value }))}
                          className="h-10 w-full rounded-lg border px-3 text-sm"
                          style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                        >
                          <option value="">Default</option>
                          {enumValues.map((value) => (
                            <option key={value.name} value={value.name}>{value.name}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          value={argumentValues[arg.name] ?? ""}
                          onChange={(event) => setArgumentValues((current) => ({ ...current, [arg.name]: event.target.value }))}
                          type={unwrapped.name === "Int" || unwrapped.name === "Float" ? "number" : "text"}
                          placeholder={arg.defaultValue ? `default ${arg.defaultValue}` : typeToString(arg.type)}
                          className="h-10 w-full rounded-lg border px-3 text-sm"
                          style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                        />
                      )}
                    </label>
                  );
                })}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex-row items-start justify-between space-y-0 gap-3">
              <div>
                <CardTitle>3. Filters</CardTitle>
                <CardDescription>Build a typed where object without writing JSON by hand.</CardDescription>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={addFilter} disabled={filterableFields.length === 0}>
                Add filter
              </Button>
            </CardHeader>
            <CardContent className="space-y-3">
              {filterableFields.length === 0 && (
                <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>This query does not expose typed filters.</p>
              )}
              {filters.map((filter) => {
                const operators = filterOperatorsByField.get(filter.field) ?? [];
                return (
                  <div key={filter.id} className="grid gap-2 sm:grid-cols-[1fr_0.8fr_1fr_auto]">
                    <select
                      value={filter.field}
                      onChange={(event) => updateFilter(filter.id, { field: event.target.value })}
                      className="h-10 rounded-lg border px-3 text-sm"
                      style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                    >
                      {filterableFields.map((field) => (
                        <option key={field.name} value={field.name}>{field.name}</option>
                      ))}
                    </select>
                    <select
                      value={filter.operator}
                      onChange={(event) => updateFilter(filter.id, { operator: event.target.value })}
                      className="h-10 rounded-lg border px-3 text-sm"
                      style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                    >
                      {operators.map((operator) => (
                        <option key={operator.name} value={operator.name}>{operator.name}</option>
                      ))}
                    </select>
                    <input
                      value={filter.value}
                      onChange={(event) => updateFilter(filter.id, { value: event.target.value })}
                      placeholder={filter.operator === "in" ? "comma,separated,values" : filter.operator === "isNull" ? "true" : "value"}
                      className="h-10 rounded-lg border px-3 text-sm"
                      style={{ backgroundColor: "var(--background)", borderColor: "var(--border)", color: "var(--foreground)" }}
                    />
                    <button
                      type="button"
                      onClick={() => setFilters((current) => current.filter((row) => row.id !== filter.id))}
                      className="h-10 rounded-lg px-3 text-sm transition-opacity hover:opacity-70"
                      style={{ color: "var(--muted-foreground)" }}
                    >
                      Remove
                    </button>
                  </div>
                );
              })}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>4. Fields</CardTitle>
              <CardDescription>Select scalar fields for each returned node.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid max-h-80 gap-2 overflow-y-auto pr-1 sm:grid-cols-2">
                {selectableFields.map((field) => (
                  <label key={field.name} className="flex items-start gap-2 rounded-lg border p-3 text-sm" style={{ borderColor: "var(--border)" }}>
                    <input
                      type="checkbox"
                      className="mt-1"
                      checked={selectedFieldNames.includes(field.name)}
                      onChange={(event) => {
                        setSelectedFieldNames((current) =>
                          event.target.checked ? [...current, field.name] : current.filter((name) => name !== field.name)
                        );
                      }}
                    />
                    <span>
                      <span className="block font-medium" style={{ color: "var(--foreground)" }}>{field.name}</span>
                      {field.description && <span className="block text-xs" style={{ color: "var(--muted-foreground)" }}>{field.description}</span>}
                    </span>
                  </label>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-5 lg:sticky lg:top-24 lg:self-start">
          <Card className="overflow-hidden">
            <CardHeader className="flex-row items-start justify-between space-y-0 gap-3">
              <div>
                <CardTitle>Generated query</CardTitle>
                <CardDescription>Copy this into GraphiQL, your app, or run it here.</CardDescription>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={copyQuery} disabled={!queryDocument}>
                {copied ? "Copied" : "Copy"}
              </Button>
            </CardHeader>
            <CardContent className="space-y-4">
              {missingRequiredArgs.length > 0 && (
                <Alert variant="warning">
                  Fill required argument{missingRequiredArgs.length > 1 ? "s" : ""}: {missingRequiredArgs.map((arg) => arg.name).join(", ")}.
                </Alert>
              )}
              <div className="rounded-xl border bg-zinc-950 p-4 text-zinc-100" style={{ borderColor: "var(--border)" }}>
                <pre className="overflow-x-auto whitespace-pre text-xs leading-relaxed"><code>{queryDocument || "# Loading schema..."}</code></pre>
              </div>
              <div>
                <p className="mb-2 text-xs font-medium uppercase tracking-wide" style={{ color: "var(--muted-foreground)" }}>Variables</p>
                <div className="rounded-xl border bg-zinc-950 p-4 text-zinc-100" style={{ borderColor: "var(--border)" }}>
                  <pre className="overflow-x-auto whitespace-pre text-xs leading-relaxed"><code>{stableStringify(variables)}</code></pre>
                </div>
              </div>
              <Button type="button" onClick={runQuery} loading={isRunning} disabled={!queryDocument || missingRequiredArgs.length > 0} className="w-full">
                Run query
              </Button>
            </CardContent>
          </Card>

          {(result || runError) && (
            <Card className="overflow-hidden">
              <CardHeader>
                <CardTitle>Result</CardTitle>
                <CardDescription>Response from the public GraphQL endpoint.</CardDescription>
              </CardHeader>
              <CardContent>
                {runError ? (
                  <Alert variant="error">{runError}</Alert>
                ) : (
                  <div className="rounded-xl border bg-zinc-950 p-4 text-zinc-100" style={{ borderColor: "var(--border)" }}>
                    <pre className="max-h-[520px] overflow-auto whitespace-pre text-xs leading-relaxed"><code>{stableStringify(result)}</code></pre>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
