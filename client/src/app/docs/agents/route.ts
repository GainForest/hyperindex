const API_ENDPOINT = "https://hypergoat-app-production.up.railway.app";
const WS_ENDPOINT = "wss://hypergoat-app-production.up.railway.app";

const agentsMd = `# Hypergoat API - Complete Integration Guide for AI Agents

## What is Hypergoat?

**Hypergoat** stands for **Hypersphere Go ATProto AppView**. It is GainForest's main AppView server for the AT Protocol Hypersphere ecosystem.

### Key Information

- **Organization**: GainForest (https://gainforest.earth)
- **Purpose**: Indexes Lexicon-defined records from the AT Protocol network and exposes them via a dynamically-generated GraphQL API
- **Ecosystem**: Part of the Hypersphere ecosystem for environmental impact tracking

### Related Resources

| Resource | URL |
|----------|-----|
| Hypersphere Explorer | https://impactindexer.org/ |
| Lexicon Reference | https://impactindexer.org/lexicon/ |
| Agent Lexicons | https://impactindexer.org/lexicon/agents |
| GainForest | https://gainforest.earth |

### What Hypergoat Indexes

Hypergoat indexes records defined by Lexicons in the Hypersphere ecosystem. The primary lexicons include:

- **Agent records** - AI and human agents in the ecosystem (see: https://impactindexer.org/lexicon/agents)
- **Impact records** - Environmental impact data
- **Conservation records** - Conservation project data

The GraphQL schema is **dynamically generated** from uploaded Lexicon definitions. Use introspection queries to discover the current schema.

---

## API Endpoints

| Purpose | URL |
|---------|-----|
| GraphQL HTTP | \`POST ${API_ENDPOINT}/graphql\` |
| GraphQL WebSocket | \`${WS_ENDPOINT}/graphql\` |
| GraphiQL Explorer | \`${API_ENDPOINT}/graphiql\` |

---

## HTTP Queries

All GraphQL queries use POST requests with JSON body.

### Required Headers

\`\`\`
Content-Type: application/json
\`\`\`

### Request Body Format

\`\`\`json
{
  "query": "your GraphQL query string",
  "variables": { "optional": "variables" },
  "operationName": "OptionalOperationName"
}
\`\`\`

### Response Format

\`\`\`json
{
  "data": { "fieldName": "result" },
  "errors": [{ "message": "error if any", "path": ["field"] }]
}
\`\`\`

---

## Code Examples

### JavaScript/TypeScript - fetch

\`\`\`javascript
async function query(graphqlQuery, variables = {}) {
  const response = await fetch("${API_ENDPOINT}/graphql", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      query: graphqlQuery,
      variables,
    }),
  });
  
  const result = await response.json();
  
  if (result.errors) {
    throw new Error(result.errors[0].message);
  }
  
  return result.data;
}

// Example usage
const data = await query(\`
  query GetRecords($collection: String!, $first: Int) {
    records(collection: $collection, first: $first) {
      edges {
        node {
          uri
          did
          collection
          record
          createdAt
        }
      }
    }
  }
\`, { collection: "app.bsky.feed.post", first: 10 });
\`\`\`

### JavaScript/TypeScript - graphql-request

\`\`\`javascript
import { GraphQLClient, gql } from "graphql-request";

const client = new GraphQLClient("${API_ENDPOINT}/graphql");

const query = gql\`
  query GetRecord($uri: String!) {
    record(uri: $uri) {
      uri
      did
      collection
      record
      createdAt
    }
  }
\`;

const data = await client.request(query, { 
  uri: "at://did:plc:xyz/app.example.record/abc123" 
});
\`\`\`

### Python - requests

\`\`\`python
import requests

def query(graphql_query, variables=None):
    response = requests.post(
        "${API_ENDPOINT}/graphql",
        json={
            "query": graphql_query,
            "variables": variables or {}
        },
        headers={"Content-Type": "application/json"}
    )
    result = response.json()
    
    if "errors" in result:
        raise Exception(result["errors"][0]["message"])
    
    return result["data"]

# Example usage
data = query("""
    query GetRecords($collection: String!, $first: Int) {
        records(collection: $collection, first: $first) {
            edges {
                node {
                    uri
                    did
                    collection
                    record
                }
            }
        }
    }
""", {"collection": "app.bsky.feed.post", "first": 10})
\`\`\`

### Python - gql

\`\`\`python
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

transport = RequestsHTTPTransport(
    url="${API_ENDPOINT}/graphql",
    headers={"Content-Type": "application/json"}
)

client = Client(transport=transport, fetch_schema_from_transport=True)

query = gql("""
    query GetRecord($uri: String!) {
        record(uri: $uri) {
            uri
            did
            collection
            record
            createdAt
        }
    }
""")

result = client.execute(query, variable_values={"uri": "at://did:plc:xyz/app.example.record/abc123"})
\`\`\`

### cURL

\`\`\`bash
# Basic query
curl -X POST "${API_ENDPOINT}/graphql" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "{ __typename }"}'

# Query with variables
curl -X POST "${API_ENDPOINT}/graphql" \\
  -H "Content-Type: application/json" \\
  -d '{
    "query": "query GetRecord($uri: String!) { record(uri: $uri) { uri did collection record } }",
    "variables": {"uri": "at://did:plc:xyz/app.example.record/abc123"}
  }'

# Introspection (discover schema)
curl -X POST "${API_ENDPOINT}/graphql" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "{ __schema { queryType { name } types { name kind fields { name type { name } } } } }"}'
\`\`\`

---

## GraphQL Schema

**IMPORTANT**: The schema is dynamically generated from uploaded Lexicon definitions. You MUST use introspection to discover available types and fields.

### Introspection Query

\`\`\`graphql
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      name
      kind
      description
      fields {
        name
        description
        type {
          name
          kind
          ofType {
            name
            kind
          }
        }
        args {
          name
          type {
            name
            kind
          }
        }
      }
    }
  }
}
\`\`\`

### Get All Type Names

\`\`\`graphql
query GetTypes {
  __schema {
    types {
      name
      kind
    }
  }
}
\`\`\`

### Get Fields for a Specific Type

\`\`\`graphql
query GetTypeFields($typeName: String!) {
  __type(name: $typeName) {
    name
    fields {
      name
      type {
        name
        kind
        ofType {
          name
        }
      }
    }
  }
}
\`\`\`

---

## Common Query Patterns

### Fetch Records by Collection

\`\`\`graphql
query GetRecords($collection: String!, $first: Int, $after: String) {
  records(collection: $collection, first: $first, after: $after) {
    edges {
      node {
        uri
        did
        collection
        record
        createdAt
        indexedAt
      }
      cursor
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      startCursor
      endCursor
    }
    totalCount
  }
}
\`\`\`

### Fetch Single Record by URI

\`\`\`graphql
query GetRecord($uri: String!) {
  record(uri: $uri) {
    uri
    did
    collection
    record
    createdAt
    indexedAt
  }
}
\`\`\`

### Fetch Records by DID (Author)

\`\`\`graphql
query GetRecordsByAuthor($did: String!, $first: Int) {
  records(did: $did, first: $first) {
    edges {
      node {
        uri
        collection
        record
        createdAt
      }
    }
  }
}
\`\`\`

### Fetch Records with Filters

\`\`\`graphql
query GetFilteredRecords(
  $collection: String
  $did: String
  $first: Int
  $after: String
) {
  records(
    collection: $collection
    did: $did
    first: $first
    after: $after
  ) {
    edges {
      node {
        uri
        did
        collection
        record
      }
      cursor
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
\`\`\`

---

## Pagination (Relay Specification)

Hypergoat uses Relay-style cursor-based pagination.

### Arguments

| Argument | Type | Description |
|----------|------|-------------|
| \`first\` | Int | Number of items from the start |
| \`after\` | String | Cursor to start after |
| \`last\` | Int | Number of items from the end |
| \`before\` | String | Cursor to start before |

### PageInfo Fields

| Field | Type | Description |
|-------|------|-------------|
| \`hasNextPage\` | Boolean | More items after current page |
| \`hasPreviousPage\` | Boolean | More items before current page |
| \`startCursor\` | String | Cursor of first item |
| \`endCursor\` | String | Cursor of last item |

### Forward Pagination Example

\`\`\`graphql
# First page
query {
  records(first: 10) {
    edges {
      node { uri }
      cursor
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}

# Next page (use endCursor from previous response)
query {
  records(first: 10, after: "cursor_value_here") {
    edges {
      node { uri }
      cursor
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
\`\`\`

### Backward Pagination Example

\`\`\`graphql
# Last page
query {
  records(last: 10) {
    edges {
      node { uri }
    }
    pageInfo {
      hasPreviousPage
      startCursor
    }
  }
}

# Previous page
query {
  records(last: 10, before: "cursor_value_here") {
    edges {
      node { uri }
    }
    pageInfo {
      hasPreviousPage
      startCursor
    }
  }
}
\`\`\`

---

## WebSocket Subscriptions

Real-time updates via WebSocket using the \`graphql-transport-ws\` protocol.

### Connection Setup

1. Connect to: \`${WS_ENDPOINT}/graphql\`
2. Set header: \`Sec-WebSocket-Protocol: graphql-transport-ws\`

### Protocol Flow

\`\`\`
Client                              Server
  |                                    |
  |-- connection_init --------------->|
  |<-- connection_ack ----------------|
  |                                    |
  |-- subscribe (id: "1") ----------->|
  |<-- next (id: "1", data) ----------|
  |<-- next (id: "1", data) ----------|
  |<-- next (id: "1", data) ----------|
  |                                    |
  |-- complete (id: "1") ------------>|
  |                                    |
\`\`\`

### Message Types

| Type | Direction | Payload | Description |
|------|-----------|---------|-------------|
| \`connection_init\` | Client→Server | \`{}\` or auth params | Initialize connection |
| \`connection_ack\` | Server→Client | \`{}\` | Connection accepted |
| \`ping\` | Both | \`{}\` | Keep-alive ping |
| \`pong\` | Both | \`{}\` | Keep-alive response |
| \`subscribe\` | Client→Server | \`{query, variables}\` | Start subscription |
| \`next\` | Server→Client | \`{data}\` | Subscription event |
| \`error\` | Server→Client | \`{errors}\` | Subscription error |
| \`complete\` | Both | \`{}\` | End subscription |

### JavaScript - graphql-ws

\`\`\`javascript
import { createClient } from "graphql-ws";

const client = createClient({
  url: "${WS_ENDPOINT}/graphql",
  connectionParams: {
    // Optional auth params
  },
  on: {
    connected: () => console.log("Connected"),
    closed: () => console.log("Disconnected"),
    error: (err) => console.error("Error:", err),
  },
});

// Subscribe to new records
const unsubscribe = client.subscribe(
  {
    query: \`
      subscription OnRecordCreated {
        recordCreated {
          uri
          did
          collection
          record
          createdAt
        }
      }
    \`,
  },
  {
    next: (result) => {
      console.log("New record:", result.data.recordCreated);
    },
    error: (err) => {
      console.error("Subscription error:", err);
    },
    complete: () => {
      console.log("Subscription ended");
    },
  }
);

// Later: unsubscribe
unsubscribe();
\`\`\`

### JavaScript - Apollo Client

\`\`\`javascript
import { ApolloClient, InMemoryCache, split, HttpLink } from "@apollo/client";
import { GraphQLWsLink } from "@apollo/client/link/subscriptions";
import { getMainDefinition } from "@apollo/client/utilities";
import { createClient } from "graphql-ws";

const httpLink = new HttpLink({
  uri: "${API_ENDPOINT}/graphql",
});

const wsLink = new GraphQLWsLink(
  createClient({
    url: "${WS_ENDPOINT}/graphql",
  })
);

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

const client = new ApolloClient({
  link: splitLink,
  cache: new InMemoryCache(),
});
\`\`\`

### Python - gql with websockets

\`\`\`python
import asyncio
from gql import gql, Client
from gql.transport.websockets import WebsocketsTransport

async def subscribe():
    transport = WebsocketsTransport(
        url="${WS_ENDPOINT}/graphql",
        subprotocols=["graphql-transport-ws"]
    )
    
    async with Client(transport=transport) as session:
        subscription = gql("""
            subscription {
                recordCreated {
                    uri
                    did
                    collection
                    record
                }
            }
        """)
        
        async for result in session.subscribe(subscription):
            print(f"New record: {result}")

asyncio.run(subscribe())
\`\`\`

### Raw WebSocket (any language)

\`\`\`
1. Connect to: ${WS_ENDPOINT}/graphql
   Header: Sec-WebSocket-Protocol: graphql-transport-ws

2. Send: {"type":"connection_init","payload":{}}

3. Receive: {"type":"connection_ack"}

4. Send: {
     "id": "unique-id-1",
     "type": "subscribe",
     "payload": {
       "query": "subscription { recordCreated { uri collection did record } }"
     }
   }

5. Receive events: {
     "id": "unique-id-1",
     "type": "next",
     "payload": {
       "data": {
         "recordCreated": {
           "uri": "at://did:plc:xxx/collection/rkey",
           "collection": "...",
           "did": "did:plc:xxx",
           "record": {...}
         }
       }
     }
   }

6. To unsubscribe: {"id":"unique-id-1","type":"complete"}
\`\`\`

---

## AT Protocol Record Structure

Records in Hypergoat follow the AT Protocol format:

### Record Fields

| Field | Type | Description |
|-------|------|-------------|
| \`uri\` | String | AT URI: \`at://{did}/{collection}/{rkey}\` |
| \`did\` | String | Decentralized Identifier of the author |
| \`collection\` | String | Lexicon NSID (e.g., \`app.bsky.feed.post\`) |
| \`rkey\` | String | Record key (unique within collection for this DID) |
| \`record\` | JSON | The actual record data (schema defined by lexicon) |
| \`createdAt\` | DateTime | When the record was created |
| \`indexedAt\` | DateTime | When Hypergoat indexed the record |

### AT URI Format

\`\`\`
at://did:plc:abcd1234/app.bsky.feed.post/3k2yihcrp6f2c
     └─────┬─────┘ └───────┬────────┘ └─────┬─────┘
          DID          Collection         Rkey
\`\`\`

### Example Record

\`\`\`json
{
  "uri": "at://did:plc:abcd1234/app.bsky.feed.post/3k2yihcrp6f2c",
  "did": "did:plc:abcd1234",
  "collection": "app.bsky.feed.post",
  "record": {
    "$type": "app.bsky.feed.post",
    "text": "Hello world!",
    "createdAt": "2024-01-15T10:30:00.000Z",
    "langs": ["en"]
  },
  "createdAt": "2024-01-15T10:30:00.000Z",
  "indexedAt": "2024-01-15T10:30:05.000Z"
}
\`\`\`

---

## Error Handling

### GraphQL Errors

Errors are returned in the \`errors\` array:

\`\`\`json
{
  "data": null,
  "errors": [
    {
      "message": "Record not found",
      "path": ["record"],
      "extensions": {
        "code": "NOT_FOUND"
      }
    }
  ]
}
\`\`\`

### Common Error Codes

| Code | Description |
|------|-------------|
| \`NOT_FOUND\` | Record/resource doesn't exist |
| \`INVALID_INPUT\` | Invalid query parameters |
| \`INTERNAL_ERROR\` | Server error |
| \`RATE_LIMITED\` | Too many requests |

### Handling Errors in Code

\`\`\`javascript
const result = await fetch("${API_ENDPOINT}/graphql", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ query, variables }),
}).then(r => r.json());

if (result.errors?.length > 0) {
  const error = result.errors[0];
  console.error(\`GraphQL Error: \${error.message}\`);
  console.error(\`Path: \${error.path?.join(".")}\`);
  console.error(\`Code: \${error.extensions?.code}\`);
  throw new Error(error.message);
}

return result.data;
\`\`\`

---

## Best Practices

### 1. Always Use Introspection First

The schema is dynamic. Before writing queries, discover what's available:

\`\`\`bash
curl -X POST "${API_ENDPOINT}/graphql" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "{ __schema { types { name kind } } }"}'
\`\`\`

### 2. Request Only Needed Fields

\`\`\`graphql
# Good - specific fields
query { records(first: 10) { edges { node { uri collection } } } }

# Avoid - requesting everything
query { records(first: 10) { edges { node { uri did collection record createdAt indexedAt } } } }
\`\`\`

### 3. Use Variables Instead of String Interpolation

\`\`\`javascript
// Good
const query = \`query GetRecord($uri: String!) { record(uri: $uri) { uri } }\`;
await client.request(query, { uri: userInput });

// Bad - SQL injection risk
const query = \`query { record(uri: "\${userInput}") { uri } }\`;
\`\`\`

### 4. Implement Pagination for Large Datasets

Never fetch all records at once. Use cursor-based pagination:

\`\`\`javascript
async function fetchAllRecords(collection) {
  const records = [];
  let cursor = null;
  
  while (true) {
    const result = await query(\`
      query($collection: String!, $cursor: String) {
        records(collection: $collection, first: 100, after: $cursor) {
          edges { node { uri record } cursor }
          pageInfo { hasNextPage endCursor }
        }
      }
    \`, { collection, cursor });
    
    records.push(...result.records.edges.map(e => e.node));
    
    if (!result.records.pageInfo.hasNextPage) break;
    cursor = result.records.pageInfo.endCursor;
  }
  
  return records;
}
\`\`\`

### 5. Handle WebSocket Reconnection

\`\`\`javascript
import { createClient } from "graphql-ws";

const client = createClient({
  url: "${WS_ENDPOINT}/graphql",
  retryAttempts: 5,
  shouldRetry: () => true,
  retryWait: async (retries) => {
    // Exponential backoff
    await new Promise(r => setTimeout(r, Math.min(1000 * 2 ** retries, 30000)));
  },
});
\`\`\`

### 6. Check for Partial Data

GraphQL can return partial results with some fields null and errors:

\`\`\`javascript
const result = await client.request(query);

if (result.errors) {
  console.warn("Partial errors:", result.errors);
}

// result.data may still contain valid data for fields that succeeded
\`\`\`

---

## Rate Limits

Currently no strict rate limits, but please be respectful:

- Avoid more than 100 requests/minute for queries
- Limit subscription connections per client
- Use pagination instead of fetching all data

---

## Useful Links

### Hypergoat & Hypersphere
- GraphiQL Explorer: ${API_ENDPOINT}/graphiql
- Hypersphere Explorer: https://impactindexer.org/
- Lexicon Reference: https://impactindexer.org/lexicon/
- Agent Lexicons: https://impactindexer.org/lexicon/agents
- GainForest: https://gainforest.earth

### AT Protocol & GraphQL
- AT Protocol Docs: https://atproto.com/docs
- GraphQL Spec: https://spec.graphql.org/
- graphql-ws Library: https://github.com/enisdenjo/graphql-ws

---

## Quick Reference

### Minimal Query Example

\`\`\`bash
curl -X POST ${API_ENDPOINT}/graphql \\
  -H "Content-Type: application/json" \\
  -d '{"query":"{ __typename }"}'
\`\`\`

### Minimal Subscription Example

\`\`\`javascript
import { createClient } from "graphql-ws";
const client = createClient({ url: "${WS_ENDPOINT}/graphql" });
client.subscribe(
  { query: "subscription { recordCreated { uri } }" },
  { next: console.log, error: console.error, complete: () => {} }
);
\`\`\`
`;

export async function GET() {
  return new Response(agentsMd, {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
}
