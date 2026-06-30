// Package graphclosure computes bounded directed graph closures.
package graphclosure

import (
	"context"
	"fmt"
	"sort"
)

const (
	// MaxDegree is the deepest hop count accepted by the closure algorithm.
	// Keeping traversal shallow prevents accidental network-wide graph scans from
	// one request.
	MaxDegree = 3

	// DefaultClosureCap is the maximum number of account nodes returned by one
	// closure computation before the response is marked truncated. The root DID
	// seed does not count against this cap.
	DefaultClosureCap = 3000

	// MaxVia limits how many same-degree predecessor DIDs are recorded for one
	// account node. This keeps dense graphs from turning a capped node set into an
	// unbounded provenance payload.
	MaxVia = 64
)

// Adjacency provides batched forward edges for a set of source DIDs.
// Implementations should return up to limit source -> target edges for the
// requested sources, grouped by source DID. Implementations must apply a
// deterministic edge order before enforcing the limit, and each source's target
// slice must also be deterministic. The boolean return value must be true when
// more edges matched than were returned.
type Adjacency interface {
	AdjacentForLimit(ctx context.Context, sources []string, limit int) (map[string][]string, bool, error)
}

// Account is one account node reached by the DID-rooted graph closure.
type Account struct {
	DID    string
	Degree int
	Via    []string
}

// Result contains the bounded graph closure and whether the traversal was cut
// short by the configured account cap or adjacency edge limit.
type Result struct {
	Accounts  []Account
	Truncated bool
}

// Compute walks directed edges breadth-first from rootDID up to maxDegree. The
// returned accounts are cumulative, assigned to their minimum reachable degree,
// and sorted by degree then DID for stable API responses.
func Compute(ctx context.Context, adjacency Adjacency, rootDID string, maxDegree, accountCap int) (Result, error) {
	if adjacency == nil {
		return Result{}, fmt.Errorf("graph adjacency source is required")
	}
	if rootDID == "" {
		return Result{}, fmt.Errorf("root DID is required")
	}
	if maxDegree < 1 || maxDegree > MaxDegree {
		return Result{}, fmt.Errorf("degree must be between 1 and %d, got %d", MaxDegree, maxDegree)
	}
	if accountCap <= 0 {
		return Result{}, fmt.Errorf("graph closure cap must be positive, got %d", accountCap)
	}

	seen := map[string]int{rootDID: 0}
	predecessors := map[string]map[string]struct{}{}
	frontier := []string{rootDID}
	truncated := false

	for degree := 1; degree <= maxDegree; degree++ {
		remainingAccounts := accountCap - (len(seen) - 1)
		if remainingAccounts <= 0 {
			truncated = true
			break
		}

		edgeLimit := remainingAccounts * MaxVia
		edges, edgesTruncated, err := adjacency.AdjacentForLimit(ctx, frontier, edgeLimit)
		if err != nil {
			return Result{}, fmt.Errorf("load graph edges for degree %d: %w", degree, err)
		}

		nextFrontier := make([]string, 0)
		for _, issuer := range frontier {
			for _, subject := range edges[issuer] {
				if subject == "" || subject == rootDID {
					continue
				}

				if existingDegree, ok := seen[subject]; ok {
					if existingDegree == degree && degree > 1 {
						if predecessors[subject] == nil {
							predecessors[subject] = map[string]struct{}{}
						}
						if len(predecessors[subject]) < MaxVia {
							predecessors[subject][issuer] = struct{}{}
						}
					}
					continue
				}

				if len(seen)-1 >= accountCap {
					truncated = true
					continue
				}

				seen[subject] = degree
				nextFrontier = append(nextFrontier, subject)
				if degree > 1 {
					predecessors[subject] = map[string]struct{}{issuer: {}}
				}
			}
		}

		if edgesTruncated {
			truncated = true
		}
		if truncated {
			break
		}
		if len(nextFrontier) == 0 {
			break
		}
		frontier = nextFrontier
	}

	accounts := make([]Account, 0, len(seen)-1)
	for did, degree := range seen {
		if did == rootDID {
			continue
		}

		via := make([]string, 0, len(predecessors[did]))
		for predecessor := range predecessors[did] {
			via = append(via, predecessor)
		}
		sort.Strings(via)

		accounts = append(accounts, Account{
			DID:    did,
			Degree: degree,
			Via:    via,
		})
	}

	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Degree != accounts[j].Degree {
			return accounts[i].Degree < accounts[j].Degree
		}
		return accounts[i].DID < accounts[j].DID
	})

	return Result{Accounts: accounts, Truncated: truncated}, nil
}
