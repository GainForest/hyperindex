//go:build api_smoke

package apismoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

const responseSnippetLimit = 4096

// GraphQLResponse carries the decoded data and errors returned by the public GraphQL API.
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors"`
}

// GraphQLError describes a single GraphQL error with optional path and extension details.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Record is the generic shape shared by record-returning smoke test responses.
type Record struct {
	URI        string         `json:"uri"`
	CID        string         `json:"cid"`
	DID        string         `json:"did"`
	Collection string         `json:"collection"`
	RKey       string         `json:"rkey"`
	Value      map[string]any `json:"value"`
}

// PageInfo is the cursor pagination metadata shared by connection responses.
type PageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage"`
	StartCursor     string `json:"startCursor"`
	EndCursor       string `json:"endCursor"`
}

type graphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

type graphQLRequestOptions struct {
	AllowGraphQLErrors bool
}

func postGraphQL(t testing.TB, ctx context.Context, config smokeConfig, operationName string, query string, variables map[string]any) GraphQLResponse {
	t.Helper()

	return postGraphQLWithOptions(t, ctx, config, operationName, query, variables, graphQLRequestOptions{})
}

func smokeLog(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func postGraphQLWithOptions(t testing.TB, ctx context.Context, config smokeConfig, operationName string, query string, variables map[string]any, options graphQLRequestOptions) GraphQLResponse {
	t.Helper()

	requestBody := graphQLRequest{
		Query:         query,
		OperationName: operationName,
		Variables:     variables,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("GraphQL %s: encode variables %s: %v", operationName, mustMarshalVariables(variables), err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.baseURL+"/graphql", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("GraphQL %s: build request with variables %s: %v", operationName, mustMarshalVariables(variables), err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := config.httpClient.Do(request)
	if err != nil {
		t.Fatalf("GraphQL %s: request failed with variables %s: %v", operationName, mustMarshalVariables(variables), err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("GraphQL %s: read HTTP %d response with variables %s: %v", operationName, response.StatusCode, mustMarshalVariables(variables), err)
	}

	var decoded GraphQLResponse
	decodeErr := json.Unmarshal(body, &decoded)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("GraphQL %s: HTTP %d with variables %s, GraphQL errors %s, response %q", operationName, response.StatusCode, mustMarshalVariables(variables), formatGraphQLErrors(decoded.Errors), responseSnippet(body))
	}
	if decodeErr != nil {
		t.Fatalf("GraphQL %s: decode HTTP %d response with variables %s: %v; response %q", operationName, response.StatusCode, mustMarshalVariables(variables), decodeErr, responseSnippet(body))
	}
	if len(decoded.Errors) > 0 && !options.AllowGraphQLErrors {
		t.Fatalf("GraphQL %s: GraphQL errors with variables %s: %s; response %q", operationName, mustMarshalVariables(variables), formatGraphQLErrors(decoded.Errors), responseSnippet(body))
	}
	if config.debug {
		smokeLog("GraphQL operation=%s variables=%s status=%d errors=%d dataBytes=%d", operationName, mustMarshalVariables(variables), response.StatusCode, len(decoded.Errors), len(decoded.Data))
	}

	return decoded
}

func mustMarshalVariables(variables map[string]any) string {
	if variables == nil {
		return "{}"
	}
	payload, err := json.Marshal(variables)
	if err != nil {
		return fmt.Sprintf("<unmarshalable variables: %v>", err)
	}
	return string(payload)
}

func formatGraphQLErrors(errors []GraphQLError) string {
	if len(errors) == 0 {
		return "[]"
	}
	payload, err := json.Marshal(errors)
	if err != nil {
		return fmt.Sprintf("<unmarshalable GraphQL errors: %v>", err)
	}
	return string(payload)
}

func responseSnippet(body []byte) string {
	snippet := string(body)
	snippet = strings.TrimSpace(snippet)
	if len(snippet) <= responseSnippetLimit {
		return snippet
	}
	return snippet[:responseSnippetLimit] + "..."
}
