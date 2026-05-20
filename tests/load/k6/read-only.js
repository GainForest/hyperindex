import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || 'https://dev.api.indexer.hypercerts.dev').replace(/\/$/, '');
const GRAPHQL_URL = __ENV.GRAPHQL_URL || `${BASE_URL}/graphql`;
const PROFILE = __ENV.PROFILE || 'smoke';
const OPERATION = __ENV.OPERATION || '';
const INCLUDE_EXPENSIVE = (__ENV.INCLUDE_EXPENSIVE || '').toLowerCase() === 'true';
const THINK_TIME_SECONDS = Number(__ENV.THINK_TIME_SECONDS || '1');
const LOG_ERRORS = (__ENV.LOG_ERRORS || '').toLowerCase() === 'true';

const profiles = {
  smoke: [
    { duration: '20s', target: 5 },
    { duration: '40s', target: 5 },
    { duration: '20s', target: 0 },
  ],
  baseline: [
    { duration: '1m', target: 5 },
    { duration: '3m', target: 20 },
    { duration: '3m', target: 50 },
    { duration: '1m', target: 0 },
  ],
  spike: [
    { duration: '10s', target: 100 },
    { duration: '2m', target: 100 },
    { duration: '20s', target: 0 },
  ],
  soak: [
    { duration: '2m', target: 25 },
    { duration: '30m', target: 25 },
    { duration: '2m', target: 0 },
  ],
};

export const options = {
  stages: profiles[PROFILE] || profiles.smoke,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<2000'],
    graphql_error_rate: ['rate<0.01'],
    graphql_request_duration: ['p(95)<2000'],
  },
  userAgent: 'hyperindex-k6-read-only-load-test/1.0',
};

const graphqlErrorRate = new Rate('graphql_error_rate');
const graphqlRequestDuration = new Trend('graphql_request_duration', true);

const operations = [
  {
    name: 'collectionStats',
    weight: 15,
    query: `
      query CollectionStats {
        collectionStats {
          collection
          count
        }
      }
    `,
  },
  {
    name: 'latestActivities',
    weight: 25,
    query: `
      query LatestActivities {
        orgHypercertsClaimActivity(first: 20) {
          edges {
            cursor
            node {
              uri
              did
              rkey
              title
              shortDescription
              createdAt
              startDate
              endDate
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
  {
    name: 'genericRecordsMaxMetadata',
    weight: 12,
    query: `
      query GenericRecordsMaxMetadata {
        records(collection: "org.hypercerts.claim.activity", first: 100) {
          totalCount
          edges {
            cursor
            node {
              uri
              did
              rkey
              cid
              collection
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
  {
    name: 'genericRecordsRawValueMax',
    weight: 4,
    expensive: true,
    query: `
      query GenericRecordsRawValueMax {
        records(collection: "org.hypercerts.claim.activity", first: 100) {
          totalCount
          edges {
            cursor
            node {
              uri
              did
              collection
              value
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
  {
    name: 'activityContainsWithCount',
    weight: 15,
    query: `
      query ActivityContainsWithCount {
        orgHypercertsClaimActivity(
          where: { title: { contains: "E2E" } }
          sortBy: createdAt
          sortDirection: DESC
          first: 20
        ) {
          totalCount
          edges {
            node {
              uri
              title
              createdAt
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
  {
    name: 'activitySortedByStartDate',
    weight: 10,
    query: `
      query ActivitySortedByStartDate {
        orgHypercertsClaimActivity(sortBy: startDate, sortDirection: ASC, first: 50) {
          edges {
            node {
              uri
              title
              startDate
              endDate
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
  {
    name: 'activityByUri',
    weight: 10,
    query: `
      query ActivityByUri($uri: String!) {
        orgHypercertsClaimActivityByUri(uri: $uri) {
          uri
          did
          rkey
          title
          shortDescription
          createdAt
        }
      }
    `,
    variables: (data) => ({ uri: data.sampleActivityUri }),
    skip: (data) => !data.sampleActivityUri,
  },
  {
    name: 'activityTimeSeries',
    weight: 6,
    query: `
      query ActivityTimeSeries {
        collectionTimeSeries(collection: "org.hypercerts.claim.activity") {
          collection
          totalRecords
          uniqueUsers
          data {
            date
            count
            cumulative
          }
        }
      }
    `,
  },
  {
    name: 'activitySearch',
    weight: 7,
    query: `
      query ActivitySearch {
        search(query: "E2E", collection: "org.hypercerts.claim.activity", first: 20) {
          edges {
            cursor
            node {
              uri
              did
              collection
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    `,
  },
];

const activeOperations = OPERATION
  ? operations.filter((operation) => operation.name === OPERATION)
  : operations.filter((operation) => INCLUDE_EXPENSIVE || !operation.expensive);

if (OPERATION && activeOperations.length === 0) {
  throw new Error(`unknown OPERATION=${OPERATION}; expected one of: ${operations.map((operation) => operation.name).join(', ')}`);
}

const totalWeight = activeOperations.reduce((sum, operation) => sum + operation.weight, 0);

export function setup() {
  const res = http.post(
    GRAPHQL_URL,
    JSON.stringify({
      query: `
        query LoadTestSetup {
          orgHypercertsClaimActivity(first: 1) {
            edges {
              node {
                uri
              }
            }
          }
        }
      `,
    }),
    { headers: { 'Content-Type': 'application/json' }, tags: { operation: 'setup' } },
  );

  if (res.status !== 200) {
    console.warn(`setup query returned HTTP ${res.status}`);
    return { sampleActivityUri: '' };
  }

  try {
    const body = res.json();
    return {
      sampleActivityUri: body.data?.orgHypercertsClaimActivity?.edges?.[0]?.node?.uri || '',
    };
  } catch (err) {
    console.warn(`setup query response was not JSON: ${err}`);
    return { sampleActivityUri: '' };
  }
}

export default function (data) {
  const operation = chooseOperation(data);
  const variables = operation.variables ? operation.variables(data) : undefined;

  const res = http.post(
    GRAPHQL_URL,
    JSON.stringify({ query: operation.query, variables }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { operation: operation.name },
      timeout: '10s',
    },
  );

  graphqlRequestDuration.add(res.timings.duration, { operation: operation.name });

  let graphqlErrors = false;
  let responseIsJSON = true;
  try {
    const body = res.json();
    graphqlErrors = Boolean(body.errors && body.errors.length > 0);
  } catch (_err) {
    responseIsJSON = false;
    graphqlErrors = true;
  }

  graphqlErrorRate.add(graphqlErrors, { operation: operation.name });

  if (LOG_ERRORS && (res.status !== 200 || graphqlErrors || !responseIsJSON)) {
    console.warn(`${operation.name} failed: status=${res.status} error=${res.error || ''} body=${String(res.body || '').slice(0, 300)}`);
  }

  check(res, {
    'status is 200': (r) => r.status === 200,
    'response is json': () => responseIsJSON,
    'no graphql errors': () => !graphqlErrors,
  }, { operation: operation.name });

  if (THINK_TIME_SECONDS > 0) {
    sleep(THINK_TIME_SECONDS);
  }
}

function chooseOperation(data) {
  let pick = Math.random() * totalWeight;
  for (const operation of activeOperations) {
    if (operation.skip && operation.skip(data)) {
      continue;
    }
    pick -= operation.weight;
    if (pick <= 0) {
      return operation;
    }
  }
  return activeOperations[0];
}
