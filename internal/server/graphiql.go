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
	// Use CDN-hosted GraphiQL.
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
	// Default title.
	title := cfg.Title
	if title == "" {
		title = "GraphiQL"
	}

	// Default query.
	defaultQuery := cfg.DefaultQuery
	if defaultQuery == "" {
		defaultQuery = `# Welcome to GraphiQL
#
# GraphiQL is an in-browser tool for writing, validating, and
# testing GraphQL queries.
#
# Open the Explorer tab in the sidebar to build queries by checking
# schema fields into the editor.
`
	}

	fetcherConfig := map[string]interface{}{
		"url": cfg.Endpoint,
		"headers": map[string]string{
			"Content-Type": "application/json",
		},
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
    #graphiql {
      height: 100dvh;
    }
    .loading {
      align-items: center;
      display: flex;
      font-family: ui-sans-serif, system-ui, sans-serif;
      height: 100%;
      justify-content: center;
    }
  </style>
  <link rel="stylesheet" href="https://esm.sh/graphiql@5.2.2/dist/style.css" />
  <link rel="stylesheet" href="https://esm.sh/@graphiql/plugin-explorer@5.1.1/dist/style.css" />
  <script type="importmap">
    {
      "imports": {
        "@emotion/is-prop-valid": "data:text/javascript,",
        "@graphiql/plugin-explorer": "https://esm.sh/@graphiql/plugin-explorer@5.1.1?standalone&external=react,@graphiql/react,graphql",
        "@graphiql/react": "https://esm.sh/@graphiql/react@0.37.3?standalone&external=react,react-dom,graphql,@graphiql/toolkit,@emotion/is-prop-valid",
        "@graphiql/toolkit": "https://esm.sh/@graphiql/toolkit@0.11.3?standalone&external=graphql",
        "graphql": "https://esm.sh/graphql@16.13.2",
        "graphiql": "https://esm.sh/graphiql@5.2.2?standalone&external=react,react-dom,@graphiql/react,graphql",
        "graphiql/": "https://esm.sh/graphiql@5.2.2/",
        "react": "https://esm.sh/react@19.2.5",
        "react/": "https://esm.sh/react@19.2.5/",
        "react-dom": "https://esm.sh/react-dom@19.2.5",
        "react-dom/": "https://esm.sh/react-dom@19.2.5/"
      }
    }
  </script>
  <script type="module">
    import React from 'react';
    import ReactDOM from 'react-dom/client';
    import { GraphiQL, HISTORY_PLUGIN } from 'graphiql';
    import { createGraphiQLFetcher } from '@graphiql/toolkit';
    import { explorerPlugin } from '@graphiql/plugin-explorer';
    import 'graphiql/setup-workers/esm.sh';

    const fetcher = createGraphiQLFetcher(` + fetcherConfigJSON + `);
    const plugins = [HISTORY_PLUGIN, explorerPlugin()];

    ReactDOM.createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher,
        defaultEditorToolsVisibility: true,
        defaultQuery: ` + defaultQueryJSON + `,
        plugins,
        visiblePlugin: 'GraphiQL Explorer',
      }),
    );
  </script>
</head>
<body>
  <div id="graphiql"><div class="loading">Loading…</div></div>
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
