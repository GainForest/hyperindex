// Package server contains HTTP handlers for the Hyperindex server.
// GraphiQL playground handler using CDN-hosted resources.
package server

import (
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
	html := generateGraphiQLHTML(cfg)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
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
# Type queries into this side of the screen, and you will see intelligent
# typeaheads aware of the current GraphQL type schema and live syntax and
# validation errors highlighted within the text.
#
# GraphQL queries typically start with a "{" character. Lines that start
# with a # are ignored.
#
# An example GraphQL query might look like:
#
#     {
#       field(arg: "value") {
#         subField
#       }
#     }
#
# Try pressing the prettify button above, or press Ctrl-Shift-P to
# automatically prettify the query editor.
`
	}

	// Build fetcher config
	fetcherConfig := `{
      url: '` + cfg.Endpoint + `',
      headers: {
        'Content-Type': 'application/json',
      },
    }`

	if cfg.SubscriptionEndpoint != "" {
		fetcherConfig = `{
      url: '` + cfg.Endpoint + `',
      headers: {
        'Content-Type': 'application/json',
      },
      subscriptionUrl: '` + cfg.SubscriptionEndpoint + `',
    }`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + escapeHTML(title) + `</title>
  <style>
    body {
      height: 100%;
      margin: 0;
      width: 100%;
      overflow: hidden;
    }
    #graphiql {
      height: 100vh;
    }
  </style>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css" />
</head>
<body>
  <div id="graphiql">Loading...</div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>
    const root = ReactDOM.createRoot(document.getElementById('graphiql'));
    const fetcher = GraphiQL.createFetcher(` + fetcherConfig + `);
    root.render(
      React.createElement(GraphiQL, {
        fetcher: fetcher,
        defaultEditorToolsVisibility: true,
        defaultQuery: ` + "`" + defaultQuery + "`" + `,
      }),
    );
  </script>
</body>
</html>`
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
