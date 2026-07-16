package graph

import (
	"sort"
	"strings"
	"time"
)

// SortKey selects which field the tree is ordered by.
type SortKey int

const (
	// SortDefault is the built-in status cascade (in_progress → ready → blocked
	// → deferred → closed, then timestamp). Zero value = today's behavior.
	SortDefault SortKey = iota
	SortPriority
	SortCreated
	SortUpdated
)

// SortSpec is a sort key plus direction. The zero value is SortDefault ascending
// (i.e. the legacy status-cascade order), so an unset spec changes nothing.
type SortSpec struct {
	Key  SortKey
	Desc bool
}

// Valid reports whether the spec is one of the canonical sort states (the
// vocabulary the overlay and persistence use). Non-canonical combinations such
// as {SortDefault, Desc:true} are rejected.
func (s SortSpec) Valid() bool {
	_, ok := sortSpecStrings[s]
	return ok
}

// Label is a compact human label for chrome (e.g. "Priority ↑"). Default sort
// returns "Default".
func (s SortSpec) Label() string {
	arrow := "↑"
	if s.Desc {
		arrow = "↓"
	}
	switch s.Key {
	case SortPriority:
		return "Priority " + arrow
	case SortCreated:
		return "Created " + arrow
	case SortUpdated:
		return "Updated " + arrow
	default:
		return "Default"
	}
}

// sortSpecStrings is the canonical persisted vocabulary (the 7 overlay choices).
var sortSpecStrings = map[SortSpec]string{
	{Key: SortDefault, Desc: false}:  "default",
	{Key: SortPriority, Desc: false}: "priority-asc",
	{Key: SortPriority, Desc: true}:  "priority-desc",
	{Key: SortCreated, Desc: true}:   "created-desc",
	{Key: SortCreated, Desc: false}:  "created-asc",
	{Key: SortUpdated, Desc: true}:   "updated-desc",
	{Key: SortUpdated, Desc: false}:  "updated-asc",
}

// String returns the canonical persisted token for the spec (default if unknown).
func (s SortSpec) String() string {
	if str, ok := sortSpecStrings[s]; ok {
		return str
	}
	return "default"
}

// ParseSortSpec maps a persisted token back to a spec. ok=false for unknown
// tokens so callers can fall back to SortDefault.
func ParseSortSpec(token string) (SortSpec, bool) {
	for spec, str := range sortSpecStrings {
		if str == token {
			return spec, true
		}
	}
	return SortSpec{}, false
}

// ApplySort orders roots and every descendant sibling group in place per spec.
// SortDefault reproduces the legacy computeSortMetrics + sortNodes cascade;
// other keys sort each sibling group by the node's OWN field (not a rollup),
// direction applied, with Issue.ID as a stable tiebreak.
func ApplySort(roots []*Node, spec SortSpec) {
	if spec.Key == SortDefault {
		for _, r := range roots {
			computeSortMetrics(r)
		}
		sortNodes(roots)
		return
	}
	sortTreeBySpec(roots, spec)
}

func sortTreeBySpec(nodes []*Node, spec SortSpec) {
	for _, n := range nodes {
		sortTreeBySpec(n.Children, spec)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return lessBySpec(nodes[i], nodes[j], spec)
	})
}

func lessBySpec(a, b *Node, spec SortSpec) bool {
	switch spec.Key {
	case SortPriority:
		if a.Issue.Priority != b.Issue.Priority {
			if spec.Desc {
				return a.Issue.Priority > b.Issue.Priority
			}
			return a.Issue.Priority < b.Issue.Priority
		}
	case SortCreated:
		if c := compareTimes(a.Issue.CreatedAt, b.Issue.CreatedAt, spec.Desc); c != 0 {
			return c < 0
		}
	case SortUpdated:
		if c := compareTimes(a.Issue.UpdatedAt, b.Issue.UpdatedAt, spec.Desc); c != 0 {
			return c < 0
		}
	}
	return a.Issue.ID < b.Issue.ID
}

// compareTimes returns <0 if a should sort before b, >0 if after, 0 if equal.
// Malformed/empty timestamps always sort LAST, regardless of direction.
func compareTimes(av, bv string, desc bool) int {
	at, aok := parseSortTime(av)
	bt, bok := parseSortTime(bv)
	switch {
	case !aok && !bok:
		return 0
	case !aok:
		return 1 // a invalid → a last
	case !bok:
		return -1 // b invalid → b last
	case at.Equal(bt):
		return 0
	case desc:
		if at.After(bt) {
			return -1
		}
		return 1
	default:
		if at.Before(bt) {
			return -1
		}
		return 1
	}
}

func parseSortTime(v string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
