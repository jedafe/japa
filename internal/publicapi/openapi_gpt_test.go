package publicapi

import (
	"strings"
	"testing"
)

func TestGetGPTActionsSpec(t *testing.T) {
	spec := GetGPTActionsSpec("https://api.japaconnect.com", "JapaConnect")

	if got, want := spec["openapi"], "3.0.3"; got != want {
		t.Fatalf("openapi version = %q, want %q", got, want)
	}

	// Servers must carry the production URL only — no localhost.
	servers, ok := spec["servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("servers = %v, want exactly one entry", spec["servers"])
	}

	serverURL, _ := servers[0].(map[string]any)["url"].(string)
	if serverURL != "https://api.japaconnect.com/api/v1" {
		t.Fatalf("server url = %q", serverURL)
	}

	if strings.Contains(serverURL, "localhost") {
		t.Fatal("localhost leaked into servers")
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("paths missing or empty")
	}

	wantPaths := []string{
		"/public/me",
		"/public/spaces",
		"/public/spaces/{slug}",
		"/public/content",
		"/public/content/{id}",
		"/public/content/{id}/comments",
		"/public/users",
		"/public/users/{id}",
		"/public/tags",
		"/public/tags/{slug}",
		"/public/search",
		"/public/trending/content",
		"/public/trending/spaces",
		"/public/visa-routes",
		"/public/visa-routes/{id}",
		"/public/scholarships",
		"/public/scholarships/{id}",
		"/public/universities",
		"/public/universities/{slug}",
		"/public/destinations/{slug}/brief",
		"/public/scam-alerts",
		"/public/scam-alerts/{id}",
	}
	for _, p := range wantPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q", p)
		}
	}

	// Read-only: no write operations anywhere in the document.
	for path, item := range paths {
		ops, ok := item.(map[string]any)
		if !ok {
			continue
		}

		for _, method := range []string{"post", "put", "patch", "delete"} {
			if _, ok := ops[method]; ok {
				t.Errorf("%s %s present — GPT Actions spec must be read-only", strings.ToUpper(method), path)
			}
		}

		if _, ok := ops["get"]; !ok {
			t.Errorf("%s has no GET operation", path)
		}
	}

	// Internal-only routes must not appear.
	for p := range paths {
		if strings.Contains(p, "developer") || strings.Contains(p, "webhook") || strings.Contains(p, "extension") {
			t.Errorf("internal-only path leaked: %s", p)
		}
	}

	// API-key security scheme.
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("components missing")
	}

	schemes, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatal("securitySchemes missing")
	}

	apiKey, ok := schemes["apiKeyAuth"].(map[string]any)
	if !ok {
		t.Fatal("apiKeyAuth scheme missing")
	}

	if apiKey["in"] != "header" || apiKey["name"] != "X-API-Key" {
		t.Fatalf("apiKeyAuth = %v, want header X-API-Key", apiKey)
	}
}
