import { NextResponse } from "next/server";
import { env } from "@/lib/env";

export const dynamic = "force-dynamic";

function toWebSocketURL(httpURL: string): string {
  const url = new URL(httpURL);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

function graphiqlHTML(graphqlURL: string, subscriptionURL: string): string {
  const fetcherConfig = JSON.stringify({
    url: graphqlURL,
    headers: {
      "Content-Type": "application/json",
    },
    subscriptionUrl: subscriptionURL,
  });

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Hyperindex GraphiQL</title>
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

    const fetcher = createGraphiQLFetcher(${fetcherConfig});
    const plugins = [HISTORY_PLUGIN, explorerPlugin()];

    ReactDOM.createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher,
        defaultEditorToolsVisibility: true,
        defaultQuery: '# Welcome to Hyperindex GraphiQL\\n# The GraphiQL Explorer plugin is open in the sidebar.\\n',
        plugins,
        visiblePlugin: 'GraphiQL Explorer',
      }),
    );
  </script>
</head>
<body>
  <div id="graphiql"><div class="loading">Loading…</div></div>
</body>
</html>`;
}

/**
 * Serves the official GraphiQL Explorer plugin on the frontend preview route.
 * The backend also serves the same native plugin at /graphiql when that service is deployed.
 */
export async function GET() {
  const backendURL = env.NEXT_PUBLIC_HYPERINDEX_URL || env.HYPERINDEX_URL;
  const graphqlURL = `${backendURL}/graphql`;
  const subscriptionURL = toWebSocketURL(`${backendURL}/graphql/ws`);

  return new NextResponse(graphiqlHTML(graphqlURL, subscriptionURL), {
    headers: {
      "Content-Type": "text/html; charset=utf-8",
    },
  });
}
