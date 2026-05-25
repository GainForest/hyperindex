# Hyperindex GraphQL Schema Reference

Generated from live introspection of `https://api.indexer.hypercerts.dev/graphql` on 2026-05-25.

## Endpoints

- Production GraphQL: `https://api.indexer.hypercerts.dev/graphql`
- Staging GraphQL: `https://dev.api.indexer.hypercerts.dev/graphql`

## Query fields

| Query | Purpose |
| --- | --- |
| `appCertifiedActorOrganization` | Query app.certified.actor.organization records |
| `appCertifiedActorOrganizationByUri` | Get a single app.certified.actor.organization by AT-URI |
| `appCertifiedActorProfile` | Query app.certified.actor.profile records |
| `appCertifiedActorProfileByUri` | Get a single app.certified.actor.profile by AT-URI |
| `appCertifiedBadgeAward` | Query app.certified.badge.award records |
| `appCertifiedBadgeAwardByUri` | Get a single app.certified.badge.award by AT-URI |
| `appCertifiedBadgeDefinition` | Query app.certified.badge.definition records |
| `appCertifiedBadgeDefinitionByUri` | Get a single app.certified.badge.definition by AT-URI |
| `appCertifiedBadgeResponse` | Query app.certified.badge.response records |
| `appCertifiedBadgeResponseByUri` | Get a single app.certified.badge.response by AT-URI |
| `appCertifiedGraphFollow` | Query app.certified.graph.follow records |
| `appCertifiedGraphFollowByUri` | Get a single app.certified.graph.follow by AT-URI |
| `appCertifiedLocation` | Query app.certified.location records |
| `appCertifiedLocationByUri` | Get a single app.certified.location by AT-URI |
| `collectionStats` | Get record counts for collections (efficient aggregate query) |
| `collectionTimeSeries` | Get time series data for a collection (records grouped by date) |
| `orgHypercertsClaimActivity` | Query org.hypercerts.claim.activity records |
| `orgHypercertsClaimActivityByUri` | Get a single org.hypercerts.claim.activity by AT-URI |
| `orgHypercertsClaimContribution` | Query org.hypercerts.claim.contribution records |
| `orgHypercertsClaimContributionByUri` | Get a single org.hypercerts.claim.contribution by AT-URI |
| `orgHypercertsClaimContributorInformation` | Query org.hypercerts.claim.contributorInformation records |
| `orgHypercertsClaimContributorInformationByUri` | Get a single org.hypercerts.claim.contributorInformation by AT-URI |
| `orgHypercertsClaimRights` | Query org.hypercerts.claim.rights records |
| `orgHypercertsClaimRightsByUri` | Get a single org.hypercerts.claim.rights by AT-URI |
| `orgHypercertsCollection` | Query org.hypercerts.collection records |
| `orgHypercertsCollectionByUri` | Get a single org.hypercerts.collection by AT-URI |
| `orgHypercertsContextAcknowledgement` | Query org.hypercerts.context.acknowledgement records |
| `orgHypercertsContextAcknowledgementByUri` | Get a single org.hypercerts.context.acknowledgement by AT-URI |
| `orgHypercertsContextAttachment` | Query org.hypercerts.context.attachment records |
| `orgHypercertsContextAttachmentByUri` | Get a single org.hypercerts.context.attachment by AT-URI |
| `orgHypercertsContextEvaluation` | Query org.hypercerts.context.evaluation records |
| `orgHypercertsContextEvaluationByUri` | Get a single org.hypercerts.context.evaluation by AT-URI |
| `orgHypercertsContextMeasurement` | Query org.hypercerts.context.measurement records |
| `orgHypercertsContextMeasurementByUri` | Get a single org.hypercerts.context.measurement by AT-URI |
| `orgHypercertsFundingReceipt` | Query org.hypercerts.funding.receipt records |
| `orgHypercertsFundingReceiptByUri` | Get a single org.hypercerts.funding.receipt by AT-URI |
| `orgHypercertsWorkscopeTag` | Query org.hypercerts.workscope.tag records |
| `orgHypercertsWorkscopeTagByUri` | Get a single org.hypercerts.workscope.tag by AT-URI |
| `records` | Query records from any collection (useful for collections without lexicon schemas) |
| `search` | Search records by text content |

## Filter operators

### `StringFilterInput`

| Operator | Type | Meaning |
| --- | --- | --- |
| `contains` | `String` | Contains substring |
| `startsWith` | `String` | Starts with prefix |
| `isNull` | `Boolean` | Field is null |
| `eq` | `String` | Equal to |
| `neq` | `String` | Not equal to |
| `in` | `[String!]` | Value is in list |

### `DateTimeFilterInput`

| Operator | Type | Meaning |
| --- | --- | --- |
| `lte` | `DateTime` | Less than or equal to |
| `isNull` | `Boolean` | Field is null |
| `eq` | `DateTime` | Equal to |
| `neq` | `DateTime` | Not equal to |
| `gt` | `DateTime` | Greater than |
| `lt` | `DateTime` | Less than |
| `gte` | `DateTime` | Greater than or equal to |

### `DIDFilterInput`

| Operator | Type | Meaning |
| --- | --- | --- |
| `eq` | `String` | Equals |
| `in` | `[String!]` | In list |

## Core Hypercert record types

## `OrgHypercertsClaimActivity`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `contributors` | `[OrgHypercertsClaimActivityContributor!]` | An array of contributor objects, each containing contributor information, weight, and contribution details. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `OrgHypercertsClaimActivityDescriptionUnion` | Long-form description of the activity. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | When the work ended |
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
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `title` | `StringFilterInput` | Filter by title |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |

Sort fields: `indexed_at`, `title`, `startDate`, `endDate`, `createdAt`, `shortDescription`

## `OrgHypercertsContextAttachment`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `content` | `[OrgHypercertsContextAttachmentItemsUnion!]` | The files, documents, or external references included in this attachment record. |
| `contentType` | `String` | The type of attachment. Values beyond the known set are permitted. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `description` | `OrgHypercertsContextAttachmentDescriptionUnion` | Long-form description of the attachment. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
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
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `contentType` | `StringFilterInput` | Filter by contentType |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |
| `title` | `StringFilterInput` | Filter by title |

Sort fields: `indexed_at`, `createdAt`, `contentType`, `shortDescription`, `title`

## `OrgHypercertsClaimContribution`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `contributionDescription` | `String` | Description of what the contribution concretely involved. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | When this contribution finished. Should fall within the parent hypercert's timeframe. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `role` | `String` | Role or title of the contributor. |
| `startDate` | `DateTime` | When this contribution started. Should fall within the parent hypercert's timeframe. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimContributionWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `role` | `StringFilterInput` | Filter by role |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `contributionDescription` | `StringFilterInput` | Filter by contributionDescription |
| `did` | `DIDFilterInput` | Filter by DID (record author) |

Sort fields: `contributionDescription`, `indexed_at`, `role`, `endDate`, `createdAt`, `startDate`

## `OrgHypercertsClaimContributorInformation`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `displayName` | `String` | Human-readable name for the contributor as it should appear in UI. |
| `identifier` | `String` | DID (did:plc:...) or URI to a social profile of the contributor. |
| `image` | `OrgHypercertsClaimContributorInformationImageUnion` | The contributor visual representation as a URI or image blob. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimContributorInformationWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `displayName` | `StringFilterInput` | Filter by displayName |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `identifier` | `StringFilterInput` | Filter by identifier |

Sort fields: `displayName`, `createdAt`, `identifier`, `indexed_at`

## `OrgHypercertsClaimRights`

| Field | Type | Notes |
| --- | --- | --- |
| `attachment` | `OrgHypercertsClaimRightsAttachmentUnion` | An attachment to define the rights further, e.g. a legal document. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `rightsDescription` | `String!` | Detailed explanation of the rights holders' permissions, restrictions, and conditions |
| `rightsName` | `String!` | Human-readable name for these rights (e.g. 'All Rights Reserved', 'CC BY-SA 4.0') |
| `rightsType` | `String!` | Short identifier code for this rights type (e.g. 'ARR', 'CC-BY-SA') to facilitate filtering and search |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsClaimRightsWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `rightsDescription` | `StringFilterInput` | Filter by rightsDescription |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `rightsName` | `StringFilterInput` | Filter by rightsName |
| `rightsType` | `StringFilterInput` | Filter by rightsType |

Sort fields: `createdAt`, `rightsName`, `rightsType`, `rightsDescription`, `indexed_at`

## `OrgHypercertsCollection`

| Field | Type | Notes |
| --- | --- | --- |
| `avatar` | `OrgHypercertsCollectionAvatarUnion` | The collection's avatar/profile image as a URI or image blob. |
| `banner` | `OrgHypercertsCollectionBannerUnion` | Larger horizontal image to display behind the collection view. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `OrgHypercertsCollectionDescriptionUnion` | Long-form description of the collection. An inline string for plain text or markdown, a Leaflet linear document for rich-text content, or a strong reference to an external description record. |
| `did` | `String!` | DID of the record author |
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
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `type` | `StringFilterInput` | Filter by type |
| `title` | `StringFilterInput` | Filter by title |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `shortDescription` | `StringFilterInput` | Filter by shortDescription |

Sort fields: `indexed_at`, `type`, `title`, `createdAt`, `shortDescription`

## `OrgHypercertsContextEvaluation`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `content` | `[OrgHypercertsContextEvaluationItemsUnion!]` | Evaluation data (URIs or blobs) containing detailed reports or methodology |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `evaluators` | `[AppCertifiedDefsDid!]!` | DIDs of the evaluators |
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
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `summary` | `StringFilterInput` | Filter by summary |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `summary`, `createdAt`, `indexed_at`

## `OrgHypercertsContextMeasurement`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `comment` | `String` | Short comment of this measurement, suitable for previews and list views. Rich text annotations may be provided via `commentFacets`. |
| `commentFacets` | `[AppBskyRichtextFacet!]` | Rich text annotations for `comment` (mentions, URLs, hashtags, etc). |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `did` | `String!` | DID of the record author |
| `endDate` | `DateTime` | The end date and time when the measurement ended. For one-time measurements, this should equal the start date. |
| `evidenceURI` | `[String!]` | URIs to related evidence or underlying data (e.g. org.hypercerts.claim.evidence records or raw datasets) |
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
| `methodURI` | `StringFilterInput` | Filter by methodURI |
| `value` | `StringFilterInput` | Filter by value |
| `metric` | `StringFilterInput` | Filter by metric |
| `comment` | `StringFilterInput` | Filter by comment |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `unit` | `StringFilterInput` | Filter by unit |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `methodType` | `StringFilterInput` | Filter by methodType |
| `startDate` | `DateTimeFilterInput` | Filter by startDate |
| `endDate` | `DateTimeFilterInput` | Filter by endDate |

Sort fields: `indexed_at`, `unit`, `value`, `metric`, `createdAt`, `startDate`, `endDate`, `methodURI`, `methodType`, `comment`

## `OrgHypercertsContextAcknowledgement`

| Field | Type | Notes |
| --- | --- | --- |
| `acknowledged` | `Boolean!` | Whether the relationship is acknowledged (true) or rejected (false). |
| `cid` | `String!` | CID of this record version |
| `comment` | `String` | Optional plain-text comment providing additional context or reasoning. |
| `context` | `OrgHypercertsContextAcknowledgementContextUnion` | Context for the acknowledgement (e.g. the collection that includes an activity, or the activity that includes a contributor). A URI for a lightweight reference or a strong reference for content-hash verification. |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `subject` | `ComAtprotoRepoStrongRef!` | The record being acknowledged (e.g. an activity, a contributor information record, an evaluation). |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsContextAcknowledgementWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `comment` | `StringFilterInput` | Filter by comment |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `acknowledged` | `BooleanFilterInput` | Filter by acknowledged |

Sort fields: `comment`, `createdAt`, `acknowledged`, `indexed_at`

## `OrgHypercertsFundingReceipt`

| Field | Type | Notes |
| --- | --- | --- |
| `amount` | `String!` | Amount of funding received as a numeric string (e.g. '1000.50'). |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this receipt record was created. |
| `currency` | `String!` | Currency of the payment (e.g. EUR, USD, ETH). |
| `did` | `String!` | DID of the record author |
| `for` | `ComAtprotoRepoStrongRef` | Optional strong reference to the activity, project, or organization this funding relates to. |
| `from` | `OrgHypercertsFundingReceiptFromUnion` | The sender of the funds (a free-text string, an account DID, or a strong reference to a record). Optional — omit to represent anonymity. |
| `notes` | `String` | Optional notes or additional context for this funding receipt. |
| `occurredAt` | `DateTime` | Timestamp when the payment occurred. |
| `paymentNetwork` | `String` | Optional network within the payment rail (e.g. arbitrum, ethereum, sepa, visa, PayPal). |
| `paymentRail` | `String` | How the funds were transferred (e.g. bank_transfer, credit_card, onchain, cash, check, payment_processor). |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `to` | `OrgHypercertsFundingReceiptToUnion!` | The recipient of the funds (a free-text string, an account DID, or a strong reference to a record). |
| `transactionId` | `String` | Identifier of the underlying payment transaction (e.g. bank reference, onchain transaction hash, or processor-specific ID). Use paymentNetwork to specify the network where applicable. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsFundingReceiptWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `paymentNetwork` | `StringFilterInput` | Filter by paymentNetwork |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `amount` | `StringFilterInput` | Filter by amount |
| `notes` | `StringFilterInput` | Filter by notes |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `occurredAt` | `DateTimeFilterInput` | Filter by occurredAt |
| `transactionId` | `StringFilterInput` | Filter by transactionId |
| `currency` | `StringFilterInput` | Filter by currency |
| `paymentRail` | `StringFilterInput` | Filter by paymentRail |

Sort fields: `amount`, `transactionId`, `notes`, `paymentNetwork`, `indexed_at`, `createdAt`, `occurredAt`, `paymentRail`, `currency`

## `OrgHypercertsWorkscopeTag`

| Field | Type | Notes |
| --- | --- | --- |
| `aliases` | `[String!]` | Alternative human-readable names for this scope (e.g., translations, abbreviations, or common synonyms). Unlike sameAs, these are plain-text labels, not links to external ontologies. |
| `category` | `String` | Category type of this scope. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `description` | `String` | Optional longer description of this scope. |
| `did` | `String!` | DID of the record author |
| `key` | `String!` | Lowercase, underscore-separated machine-readable key for this scope (e.g., 'mangrove_restoration', 'biodiversity_monitoring'). Used as the canonical identifier in CEL expressions. |
| `name` | `String!` | Human-readable name for this scope. |
| `parent` | `ComAtprotoRepoStrongRef` | Optional strong reference to a parent work scope tag record for taxonomy/hierarchy support. The record referenced must conform with the lexicon org.hypercerts.workscope.tag. |
| `referenceDocument` | `OrgHypercertsWorkscopeTagReferenceDocumentUnion` | Link to a governance or reference document where this work scope tag is defined and further explained. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `sameAs` | `[String!]` | URIs to semantically equivalent concepts in external ontologies or taxonomies (e.g., Wikidata QIDs, ENVO terms, SDG targets). Used for interoperability, not as documentation. |
| `status` | `String` | Lifecycle status of this tag. Communities propose tags, curators accept them, deprecated tags point to replacements via supersededBy. |
| `supersededBy` | `ComAtprotoRepoStrongRef` | When status is 'deprecated', points to the replacement work scope tag record. The record referenced must conform with the lexicon org.hypercerts.workscope.tag. |
| `uri` | `String!` | AT-URI of this record |

### `OrgHypercertsWorkscopeTagWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `key` | `StringFilterInput` | Filter by key |
| `name` | `StringFilterInput` | Filter by name |
| `description` | `StringFilterInput` | Filter by description |
| `status` | `StringFilterInput` | Filter by status |
| `category` | `StringFilterInput` | Filter by category |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `key`, `name`, `description`, `status`, `category`, `createdAt`, `indexed_at`

## `AppCertifiedActorProfile`

| Field | Type | Notes |
| --- | --- | --- |
| `avatar` | `AppCertifiedActorProfileAvatarUnion` | Small image to be displayed next to posts from account. AKA, 'profile picture' |
| `banner` | `AppCertifiedActorProfileBannerUnion` | Larger horizontal image to display behind profile view. |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created |
| `description` | `String` | Free-form profile description text. |
| `did` | `String!` | DID of the record author |
| `displayName` | `String` | Display name for the account |
| `pronouns` | `String` | Free-form pronouns text. |
| `rkey` | `String!` | Record key (last segment of AT-URI) |
| `uri` | `String!` | AT-URI of this record |
| `website` | `String` | Account website URL |

### `AppCertifiedActorProfileWhereInput`

| Filter field | Type | Notes |
| --- | --- | --- |
| `description` | `StringFilterInput` | Filter by description |
| `displayName` | `StringFilterInput` | Filter by displayName |
| `website` | `StringFilterInput` | Filter by website |
| `pronouns` | `StringFilterInput` | Filter by pronouns |
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |

Sort fields: `indexed_at`, `createdAt`, `description`, `displayName`, `website`, `pronouns`

## `AppCertifiedActorOrganization`

| Field | Type | Notes |
| --- | --- | --- |
| `cid` | `String!` | CID of this record version |
| `createdAt` | `DateTime!` | Client-declared timestamp when this record was originally created. |
| `did` | `String!` | DID of the record author |
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
| `did` | `DIDFilterInput` | Filter by DID (record author) |
| `createdAt` | `DateTimeFilterInput` | Filter by createdAt |
| `visibility` | `StringFilterInput` | Filter by visibility |
| `foundedDate` | `DateTimeFilterInput` | Filter by foundedDate |

Sort fields: `foundedDate`, `indexed_at`, `createdAt`, `visibility`

## Common union selections

Use inline fragments for union fields. Common current unions:

- `OrgHypercertsClaimActivityDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsClaimActivityImageUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `OrgHypercertsContextAttachmentItemsUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallBlob`
- `OrgHypercertsCollectionDescriptionUnion`: `OrgHypercertsDefsDescriptionString`, `PubLeafletPagesLinearDocument`, `ComAtprotoRepoStrongRef`
- `OrgHypercertsCollectionAvatarUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsSmallImage`
- `OrgHypercertsCollectionBannerUnion`: `OrgHypercertsDefsUri`, `OrgHypercertsDefsLargeImage`
