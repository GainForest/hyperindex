//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/endorsement"
)

const (
	endorsementSmokeBadgeAwardCollection      = "app.certified.badge.award"
	endorsementSmokeBadgeDefinitionCollection = "app.certified.badge.definition"
	endorsementSmokeBadgeResponseCollection   = "app.certified.badge.response"
	endorsementSmokeSubjectType               = "app.certified.defs#did"
)

type endorsementSmokeEdge struct {
	Issuer  string
	Subject string
}

type endorsementSmokeAccount struct {
	DID    string   `json:"did"`
	Degree int      `json:"degree"`
	Via    []string `json:"via"`
}

type endorsementClosureQueryResponse struct {
	EndorsementClosure struct {
		Truncated bool `json:"truncated"`
		Edges     []struct {
			Node endorsementSmokeAccount `json:"node"`
		} `json:"edges"`
	} `json:"endorsementClosure"`
}

func TestEndorsementClosureBehaviorSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	if !config.expectations.EndorsementClosure.configured() {
		t.Skip("endorsementClosure is not configured in the smoke expectations file")
	}

	edges := fetchActiveEndorsementSmokeEdges(t, config)
	if len(edges) < config.expectations.EndorsementClosure.MinimumActiveEdges {
		t.Fatalf("endorsementClosure smoke found %d active account endorsement edges, want at least %d", len(edges), config.expectations.EndorsementClosure.MinimumActiveEdges)
	}

	rootDID, expectedAccounts, ok := selectEndorsementSmokeRoot(edges, config.expectations.EndorsementClosure.RequireIndirect)
	if !ok {
		t.Fatalf("endorsementClosure smoke found %d active edges but no root DID with an indirect closure path; edges: %s", len(edges), formatEndorsementSmokeEdges(edges))
	}

	got := queryEndorsementClosure(t, config, rootDID)
	if got.EndorsementClosure.Truncated {
		t.Fatalf("endorsementClosure(%q) truncated = true, want false for smoke fixture", rootDID)
	}
	gotAccounts := got.accounts()
	if !reflect.DeepEqual(gotAccounts, expectedAccounts) {
		expectedJSON, _ := json.Marshal(expectedAccounts)
		gotJSON, _ := json.Marshal(gotAccounts)
		t.Fatalf("endorsementClosure(%q) accounts = %s, want %s", rootDID, gotJSON, expectedJSON)
	}

	smokeLog("✓ endorsementClosure returns active account endorsement closure for %s (%d accounts)", rootDID, len(expectedAccounts))
}

func fetchActiveEndorsementSmokeEdges(t testing.TB, config smokeConfig) []endorsementSmokeEdge {
	t.Helper()

	definitionRecords := fetchGenericRecords(t, config, endorsementSmokeBadgeDefinitionCollection, 1000).Records.Edges
	awardRecords := fetchGenericRecords(t, config, endorsementSmokeBadgeAwardCollection, 1000).Records.Edges
	responseRecords := fetchGenericRecords(t, config, endorsementSmokeBadgeResponseCollection, 1000).Records.Edges

	endorsementDefinitions := make(map[string]Record, len(definitionRecords))
	for _, edge := range definitionRecords {
		if endorsementSmokeStringValue(edge.Node.Value, "badgeType") == "endorsement" {
			endorsementDefinitions[edge.Node.URI] = edge.Node
		}
	}

	rejectedAwards := make(map[string]bool, len(responseRecords))
	for _, edge := range responseRecords {
		if endorsementSmokeStringValue(edge.Node.Value, "response") != "rejected" {
			continue
		}
		awardURI := nestedStringValue(edge.Node.Value, "badgeAward", "uri")
		if awardURI == "" {
			continue
		}
		rejectedAwards[endorsementSmokeRejectionKey(edge.Node.DID, awardURI)] = true
	}

	edgeSet := make(map[endorsementSmokeEdge]bool)
	for _, edge := range awardRecords {
		award := edge.Node
		badgeURI := nestedStringValue(award.Value, "badge", "uri")
		definition, ok := endorsementDefinitions[badgeURI]
		if !ok || !endorsementDefinitionAllowsIssuer(definition, award.DID) {
			continue
		}

		subjectDID, ok := endorsementAwardAccountSubjectDID(award.Value)
		if !ok || subjectDID == award.DID {
			continue
		}
		if rejectedAwards[endorsementSmokeRejectionKey(subjectDID, award.URI)] {
			continue
		}
		edgeSet[endorsementSmokeEdge{Issuer: award.DID, Subject: subjectDID}] = true
	}

	edges := make([]endorsementSmokeEdge, 0, len(edgeSet))
	for edge := range edgeSet {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Issuer != edges[j].Issuer {
			return edges[i].Issuer < edges[j].Issuer
		}
		return edges[i].Subject < edges[j].Subject
	})
	return edges
}

func selectEndorsementSmokeRoot(edges []endorsementSmokeEdge, requireIndirect bool) (string, []endorsementSmokeAccount, bool) {
	roots := make(map[string]bool)
	for _, edge := range edges {
		roots[edge.Issuer] = true
	}

	rootList := make([]string, 0, len(roots))
	for rootDID := range roots {
		rootList = append(rootList, rootDID)
	}
	sort.Strings(rootList)

	for _, rootDID := range rootList {
		accounts := computeEndorsementSmokeClosure(edges, rootDID, 3)
		if len(accounts) == 0 {
			continue
		}
		if requireIndirect && !endorsementSmokeHasIndirect(accounts) {
			continue
		}
		return rootDID, accounts, true
	}

	return "", nil, false
}

func computeEndorsementSmokeClosure(edges []endorsementSmokeEdge, rootDID string, maxDegree int) []endorsementSmokeAccount {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		adjacency[edge.Issuer] = append(adjacency[edge.Issuer], edge.Subject)
	}
	for issuer := range adjacency {
		sort.Strings(adjacency[issuer])
	}

	seen := map[string]int{rootDID: 0}
	predecessors := map[string]map[string]bool{}
	frontier := []string{rootDID}

	for degree := 1; degree <= maxDegree; degree++ {
		nextFrontier := make([]string, 0)
		for _, issuer := range frontier {
			for _, subject := range adjacency[issuer] {
				if subject == "" || subject == rootDID {
					continue
				}

				if existingDegree, ok := seen[subject]; ok {
					if existingDegree == degree && degree > 1 {
						if predecessors[subject] == nil {
							predecessors[subject] = map[string]bool{}
						}
						if len(predecessors[subject]) < endorsement.MaxVia {
							predecessors[subject][issuer] = true
						}
					}
					continue
				}

				seen[subject] = degree
				nextFrontier = append(nextFrontier, subject)
				if degree > 1 {
					predecessors[subject] = map[string]bool{issuer: true}
				}
			}
		}
		if len(nextFrontier) == 0 {
			break
		}
		sort.Strings(nextFrontier)
		frontier = nextFrontier
	}

	accounts := make([]endorsementSmokeAccount, 0, len(seen)-1)
	for did, degree := range seen {
		if did == rootDID {
			continue
		}
		via := make([]string, 0, len(predecessors[did]))
		for predecessor := range predecessors[did] {
			via = append(via, predecessor)
		}
		sort.Strings(via)
		accounts = append(accounts, endorsementSmokeAccount{DID: did, Degree: degree, Via: via})
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Degree != accounts[j].Degree {
			return accounts[i].Degree < accounts[j].Degree
		}
		return accounts[i].DID < accounts[j].DID
	})
	return accounts
}

func endorsementSmokeHasIndirect(accounts []endorsementSmokeAccount) bool {
	for _, account := range accounts {
		if account.Degree > 1 {
			return true
		}
	}
	return false
}

func (r endorsementClosureQueryResponse) accounts() []endorsementSmokeAccount {
	accounts := make([]endorsementSmokeAccount, 0, len(r.EndorsementClosure.Edges))
	for _, edge := range r.EndorsementClosure.Edges {
		accounts = append(accounts, edge.Node)
	}
	return accounts
}

func queryEndorsementClosure(t testing.TB, config smokeConfig, rootDID string) endorsementClosureQueryResponse {
	t.Helper()

	response := postGraphQL(t, context.Background(), config, "SmokeEndorsementClosure", `
		query SmokeEndorsementClosure($did: String!) {
			endorsementClosure(where: { did: { eq: $did }, degree: { lte: 3 } }, first: 1000) {
				truncated
				edges {
					node {
						did
						degree
						via
					}
				}
			}
		}
	`, map[string]any{
		"did": rootDID,
	})

	var decoded endorsementClosureQueryResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("SmokeEndorsementClosure: decode response data for root DID %q: %v", rootDID, err)
	}
	for index, account := range decoded.accounts() {
		if account.DID == "" {
			t.Fatalf("SmokeEndorsementClosure: account[%d].did is empty", index)
		}
		if account.Degree < 1 || account.Degree > 3 {
			t.Fatalf("SmokeEndorsementClosure: account[%d].degree = %d, want 1..3", index, account.Degree)
		}
		if account.Degree == 1 && len(account.Via) != 0 {
			t.Fatalf("SmokeEndorsementClosure: account[%d].via = %#v, want empty for degree 1", index, account.Via)
		}
	}
	return decoded
}

func endorsementDefinitionAllowsIssuer(definition Record, issuer string) bool {
	rawAllowed, exists := definition.Value["allowedIssuers"]
	if !exists {
		return true
	}
	allowedIssuers, ok := rawAllowed.([]any)
	if !ok {
		return false
	}
	for _, rawIssuer := range allowedIssuers {
		issuerMap, ok := rawIssuer.(map[string]any)
		if !ok {
			continue
		}
		if endorsementSmokeStringValue(issuerMap, "did") == issuer {
			return true
		}
	}
	return false
}

func endorsementAwardAccountSubjectDID(value map[string]any) (string, bool) {
	subject, ok := value["subject"].(map[string]any)
	if !ok || endorsementSmokeStringValue(subject, "$type") != endorsementSmokeSubjectType {
		return "", false
	}
	did := endorsementSmokeStringValue(subject, "did")
	if !isSmokeDID(did) {
		return "", false
	}
	return did, true
}

func endorsementSmokeRejectionKey(subjectDID string, awardURI string) string {
	return subjectDID + "\x00" + awardURI
}

func nestedStringValue(value map[string]any, objectKey string, stringKey string) string {
	nested, ok := value[objectKey].(map[string]any)
	if !ok {
		return ""
	}
	return endorsementSmokeStringValue(nested, stringKey)
}

func endorsementSmokeStringValue(value map[string]any, key string) string {
	raw, ok := value[key].(string)
	if !ok {
		return ""
	}
	return raw
}

func isSmokeDID(value string) bool {
	return strings.HasPrefix(value, "did:plc:") || strings.HasPrefix(value, "did:web:")
}

func formatEndorsementSmokeEdges(edges []endorsementSmokeEdge) string {
	items := make([]string, 0, len(edges))
	for _, edge := range edges {
		items = append(items, fmt.Sprintf("%s -> %s", edge.Issuer, edge.Subject))
	}
	return strings.Join(items, ", ")
}
