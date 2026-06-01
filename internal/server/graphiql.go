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
      height: 100vh;
    }
  </style>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3.9.0/graphiql.min.css" />
  <link rel="stylesheet" href="https://unpkg.com/@graphiql/plugin-explorer@3.2.6/dist/style.css" />
</head>
<body>
  <div id="graphiql">Loading...</div>
  <script type="module">
    import React from 'https://esm.sh/react@19.2.3';
    import { createRoot } from 'https://esm.sh/react-dom@19.2.3/client?deps=react@19.2.3';
    import { GraphiQL } from 'https://esm.sh/graphiql@3.9.0?deps=react@19.2.3,react-dom@19.2.3,@graphiql/react@0.29.0,graphql@16.12.0';
    import { createGraphiQLFetcher } from 'https://esm.sh/@graphiql/toolkit@0.12.0?deps=graphql@16.12.0,graphql-ws@5.16.0';
    import { explorerPlugin } from 'https://esm.sh/@graphiql/plugin-explorer@3.2.6?deps=react@19.2.3,react-dom@19.2.3,@graphiql/react@0.29.0,graphql@16.12.0';

    const fetcher = createGraphiQLFetcher(` + fetcherConfigJSON + `);
    const plugins = [explorerPlugin()];

    createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher,
        defaultEditorToolsVisibility: true,
        defaultQuery: ` + defaultQueryJSON + `,
        plugins,
      }),
    );
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
