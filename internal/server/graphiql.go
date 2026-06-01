// Package server contains HTTP handlers for the Hyperindex server.
// GraphiQL playground handler using CDN-hosted resources.
package server

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"
)

// GraphiQLConfig contains configuration for the GraphiQL handler.
type GraphiQLConfig struct {
	// Endpoint is the GraphQL endpoint URL.
	Endpoint string
	// SubscriptionEndpoint is the WebSocket endpoint for subscriptions (optional).
	SubscriptionEndpoint string
	// Title is the page title.
	Title string
	// DefaultQuery is the initial query to display.
	DefaultQuery string
}

// HandleGraphiQL creates an HTTP handler that serves the GraphiQL IDE.
func HandleGraphiQL(cfg GraphiQLConfig) http.HandlerFunc {
	// Use CDN-hosted GraphiQL
	pageHTML := generateGraphiQLHTML(cfg)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(pageHTML))
	}
}

// generateGraphiQLHTML generates the HTML for the GraphiQL IDE.
func generateGraphiQLHTML(cfg GraphiQLConfig) string {
	// Default title
	title := cfg.Title
	if title == "" {
		title = "GraphiQL"
	}

	// Default query
	defaultQuery := cfg.DefaultQuery
	if defaultQuery == "" {
		defaultQuery = `# Welcome to GraphiQL
#
# GraphiQL is an in-browser tool for writing, validating, and
# testing GraphQL queries.
#
# Use the Schema Builder panel on the left to check fields into a query.
# You can still edit the generated query by hand in the GraphiQL editor.
`
	}

	fetcherConfig := map[string]string{
		"url": cfg.Endpoint,
	}
	if cfg.SubscriptionEndpoint != "" {
		fetcherConfig["subscriptionUrl"] = cfg.SubscriptionEndpoint
	}

	fetcherConfigJSON := mustJSON(fetcherConfig)
	defaultQueryJSON := mustJSON(defaultQuery)

	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + escapeHTML(title) + `</title>
  <style>
    html, body {
      height: 100%;
      margin: 0;
      width: 100%;
      overflow: hidden;
    }
    #root {
      display: grid;
      grid-template-columns: minmax(280px, 24vw) minmax(0, 1fr);
      height: 100vh;
      width: 100vw;
    }
    #schema-builder {
      background: hsl(220deg 14% 96%);
      border-right: 1px solid hsl(220deg 13% 88%);
      color: hsl(220deg 20% 18%);
      display: flex;
      flex-direction: column;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      min-width: 0;
    }
    .schema-builder__header {
      border-bottom: 1px solid hsl(220deg 13% 88%);
      padding: 14px 14px 12px;
    }
    .schema-builder__title {
      font-size: 13px;
      font-weight: 700;
      letter-spacing: 0.08em;
      margin: 0 0 5px;
      text-transform: uppercase;
    }
    .schema-builder__subtitle {
      color: hsl(220deg 9% 42%);
      font-size: 12px;
      line-height: 1.35;
      margin: 0;
    }
    .schema-builder__body {
      overflow: auto;
      padding: 12px;
    }
    .schema-builder__section {
      margin-bottom: 14px;
    }
    .schema-builder__label {
      color: hsl(220deg 9% 42%);
      display: block;
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      margin-bottom: 6px;
      text-transform: uppercase;
    }
    .schema-builder__select,
    .schema-builder__input {
      background: white;
      border: 1px solid hsl(220deg 13% 82%);
      border-radius: 7px;
      box-sizing: border-box;
      color: hsl(220deg 20% 18%);
      font-size: 12px;
      height: 34px;
      padding: 0 9px;
      width: 100%;
    }
    .schema-builder__args {
      display: grid;
      gap: 7px;
    }
    .schema-builder__arg-name {
      color: hsl(220deg 12% 28%);
      display: flex;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 11px;
      justify-content: space-between;
      margin-bottom: 3px;
    }
    .schema-builder__type {
      color: hsl(220deg 8% 48%);
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 10px;
      margin-left: 6px;
    }
    .schema-builder__tree {
      display: grid;
      gap: 1px;
    }
    .schema-builder__row {
      align-items: center;
      border-radius: 6px;
      display: flex;
      gap: 7px;
      min-height: 26px;
      padding: 2px 5px;
    }
    .schema-builder__row:hover {
      background: hsl(220deg 18% 91%);
    }
    .schema-builder__checkbox {
      accent-color: hsl(216deg 92% 52%);
      margin: 0;
    }
    .schema-builder__field-name {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 12px;
    }
    .schema-builder__empty,
    .schema-builder__error {
      border: 1px dashed hsl(220deg 13% 78%);
      border-radius: 8px;
      color: hsl(220deg 9% 42%);
      font-size: 12px;
      line-height: 1.45;
      padding: 12px;
    }
    .schema-builder__error {
      border-color: hsl(0deg 70% 78%);
      color: hsl(0deg 64% 40%);
    }
    #graphiql {
      height: 100vh;
      min-width: 0;
    }
    @media (max-width: 760px) {
      #root {
        grid-template-columns: 1fr;
        grid-template-rows: minmax(260px, 42vh) minmax(0, 1fr);
      }
      #schema-builder {
        border-bottom: 1px solid hsl(220deg 13% 88%);
        border-right: 0;
      }
      #graphiql {
        height: auto;
        min-height: 0;
      }
    }
  </style>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css" />
</head>
<body>
  <div id="root">Loading...</div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>
    const graphQLFetcherConfig = ` + fetcherConfigJSON + `;
    const initialQuery = ` + defaultQueryJSON + `;
    const endpointURL = graphQLFetcherConfig.url;
    const h = React.createElement;

    const introspectionQuery = ` + "`" + `
      query GraphiQLSchemaBuilderIntrospection {
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
            fields(includeDeprecated: true) {
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
                }
              }
            }
          }
        }
      }
    ` + "`" + `;

    function unwrapType(type) {
      let current = type;
      let isList = false;
      let isRequired = false;
      while (current) {
        if (current.kind === 'NON_NULL') {
          isRequired = true;
          current = current.ofType;
          continue;
        }
        if (current.kind === 'LIST') {
          isList = true;
          current = current.ofType;
          continue;
        }
        return { kind: current.kind, name: current.name || '', isList, isRequired };
      }
      return { kind: '', name: '', isList, isRequired };
    }

    function typeToString(type) {
      if (type.kind === 'NON_NULL' && type.ofType) return typeToString(type.ofType) + '!';
      if (type.kind === 'LIST' && type.ofType) return '[' + typeToString(type.ofType) + ']';
      return type.name || 'String';
    }

    function isLeafType(typeRef) {
      const type = unwrapType(typeRef);
      return type.kind === 'SCALAR' || type.kind === 'ENUM';
    }

    function typeFields(typeMap, typeRef) {
      const type = typeMap.get(unwrapType(typeRef).name);
      return type && type.fields ? type.fields : [];
    }

    function fieldKey(path) {
      return path.join('.');
    }

    function literalForType(typeRef, value) {
      const unwrapped = unwrapType(typeRef);
      const trimmed = String(value || '').trim();
      if (!trimmed) return null;
      if (unwrapped.name === 'String' || unwrapped.name === 'ID' || unwrapped.name === 'DateTime' || unwrapped.name === 'JSON') {
        return JSON.stringify(trimmed);
      }
      if (unwrapped.name === 'Int' || unwrapped.name === 'Float') return trimmed;
      if (unwrapped.name === 'Boolean') return trimmed.toLowerCase() === 'false' ? 'false' : 'true';
      return trimmed;
    }

    function defaultArgValue(arg) {
      if (arg.defaultValue != null) return '';
      const unwrapped = unwrapType(arg.type);
      if (arg.name === 'first') return '20';
      if (!unwrapped.isRequired) return '';
      if (unwrapped.name === 'Int') return '10';
      if (unwrapped.name === 'Float') return '1.0';
      if (unwrapped.name === 'Boolean') return 'true';
      return 'TODO';
    }

    function buildDefaultArgValues(field) {
      const values = {};
      (field.args || []).forEach((arg) => {
        const value = defaultArgValue(arg);
        if (value) values[arg.name] = value;
      });
      return values;
    }

    function scalarDescendants(typeMap, typeRef, path, depth) {
      if (depth > 5) return [];
      const fields = typeFields(typeMap, typeRef);
      const keys = [];
      fields.forEach((field) => {
        const nextPath = path.concat(field.name);
        if (isLeafType(field.type)) {
          keys.push(fieldKey(nextPath));
          return;
        }
        keys.push(...scalarDescendants(typeMap, field.type, nextPath, depth + 1));
      });
      return keys;
    }

    function preferredDefaultSelection(typeMap, rootField) {
      const allScalarKeys = scalarDescendants(typeMap, rootField.type, [rootField.name], 0);
      const preferredEndings = [
        'totalCount',
        'pageInfo.hasNextPage',
        'pageInfo.endCursor',
        'edges.cursor',
        'edges.node.uri',
        'edges.node.cid',
        'edges.node.did',
        'edges.node.rkey',
        'edges.node.collection',
        'edges.node.value',
        'uri',
        'cid',
        'did',
        'rkey',
      ];
      const preferred = allScalarKeys.filter((key) => preferredEndings.some((ending) => key.endsWith('.' + ending) || key.endsWith(ending)));
      return new Set((preferred.length ? preferred : allScalarKeys.slice(0, 8)));
    }

    function buildSelectionSet(typeMap, typeRef, path, selectedKeys, depth) {
      if (depth > 6) return [];
      const lines = [];
      typeFields(typeMap, typeRef).forEach((field) => {
        const nextPath = path.concat(field.name);
        const key = fieldKey(nextPath);
        if (isLeafType(field.type)) {
          if (selectedKeys.has(key)) lines.push(field.name);
          return;
        }
        const childLines = buildSelectionSet(typeMap, field.type, nextPath, selectedKeys, depth + 1);
        if (childLines.length) {
          lines.push(field.name + ' {\n' + indent(childLines.join('\n'), 2) + '\n}');
        }
      });
      return lines;
    }

    function indent(value, spaces) {
      const pad = ' '.repeat(spaces);
      return value.split('\n').map((line) => pad + line).join('\n');
    }

    function buildQuery(typeMap, rootField, selectedKeys, argValues) {
      if (!rootField) return initialQuery;
      const argParts = (rootField.args || [])
        .map((arg) => {
          const literal = literalForType(arg.type, argValues[arg.name]);
          return literal ? arg.name + ': ' + literal : null;
        })
        .filter(Boolean);
      const fieldCall = rootField.name + (argParts.length ? '(' + argParts.join(', ') + ')' : '');
      const selections = buildSelectionSet(typeMap, rootField.type, [rootField.name], selectedKeys, 0);
      if (!selections.length) {
        return 'query BuiltWithGraphiQLSchemaBuilder {\n  ' + fieldCall + '\n}';
      }
      return 'query BuiltWithGraphiQLSchemaBuilder {\n  ' + fieldCall + ' {\n' + indent(selections.join('\n'), 4) + '\n  }\n}';
    }

    function SchemaBuilder({ onBuildQuery }) {
      const [schema, setSchema] = React.useState(null);
      const [error, setError] = React.useState(null);
      const [selectedRootName, setSelectedRootName] = React.useState('');
      const [selectedKeys, setSelectedKeys] = React.useState(new Set());
      const [argValues, setArgValues] = React.useState({});

      React.useEffect(() => {
        let cancelled = false;
        fetch(endpointURL, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: introspectionQuery }),
        })
          .then((response) => response.json())
          .then((payload) => {
            if (cancelled) return;
            if (payload.errors) throw new Error(payload.errors.map((err) => err.message).join('; '));
            setSchema(payload.data.__schema);
          })
          .catch((err) => {
            if (!cancelled) setError(err.message || 'Failed to load schema');
          });
        return () => { cancelled = true; };
      }, []);

      const typeMap = React.useMemo(() => {
        return new Map((schema ? schema.types : []).filter((type) => type.name).map((type) => [type.name, type]));
      }, [schema]);

      const queryFields = React.useMemo(() => {
        return schema && schema.queryType ? [...schema.queryType.fields].sort((a, b) => a.name.localeCompare(b.name)) : [];
      }, [schema]);

      const selectedRoot = queryFields.find((field) => field.name === selectedRootName) || queryFields[0];

      React.useEffect(() => {
        if (!selectedRootName && queryFields.length) setSelectedRootName(queryFields[0].name);
      }, [queryFields, selectedRootName]);

      React.useEffect(() => {
        if (!selectedRoot || !typeMap.size) return;
        const nextSelectedKeys = preferredDefaultSelection(typeMap, selectedRoot);
        const nextArgValues = buildDefaultArgValues(selectedRoot);
        setSelectedKeys(nextSelectedKeys);
        setArgValues(nextArgValues);
        onBuildQuery(buildQuery(typeMap, selectedRoot, nextSelectedKeys, nextArgValues));
      }, [selectedRootName, typeMap]);

      React.useEffect(() => {
        if (!selectedRoot || !typeMap.size) return;
        onBuildQuery(buildQuery(typeMap, selectedRoot, selectedKeys, argValues));
      }, [selectedKeys, argValues, selectedRoot, typeMap]);

      function togglePath(path, field) {
        setSelectedKeys((current) => {
          const next = new Set(current);
          const key = fieldKey(path);
          if (isLeafType(field.type)) {
            if (next.has(key)) next.delete(key); else next.add(key);
            return next;
          }
          const descendants = scalarDescendants(typeMap, field.type, path, 0);
          const anySelected = descendants.some((descendant) => next.has(descendant));
          descendants.forEach((descendant) => {
            if (anySelected) next.delete(descendant); else next.add(descendant);
          });
          return next;
        });
      }

      function renderFieldTree(typeRef, path, depth, visitedTypes) {
        const typeName = unwrapType(typeRef).name;
        if (depth > 5) {
          return [h('div', {
            key: fieldKey(path) + '.__max_depth',
            className: 'schema-builder__row',
            style: { paddingLeft: (depth * 14 + 5) + 'px' },
          }, h('span', { className: 'schema-builder__type' }, 'Max depth reached'))];
        }
        if (typeName && visitedTypes.has(typeName)) {
          return [h('div', {
            key: fieldKey(path) + '.__recursive',
            className: 'schema-builder__row',
            style: { paddingLeft: (depth * 14 + 5) + 'px' },
          }, h('span', { className: 'schema-builder__type' }, 'Recursive type ' + typeName + ' omitted'))];
        }

        const nextVisitedTypes = new Set(visitedTypes);
        if (typeName) nextVisitedTypes.add(typeName);

        return typeFields(typeMap, typeRef).map((field) => {
          const nextPath = path.concat(field.name);
          const key = fieldKey(nextPath);
          const leaf = isLeafType(field.type);
          const descendantKeys = leaf ? [key] : scalarDescendants(typeMap, field.type, nextPath, 0);
          const checked = descendantKeys.length > 0 && descendantKeys.some((descendant) => selectedKeys.has(descendant));
          const children = !leaf && (checked || depth < 1) ? renderFieldTree(field.type, nextPath, depth + 1, nextVisitedTypes) : null;
          return h('div', { key },
            h('label', { className: 'schema-builder__row', style: { paddingLeft: (depth * 14 + 5) + 'px' }, title: field.description || '' },
              h('input', {
                className: 'schema-builder__checkbox',
                type: 'checkbox',
                checked,
                onChange: () => togglePath(nextPath, field),
              }),
              h('span', { className: 'schema-builder__field-name' }, field.name),
              h('span', { className: 'schema-builder__type' }, typeToString(field.type))
            ),
            children
          );
        });
      }

      if (error) {
        return h('aside', { id: 'schema-builder' },
          h('div', { className: 'schema-builder__header' },
            h('p', { className: 'schema-builder__title' }, 'Schema Builder'),
            h('p', { className: 'schema-builder__subtitle' }, 'Check fields here to build the GraphiQL query editor.')
          ),
          h('div', { className: 'schema-builder__body' }, h('div', { className: 'schema-builder__error' }, error))
        );
      }

      if (!schema) {
        return h('aside', { id: 'schema-builder' },
          h('div', { className: 'schema-builder__header' },
            h('p', { className: 'schema-builder__title' }, 'Schema Builder'),
            h('p', { className: 'schema-builder__subtitle' }, 'Loading schema introspection…')
          ),
          h('div', { className: 'schema-builder__body' }, h('div', { className: 'schema-builder__empty' }, 'Loading query fields from the GraphQL schema.'))
        );
      }

      return h('aside', { id: 'schema-builder' },
        h('div', { className: 'schema-builder__header' },
          h('p', { className: 'schema-builder__title' }, 'Schema Builder'),
          h('p', { className: 'schema-builder__subtitle' }, 'Pick a root query, fill any args, then check fields to add them to the editor.')
        ),
        h('div', { className: 'schema-builder__body' },
          h('div', { className: 'schema-builder__section' },
            h('label', { className: 'schema-builder__label', htmlFor: 'schema-builder-root-field' }, 'Root query'),
            h('select', {
              id: 'schema-builder-root-field',
              className: 'schema-builder__select',
              value: selectedRoot ? selectedRoot.name : '',
              onChange: (event) => setSelectedRootName(event.target.value),
            }, queryFields.map((field) => h('option', { key: field.name, value: field.name }, field.name)))
          ),
          selectedRoot && selectedRoot.args && selectedRoot.args.length ? h('div', { className: 'schema-builder__section' },
            h('span', { className: 'schema-builder__label' }, 'Arguments'),
            h('div', { className: 'schema-builder__args' }, selectedRoot.args.map((arg) => h('label', { key: arg.name },
              h('span', { className: 'schema-builder__arg-name' },
                h('span', null, arg.name),
                h('span', null, typeToString(arg.type))
              ),
              h('input', {
                className: 'schema-builder__input',
                value: argValues[arg.name] || '',
                placeholder: arg.defaultValue ? 'default ' + arg.defaultValue : '',
                onChange: (event) => setArgValues((current) => ({ ...current, [arg.name]: event.target.value })),
              })
            )))
          ) : null,
          h('div', { className: 'schema-builder__section' },
            h('span', { className: 'schema-builder__label' }, 'Fields'),
            h('div', { className: 'schema-builder__tree' },
              selectedRoot ? renderFieldTree(selectedRoot.type, [selectedRoot.name], 0, new Set()) : h('div', { className: 'schema-builder__empty' }, 'No query fields found.')
            )
          )
        )
      );
    }

    function App() {
      const [query, setQuery] = React.useState(initialQuery);
      const fetcher = React.useMemo(() => GraphiQL.createFetcher({
        ...graphQLFetcherConfig,
        headers: { 'Content-Type': 'application/json' },
      }), []);

      return h(React.Fragment, null,
        h(SchemaBuilder, { onBuildQuery: setQuery }),
        h('main', { id: 'graphiql' },
          h(GraphiQL, {
            fetcher,
            query,
            onEditQuery: setQuery,
            defaultEditorToolsVisibility: true,
          })
        )
      );
    }

    ReactDOM.createRoot(document.getElementById('root')).render(h(App));
  </script>
</body>
</html>`
}

func mustJSON(value interface{}) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	return html.EscapeString(strings.TrimSpace(s))
}
