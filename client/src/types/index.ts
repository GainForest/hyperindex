// Statistics
export interface Statistics {
  recordCount: number;
  actorCount: number;
  lexiconCount: number;
}

// Settings
export interface Settings {
  domainAuthority: string;
  adminDids: string[];
  relayUrl: string;
  plcDirectoryUrl: string;
  jetstreamUrl: string;
  oauthSupportedScopes: string;
  labelerSubscribeEnabled: boolean;
  labelerSubscribeUrls: string[];
}

// Session
export interface Session {
  did: string;
  handle: string;
  isAdmin: boolean;
}

// Activity Bucket
export interface ActivityBucket {
  timestamp: string;
  creates: number;
  updates: number;
  deletes: number;
}

// Activity Entry
export interface ActivityEntry {
  id: number;
  timestamp: string;
  operation: string;
  collection: string;
  did: string;
  rkey?: string;
  status: string;
  errorMessage?: string;
}

// Lexicon
export interface Lexicon {
  id: string;
  json: string;
  createdAt: string;
}

// OAuth Client
export interface OAuthClient {
  clientId: string;
  clientSecret?: string;
  clientName: string;
  clientType: string;
  redirectUris: string[];
  createdAt: string;
}

// Time Range
export type TimeRange = "ONE_HOUR" | "THREE_HOURS" | "SIX_HOURS" | "ONE_DAY" | "SEVEN_DAYS";

// Query Responses
export interface StatisticsResponse {
  statistics: Statistics;
}

export interface SettingsResponse {
  settings: Settings;
}

export interface CurrentSessionResponse {
  currentSession: Session | null;
}

export interface ActivityBucketsResponse {
  activityBuckets: ActivityBucket[];
}

export interface RecentActivityResponse {
  recentActivity: ActivityEntry[];
}

export interface LexiconsResponse {
  lexicons: Lexicon[];
}

export interface OAuthClientsResponse {
  oauthClients: OAuthClient[];
}

export interface IsBackfillingResponse {
  isBackfilling: boolean;
}

export interface PurgeActorPreview {
  did: string;
  isValidDid: boolean;
  actorExists: boolean;
  recordCount: number;
}

export interface PurgeActorPreviewResponse {
  purgeActorPreview: PurgeActorPreview;
}
