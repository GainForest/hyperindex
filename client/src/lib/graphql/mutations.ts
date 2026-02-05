import { gql } from "graphql-request";

// Update Settings
export const UPDATE_SETTINGS = gql`
  mutation UpdateSettings(
    $domainAuthority: String
    $adminDids: [String!]
    $relayUrl: String
    $plcDirectoryUrl: String
    $jetstreamUrl: String
    $oauthSupportedScopes: String
  ) {
    updateSettings(
      domainAuthority: $domainAuthority
      adminDids: $adminDids
      relayUrl: $relayUrl
      plcDirectoryUrl: $plcDirectoryUrl
      jetstreamUrl: $jetstreamUrl
      oauthSupportedScopes: $oauthSupportedScopes
    ) {
      domainAuthority
      adminDids
      relayUrl
      plcDirectoryUrl
      jetstreamUrl
      oauthSupportedScopes
    }
  }
`;

// Trigger Backfill
export const TRIGGER_BACKFILL = gql`
  mutation TriggerBackfill {
    triggerBackfill
  }
`;

// Backfill Actor
export const BACKFILL_ACTOR = gql`
  mutation BackfillActor($did: String!) {
    backfillActor(did: $did)
  }
`;

// Upload Lexicons
export const UPLOAD_LEXICONS = gql`
  mutation UploadLexicons($zipBase64: String!) {
    uploadLexicons(zipBase64: $zipBase64)
  }
`;

// Reset All
export const RESET_ALL = gql`
  mutation ResetAll($confirm: String!) {
    resetAll(confirm: $confirm)
  }
`;

// Create OAuth Client
export const CREATE_OAUTH_CLIENT = gql`
  mutation CreateOAuthClient(
    $clientName: String!
    $clientType: String!
    $redirectUris: [String!]!
  ) {
    createOAuthClient(
      clientName: $clientName
      clientType: $clientType
      redirectUris: $redirectUris
    ) {
      clientId
      clientSecret
      clientName
      clientType
      redirectUris
      createdAt
    }
  }
`;

// Update OAuth Client
export const UPDATE_OAUTH_CLIENT = gql`
  mutation UpdateOAuthClient(
    $clientId: String!
    $clientName: String!
    $redirectUris: [String!]!
  ) {
    updateOAuthClient(
      clientId: $clientId
      clientName: $clientName
      redirectUris: $redirectUris
    ) {
      clientId
      clientSecret
      clientName
      clientType
      redirectUris
      createdAt
    }
  }
`;

// Delete OAuth Client
export const DELETE_OAUTH_CLIENT = gql`
  mutation DeleteOAuthClient($clientId: String!) {
    deleteOAuthClient(clientId: $clientId)
  }
`;

// Add Admin
export const ADD_ADMIN = gql`
  mutation AddAdmin($did: String!) {
    addAdmin(did: $did)
  }
`;

// Remove Admin
export const REMOVE_ADMIN = gql`
  mutation RemoveAdmin($did: String!) {
    removeAdmin(did: $did)
  }
`;

// Register Lexicon (resolves via DNS)
export const REGISTER_LEXICON = gql`
  mutation RegisterLexicon($nsid: String!) {
    registerLexicon(nsid: $nsid) {
      id
      json
      createdAt
    }
  }
`;

// Delete Lexicon
export const DELETE_LEXICON = gql`
  mutation DeleteLexicon($nsid: String!) {
    deleteLexicon(nsid: $nsid)
  }
`;
