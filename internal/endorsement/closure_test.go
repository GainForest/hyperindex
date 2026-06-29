package endorsement

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

type stubAdjacency map[string][]string

func (s stubAdjacency) EndorsementAdjacencyFor(_ context.Context, issuers []string) (map[string][]string, error) {
	out := make(map[string][]string, len(issuers))
	for _, issuer := range issuers {
		out[issuer] = append([]string(nil), s[issuer]...)
	}
	return out, nil
}

var errAdjacencyUnavailable = errors.New("database unavailable")

type failingAdjacency struct{}

func (failingAdjacency) EndorsementAdjacencyFor(context.Context, []string) (map[string][]string, error) {
	return nil, errAdjacencyUnavailable
}

func TestCompute(t *testing.T) {
	ctx := context.Background()
	graph := stubAdjacency{
		"did:plc:viewer": {"did:plc:alice", "did:plc:bob"},
		"did:plc:alice":  {"did:plc:carol", "did:plc:shared", "did:plc:viewer"},
		"did:plc:bob":    {"did:plc:dana", "did:plc:shared"},
		"did:plc:carol":  {"did:plc:erin", "did:plc:alice"},
		"did:plc:dana":   {"did:plc:erin"},
	}

	tests := []struct {
		name      string
		degree    int
		cap       int
		want      []Account
		truncated bool
	}{
		{
			name:   "degree one returns direct endorsements without via",
			degree: 1,
			cap:    100,
			want: []Account{
				{DID: "did:plc:alice", Degree: 1, Via: []string{}},
				{DID: "did:plc:bob", Degree: 1, Via: []string{}},
			},
		},
		{
			name:   "degree two is cumulative and records same-ring provenance",
			degree: 2,
			cap:    100,
			want: []Account{
				{DID: "did:plc:alice", Degree: 1, Via: []string{}},
				{DID: "did:plc:bob", Degree: 1, Via: []string{}},
				{DID: "did:plc:carol", Degree: 2, Via: []string{"did:plc:alice"}},
				{DID: "did:plc:dana", Degree: 2, Via: []string{"did:plc:bob"}},
				{DID: "did:plc:shared", Degree: 2, Via: []string{"did:plc:alice", "did:plc:bob"}},
			},
		},
		{
			name:   "degree three keeps minimum degree and ignores cycles to shallower nodes",
			degree: 3,
			cap:    100,
			want: []Account{
				{DID: "did:plc:alice", Degree: 1, Via: []string{}},
				{DID: "did:plc:bob", Degree: 1, Via: []string{}},
				{DID: "did:plc:carol", Degree: 2, Via: []string{"did:plc:alice"}},
				{DID: "did:plc:dana", Degree: 2, Via: []string{"did:plc:bob"}},
				{DID: "did:plc:shared", Degree: 2, Via: []string{"did:plc:alice", "did:plc:bob"}},
				{DID: "did:plc:erin", Degree: 3, Via: []string{"did:plc:carol", "did:plc:dana"}},
			},
		},
		{
			name:      "cap truncates in-flight ring",
			degree:    3,
			cap:       3,
			truncated: true,
			want: []Account{
				{DID: "did:plc:alice", Degree: 1, Via: []string{}},
				{DID: "did:plc:bob", Degree: 1, Via: []string{}},
				{DID: "did:plc:carol", Degree: 2, Via: []string{"did:plc:alice"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(ctx, graph, "did:plc:viewer", tt.degree, tt.cap)
			if err != nil {
				t.Fatalf("Compute() error = %v", err)
			}
			if got.Truncated != tt.truncated {
				t.Fatalf("Truncated = %v, want %v", got.Truncated, tt.truncated)
			}
			if !reflect.DeepEqual(got.Accounts, tt.want) {
				t.Fatalf("Accounts = %#v, want %#v", got.Accounts, tt.want)
			}
		})
	}
}

func TestComputeCapsAndSortsVia(t *testing.T) {
	ctx := context.Background()
	graph := stubAdjacency{"did:plc:viewer": {}}
	for i := 0; i < MaxVia+1; i++ {
		issuer := fmt.Sprintf("did:plc:issuer%03d", i)
		graph["did:plc:viewer"] = append(graph["did:plc:viewer"], issuer)
		graph[issuer] = []string{"did:plc:target"}
	}

	got, err := Compute(ctx, graph, "did:plc:viewer", 2, 1000)
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}

	var target *Account
	for i := range got.Accounts {
		if got.Accounts[i].DID == "did:plc:target" {
			target = &got.Accounts[i]
			break
		}
	}
	if target == nil {
		t.Fatal("target account was not returned")
	}
	if len(target.Via) != MaxVia {
		t.Fatalf("len(target.Via) = %d, want %d", len(target.Via), MaxVia)
	}
	if !sort.StringsAreSorted(target.Via) {
		t.Fatalf("target.Via = %#v, want sorted DIDs", target.Via)
	}
	if target.Via[MaxVia-1] != "did:plc:issuer063" {
		t.Fatalf("last target.Via = %q, want cap before issuer064", target.Via[MaxVia-1])
	}
}

func TestComputeValidation(t *testing.T) {
	ctx := context.Background()
	graph := stubAdjacency{}

	tests := []struct {
		name   string
		viewer string
		degree int
		cap    int
	}{
		{name: "missing viewer", viewer: "", degree: 1, cap: 10},
		{name: "degree too low", viewer: "did:plc:viewer", degree: 0, cap: 10},
		{name: "degree too high", viewer: "did:plc:viewer", degree: 4, cap: 10},
		{name: "cap not positive", viewer: "did:plc:viewer", degree: 1, cap: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compute(ctx, graph, tt.viewer, tt.degree, tt.cap); err == nil {
				t.Fatal("Compute() error = nil, want validation error")
			}
		})
	}

	if _, err := Compute(ctx, nil, "did:plc:viewer", 1, 10); err == nil {
		t.Fatal("Compute() with nil adjacency error = nil, want validation error")
	}
}

func TestComputeWrapsAdjacencyErrors(t *testing.T) {
	_, err := Compute(context.Background(), failingAdjacency{}, "did:plc:viewer", 1, 10)
	if err == nil {
		t.Fatal("Compute() error = nil, want adjacency error")
	}
	if !errors.Is(err, errAdjacencyUnavailable) {
		t.Fatalf("Compute() error = %v, want wrapped errAdjacencyUnavailable", err)
	}
	if err.Error() == errAdjacencyUnavailable.Error() {
		t.Fatalf("Compute() error was not wrapped with degree context: %v", err)
	}
}
