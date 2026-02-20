"use client";

import { useState } from "react";
import Link from "next/link";

const API_ENDPOINT = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

type Tab = "http" | "websocket";
type Language = "javascript" | "python" | "curl";

// Simple syntax highlighting
function highlightCode(code: string, language: string): React.ReactNode[] {
  const lines = code.split("\n");
  
  return lines.map((line, lineIndex) => {
    let highlighted: React.ReactNode;
    
    if (language === "javascript") {
      highlighted = highlightJS(line);
    } else if (language === "python") {
      highlighted = highlightPython(line);
    } else {
      highlighted = highlightShell(line);
    }
    
    return (
      <div key={lineIndex} className="table-row group/line">
        <span className="table-cell pr-4 text-right text-zinc-600 select-none w-8 text-xs">
          {lineIndex + 1}
        </span>
        <span className="table-cell">{highlighted}</span>
      </div>
    );
  });
}

function highlightJS(line: string): React.ReactNode {
  // Comments
  if (line.trim().startsWith("//")) {
    return <span className="text-zinc-500 italic">{line}</span>;
  }
  
  // Process the line with regex replacements
  const parts: React.ReactNode[] = [];
  let remaining = line;
  let key = 0;
  
  // Keywords
  const keywords = /\b(const|let|var|function|async|await|import|from|export|return|if|else|new|class|extends)\b/g;
  // Strings
  const strings = /("[^"]*"|'[^']*'|`[^`]*`)/g;
  // Functions/methods
  const funcs = /\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\(/g;
  // Properties after dot
  const props = /\.([a-zA-Z_][a-zA-Z0-9_]*)/g;
  
  // Simple approach: apply styles to the whole line
  let result = line;
  
  // Build JSX with highlighting
  const tokens: { start: number; end: number; type: string; text: string }[] = [];
  
  let match;
  while ((match = keywords.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "keyword", text: match[0] });
  }
  
  const stringsRegex = /("[^"]*"|'[^']*'|`[^`]*`)/g;
  while ((match = stringsRegex.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "string", text: match[0] });
  }
  
  // Sort by position and build result
  tokens.sort((a, b) => a.start - b.start);
  
  if (tokens.length === 0) {
    return <span>{line}</span>;
  }
  
  let lastEnd = 0;
  tokens.forEach((token, i) => {
    if (token.start > lastEnd) {
      parts.push(<span key={key++}>{line.slice(lastEnd, token.start)}</span>);
    }
    if (token.type === "keyword") {
      parts.push(<span key={key++} className="text-purple-400 font-medium">{token.text}</span>);
    } else if (token.type === "string") {
      parts.push(<span key={key++} className="text-amber-300">{token.text}</span>);
    }
    lastEnd = token.end;
  });
  
  if (lastEnd < line.length) {
    parts.push(<span key={key++}>{line.slice(lastEnd)}</span>);
  }
  
  return <>{parts}</>;
}

function highlightPython(line: string): React.ReactNode {
  // Comments
  if (line.trim().startsWith("#")) {
    return <span className="text-zinc-500 italic">{line}</span>;
  }
  
  const parts: React.ReactNode[] = [];
  let key = 0;
  
  const keywords = /\b(import|from|def|async|await|return|if|else|class|with|as|for|in|while|try|except|finally|True|False|None)\b/g;
  const tokens: { start: number; end: number; type: string; text: string }[] = [];
  
  let match;
  while ((match = keywords.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "keyword", text: match[0] });
  }
  
  const stringsRegex = /("""[\s\S]*?"""|'''[\s\S]*?'''|"[^"]*"|'[^']*'|f"[^"]*"|f'[^']*')/g;
  while ((match = stringsRegex.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "string", text: match[0] });
  }
  
  tokens.sort((a, b) => a.start - b.start);
  
  if (tokens.length === 0) {
    return <span>{line}</span>;
  }
  
  let lastEnd = 0;
  tokens.forEach((token) => {
    if (token.start > lastEnd) {
      parts.push(<span key={key++}>{line.slice(lastEnd, token.start)}</span>);
    }
    if (token.type === "keyword") {
      parts.push(<span key={key++} className="text-purple-400 font-medium">{token.text}</span>);
    } else if (token.type === "string") {
      parts.push(<span key={key++} className="text-amber-300">{token.text}</span>);
    }
    lastEnd = token.end;
  });
  
  if (lastEnd < line.length) {
    parts.push(<span key={key++}>{line.slice(lastEnd)}</span>);
  }
  
  return <>{parts}</>;
}

function highlightShell(line: string): React.ReactNode {
  // Comments
  if (line.trim().startsWith("#")) {
    return <span className="text-zinc-500 italic">{line}</span>;
  }
  
  const parts: React.ReactNode[] = [];
  let key = 0;
  
  // Commands at start
  const commands = /^(\s*)(curl|websocat|brew|cargo)\b/;
  const flags = /(\s-[a-zA-Z]+|\s--[a-zA-Z-]+)/g;
  const urls = /(https?:\/\/[^\s"']+|wss?:\/\/[^\s"']+)/g;
  
  const tokens: { start: number; end: number; type: string; text: string }[] = [];
  
  let match;
  if ((match = commands.exec(line)) !== null) {
    tokens.push({ start: match.index + match[1].length, end: match.index + match[0].length, type: "command", text: match[2] });
  }
  
  while ((match = flags.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "flag", text: match[0] });
  }
  
  while ((match = urls.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "url", text: match[0] });
  }
  
  const stringsRegex = /('[^']*'|"[^"]*")/g;
  while ((match = stringsRegex.exec(line)) !== null) {
    tokens.push({ start: match.index, end: match.index + match[0].length, type: "string", text: match[0] });
  }
  
  // Remove overlapping tokens (prefer later types)
  const filtered = tokens.filter((token, i) => {
    for (let j = i + 1; j < tokens.length; j++) {
      if (tokens[j].start <= token.start && tokens[j].end >= token.end) {
        return false;
      }
    }
    return true;
  });
  
  filtered.sort((a, b) => a.start - b.start);
  
  if (filtered.length === 0) {
    return <span>{line}</span>;
  }
  
  let lastEnd = 0;
  filtered.forEach((token) => {
    if (token.start > lastEnd) {
      parts.push(<span key={key++}>{line.slice(lastEnd, token.start)}</span>);
    }
    if (token.type === "command") {
      parts.push(<span key={key++} className="text-emerald-400 font-medium">{token.text}</span>);
    } else if (token.type === "flag") {
      parts.push(<span key={key++} className="text-cyan-400">{token.text}</span>);
    } else if (token.type === "url") {
      parts.push(<span key={key++} className="text-blue-400 underline">{token.text}</span>);
    } else if (token.type === "string") {
      parts.push(<span key={key++} className="text-amber-300">{token.text}</span>);
    }
    lastEnd = token.end;
  });
  
  if (lastEnd < line.length) {
    parts.push(<span key={key++}>{line.slice(lastEnd)}</span>);
  }
  
  return <>{parts}</>;
}

function CodeBlock({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative group rounded-xl overflow-hidden">
      {/* Header bar */}
      <div className="flex items-center justify-between px-4 py-2 bg-zinc-800 border-b border-zinc-700/50">
        <div className="flex items-center gap-2">
          <div className="flex gap-1.5">
            <div className="w-3 h-3 rounded-full bg-red-500/80" />
            <div className="w-3 h-3 rounded-full bg-yellow-500/80" />
            <div className="w-3 h-3 rounded-full bg-green-500/80" />
          </div>
          <span className="text-xs text-zinc-500 ml-2 font-medium uppercase tracking-wide">
            {language}
          </span>
        </div>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-2.5 py-1 text-xs bg-zinc-700/50 hover:bg-zinc-600/50 
                     text-zinc-400 hover:text-zinc-200 rounded-md transition-all"
        >
          {copied ? (
            <>
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
              Copied!
            </>
          ) : (
            <>
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
              </svg>
              Copy
            </>
          )}
        </button>
      </div>
      {/* Code content */}
      <div className="bg-zinc-900 p-4 overflow-x-auto">
        <div className="table text-sm font-mono leading-relaxed text-zinc-100">
          {highlightCode(code, language)}
        </div>
      </div>
    </div>
  );
}

function LanguageTabs({
  selected,
  onChange,
}: {
  selected: Language;
  onChange: (lang: Language) => void;
}) {
  const languages: { value: Language; label: string; icon: React.ReactNode }[] = [
    { 
      value: "javascript", 
      label: "JavaScript",
      icon: (
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path d="M0 0h24v24H0V0zm22.034 18.276c-.175-1.095-.888-2.015-3.003-2.873-.736-.345-1.554-.585-1.797-1.14-.091-.33-.105-.51-.046-.705.15-.646.915-.84 1.515-.66.39.12.75.42.976.9 1.034-.676 1.034-.676 1.755-1.125-.27-.42-.404-.601-.586-.78-.63-.705-1.469-1.065-2.834-1.034l-.705.089c-.676.165-1.32.525-1.71 1.005-1.14 1.291-.811 3.541.569 4.471 1.365 1.02 3.361 1.244 3.616 2.205.24 1.17-.87 1.545-1.966 1.41-.811-.18-1.26-.586-1.755-1.336l-1.83 1.051c.21.48.45.689.81 1.109 1.74 1.756 6.09 1.666 6.871-1.004.029-.09.24-.705.074-1.65l.046.067zm-8.983-7.245h-2.248c0 1.938-.009 3.864-.009 5.805 0 1.232.063 2.363-.138 2.711-.33.689-1.18.601-1.566.48-.396-.196-.597-.466-.83-.855-.063-.105-.11-.196-.127-.196l-1.825 1.125c.305.63.75 1.172 1.324 1.517.855.51 2.004.675 3.207.405.783-.226 1.458-.691 1.811-1.411.51-.93.402-2.07.397-3.346.012-2.054 0-4.109 0-6.179l.004-.056z"/>
        </svg>
      )
    },
    { 
      value: "python", 
      label: "Python",
      icon: (
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
          <path d="M14.25.18l.9.2.73.26.59.3.45.32.34.34.25.34.16.33.1.3.04.26.02.2-.01.13V8.5l-.05.63-.13.55-.21.46-.26.38-.3.31-.33.25-.35.19-.35.14-.33.1-.3.07-.26.04-.21.02H8.77l-.69.05-.59.14-.5.22-.41.27-.33.32-.27.35-.2.36-.15.37-.1.35-.07.32-.04.27-.02.21v3.06H3.17l-.21-.03-.28-.07-.32-.12-.35-.18-.36-.26-.36-.36-.35-.46-.32-.59-.28-.73-.21-.88-.14-1.05-.05-1.23.06-1.22.16-1.04.24-.87.32-.71.36-.57.4-.44.42-.33.42-.24.4-.16.36-.1.32-.05.24-.01h.16l.06.01h8.16v-.83H6.18l-.01-2.75-.02-.37.05-.34.11-.31.17-.28.25-.26.31-.23.38-.2.44-.18.51-.15.58-.12.64-.1.71-.06.77-.04.84-.02 1.27.05zm-6.3 1.98l-.23.33-.08.41.08.41.23.34.33.22.41.09.41-.09.33-.22.23-.34.08-.41-.08-.41-.23-.33-.33-.22-.41-.09-.41.09zm13.09 3.95l.28.06.32.12.35.18.36.27.36.35.35.47.32.59.28.73.21.88.14 1.04.05 1.23-.06 1.23-.16 1.04-.24.86-.32.71-.36.57-.4.45-.42.33-.42.24-.4.16-.36.09-.32.05-.24.02-.16-.01h-8.22v.82h5.84l.01 2.76.02.36-.05.34-.11.31-.17.29-.25.25-.31.24-.38.2-.44.17-.51.15-.58.13-.64.09-.71.07-.77.04-.84.01-1.27-.04-1.07-.14-.9-.2-.73-.25-.59-.3-.45-.33-.34-.34-.25-.34-.16-.33-.1-.3-.04-.25-.02-.2.01-.13v-5.34l.05-.64.13-.54.21-.46.26-.38.3-.32.33-.24.35-.2.35-.14.33-.1.3-.06.26-.04.21-.02.13-.01h5.84l.69-.05.59-.14.5-.21.41-.28.33-.32.27-.35.2-.36.15-.36.1-.35.07-.32.04-.28.02-.21V6.07h2.09l.14.01zm-6.47 14.25l-.23.33-.08.41.08.41.23.33.33.23.41.08.41-.08.33-.23.23-.33.08-.41-.08-.41-.23-.33-.33-.23-.41-.08-.41.08z"/>
        </svg>
      )
    },
    { 
      value: "curl", 
      label: "cURL",
      icon: (
        <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
      )
    },
  ];

  return (
    <div className="flex gap-1 mb-4 p-1 rounded-lg w-fit" style={{ backgroundColor: "var(--muted)" }}>
      {languages.map(({ value, label, icon }) => (
        <button
          key={value}
          onClick={() => onChange(value)}
          className="flex items-center gap-2 px-3 py-2 text-sm font-medium rounded-md transition-all"
          style={selected === value
            ? { backgroundColor: "var(--card)", color: "var(--foreground)" }
            : { color: "var(--muted-foreground)" }}
        >
          {icon}
          {label}
        </button>
      ))}
    </div>
  );
}

export default function DocsPage() {
  const [activeTab, setActiveTab] = useState<Tab>("http");
  const [httpLang, setHttpLang] = useState<Language>("javascript");
  const [wsLang, setWsLang] = useState<Language>("javascript");

  const httpExamples: Record<Language, string> = {
    javascript: `// Using fetch
const response = await fetch("${API_ENDPOINT}/graphql", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    query: \`
      query {
        # Query your indexed records here
        # Schema is dynamically generated from your lexicons
      }
    \`,
    variables: {}
  })
});

const data = await response.json();
console.log(data);

// Using graphql-request
import { GraphQLClient } from "graphql-request";

const client = new GraphQLClient("${API_ENDPOINT}/graphql");

const query = \`
  query GetRecords {
    # Your query here
  }
\`;

const result = await client.request(query);`,

    python: `import requests

# Using requests
url = "${API_ENDPOINT}/graphql"

query = """
query {
  # Query your indexed records here
  # Schema is dynamically generated from your lexicons
}
"""

response = requests.post(
    url,
    json={"query": query, "variables": {}},
    headers={"Content-Type": "application/json"}
)

data = response.json()
print(data)

# Using gql
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

transport = RequestsHTTPTransport(
    url="${API_ENDPOINT}/graphql",
    headers={"Content-Type": "application/json"}
)

client = Client(transport=transport, fetch_schema_from_transport=True)

query = gql("""
    query GetRecords {
        # Your query here
    }
""")

result = client.execute(query)`,

    curl: `# Basic GraphQL query
curl -X POST ${API_ENDPOINT}/graphql \\
  -H "Content-Type: application/json" \\
  -d '{
    "query": "query { __typename }",
    "variables": {}
  }'

# Query with variables
curl -X POST ${API_ENDPOINT}/graphql \\
  -H "Content-Type: application/json" \\
  -d '{
    "query": "query GetRecord($uri: String!) { ... }",
    "variables": { "uri": "at://did:plc:xxx/app.example.record/123" }
  }'

# Introspection query (discover schema)
curl -X POST ${API_ENDPOINT}/graphql \\
  -H "Content-Type: application/json" \\
  -d '{
    "query": "{ __schema { types { name } } }"
  }'`,
  };

  const wsExamples: Record<Language, string> = {
    javascript: `// Using graphql-ws (recommended)
import { createClient } from "graphql-ws";

const client = createClient({
  url: "${API_ENDPOINT.replace("https://", "wss://")}/graphql",
  connectionParams: {
    // Optional: authentication token
    // authToken: "your-token"
  },
});

// Subscribe to real-time events
const unsubscribe = client.subscribe(
  {
    query: \`
      subscription {
        recordCreated {
          uri
          collection
          did
          record
          createdAt
        }
      }
    \`,
  },
  {
    next: (data) => {
      console.log("New record:", data);
    },
    error: (err) => {
      console.error("Subscription error:", err);
    },
    complete: () => {
      console.log("Subscription complete");
    },
  }
);

// Cleanup when done
// unsubscribe();

// Using Apollo Client
import { 
  ApolloClient, 
  InMemoryCache, 
  split, 
  HttpLink 
} from "@apollo/client";
import { GraphQLWsLink } from "@apollo/client/link/subscriptions";
import { getMainDefinition } from "@apollo/client/utilities";
import { createClient as createWsClient } from "graphql-ws";

const httpLink = new HttpLink({
  uri: "${API_ENDPOINT}/graphql",
});

const wsLink = new GraphQLWsLink(
  createWsClient({
    url: "${API_ENDPOINT.replace("https://", "wss://")}/graphql",
  })
);

// Split traffic between HTTP and WebSocket
const splitLink = split(
  ({ query }) => {
    const definition = getMainDefinition(query);
    return (
      definition.kind === "OperationDefinition" &&
      definition.operation === "subscription"
    );
  },
  wsLink,
  httpLink
);

const apolloClient = new ApolloClient({
  link: splitLink,
  cache: new InMemoryCache(),
});`,

    python: `# Using gql with websockets
import asyncio
from gql import gql, Client
from gql.transport.websockets import WebsocketsTransport

async def subscribe_to_records():
    transport = WebsocketsTransport(
        url="${API_ENDPOINT.replace("https://", "wss://")}/graphql",
        subprotocols=["graphql-transport-ws"]
    )

    async with Client(
        transport=transport,
        fetch_schema_from_transport=True
    ) as session:
        subscription = gql("""
            subscription {
                recordCreated {
                    uri
                    collection
                    did
                    record
                    createdAt
                }
            }
        """)

        async for result in session.subscribe(subscription):
            print(f"New record: {result}")

# Run the subscription
asyncio.run(subscribe_to_records())

# Alternative: Using websockets directly
import websockets
import json

async def raw_websocket_subscription():
    uri = "${API_ENDPOINT.replace("https://", "wss://")}/graphql"
    
    async with websockets.connect(
        uri, 
        subprotocols=["graphql-transport-ws"]
    ) as ws:
        # Connection init
        await ws.send(json.dumps({
            "type": "connection_init",
            "payload": {}
        }))
        
        # Wait for connection_ack
        response = await ws.recv()
        print(f"Connection: {response}")
        
        # Subscribe
        await ws.send(json.dumps({
            "id": "1",
            "type": "subscribe",
            "payload": {
                "query": """
                    subscription {
                        recordCreated {
                            uri
                            collection
                        }
                    }
                """
            }
        }))
        
        # Listen for events
        while True:
            message = await ws.recv()
            data = json.loads(message)
            if data["type"] == "next":
                print(f"Event: {data['payload']}")

asyncio.run(raw_websocket_subscription())`,

    curl: `# WebSocket connections require a proper WebSocket client.
# Here's how to test with websocat:

# Install websocat: brew install websocat (macOS)
# or: cargo install websocat

# Connect to the WebSocket endpoint
websocat ${API_ENDPOINT.replace("https://", "wss://")}/graphql \\
  --protocol graphql-transport-ws

# Then send these messages in order:

# 1. Initialize connection
{"type":"connection_init","payload":{}}

# 2. Subscribe to events (after receiving connection_ack)
{"id":"1","type":"subscribe","payload":{"query":"subscription { recordCreated { uri collection did } }"}}

# You'll receive messages like:
# {"id":"1","type":"next","payload":{"data":{"recordCreated":{...}}}}

# 3. Unsubscribe when done
{"id":"1","type":"complete"}`,
  };

  return (
    <div className="pt-8 sm:pt-12 space-y-10">
      {/* Hero Section */}
      <div className="max-w-xl">
        <h2 className="font-[family-name:var(--font-syne)] text-3xl sm:text-4xl leading-tight" style={{ color: "var(--foreground)" }}>
          API Documentation
        </h2>
        <p className="mt-3 leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
          <strong style={{ color: "var(--foreground)" }}>Hyperindex</strong> is{" "}
          <a href="https://gainforest.earth" target="_blank" rel="noopener noreferrer" style={{ color: "var(--primary)" }} className="hover:underline">GainForest&apos;s</a>{" "}
          main AppView for the AT Protocol Hypersphere ecosystem. It indexes Lexicon-defined records and exposes them via a dynamically-generated GraphQL API.
        </p>
        <div className="flex flex-wrap gap-x-4 gap-y-2 mt-4 text-sm">
          <a
            href="https://impactindexer.org/"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418" />
            </svg>
            Hypersphere Explorer
          </a>
          <a
            href="https://impactindexer.org/lexicon/"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
            </svg>
            Lexicon Reference
          </a>
          <Link
            href="/docs/agents"
            className="inline-flex items-center gap-1.5 transition-colors"
            style={{ color: "var(--muted-foreground)" }}
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 3.104v5.714a2.25 2.25 0 01-.659 1.591L5 14.5M9.75 3.104c-.251.023-.501.05-.75.082m.75-.082a24.301 24.301 0 014.5 0m0 0v5.714c0 .597.237 1.17.659 1.591L19.8 15.3M14.25 3.104c.251.023.501.05.75.082M19.8 15.3l-1.57.393A9.065 9.065 0 0112 15a9.065 9.065 0 00-6.23.693L5 14.5m14.8.8l1.402 1.402c1.232 1.232.65 3.318-1.067 3.611A48.309 48.309 0 0112 21c-2.773 0-5.491-.235-8.135-.687-1.718-.293-2.3-2.379-1.067-3.61L5 14.5" />
            </svg>
            For AI Agents
          </Link>
        </div>
      </div>

      {/* Endpoint Info */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Endpoints
        </h3>
        <div className="rounded-xl border p-6 space-y-3" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4">
            <span className="text-sm font-medium w-24 flex items-center gap-2" style={{ color: "var(--muted-foreground)" }}>
              <span className="w-2 h-2 rounded-full bg-emerald-500" />
              HTTP
            </span>
            <code className="flex-1 text-sm bg-zinc-900 text-emerald-400 px-3 py-2 rounded-lg font-mono break-all">
              POST {API_ENDPOINT}/graphql
            </code>
          </div>
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4">
            <span className="text-sm font-medium w-24 flex items-center gap-2" style={{ color: "var(--muted-foreground)" }}>
              <span className="w-2 h-2 rounded-full bg-blue-500" />
              WebSocket
            </span>
            <code className="flex-1 text-sm bg-zinc-900 text-blue-400 px-3 py-2 rounded-lg font-mono break-all">
              {API_ENDPOINT.replace("https://", "wss://")}/graphql
            </code>
          </div>
          <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4">
            <span className="text-sm font-medium w-24 flex items-center gap-2" style={{ color: "var(--muted-foreground)" }}>
              <span className="w-2 h-2 rounded-full bg-purple-500" />
              GraphiQL
            </span>
            <a 
              href={`${API_ENDPOINT}/graphiql`}
              target="_blank"
              rel="noopener noreferrer"
              className="flex-1 text-sm bg-zinc-900 text-purple-400 hover:text-purple-300 px-3 py-2 rounded-lg font-mono break-all transition-colors"
            >
              {API_ENDPOINT}/graphiql
            </a>
          </div>
        </div>
      </div>

      {/* Protocol Info */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Protocol Details
        </h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="rounded-xl border p-5" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
            <div className="flex items-center gap-2 mb-3">
              <div className="w-8 h-8 rounded-lg flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)" }}>
                <svg className="w-4 h-4" style={{ color: "oklch(0.55 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418" />
                </svg>
              </div>
              <span className="font-medium" style={{ color: "var(--foreground)" }}>HTTP/HTTPS</span>
            </div>
            <p className="text-sm leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
              Standard GraphQL over HTTP. Send POST requests with JSON body containing 
              <code className="text-xs px-1.5 py-0.5 rounded mx-1" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>query</code> 
              and optional 
              <code className="text-xs px-1.5 py-0.5 rounded mx-1" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>variables</code>.
            </p>
          </div>
          <div className="rounded-xl border p-5" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
            <div className="flex items-center gap-2 mb-3">
              <div className="w-8 h-8 rounded-lg flex items-center justify-center" style={{ backgroundColor: "oklch(0.60 0.15 250 / 0.15)" }}>
                <svg className="w-4 h-4" style={{ color: "oklch(0.50 0.15 250)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
                </svg>
              </div>
              <span className="font-medium" style={{ color: "var(--foreground)" }}>WebSocket</span>
            </div>
            <p className="text-sm leading-relaxed" style={{ color: "var(--muted-foreground)" }}>
              Uses the 
              <code className="text-xs px-1.5 py-0.5 rounded mx-1" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>graphql-transport-ws</code> 
              protocol for subscriptions. Connect with subprotocol header set accordingly.
            </p>
          </div>
        </div>
      </div>

      {/* Connection Tabs */}
      <div className="space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center gap-4">
          <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
            Code Examples
          </h3>
          <div className="flex gap-1 p-1 rounded-lg w-fit" style={{ backgroundColor: "var(--muted)" }}>
            <button
              onClick={() => setActiveTab("http")}
              className="px-4 py-2 text-sm font-medium rounded-md transition-all shadow-sm"
              style={activeTab === "http"
                ? { backgroundColor: "var(--card)", color: "var(--foreground)" }
                : { color: "var(--muted-foreground)" }}
            >
              HTTP Queries
            </button>
            <button
              onClick={() => setActiveTab("websocket")}
              className="px-4 py-2 text-sm font-medium rounded-md transition-all"
              style={activeTab === "websocket"
                ? { backgroundColor: "var(--card)", color: "var(--foreground)" }
                : { color: "var(--muted-foreground)" }}
            >
              WebSocket Subscriptions
            </button>
          </div>
        </div>

        {activeTab === "http" && (
          <div className="space-y-4">
            <LanguageTabs selected={httpLang} onChange={setHttpLang} />
            <CodeBlock code={httpExamples[httpLang]} language={httpLang} />
          </div>
        )}

        {activeTab === "websocket" && (
          <div className="space-y-4">
            <LanguageTabs selected={wsLang} onChange={setWsLang} />
            <CodeBlock code={wsExamples[wsLang]} language={wsLang} />
          </div>
        )}
      </div>

      {/* WebSocket Protocol Details */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          WebSocket Protocol Reference
        </h3>
        <div className="rounded-xl border p-6 space-y-6" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          <p className="text-sm leading-relaxed" style={{ color: "var(--secondary-foreground)" }}>
            Hyperindex implements the <code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>graphql-transport-ws</code> protocol. 
            This is the modern standard for GraphQL subscriptions over WebSocket.
          </p>

          <div className="space-y-4">
            <h4 className="font-medium" style={{ color: "var(--foreground)" }}>Connection Flow</h4>
            <ol className="space-y-3 text-sm" style={{ color: "var(--secondary-foreground)" }}>
              <li className="flex gap-3">
                <span className="flex-shrink-0 w-6 h-6 rounded-full text-xs font-medium flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)", color: "oklch(0.55 0.15 155)" }}>1</span>
                <div>
                  <strong>Connect</strong> - Open WebSocket with <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>Sec-WebSocket-Protocol: graphql-transport-ws</code>
                </div>
              </li>
              <li className="flex gap-3">
                <span className="flex-shrink-0 w-6 h-6 rounded-full text-xs font-medium flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)", color: "oklch(0.55 0.15 155)" }}>2</span>
                <div>
                  <strong>Initialize</strong> - Send <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>{`{"type":"connection_init"}`}</code>
                </div>
              </li>
              <li className="flex gap-3">
                <span className="flex-shrink-0 w-6 h-6 rounded-full text-xs font-medium flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)", color: "oklch(0.55 0.15 155)" }}>3</span>
                <div>
                  <strong>Acknowledge</strong> - Receive <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>{`{"type":"connection_ack"}`}</code>
                </div>
              </li>
              <li className="flex gap-3">
                <span className="flex-shrink-0 w-6 h-6 rounded-full text-xs font-medium flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)", color: "oklch(0.55 0.15 155)" }}>4</span>
                <div>
                  <strong>Subscribe</strong> - Send subscription with unique ID
                </div>
              </li>
              <li className="flex gap-3">
                <span className="flex-shrink-0 w-6 h-6 rounded-full text-xs font-medium flex items-center justify-center" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)", color: "oklch(0.55 0.15 155)" }}>5</span>
                <div>
                  <strong>Receive</strong> - Get <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>{`{"type":"next"}`}</code> messages with data
                </div>
              </li>
            </ol>
          </div>

          <div className="space-y-3">
            <h4 className="font-medium" style={{ color: "var(--foreground)" }}>Message Types</h4>
            <div className="overflow-x-auto -mx-6 px-6">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <th className="text-left py-2 pr-4 font-medium" style={{ color: "var(--foreground)" }}>Type</th>
                    <th className="text-left py-2 pr-4 font-medium" style={{ color: "var(--foreground)" }}>Direction</th>
                    <th className="text-left py-2 font-medium" style={{ color: "var(--foreground)" }}>Description</th>
                  </tr>
                </thead>
                <tbody style={{ color: "var(--secondary-foreground)" }}>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>connection_init</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Client → Server</td>
                    <td className="py-2.5">Initialize connection</td>
                  </tr>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>connection_ack</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Server → Client</td>
                    <td className="py-2.5">Connection accepted</td>
                  </tr>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>subscribe</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Client → Server</td>
                    <td className="py-2.5">Start subscription</td>
                  </tr>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>next</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Server → Client</td>
                    <td className="py-2.5">Data payload</td>
                  </tr>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>error</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Server → Client</td>
                    <td className="py-2.5">Subscription error</td>
                  </tr>
                  <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>complete</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Both</td>
                    <td className="py-2.5">End subscription</td>
                  </tr>
                  <tr>
                    <td className="py-2.5 pr-4"><code className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--muted)", color: "var(--foreground)" }}>ping/pong</code></td>
                    <td className="py-2.5 pr-4" style={{ color: "var(--muted-foreground)" }}>Both</td>
                    <td className="py-2.5">Keep-alive</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      {/* Tips */}
      <div className="space-y-4">
        <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
          Tips & Best Practices
        </h3>
        <div className="rounded-xl border p-6" style={{ borderColor: "var(--border)", backgroundColor: "var(--card)" }}>
          <ul className="space-y-4 text-sm" style={{ color: "var(--foreground)" }}>
            <li className="flex gap-3">
              <div className="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-0.5" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)" }}>
                <svg className="w-3.5 h-3.5" style={{ color: "oklch(0.55 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </div>
              <span>
                <strong style={{ color: "var(--foreground)" }}>Use GraphiQL</strong> — Explore the schema interactively at <a href={`${API_ENDPOINT}/graphiql`} target="_blank" rel="noopener noreferrer" style={{ color: "var(--primary)" }} className="hover:underline">/graphiql</a>
              </span>
            </li>
            <li className="flex gap-3">
              <div className="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-0.5" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)" }}>
                <svg className="w-3.5 h-3.5" style={{ color: "oklch(0.55 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </div>
              <span>
                <strong style={{ color: "var(--foreground)" }}>Schema introspection</strong> — Dynamically generated from uploaded lexicons
              </span>
            </li>
            <li className="flex gap-3">
              <div className="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-0.5" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)" }}>
                <svg className="w-3.5 h-3.5" style={{ color: "oklch(0.55 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </div>
              <span>
                <strong style={{ color: "var(--foreground)" }}>Relay pagination</strong> — Use <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--accent)" }}>first</code>, <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--accent)" }}>after</code>, <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--accent)" }}>last</code>, <code className="text-xs px-1 py-0.5 rounded" style={{ backgroundColor: "var(--accent)" }}>before</code> for cursors
              </span>
            </li>
            <li className="flex gap-3">
              <div className="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-0.5" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.15)" }}>
                <svg className="w-3.5 h-3.5" style={{ color: "oklch(0.55 0.15 155)" }} fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </div>
              <span>
                <strong style={{ color: "var(--foreground)" }}>Handle reconnection</strong> — Implement exponential backoff for WebSocket subscriptions
              </span>
            </li>
          </ul>
        </div>
      </div>
    </div>
  );
}
