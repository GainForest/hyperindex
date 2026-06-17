import { env } from "@/lib/env";
const API_ENDPOINT = env.NEXT_PUBLIC_HYPERINDEX_URL || "http://localhost:8080";
const WS_ENDPOINT = API_ENDPOINT.replace("https://", "wss://").replace("http://", "ws://");

const agentsMd = `# Hyperindex API - Complete Integration Guide for AI Agents

## What is Hyperindex?

**Hyperindex** is GainForest's AT Protocol AppView server for the Hypersphere ecosystem. It indexes Lexicon-defined records from the AT Protocol network and exposes them via a dynamically-generated GraphQL API.

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

### What Hyperindex Indexes

Hyperindex indexes records defined by Lexicons in the Hypersphere ecosystem. The primary lexicons include:

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

### Fetch Records by Collection (Generic)

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

### Typed Collection Query with Filters

When lexicons are loaded, typed query fields are generated. Each supports \`where\`, \`sortBy\`, and \`sortDirection\` arguments.

\`\`\`graphql
query GetFilteredPosts($first: Int, $after: String) {
  appBskyFeedPost(
    where: {
      text: { contains: "climate" }
      did: { eq: "did:plc:abc123" }
    }
    sortBy: "createdAt"
    sortDirection: DESC
    first: $first
    after: $after
  ) {
    edges {
      node {
        uri
        did
        rkey
        text
        createdAt
      }
      cursor
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
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

### Cross-Collection Text Search

\`\`\`graphql
query SearchRecords($query: String!, $collection: String, $first: Int) {
  search(query: $query, collection: $collection, first: $first) {
    edges {
      node {
        uri
        did
        collection
        value
      }
      cursor
    }
    pageInfo {
      hasNextPage
      endCursor
    }
    totalCount
  }
}
\`\`\`

---

## Filtering (\`where\` argument)

Typed collection queries accept a \`where\` argument for field-level filtering. The available filter fields are generated from each lexicon's schema.

### Filter Operators

| Operator | Applicable Types | Description | Example |
|----------|-----------------|-------------|---------|
| \`eq\` | String, Int, Float, Boolean, DateTime | Exact equality | \`{ title: { eq: "Hello" } }\` |
| \`neq\` | String, Int, Float, DateTime | Not equal | \`{ status: { neq: "draft" } }\` |
| \`gt\` | Int, Float, DateTime | Greater than | \`{ score: { gt: 5 } }\` |
| \`lt\` | Int, Float, DateTime | Less than | \`{ score: { lt: 100 } }\` |
| \`gte\` | Int, Float, DateTime | Greater or equal | \`{ createdAt: { gte: "2024-01-01" } }\` |
| \`lte\` | Int, Float, DateTime | Less or equal | \`{ createdAt: { lte: "2024-12-31" } }\` |
| \`in\` | String, Int | Value in list | \`{ type: { in: ["post", "reply"] } }\` |
| \`contains\` | String | Substring match | \`{ text: { contains: "forest" } }\` |
| \`startsWith\` | String | Prefix match | \`{ name: { startsWith: "Gain" } }\` |
| \`isNull\` | Scalar fields and complex top-level fields | Missing/null or present check | \`{ optionalField: { isNull: true } }\` |

### Complex and Nested Field Filtering

Scalar fields keep the typed operators above. Complex fields expose \`isNull\` for presence checks. Some complex fields use the shared \`PresenceFilterInput\`; arrays, refs, and unions may instead expose generated nested filter inputs up to three lexicon path segments deep. Do not rely on the input type name for presence checks; introspect the field and use \`isNull\`:

\`\`\`graphql
where: {
  image: { isNull: false }
  contributors: {
    any: {
      contributorIdentity: { identity: { eq: "did:plc:example" } }
    }
  }
}
\`\`\`

Nested scalar leaves support exact operators only: \`eq\`, \`in\`, and \`isNull\`. Multiple predicates inside the same array \`any\` must match the same array item. Nested filters do not support substring operators (\`contains\`, \`startsWith\`), comparison operators (\`gt\`, \`lt\`, \`gte\`, \`lte\`), nested sorting, arbitrary JSON paths, or strong-ref dereferencing.

\`isNull: true\` matches records where the JSON field is missing or explicitly \`null\`. \`isNull: false\` matches records where the field is present and non-null. Empty arrays \`[]\`, empty objects \`{}\`, and empty strings \`""\` count as present.

Presence filtering does not guarantee a complex value conforms to the generated typed GraphQL field. Nullable union fields with unknown \`$type\` values or missing required top-level fields resolve to \`null\` instead of being coerced to another union member.

For Hypercerts activities, \`contributorDid\` is a compatibility filter that matches inline contributor DIDs, legacy bare DID array items, and contributorInformation strong refs by their referenced \`identifier\`:

\`\`\`graphql
where: { contributorDid: { eq: "did:plc:example" } }
\`\`\`

The generic \`records(collection: ...)\` query is unchanged and does not accept generated \`where\` filters.

To discover generated where inputs, introspect the collection input object with \`inputFields\`:

\`\`\`graphql
{
  __type(name: "OrgHypercertsClaimActivityWhereInput") {
    inputFields {
      name
      type { name kind ofType { name kind } }
    }
  }
}
\`\`\`

### DID (Author) Filtering

Every \`where\` input includes a \`did\` field for filtering by record author:

\`\`\`graphql
query {
  appBskyFeedPost(where: { did: { eq: "did:plc:abc123" } }) {
    edges { node { uri text } }
  }
}
\`\`\`

### Combining Filters

Multiple filter fields are ANDed together. Multiple operators on the same field are also ANDed:

\`\`\`graphql
query {
  appBskyFeedPost(
    where: {
      did: { eq: "did:plc:abc123" }
      text: { contains: "carbon" }
      createdAt: { gte: "2024-01-01", lte: "2024-12-31" }
    }
    first: 20
  ) {
    edges { node { uri text createdAt } }
    totalCount
  }
}
\`\`\`

## Sorting

Typed collection queries support sorting by any scalar field with \`sortBy\` and \`sortDirection\` arguments.

### Arguments

| Argument | Type | Default | Description |
|----------|------|---------|-------------|
| \`sortBy\` | Enum (per-collection) | \`indexed_at\` | Field to sort by |
| \`sortDirection\` | \`ASC\` \| \`DESC\` | \`DESC\` | Sort direction |

### Example

\`\`\`graphql
query {
  appBskyFeedPost(sortBy: "createdAt", sortDirection: ASC, first: 10) {
    edges {
      node { uri text createdAt }
      cursor
    }
    pageInfo { hasNextPage endCursor }
  }
}
\`\`\`

Sortable fields are generated per-collection from the lexicon. Only scalar types (string, integer, number, boolean, datetime) are sortable. The meta-field \`indexed_at\` is always available.

## Text Search

Use the \`search\` query for cross-collection or per-collection text search:

\`\`\`graphql
query {
  search(query: "carbon credits", collection: "app.bsky.feed.post", first: 20) {
    edges {
      node { uri did collection value }
    }
    totalCount
  }
}
\`\`\`

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| \`query\` | String! | Yes | Search text (minimum 3 characters) |
| \`collection\` | String | No | Restrict search to a collection NSID |
| \`first\` | Int | No | Page size (default 20, max 1000) |
| \`after\` | String | No | Cursor for pagination |

Search is case-insensitive and matches against the full JSON content of records.

---

## Pagination (Relay Specification)

Hyperindex uses Relay-style cursor-based pagination.

### Arguments

| Argument | Type | Description |
|----------|------|-------------|
| \`first\` | Int | Number of items from the start |
| \`after\` | String | Cursor to start after |
| \`last\` | Int | Number of items from the end |
| \`before\` | String | Cursor to start before |

**Note:** The maximum page size is 1000. Requests with \`first\` or \`last\` greater than 1000 are silently clamped. You cannot use \`first\`/\`after\` and \`last\`/\`before\` simultaneously — doing so returns a GraphQL error.

### totalCount

Connections include an optional \`totalCount\` field. It is **opt-in**: the count query only executes when \`totalCount\` is included in your selection set.

\`\`\`graphql
# totalCount IS computed (you selected it)
query {
  records(collection: "app.bsky.feed.post", first: 10) {
    edges { node { uri } }
    totalCount
  }
}

# totalCount is NOT computed (not selected — no performance cost)
query {
  records(collection: "app.bsky.feed.post", first: 10) {
    edges { node { uri } }
  }
}
\`\`\`

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

Records in Hyperindex follow the AT Protocol format:

### Record Fields

| Field | Type | Description |
|-------|------|-------------|
| \`uri\` | String | AT URI: \`at://{did}/{collection}/{rkey}\` |
| \`did\` | String | Decentralized Identifier of the author |
| \`collection\` | String | Lexicon NSID (e.g., \`app.bsky.feed.post\`) |
| \`rkey\` | String | Record key (unique within collection for this DID) |
| \`record\` | JSON | The actual record data (schema defined by lexicon) |
| \`createdAt\` | DateTime | When the record was created |
| \`indexedAt\` | DateTime | When Hyperindex indexed the record |

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

### Hyperindex & Hypersphere
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
