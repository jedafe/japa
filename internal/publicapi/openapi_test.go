package publicapi

import (
	"fmt"
	"testing"
)

func TestGetPublicOpenAPISpec(t *testing.T) {
	spec := GetPublicOpenAPISpec("https://japaconnect.com", "JapaConnect")

	if got, want := spec["openapi"], "3.0.3"; got != want {
		t.Fatalf("openapi version = %q, want %q", got, want)
	}

	info, ok := spec["info"].(map[string]any)
	if !ok {
		t.Fatal("info is not a map")
	}
	if info["title"] != "JapaConnect Public API" {
		t.Fatalf("title = %q, want %q", info["title"], "JapaConnect Public API")
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths is not a map")
	}
	if len(paths) == 0 {
		t.Fatal("paths is empty")
	}

	wantPaths := []string{
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
		"/public/me",
		"/public/trending/content",
		"/public/trending/tags",
		"/public/trending/spaces",
		"/public/trending/users",
	}
	for _, p := range wantPaths {
		if _, ok := paths[p]; !ok {
			t.Fatalf("expected path %q not found in spec", p)
		}
	}

	// Every public operation must have a stable operationId and a required scope.
	for path, pathItem := range paths {
		pi, ok := pathItem.(map[string]any)
		if !ok {
			t.Fatalf("path %q is not a map", path)
		}
		for method, opAny := range pi {
			op, ok := opAny.(map[string]any)
			if !ok {
				t.Fatalf("%s %s operation is not a map", method, path)
			}
			if op["operationId"] == nil || op["operationId"] == "" {
				t.Fatalf("%s %s is missing operationId", method, path)
			}
			scope, ok := op["x-required-scope"].(string)
			if !ok || scope == "" {
				t.Fatalf("%s %s is missing x-required-scope", method, path)
			}
		}
	}

	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("components is not a map")
	}
	securitySchemes, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatal("components.securitySchemes is not a map")
	}
	if _, ok := securitySchemes["apiKeyAuth"]; !ok {
		t.Fatal("securitySchemes.apiKeyAuth not found")
	}
}

func TestGetPublicOpenAPISpecServerURLs(t *testing.T) {
	spec := GetPublicOpenAPISpec("https://japaconnect.com", "JapaConnect")
	servers, ok := spec["servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatal("servers missing or empty")
	}
	first, ok := servers[0].(map[string]any)
	if !ok {
		t.Fatal("first server is not a map")
	}
	if first["url"] != "https://japaconnect.com/api/v1" {
		t.Fatalf("production server url = %q, want %q", first["url"], "https://japaconnect.com/api/v1")
	}
}

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://example.com", "example.com"},
		{"http://example.com/path", "example.com"},
		{"https://api.example.com:8080/", "api.example.com:8080"},
		{"example.com", "example.com"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s", c.in), func(t *testing.T) {
			if got := extractDomain(c.in); got != c.want {
				t.Fatalf("extractDomain(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
