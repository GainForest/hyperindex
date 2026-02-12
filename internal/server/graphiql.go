// Package server contains HTTP handlers for the hypergoat server.
// GraphiQL playground handler using CDN-hosted resources.
package server

import (
	"net/http"
	"strings"
)

// GraphiQLConfig contains configuration for the GraphiQL handler.
type GraphiQLConfig struct {
	// EndpointPath is the path to the GraphQL endpoint (e.g. "/graphql").
	// The full URL is derived at runtime from the browser's window.location.
	EndpointPath string
	// SubscriptionPath is the path for WebSocket subscriptions (optional, e.g. "/graphql/ws").
	SubscriptionPath string
	// Title is the page title.
	Title string
	// DefaultQuery is the initial query to display.
	DefaultQuery string
	// AdminAuth enables the admin authentication bar (API key + DID inputs).
	AdminAuth bool
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

	// Build subscription URL JavaScript snippet.
	// Uses window.location to derive the correct WebSocket URL at runtime,
	// so the page works regardless of which domain it's accessed through.
	subscriptionJS := ""
	if cfg.SubscriptionPath != "" {
		subscriptionJS = `
      const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      fetcherOpts.subscriptionUrl = wsProto + '//' + location.host + '` + cfg.SubscriptionPath + `';`
	}

	// Admin auth bar: CSS, HTML, and JS for the credential inputs.
	adminAuthCSS := ""
	adminAuthHTML := ""
	adminAuthJS := ""
	graphiqlHeight := "100vh"

	if cfg.AdminAuth {
		graphiqlHeight = "calc(100vh - 44px)"
		adminAuthCSS = `
    #admin-auth {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 6px 12px;
      background: #1e1e1e;
      border-bottom: 1px solid #333;
      font-family: system-ui, sans-serif;
      font-size: 13px;
      color: #ccc;
    }
    #admin-auth label { white-space: nowrap; }
    #admin-auth input {
      padding: 3px 6px;
      border: 1px solid #555;
      border-radius: 3px;
      background: #2a2a2a;
      color: #eee;
      font-size: 13px;
      font-family: monospace;
    }
    #admin-auth input[type="password"] { width: 220px; }
    #admin-auth input[type="text"] { width: 280px; }
    #admin-auth .status {
      margin-left: auto;
      font-size: 12px;
      opacity: 0.7;
    }`
		adminAuthHTML = `
  <div id="admin-auth">
    <label for="admin-api-key">API Key:</label>
    <input id="admin-api-key" type="password" placeholder="ADMIN_API_KEY" />
    <label for="admin-did">DID:</label>
    <input id="admin-did" type="text" placeholder="did:plc:..." />
    <span class="status" id="auth-status"></span>
  </div>`
		adminAuthJS = `
    // Persist credentials in localStorage
    const KEY_API = 'hypergoat_admin_api_key';
    const KEY_DID = 'hypergoat_admin_did';
    const apiKeyInput = document.getElementById('admin-api-key');
    const didInput = document.getElementById('admin-did');
    const authStatus = document.getElementById('auth-status');

    apiKeyInput.value = localStorage.getItem(KEY_API) || '';
    didInput.value = localStorage.getItem(KEY_DID) || '';

    apiKeyInput.addEventListener('input', () => localStorage.setItem(KEY_API, apiKeyInput.value));
    didInput.addEventListener('input', () => localStorage.setItem(KEY_DID, didInput.value));

    function getAdminHeaders() {
      const headers = { 'Content-Type': 'application/json' };
      const apiKey = apiKeyInput.value.trim();
      const did = didInput.value.trim();
      if (apiKey) headers['Authorization'] = 'Bearer ' + apiKey;
      if (did) headers['X-User-DID'] = did;
      return headers;
    }

    function updateStatus() {
      const apiKey = apiKeyInput.value.trim();
      const did = didInput.value.trim();
      if (apiKey && did) {
        authStatus.textContent = 'Authenticated';
        authStatus.style.color = '#4caf50';
      } else {
        authStatus.textContent = apiKey || did ? 'Incomplete' : 'Not authenticated';
        authStatus.style.color = apiKey || did ? '#ff9800' : '#999';
      }
    }
    apiKeyInput.addEventListener('input', updateStatus);
    didInput.addEventListener('input', updateStatus);
    updateStatus();`
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
      height: ` + graphiqlHeight + `;
    }` + adminAuthCSS + `
  </style>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css" />
</head>
<body>` + adminAuthHTML + `
  <div id="graphiql">Loading...</div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>` + adminAuthJS + `
    const fetcherOpts = {
      url: location.origin + '` + cfg.EndpointPath + `',` + func() string {
		if cfg.AdminAuth {
			return `
      get headers() { return getAdminHeaders(); },`
		}
		return `
      headers: { 'Content-Type': 'application/json' },`
	}() + `
    };` + subscriptionJS + `
    const root = ReactDOM.createRoot(document.getElementById('graphiql'));
    const fetcher = GraphiQL.createFetcher(fetcherOpts);
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
