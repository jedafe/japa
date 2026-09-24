package publicapi

import "testing"

// scopeIndex returns the position of a scope value in the catalog, or -1.
func scopeIndex(value string) int {
	for i, s := range ScopeCatalog {
		if s.Value == value {
			return i
		}
	}
	return -1
}

// The Phase 2 scopes must sit immediately after api.read.brief and before
// agent.use and the wildcard entries — the portal renders them in catalog
// order.
func TestScopeCatalogPhase2Ordering(t *testing.T) {
	brief := scopeIndex("api.read.brief")
	safety := scopeIndex("api.read.safety")
	search := scopeIndex("api.read.search")
	agentUse := scopeIndex("agent.use")
	readWildcard := scopeIndex("api.read.*")
	fullWildcard := scopeIndex("*")

	for _, tc := range []struct {
		name string
		idx  int
	}{
		{"api.read.brief", brief},
		{"api.read.safety", safety},
		{"api.read.search", search},
		{"agent.use", agentUse},
		{"api.read.*", readWildcard},
		{"*", fullWildcard},
	} {
		if tc.idx < 0 {
			t.Fatalf("scope %s missing from ScopeCatalog", tc.name)
		}
	}

	if safety != brief+1 || search != brief+2 {
		t.Errorf("phase 2 scopes must follow api.read.brief: brief=%d safety=%d search=%d", brief, safety, search)
	}
	if search >= agentUse {
		t.Errorf("phase 2 scopes must precede agent.use: search=%d agent.use=%d", search, agentUse)
	}
	if agentUse >= readWildcard || readWildcard >= fullWildcard {
		t.Errorf("wildcard ordering broken: agent.use=%d api.read.*=%d *=%d", agentUse, readWildcard, fullWildcard)
	}
}

func TestScopeCatalogPhase2Entries(t *testing.T) {
	labels := map[string]string{
		"api.read.safety": "Read Scam Alerts",
		"api.read.search": "Semantic Search",
	}
	for value, wantLabel := range labels {
		idx := scopeIndex(value)
		if idx < 0 {
			t.Fatalf("scope %s missing from ScopeCatalog", value)
		}
		got := ScopeCatalog[idx]
		if got.Label != wantLabel {
			t.Errorf("scope %s label = %q, want %q", value, got.Label, wantLabel)
		}
		if got.Description == "" {
			t.Errorf("scope %s has empty description", value)
		}
	}
}
