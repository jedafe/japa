package publicapi

// GetGPTActionsSpec returns a curated OpenAPI 3.0.3 subset sized for ChatGPT
// GPT Actions (operation-count and schema-size limits). Only stable
// agent-friendly GET endpoints are included — writes, webhooks, and the
// session-authenticated extension-token routes are omitted — and the servers
// list carries the production URL only (GPTs cannot reach localhost).
//
// Served at GET /docs/gpt-actions.json (§120, ADR-126).
func GetGPTActionsSpec(siteURL, siteName string) map[string]any {
	paths := map[string]any{
		"/public/me":                        mePath(),
		"/public/spaces":                    spacesPath(),
		"/public/spaces/{slug}":             spaceBySlugPath(),
		"/public/content":                   contentListPath(),
		"/public/content/{id}":              contentByIDPath(),
		"/public/content/{id}/comments":     contentCommentsPath(),
		"/public/users":                     usersListPath(),
		"/public/users/{id}":                userByIDPath(),
		"/public/tags":                      tagsPath(),
		"/public/tags/{slug}":               tagBySlugPath(),
		"/public/search":                    searchPath(),
		"/public/trending/content":          trendingEndpoint("listTrendingContent", "Trending content", "Retrieve trending content snapshots scored by recent engagement.", "api.read.content"),
		"/public/trending/spaces":           trendingEndpoint("listTrendingSpaces", "Trending spaces", "Retrieve trending space snapshots scored by recent member activity.", "api.read.spaces"),
		"/public/visa-routes":               visaRoutesPath(),
		"/public/visa-routes/{id}":          visaRouteByIDPath(),
		"/public/scholarships":              scholarshipsPath(),
		"/public/scholarships/{id}":         scholarshipByIDPath(),
		"/public/universities":              universitiesPath(),
		"/public/universities/{slug}":       universityBySlugPath(),
		"/public/destinations/{slug}/brief": destinationBriefPath(),
		"/public/scam-alerts":               scamAlertsPath(),
		"/public/scam-alerts/{id}":          scamAlertByIDPath(),
	}

	// GPT Actions is read-only — strip the write operations the shared path
	// builders include (POST /public/content, POST /public/content/{id}/comments).
	for _, item := range paths {
		if ops, ok := item.(map[string]any); ok {
			delete(ops, "post")
			delete(ops, "put")
			delete(ops, "patch")
			delete(ops, "delete")
		}
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       siteName + " Public API",
			"version":     "1.0.0",
			"description": "Read-only access to " + siteName + " public data: community spaces, published content, visa and immigration routes, scholarships, universities, destination briefs, trending items, and scam alerts. Authenticate every request with the X-API-Key header.",
		},
		"servers": []any{
			map[string]any{"url": siteURL + "/api/v1"},
		},
		"security": []any{
			map[string]any{"apiKeyAuth": []any{}},
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"apiKeyAuth": map[string]any{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-API-Key",
					"description": "API key obtained from the Developer Portal. Prefixed with jc_live_ (production) or jc_test_ (sandbox).",
				},
			},
			"schemas":   buildPublicSchemas(),
			"responses": buildPublicResponses(),
		},
	}
}
