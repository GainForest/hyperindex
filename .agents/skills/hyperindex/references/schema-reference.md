# Hyperindex GraphQL Schema Reference

Generated from live introspection of `https://api.indexer.hypercerts.dev/graphql` on 2026-06-10.

## Endpoints

- Production GraphQL: `https://api.indexer.hypercerts.dev/graphql`
- Staging GraphQL: `https://dev.api.indexer.hypercerts.dev/graphql`

## Query fields

| Query | Arguments | Purpose |
| --- | --- | --- |
| `appCertifiedActorOrganization` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedActorOrganizationSortField`, sortDirection: `SortDirection`, where: `AppCertifiedActorOrganizationWhereInput` | Query app.certified.actor.organization records |
| `appCertifiedActorOrganizationByUri` | uri: `String!` | Get a single app.certified.actor.organization by AT-URI |
| `appCertifiedActorProfile` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedActorProfileSortField`, sortDirection: `SortDirection`, where: `AppCertifiedActorProfileWhereInput` | Query app.certified.actor.profile records |
| `appCertifiedActorProfileByUri` | uri: `String!` | Get a single app.certified.actor.profile by AT-URI |
| `appCertifiedBadgeAward` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedBadgeAwardSortField`, sortDirection: `SortDirection`, where: `AppCertifiedBadgeAwardWhereInput` | Query app.certified.badge.award records |
| `appCertifiedBadgeAwardByUri` | uri: `String!` | Get a single app.certified.badge.award by AT-URI |
| `appCertifiedBadgeDefinition` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedBadgeDefinitionSortField`, sortDirection: `SortDirection`, where: `AppCertifiedBadgeDefinitionWhereInput` | Query app.certified.badge.definition records |
| `appCertifiedBadgeDefinitionByUri` | uri: `String!` | Get a single app.certified.badge.definition by AT-URI |
| `appCertifiedBadgeResponse` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedBadgeResponseSortField`, sortDirection: `SortDirection`, where: `AppCertifiedBadgeResponseWhereInput` | Query app.certified.badge.response records |
| `appCertifiedBadgeResponseByUri` | uri: `String!` | Get a single app.certified.badge.response by AT-URI |
| `appCertifiedGraphFollow` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedGraphFollowSortField`, sortDirection: `SortDirection`, where: `AppCertifiedGraphFollowWhereInput` | Query app.certified.graph.follow records |
| `appCertifiedGraphFollowByUri` | uri: `String!` | Get a single app.certified.graph.follow by AT-URI |
| `appCertifiedLinkEvm` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedLinkEvmSortField`, sortDirection: `SortDirection`, where: `AppCertifiedLinkEvmWhereInput` | Query app.certified.link.evm records |
| `appCertifiedLinkEvmByUri` | uri: `String!` | Get a single app.certified.link.evm by AT-URI |
| `appCertifiedLocation` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `AppCertifiedLocationSortField`, sortDirection: `SortDirection`, where: `AppCertifiedLocationWhereInput` | Query app.certified.location records |
| `appCertifiedLocationByUri` | uri: `String!` | Get a single app.certified.location by AT-URI |
| `orgHypercertsClaimActivity` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsClaimActivitySortField`, sortDirection: `SortDirection`, where: `OrgHypercertsClaimActivityWhereInput` | Query org.hypercerts.claim.activity records |
| `orgHypercertsClaimActivityByUri` | uri: `String!` | Get a single org.hypercerts.claim.activity by AT-URI |
| `orgHypercertsClaimContribution` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsClaimContributionSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsClaimContributionWhereInput` | Query org.hypercerts.claim.contribution records |
| `orgHypercertsClaimContributionByUri` | uri: `String!` | Get a single org.hypercerts.claim.contribution by AT-URI |
| `orgHypercertsClaimContributorInformation` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsClaimContributorInformationSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsClaimContributorInformationWhereInput` | Query org.hypercerts.claim.contributorInformation records |
| `orgHypercertsClaimContributorInformationByUri` | uri: `String!` | Get a single org.hypercerts.claim.contributorInformation by AT-URI |
| `orgHypercertsClaimRights` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsClaimRightsSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsClaimRightsWhereInput` | Query org.hypercerts.claim.rights records |
| `orgHypercertsClaimRightsByUri` | uri: `String!` | Get a single org.hypercerts.claim.rights by AT-URI |
| `orgHypercertsCollection` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsCollectionSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsCollectionWhereInput` | Query org.hypercerts.collection records |
| `orgHypercertsCollectionByUri` | uri: `String!` | Get a single org.hypercerts.collection by AT-URI |
| `orgHypercertsContextAcknowledgement` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsContextAcknowledgementSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsContextAcknowledgementWhereInput` | Query org.hypercerts.context.acknowledgement records |
| `orgHypercertsContextAcknowledgementByUri` | uri: `String!` | Get a single org.hypercerts.context.acknowledgement by AT-URI |
| `orgHypercertsContextAttachment` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsContextAttachmentSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsContextAttachmentWhereInput` | Query org.hypercerts.context.attachment records |
| `orgHypercertsContextAttachmentByUri` | uri: `String!` | Get a single org.hypercerts.context.attachment by AT-URI |
| `orgHypercertsContextEvaluation` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsContextEvaluationSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsContextEvaluationWhereInput` | Query org.hypercerts.context.evaluation records |
| `orgHypercertsContextEvaluationByUri` | uri: `String!` | Get a single org.hypercerts.context.evaluation by AT-URI |
| `orgHypercertsContextMeasurement` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsContextMeasurementSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsContextMeasurementWhereInput` | Query org.hypercerts.context.measurement records |
| `orgHypercertsContextMeasurementByUri` | uri: `String!` | Get a single org.hypercerts.context.measurement by AT-URI |
| `orgHypercertsFundingReceipt` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsFundingReceiptSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsFundingReceiptWhereInput` | Query org.hypercerts.funding.receipt records |
| `orgHypercertsFundingReceiptByUri` | uri: `String!` | Get a single org.hypercerts.funding.receipt by AT-URI |
| `orgHypercertsWorkscopeTag` | after: `String`, before: `String`, first: `Int`, last: `Int`, sortBy: `OrgHypercertsWorkscopeTagSortField`, sortDirection: `SortDirection`, where: `OrgHypercertsWorkscopeTagWhereInput` | Query org.hypercerts.workscope.tag records |
| `orgHypercertsWorkscopeTagByUri` | uri: `String!` | Get a single org.hypercerts.workscope.tag by AT-URI |
| `records` | after: `String`, before: `String`, collection: `String!`, first: `Int`, last: `Int` | Query records from any collection (useful for collections without lexicon schemas) |
| `externalLabels` | activeOnly: `Boolean`, sources: `[String!]`, subjects: `[String!]!`, values: `[String!]` | Query locally ingested external ATProto labels by DID or AT-URI subject. |
| `search` | after: `String`, collection: `String`, first: `Int`, query: `String!` | Search records by text content |
| `collectionStats` | collections: `[String!]` | Get record counts for collections (efficient aggregate query) |
| `collectionTimeSeries` | collection: `String!` | Get time series data for a collection (records grouped by date) |

## Typed record collections

| Collection | List query | Single-record query | Record type |
| --- | --- | --- | --- |
| `app.certified.actor.organization` | `appCertifiedActorOrganization` | `appCertifiedActorOrganizationByUri` | `AppCertifiedActorOrganization` |
| `app.certified.actor.profile` | `appCertifiedActorProfile` | `appCertifiedActorProfileByUri` | `AppCertifiedActorProfile` |
| `app.certified.badge.award` | `appCertifiedBadgeAward` | `appCertifiedBadgeAwardByUri` | `AppCertifiedBadgeAward` |
| `app.certified.badge.definition` | `appCertifiedBadgeDefinition` | `appCertifiedBadgeDefinitionByUri` | `AppCertifiedBadgeDefinition` |
| `app.certified.badge.response` | `appCertifiedBadgeResponse` | `appCertifiedBadgeResponseByUri` | `AppCertifiedBadgeResponse` |
| `app.certified.graph.follow` | `appCertifiedGraphFollow` | `appCertifiedGraphFollowByUri` | `AppCertifiedGraphFollow` |
| `app.certified.link.evm` | `appCertifiedLinkEvm` | `appCertifiedLinkEvmByUri` | `AppCertifiedLinkEvm` |
| `app.certified.location` | `appCertifiedLocation` | `appCertifiedLocationByUri` | `AppCertifiedLocation` |
| `org.hypercerts.claim.activity` | `orgHypercertsClaimActivity` | `orgHypercertsClaimActivityByUri` | `OrgHypercertsClaimActivity` |
| `org.hypercerts.claim.contribution` | `orgHypercertsClaimContribution` | `orgHypercertsClaimContributionByUri` | `OrgHypercertsClaimContribution` |
| `org.hypercerts.claim.contributorInformation` | `orgHypercertsClaimContributorInformation` | `orgHypercertsClaimContributorInformationByUri` | `OrgHypercertsClaimContributorInformation` |
| `org.hypercerts.claim.rights` | `orgHypercertsClaimRights` | `orgHypercertsClaimRightsByUri` | `OrgHypercertsClaimRights` |
| `org.hypercerts.collection` | `orgHypercertsCollection` | `orgHypercertsCollectionByUri` | `OrgHypercertsCollection` |
| `org.hypercerts.context.acknowledgement` | `orgHypercertsContextAcknowledgement` | `orgHypercertsContextAcknowledgementByUri` | `OrgHypercertsContextAcknowledgement` |
| `org.hypercerts.context.attachment` | `orgHypercertsContextAttachment` | `orgHypercertsContextAttachmentByUri` | `OrgHypercertsContextAttachment` |
| `org.hypercerts.context.evaluation` | `orgHypercertsContextEvaluation` | `orgHypercertsContextEvaluationByUri` | `OrgHypercertsContextEvaluation` |
| `org.hypercerts.context.measurement` | `orgHypercertsContextMeasurement` | `orgHypercertsContextMeasurementByUri` | `OrgHypercertsContextMeasurement` |
| `org.hypercerts.funding.receipt` | `orgHypercertsFundingReceipt` | `orgHypercertsFundingReceiptByUri` | `OrgHypercertsFundingReceipt` |
| `org.hypercerts.workscope.tag` | `orgHypercertsWorkscopeTag` | `orgHypercertsWorkscopeTagByUri` | `OrgHypercertsWorkscopeTag` |

Typed list queries accept Relay-style pagination arguments (`first`, `after`, `last`, `before`), plus `where`, `sortBy`, and `sortDirection` when the collection exposes those inputs.

## Filter inputs

Scalar field filters support value comparisons. Generated typed `where` inputs also include a metadata-level `uri: URIFilterInput` field so clients can filter by exact AT-URI or batch hydrate records by URI without querying JSON payload fields. Complex fields expose `isNull` presence checks. Arrays, refs, and unions may also expose generated nested filters up to three lexicon path segments deep; nested scalar leaves use exact filters (`eq`, `in`, `isNull`) and array filters use `any`. Nested filters do not support `contains`, `startsWith`, arbitrary JSON paths, nested sorting, or automatic strong-ref dereferencing.

### `StringFilterInput`

Filter conditions for string fields

| Field | Type | Description |
| --- | --- | --- |
| `contains` | `String` | Contains substring |
| `startsWith` | `String` | Starts with prefix |
| `isNull` | `Boolean` | Field is null |
| `eq` | `String` | Equal to |
| `neq` | `String` | Not equal to |
| `in` | `[String!]` | Value is in list |

### `ExactStringFilterInput`

Exact filter conditions for nested string fields.

| Field | Type | Description |
| --- | --- | --- |
| `eq` | `String` | Equal to |
| `in` | `[String!]` | Value is in list |
| `isNull` | `Boolean` | Field is null |

### `BooleanFilterInput`

Filter conditions for boolean fields

| Field | Type | Description |
| --- | --- | --- |
| `eq` | `Boolean` | Equal to |
| `isNull` | `Boolean` | Field is null |

### `DateTimeFilterInput`

Filter conditions for datetime fields

| Field | Type | Description |
| --- | --- | --- |
| `neq` | `DateTime` | Not equal to |
| `gt` | `DateTime` | Greater than |
| `lt` | `DateTime` | Less than |
| `gte` | `DateTime` | Greater than or equal to |
| `lte` | `DateTime` | Less than or equal to |
| `isNull` | `Boolean` | Field is null |
| `eq` | `DateTime` | Equal to |

### `ExactDateTimeFilterInput`

Exact filter conditions for nested datetime fields.

| Field | Type | Description |
| --- | --- | --- |
| `eq` | `DateTime` | Equal to |
| `in` | `[DateTime!]` | Value is in list |
| `isNull` | `Boolean` | Field is null |

### `DIDFilterInput`

Filter conditions for DID fields (column-level). Only eq and in are supported.

| Field | Type | Description |
| --- | --- | --- |
| `eq` | `String` | Equals |
| `in` | `[String!]` | In list |

### `URIFilterInput`

Filter conditions for AT-URI metadata fields. Only eq and in are supported.

| Field | Type | Description |
| --- | --- | --- |
| `eq` | `String` | Equals |
| `in` | `[String!]` | In list |

### `PresenceFilterInput`

Filter conditions for checking whether a top-level JSON field is missing/null or present.

| Field | Type | Description |
| --- | --- | --- |
| `isNull` | `Boolean!` | True matches missing or null fields; false matches present and non-null fields. |

### `ExternalLabelPredicateInput`

Filter conditions for matching external labels on records.

| Field | Type | Description |
| --- | --- | --- |
| `src` | `StringFilterInput` | Filter by label source DID. |
| `val` | `StringFilterInput` | Filter by label value. |
| `activeOnly` | `Boolean` | When true, only active external labels can match records. |

### `ExternalLabelWhereInput`

Record-level external label predicates.

| Field | Type | Description |
| --- | --- | --- |
| `has` | `ExternalLabelPredicateInput` | Keep records that have a matching external label. |
| `none` | `ExternalLabelPredicateInput` | Keep records that do not have a matching external label. |

## External label support

Production exposes external ATProto labels through the root `externalLabels` query, each generated record type's virtual `externalLabels` field, and `where.externalLabels.has` / `where.externalLabels.none` predicates on typed list queries.

### `ExternalLabel`

| Field | Type | Description |
| --- | --- | --- |
| `cid` | `String` | Optional CID restriction for labels that apply to one record version. |
| `cts` | `String!` | Label creation timestamp from the label event. |
| `exp` | `String` | Optional label expiration timestamp. |
| `neg` | `Boolean!` | Whether this label negates an earlier label with the same source, subject, and value. |
| `src` | `String!` | DID of the labeler that produced this label. |
| `uri` | `String!` | DID or AT-URI subject that this label applies to. |
| `val` | `String!` | Label value emitted by the labeler. |
| `ver` | `Int` | Optional label schema version. |

## Record types

## `AppCertifiedActorOrganization`

Collection: `app.certified.actor.organization`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `foundedDate` | `DateTime` | When the organization was established. Stored as datetime per ATProto conventions (no date-only format exists). Clients should use midnight UTC (e.g., '2005-01-01T00:00:00.000Z'); consumers should treat only the date portion as canonical. |
| `location` | `ComAtprotoRepoStrongRef` | A strong reference to the location where the organization is based. The record referenced must conform with the lexicon app.certified.location. |
| `longDescription` | `AppCertifiedActorOrganizationLongDescriptionUnion` | Long-form description of the organization, such as its mission, history, or detailed project narrative. An inline string for plain text or markdown, a Leaflet linear document record embedded directly, or a strong reference to an existing document record. |
| `organizationType` | `[String!]` | Legal or operational structures of the organization (e.g. 'nonprofit', 'ngo', 'government', 'social-enterprise', 'cooperative'). |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |
| `urls` | `[AppCertifiedActorOrganizationUrlItem!]` | Additional reference URLs (social media profiles, contact pages, donation links, etc.) with a display label for each URL. |
| `visibility` | `String` | Controls whether the organization or project is publicly discoverable on platforms that honor this setting. |

### `AppCertifiedActorOrganizationWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `longDescription` | `PresenceFilterInput` | Filter by whether longDescription is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `organizationType` | `PresenceFilterInput` | Filter by whether organizationType is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `urls` | `PresenceFilterInput` | Filter by whether urls is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `foundedDate` | `DateTimeFilterInput` | Filter by foundedDate |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `visibility` | `StringFilterInput` | Filter by visibility |
| `location` | `PresenceFilterInput` | Filter by whether location is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |

Sort fields: `indexed_at`, `createdAt`, `visibility`, `foundedDate`

## `AppCertifiedActorProfile`

Collection: `app.certified.actor.profile`

| Field | Type | Notes |
| --- | --- | --- |
| `avatar` | `AppCertifiedActorProfileAvatarUnion` | Small image to be displayed next to posts from account. AKA, 'profile picture' |
| `banner` | `AppCertifiedActorProfileBannerUnion` | Larger horizontal image to display behind profile view. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `String` | Free-form profile description text. |
| `did` | `String!` | DID of the record author |
| `displayName` | `String` | Display name for the account |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `pronouns` | `String` | Free-form pronouns text. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |
| `website` | `String` | Account website URL |

### `AppCertifiedActorProfileWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `pronouns` | `StringFilterInput` | Filter by pronouns |
| `avatar` | `PresenceFilterInput` | Filter by whether avatar is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `banner` | `PresenceFilterInput` | Filter by whether banner is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `website` | `StringFilterInput` | Filter by website |
| `displayName` | `StringFilterInput` | Filter by displayName |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `description` | `StringFilterInput` | Filter by description |

Sort fields: `description`, `displayName`, `indexed_at`, `website`, `pronouns`, `createdAt`

## `AppCertifiedBadgeAward`

Collection: `app.certified.badge.award`

| Field | Type | Notes |
| --- | --- | --- |
| `badge` | `ComAtprotoRepoStrongRef!` | Strong reference to the badge definition at the time of award. The record referenced must conform with the lexicon app.certified.badge.definition. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `note` | `String` | Optional statement explaining the reason for this badge award. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `subject` | `AppCertifiedBadgeAwardSubjectUnion!` | Entity the badge award is for (either an account DID or any specific AT Protocol record), e.g. a user, a project, or a specific activity claim. |
| `uri` | `String!` | AT-URI of this record |
| `url` | `String` | Optional URL the badge award links to. |

### `AppCertifiedBadgeAwardWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `note` | `StringFilterInput` | Filter by note |
| `badge` | generated nested strong-ref filter | Filter by whether badge is present or by nested exact fields such as `uri` and `cid` |
| `subject` | generated nested union filter | Filter by whether subject is present or by nested exact fields such as `did`, `uri`, and `cid` |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `url` | `StringFilterInput` | Filter by url |
| `did` | `DIDFilterInput` | Filter by DID (record author) |

Sort fields: `indexed_at`, `note`, `createdAt`, `url`

## `AppCertifiedBadgeDefinition`

Collection: `app.certified.badge.definition`

| Field | Type | Notes |
| --- | --- | --- |
| `allowedIssuers` | `[AppCertifiedDefsDid!]` | Optional allowlist of DIDs allowed to issue this badge. If omitted, anyone may issue it. |
| `badgeType` | `String!` | Category of the badge. Values beyond the known set are permitted. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `String` | Optional short statement describing what the badge represents. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `icon` | `Blob` | Icon representing the badge, stored as a blob for compact visual display. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `title` | `String!` | Human-readable title of the badge. |
| `uri` | `String!` | AT-URI of this record |

### `AppCertifiedBadgeDefinitionWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `description` | `StringFilterInput` | Filter by description |
| `allowedIssuers` | `PresenceFilterInput` | Filter by whether allowedIssuers is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `icon` | `PresenceFilterInput` | Filter by whether icon is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `title` | `StringFilterInput` | Filter by title |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `badgeType` | `StringFilterInput` | Filter by badgeType |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `createdAt`, `description`, `title`, `indexed_at`, `badgeType`

## `AppCertifiedBadgeResponse`

Collection: `app.certified.badge.response`

| Field | Type | Notes |
| --- | --- | --- |
| `badgeAward` | `ComAtprotoRepoStrongRef!` | Strong reference to the badge award being responded to. The record referenced must conform with the lexicon app.certified.badge.award. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `response` | `String!` | The recipient’s response for the badge (accepted or rejected). |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |
| `weight` | `String` | Optional relative weight for accepted badges, assigned by the recipient. |

### `AppCertifiedBadgeResponseWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `response` | `StringFilterInput` | Filter by response |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `badgeAward` | `PresenceFilterInput` | Filter by whether badgeAward is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `weight` | `StringFilterInput` | Filter by weight |

Sort fields: `indexed_at`, `weight`, `response`, `createdAt`

## `AppCertifiedGraphFollow`

Collection: `app.certified.graph.follow`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `subject` | `String!` | DID of the account being followed. |
| `uri` | `String!` | AT-URI of this record |
| `via` | `ComAtprotoRepoStrongRef` | Optional strong reference to a record that mediated this follow (e.g. a starter pack or other curated list). Mirrors the optional `via` field on app.bsky.graph.follow; the referenced record may conform with any lexicon. |

### `AppCertifiedGraphFollowWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `via` | `PresenceFilterInput` | Filter by whether via is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `subject` | `StringFilterInput` | Filter by subject |

Sort fields: `indexed_at`, `subject`, `createdAt`

## `AppCertifiedLinkEvm`

Collection: `app.certified.link.evm`

| Field | Type | Notes |
| --- | --- | --- |
| `address` | `String!` | EVM wallet address (0x-prefixed, with EIP-55 checksum recommended). |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `proof` | `AppCertifiedLinkEvmProofUnion!` | Cryptographic proof of wallet ownership. The union is open to allow future proof methods (e.g. ERC-1271, ERC-6492). Each variant bundles its signature with the corresponding message format. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |

### `AppCertifiedLinkEvmWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `proof` | `PresenceFilterInput` | Filter by whether proof is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `address` | `StringFilterInput` | Filter by address |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `createdAt`, `indexed_at`, `address`

## `AppCertifiedLocation`

Collection: `app.certified.location`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `String` | Additional context about this location, such as its significance to the work or specific boundaries |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `location` | `AppCertifiedLocationLocationUnion!` | The location of where the work was performed as a URI, blob, or inline string. |
| `locationType` | `String!` | An identifier for the format of the location data. Use `geojson-point` for a single GeoJSON Point; use `geojson` as the catch-all for any other GeoJSON geometry (Polygon, MultiPolygon, FeatureCollection, etc.) — the inner payload's own GeoJSON `type` field carries the specifics. Values beyond the known set are permitted; see the Location Protocol spec for the canonical registry: https://spec.decentralizedgeo.org/specification/location-types/#location-type-registry |
| `lpVersion` | `String!` | The version of the Location Protocol |
| `name` | `String` | Human-readable name for this location (e.g. 'Golden Gate Park', 'San Francisco Bay Area') |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `srs` | `String!` | The Spatial Reference System URI (e.g., http://www.opengis.net/def/crs/OGC/1.3/CRS84) that defines the coordinate system. |
| `uri` | `String!` | AT-URI of this record |

### `AppCertifiedLocationWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `srs` | `StringFilterInput` | Filter by srs |
| `location` | `PresenceFilterInput` | Filter by whether location is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `lpVersion` | `StringFilterInput` | Filter by lpVersion |
| `locationType` | `StringFilterInput` | Filter by locationType |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `name` | `StringFilterInput` | Filter by name |
| `description` | `StringFilterInput` | Filter by description |

Sort fields: `indexed_at`, `locationType`, `srs`, `name`, `createdAt`, `lpVersion`, `description`

## `OrgHypercertsClaimActivity`

Collection: `org.hypercerts.claim.activity`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `contributors` | `[OrgHypercertsClaimActivityContributor!]` | An array of contributor objects, each containing contributor information, weight, and contribution details. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `OrgHypercertsClaimActivityDescriptionUnion` | Long-form description of the activity. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | When the work ended |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `image` | `OrgHypercertsClaimActivityImageUnion` | The hypercert visual representation as a URI or image blob. |
| `locations` | `[ComAtprotoRepoStrongRef!]` | An array of strong references to the location where activity was performed. The record referenced must conform with the lexicon app.certified.location. |
| `rights` | `ComAtprotoRepoStrongRef` | A strong reference to the rights that this hypercert has. The record referenced must conform with the lexicon org.hypercerts.claim.rights. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `shortDescription` | `String!` | Short summary of this activity claim, suitable for previews and list views. Rich text annotations may be provided via `shortDescriptionFacets`. |
| `shortDescriptionFacets` | `[AppBskyRichtextFacet!]` | Rich text annotations for `shortDescription` (mentions, URLs, hashtags, etc). |
| `startDate` | `DateTime` | When the work began |
| `title` | `String!` | Display title summarizing the impact work (e.g. 'Reforestation in Amazon Basin 2024') |
| `uri` | `String!` | AT-URI of this record |
| `workScope` | `OrgHypercertsClaimActivityWorkScopeUnion` | Work scope definition. A CEL expression for structured, machine-evaluable scopes or a free-form string for simple and legacy scopes. |

### `OrgHypercertsClaimActivityWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `workScope` | `PresenceFilterInput` | Filter by whether workScope is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `title` | `StringFilterInput` | Filter by title |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `description` | `PresenceFilterInput` | Filter by whether description is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |
| `image` | `PresenceFilterInput` | Filter by whether image is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `rights` | generated nested strong-ref filter | Filter by whether rights is present or by nested exact fields such as `uri` and `cid` |
| `locations` | generated nested array filter | Filter by whether locations is present or by `any` strong-ref item fields such as `uri` and `cid` |
| `contributors` | generated nested array filter | Filter by whether contributors is present or by `any` nested contributor fields, such as `contributorIdentity.identity` |
| `contributorDid` | `DIDFilterInput` | Compatibility filter for inline contributor DIDs, legacy bare DID array items, and contributorInformation strong refs by referenced `identifier` |
| `shortDescriptionFacets` | generated nested array filter | Filter by whether shortDescriptionFacets is present or by supported nested facet fields |
| `did` | `DIDFilterInput` | Filter by DID (record author) |

Sort fields: `createdAt`, `startDate`, `indexed_at`, `endDate`, `shortDescription`, `title`

## `OrgHypercertsClaimContribution`

Collection: `org.hypercerts.claim.contribution`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `contributionDescription` | `String` | Description of what the contribution concretely involved. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | When this contribution finished. Should fall within the parent hypercert's timeframe. |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `role` | `String` | Role or title of the contributor. |
| `startDate` | `DateTime` | When this contribution started. Should fall within the parent hypercert's timeframe. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimContributionWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `contributionDescription` | `StringFilterInput` | Filter by contributionDescription |
| `role` | `StringFilterInput` | Filter by role |

Sort fields: `startDate`, `contributionDescription`, `role`, `endDate`, `indexed_at`, `createdAt`

## `OrgHypercertsClaimContributorInformation`

Collection: `org.hypercerts.claim.contributorInformation`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `displayName` | `String` | Human-readable name for the contributor as it should appear in UI. |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `identifier` | `String` | DID (did:plc:...) or URI to a social profile of the contributor. |
| `image` | `OrgHypercertsClaimContributorInformationImageUnion` | The contributor visual representation as a URI or image blob. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimContributorInformationWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `image` | `PresenceFilterInput` | Filter by whether image is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `identifier` | `StringFilterInput` | Filter by identifier |
| `displayName` | `StringFilterInput` | Filter by displayName |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |

Sort fields: `createdAt`, `identifier`, `displayName`, `indexed_at`

## `OrgHypercertsClaimRights`

Collection: `org.hypercerts.claim.rights`

| Field | Type | Notes |
| --- | --- | --- |
| `attachment` | `OrgHypercertsClaimRightsAttachmentUnion` | An attachment to define the rights further, e.g. a legal document. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `rightsDescription` | `String!` | Detailed explanation of the rights holders' permissions, restrictions, and conditions |
| `rightsName` | `String!` | Human-readable name for these rights (e.g. 'All Rights Reserved', 'CC BY-SA 4.0') |
| `rightsType` | `String!` | Short identifier code for this rights type (e.g. 'ARR', 'CC-BY-SA') to facilitate filtering and search |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimRightsWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `attachment` | `PresenceFilterInput` | Filter by whether attachment is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `rightsName` | `StringFilterInput` | Filter by rightsName |
| `rightsType` | `StringFilterInput` | Filter by rightsType |
| `rightsDescription` | `StringFilterInput` | Filter by rightsDescription |
| `did` | `DIDFilterInput` | Filter by DID (record author) |

Sort fields: `rightsName`, `rightsType`, `rightsDescription`, `indexed_at`, `createdAt`

## `OrgHypercertsCollection`

Collection: `org.hypercerts.collection`

| Field | Type | Notes |
| --- | --- | --- |
| `avatar` | `OrgHypercertsCollectionAvatarUnion` | The collection's avatar/profile image as a URI or image blob. |
| `banner` | `OrgHypercertsCollectionBannerUnion` | Larger horizontal image to display behind the collection view. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `OrgHypercertsCollectionDescriptionUnion` | Long-form description of the collection. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `items` | `[OrgHypercertsCollectionItem!]` | Array of items in this collection with optional weights. |
| `location` | `ComAtprotoRepoStrongRef` | A strong reference to the location where this collection's activities were performed. The record referenced must conform with the lexicon app.certified.location. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `shortDescription` | `String` | Short summary of this collection, suitable for previews and list views. Rich text annotations may be provided via `shortDescriptionFacets`. |
| `shortDescriptionFacets` | `[AppBskyRichtextFacet!]` | Rich text annotations for `shortDescription` (mentions, URLs, hashtags, etc). |
| `title` | `String!` | Display name for this collection (e.g. 'Q1 2025 Impact Projects') |
| `type` | `String` | The type of this collection. Values beyond the known set are permitted. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsCollectionWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `banner` | `PresenceFilterInput` | Filter by whether banner is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `location` | generated nested strong-ref filter | Filter by whether location is present or by nested exact fields such as `uri` and `cid` |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `items` | generated nested array filter | Filter by whether items is present or by `any` item fields such as `itemIdentifier.uri` |
| `description` | `PresenceFilterInput` | Filter by whether description is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `type` | `StringFilterInput` | Filter by type |
| `avatar` | `PresenceFilterInput` | Filter by whether avatar is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `shortDescriptionFacets` | `PresenceFilterInput` | Filter by whether shortDescriptionFacets is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |
| `title` | `StringFilterInput` | Filter by title |

Sort fields: `indexed_at`, `createdAt`, `shortDescription`, `type`, `title`

## `OrgHypercertsContextAcknowledgement`

Collection: `org.hypercerts.context.acknowledgement`

| Field | Type | Notes |
| --- | --- | --- |
| `acknowledged` | `Boolean!` | Whether the relationship is acknowledged (true) or rejected (false). |
| `cid` | `String!` | CID of this record version |
| `comment` | `String` | Optional plain-text comment providing additional context or reasoning. |
| `context` | `OrgHypercertsContextAcknowledgementContextUnion` | Context for the acknowledgement (e.g. the collection that includes an activity, or the activity that includes a contributor). A URI for a lightweight reference or a strong reference for content-hash verification. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `subject` | `ComAtprotoRepoStrongRef!` | The record being acknowledged (e.g. an activity, a contributor information record, an evaluation). |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsContextAcknowledgementWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `comment` | `StringFilterInput` | Filter by comment |
| `context` | `PresenceFilterInput` | Filter by whether context is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `subject` | `PresenceFilterInput` | Filter by whether subject is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `acknowledged` | `BooleanFilterInput` | Filter by acknowledged |

Sort fields: `comment`, `createdAt`, `acknowledged`, `indexed_at`

## `OrgHypercertsContextAttachment`

Collection: `org.hypercerts.context.attachment`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `content` | `[OrgHypercertsContextAttachmentItemsUnion!]` | The files, documents, or external references included in this attachment record. |
| `contentType` | `String` | The type of attachment. Values beyond the known set are permitted. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `description` | `OrgHypercertsContextAttachmentDescriptionUnion` | Long-form description of the attachment. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `location` | `ComAtprotoRepoStrongRef` | A strong reference to the location where this attachment's subject matter occurred. The record referenced must conform with the lexicon app.certified.location. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `shortDescription` | `String` | Short summary of this attachment, suitable for previews and list views. Rich text annotations may be provided via `shortDescriptionFacets`. |
| `shortDescriptionFacets` | `[AppBskyRichtextFacet!]` | Rich text annotations for `shortDescription` (mentions, URLs, hashtags, etc). |
| `subjects` | `[ComAtprotoRepoStrongRef!]` | References to the subject(s) the attachment is connected to—this may be an activity claim, outcome claim, measurement, evaluation, or even another attachment. This is optional as the attachment can exist before the claim is recorded. |
| `title` | `String!` | Display title for this attachment (e.g. 'Impact Assessment Report', 'Audit Findings') |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsContextAttachmentWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `location` | `PresenceFilterInput` | Filter by whether location is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `shortDescriptionFacets` | `PresenceFilterInput` | Filter by whether shortDescriptionFacets is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `content` | `PresenceFilterInput` | Filter by whether content is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `description` | `PresenceFilterInput` | Filter by whether description is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `title` | `StringFilterInput` | Filter by title |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |
| `subjects` | `PresenceFilterInput` | Filter by whether subjects is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `contentType` | `StringFilterInput` | Filter by contentType |

Sort fields: `contentType`, `title`, `shortDescription`, `createdAt`, `indexed_at`

## `OrgHypercertsContextEvaluation`

Collection: `org.hypercerts.context.evaluation`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `content` | `[OrgHypercertsContextEvaluationItemsUnion!]` | Evaluation data (URIs or blobs) containing detailed reports or methodology |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `evaluators` | `[AppCertifiedDefsDid!]!` | DIDs of the evaluators |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `location` | `ComAtprotoRepoStrongRef` | An optional reference for georeferenced evaluations. The record referenced must conform with the lexicon app.certified.location. |
| `measurements` | `[ComAtprotoRepoStrongRef!]` | Optional references to the measurements that contributed to this evaluation. The record(s) referenced must conform with the lexicon org.hypercerts.context.measurement |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `score` | `OrgHypercertsContextEvaluationScore` | Optional overall score for this evaluation on a numeric scale. |
| `subject` | `ComAtprotoRepoStrongRef` | A strong reference to what is being evaluated (e.g. activity, measurement, contribution, etc.) |
| `summary` | `String!` | Brief evaluation summary |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsContextEvaluationWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `evaluators` | `PresenceFilterInput` | Filter by whether evaluators is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `subject` | `PresenceFilterInput` | Filter by whether subject is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `summary` | `StringFilterInput` | Filter by summary |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `measurements` | `PresenceFilterInput` | Filter by whether measurements is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `score` | `PresenceFilterInput` | Filter by whether score is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `content` | `PresenceFilterInput` | Filter by whether content is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `location` | `PresenceFilterInput` | Filter by whether location is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |

Sort fields: `indexed_at`, `summary`, `createdAt`

## `OrgHypercertsContextMeasurement`

Collection: `org.hypercerts.context.measurement`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `comment` | `String` | Short comment of this measurement, suitable for previews and list views. Rich text annotations may be provided via `commentFacets`. |
| `commentFacets` | `[AppBskyRichtextFacet!]` | Rich text annotations for `comment` (mentions, URLs, hashtags, etc). |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | The end date and time when the measurement ended. For one-time measurements, this should equal the start date. |
| `evidenceURI` | `[String!]` | URIs to related evidence or underlying data (e.g. org.hypercerts.claim.evidence records or raw datasets) |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `locations` | `[ComAtprotoRepoStrongRef!]` | Optional geographic references related to where the measurement was taken. Each referenced record must conform with the app.certified.location lexicon. |
| `measurers` | `[AppCertifiedDefsDid!]` | DIDs of the entities that performed this measurement |
| `methodType` | `String` | Short identifier for the measurement methodology |
| `methodURI` | `String` | URI to methodology documentation, standard protocol, or measurement procedure |
| `metric` | `String!` | The metric being measured, e.g. forest area restored, number of users, etc. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `startDate` | `DateTime` | The start date and time when the measurement began. |
| `subjects` | `[ComAtprotoRepoStrongRef!]` | Strong references to the records this measurement refers to (e.g. activities, projects, or claims). |
| `unit` | `String!` | The unit of the measured value (e.g. kg CO₂e, hectares, %, index score). |
| `uri` | `String!` | AT-URI of this record |
| `value` | `String!` | The measured value as a numeric string (e.g. '1234.56') |

### `OrgHypercertsContextMeasurementWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `locations` | `PresenceFilterInput` | Filter by whether locations is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `unit` | `StringFilterInput` | Filter by unit |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `metric` | `StringFilterInput` | Filter by metric |
| `comment` | `StringFilterInput` | Filter by comment |
| `value` | `StringFilterInput` | Filter by value |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |
| `methodURI` | `StringFilterInput` | Filter by methodURI |
| `commentFacets` | `PresenceFilterInput` | Filter by whether commentFacets is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `subjects` | `PresenceFilterInput` | Filter by whether subjects is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `evidenceURI` | `PresenceFilterInput` | Filter by whether evidenceURI is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `measurers` | `PresenceFilterInput` | Filter by whether measurers is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `methodType` | `StringFilterInput` | Filter by methodType |

Sort fields: `endDate`, `startDate`, `unit`, `comment`, `methodType`, `indexed_at`, `methodURI`, `createdAt`, `value`, `metric`

## `OrgHypercertsFundingReceipt`

Collection: `org.hypercerts.funding.receipt`

| Field | Type | Notes |
| --- | --- | --- |
| `amount` | `String!` | Amount of funding received as a numeric string (e.g. '1000.50'). |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this receipt record was created. |
| `currency` | `String!` | Currency of the payment (e.g. EUR, USD, ETH). |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `for` | `ComAtprotoRepoStrongRef` | Optional strong reference to the activity, project, or organization this funding relates to. |
| `from` | `OrgHypercertsFundingReceiptFromUnion` | The sender of the funds (a free-text string, an account DID, or a strong reference to a record). Optional — omit to represent anonymity. |
| `notes` | `String` | Optional notes or additional context for this funding receipt. |
| `occurredAt` | `DateTime` | Timestamp when the payment occurred. |
| `paymentNetwork` | `String` | Optional network within the payment rail (e.g. arbitrum, ethereum, sepa, visa, paypal). |
| `paymentRail` | `String` | How the funds were transferred (e.g. bank_transfer, credit_card, onchain, cash, check, payment_processor). |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `to` | `OrgHypercertsFundingReceiptToUnion!` | The recipient of the funds (a free-text string, an account DID, or a strong reference to a record). |
| `transactionId` | `String` | Identifier of the underlying payment transaction (e.g. bank reference, onchain transaction hash, or processor-specific ID). Use paymentNetwork to specify the network where applicable. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsFundingReceiptWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `occurredAt` | `DateTimeFilterInput` | Filter by occurredAt |
| `paymentNetwork` | `StringFilterInput` | Filter by paymentNetwork |
| `to` | `PresenceFilterInput` | Filter by whether to is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `for` | `PresenceFilterInput` | Filter by whether for is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `currency` | `StringFilterInput` | Filter by currency |
| `notes` | `StringFilterInput` | Filter by notes |
| `paymentRail` | `StringFilterInput` | Filter by paymentRail |
| `transactionId` | `StringFilterInput` | Filter by transactionId |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `from` | `PresenceFilterInput` | Filter by whether from is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `amount` | `StringFilterInput` | Filter by amount |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `transactionId`, `indexed_at`, `occurredAt`, `paymentNetwork`, `notes`, `createdAt`, `amount`, `currency`, `paymentRail`

## `OrgHypercertsWorkscopeTag`

Collection: `org.hypercerts.workscope.tag`

| Field | Type | Notes |
| --- | --- | --- |
| `aliases` | `[String!]` | Alternative human-readable names for this scope (e.g., translations, abbreviations, or common synonyms). Unlike sameAs, these are plain-text labels, not links to external ontologies. |
| `category` | `String` | Category type of this scope. Values beyond the known set are permitted. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `description` | `String` | Optional longer description of this scope. |
| `did` | `String!` | DID of the record author |
| `externalLabels` | `[ExternalLabel!]!` | External ATProto labels attached to this record. |
| `key` | `String!` | Lowercase, underscore-separated machine-readable key for this scope (e.g., 'mangrove_restoration', 'biodiversity_monitoring'). Used as the canonical identifier in CEL expressions. |
| `name` | `String!` | Human-readable name for this scope. |
| `parent` | `ComAtprotoRepoStrongRef` | Optional strong reference to a parent work scope tag record for taxonomy/hierarchy support. The record referenced must conform with the lexicon org.hypercerts.workscope.tag. |
| `referenceDocument` | `OrgHypercertsWorkscopeTagReferenceDocumentUnion` | Link to a governance or reference document where this work scope tag is defined and further explained. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `sameAs` | `[String!]` | URIs to semantically equivalent concepts in external ontologies or taxonomies (e.g., Wikidata QIDs, ENVO terms, SDG targets). Used for interoperability, not as documentation. |
| `status` | `String` | Lifecycle status of this tag. Communities propose tags, curators accept them, deprecated tags point to replacements via supersededBy. Values beyond the known set are permitted. |
| `supersededBy` | `ComAtprotoRepoStrongRef` | When status is 'deprecated', points to the replacement work scope tag record. The record referenced must conform with the lexicon org.hypercerts.workscope.tag. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsWorkscopeTagWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `uri` | `URIFilterInput` | Filter by AT-URI |
| `referenceDocument` | `PresenceFilterInput` | Filter by whether referenceDocument is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `sameAs` | `PresenceFilterInput` | Filter by whether sameAs is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `description` | `StringFilterInput` | Filter by description |
| `supersededBy` | `PresenceFilterInput` | Filter by whether supersededBy is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `key` | `StringFilterInput` | Filter by key |
| `externalLabels` | `ExternalLabelWhereInput` | Filter records by locally ingested external labels before pagination. |
| `parent` | `PresenceFilterInput` | Filter by whether parent is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |
| `name` | `StringFilterInput` | Filter by name |
| `status` | `StringFilterInput` | Filter by status |
| `category` | `StringFilterInput` | Filter by category |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `aliases` | `PresenceFilterInput` | Filter by whether aliases is missing/null or present; nested filters may be generated up to depth 3; introspect the endpoint for the exact input shape |

Sort fields: `createdAt`, `description`, `key`, `indexed_at`, `name`, `status`, `category`

## Common union selections

Use inline fragments for union fields. Current production unions referenced by typed record fields:

- `AppCertifiedActorOrganizationLongDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `AppCertifiedActorProfileAvatarUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `AppCertifiedActorProfileBannerUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsLargeImage`
- `AppCertifiedBadgeAwardSubjectUnion`: `AppCertifiedDefsDid`, `ComAtprotoRepoStrongRef`
- `AppCertifiedLinkEvmProofUnion`: `AppCertifiedLinkEvmEip712Proof`
- `AppCertifiedLocationLocationUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`, `AppCertifiedLocationString`
- `OrgHypercertsClaimActivityDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsClaimActivityImageUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `OrgHypercertsClaimActivityWorkScopeUnion`: `OrgHypercertsWorkscopeCel`, `OrgHypercertsClaimActivityWorkScopeString`
- `OrgHypercertsClaimContributorInformationImageUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `OrgHypercertsClaimRightsAttachmentUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`
- `OrgHypercertsCollectionAvatarUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `OrgHypercertsCollectionBannerUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsLargeImage`
- `OrgHypercertsCollectionDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsContextAcknowledgementContextUnion`: `OrgHypercertsDefsUri`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsContextAttachmentDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsContextAttachmentItemsUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`
- `OrgHypercertsContextEvaluationItemsUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`
- `OrgHypercertsFundingReceiptFromUnion`: `OrgHypercertsFundingReceiptText`, `AppCertifiedDefsDid`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsFundingReceiptToUnion`: `OrgHypercertsFundingReceiptText`, `AppCertifiedDefsDid`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsWorkscopeTagReferenceDocumentUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`
