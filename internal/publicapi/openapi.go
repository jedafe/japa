package publicapi

// GetPublicOpenAPISpec returns an OpenAPI 3.0.3 spec scoped to the public
// developer API. Only read-only endpoints exposed via X-API-Key auth are
// included. The spec is served at /api/v1/developer/swagger.json.
func GetPublicOpenAPISpec(siteURL string, siteName string) map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       siteName + " Public API",
			"version":     "1.0.0",
			"description": "Public API for the " + siteName + " platform. Requires API key authentication via the X-API-Key header.",
			"contact": map[string]any{
				"name":  siteName + " Developer Support",
				"email": "api@" + extractDomain(siteURL),
				"url":   siteURL + "/docs",
			},
		},
		"servers": []any{
			map[string]any{"url": siteURL + "/api/v1", "description": "Production"},
			map[string]any{"url": "http://localhost:8080/api/v1", "description": "Local development"},
		},
		"security": []any{
			map[string]any{"apiKeyAuth": []any{}},
		},
		"paths": buildPublicPaths(),
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

// buildPublicPaths constructs the paths object for every public endpoint.
//
// Public API routes are mounted under /public/ to avoid chi radix-tree
// shadowing with the main API routes (server.go). The OpenAPI spec reflects
// these actual paths.
func buildPublicPaths() map[string]any {
	return map[string]any{
		"/public/spaces":                spacesPath(),
		"/public/spaces/{slug}":         spaceBySlugPath(),
		"/public/content":               contentListPath(),
		"/public/content/{id}":          contentByIDPath(),
		"/public/content/{id}/comments": contentCommentsPath(),
		"/public/users":                 usersListPath(),
		"/public/users/{id}":            userByIDPath(),
		"/public/tags":                  tagsPath(),
		"/public/tags/{slug}":           tagBySlugPath(),
		"/public/search":                searchPath(),
		"/public/me":                    mePath(),
		"/public/trending/content":      trendingEndpoint("Trending content", "Retrieve trending content snapshots scored by recent engagement."),
		"/public/trending/tags":         trendingEndpoint("Trending tags", "Retrieve trending tag snapshots scored by recent follower growth."),
		"/public/trending/spaces":       trendingEndpoint("Trending spaces", "Retrieve trending space snapshots scored by recent member activity."),
		"/public/trending/users":        trendingEndpoint("Trending users", "Retrieve trending user snapshots scored by recent engagement."),
	}
}

// ---------------------------------------------------------------------------
// Individual path builders
// ---------------------------------------------------------------------------

func spacesPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "List public spaces",
			"description": "Retrieve a paginated list of public spaces visible to the API key's tenant.",
			"tags":        []any{"Spaces"},
			"parameters":  paginationParams("spaces"),
			"responses": map[string]any{
				"200": paginatedResponse("List of spaces", "Space"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func spaceBySlugPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "Get space detail",
			"description": "Retrieve a single space by its URL slug.",
			"tags":        []any{"Spaces"},
			"parameters":  []any{stringPathParam("slug", "Space slug")},
			"responses": map[string]any{
				"200": singleResponse("Space details", "Space"),
				"401": refResponse("Unauthorized"),
				"404": refResponse("NotFound"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func contentByIDPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "Get content detail",
			"description": "Retrieve a single published content item by its unique identifier.",
			"tags":        []any{"Content"},
			"parameters":  []any{uuidPathParam("id", "Content ID")},
			"responses": map[string]any{
				"200": singleResponse("Content details", "Content"),
				"401": refResponse("Unauthorized"),
				"404": refResponse("NotFound"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func contentListPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "List published content",
			"description": "Retrieve a paginated list of published, public content within the API key's tenant. Drafts, scheduled, and private content are never exposed.",
			"tags":        []any{"Content"},
			"parameters":  paginationParams("content"),
			"responses": map[string]any{
				"200": paginatedResponse("List of published content", "Content"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func contentCommentsPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "List content comments",
			"description": "Retrieve top-level published comments for a content item, ordered by pinned then vote score.",
			"tags":        []any{"Content"},
			"parameters":  append([]any{uuidPathParam("id", "Content ID")}, paginationParams("comments")...),
			"responses": map[string]any{
				"200": paginatedResponse("List of comments", "Comment"),
				"401": refResponse("Unauthorized"),
				"404": refResponse("NotFound"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func usersListPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "List public users",
			"description": "Retrieve a paginated list of public user profiles. PII-safe: no email, status, last_login, or group membership. Filter by ?q= (username/display_name search) and ?type= (profile_type).",
			"tags":        []any{"Users"},
			"parameters": []any{
				queryParam("q", "Search by username or display name"),
				filterParam("type", "Filter by profile type (e.g. individual, organization)"),
				limitParam(20, 100),
				offsetParam(),
			},
			"responses": map[string]any{
				"200": paginatedResponse("List of public user profiles", "User"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func tagBySlugPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "Get tag detail",
			"description": "Retrieve a single tag by its URL slug, including follower and content counts.",
			"tags":        []any{"Tags"},
			"parameters":  []any{stringPathParam("slug", "Tag slug")},
			"responses": map[string]any{
				"200": singleResponse("Tag details", "Tag"),
				"401": refResponse("Unauthorized"),
				"404": refResponse("NotFound"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func userByIDPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "Get user profile",
			"description": "Retrieve a public user profile by their unique identifier.",
			"tags":        []any{"Users"},
			"parameters":  []any{uuidPathParam("id", "User ID")},
			"responses": map[string]any{
				"200": singleResponse("User profile", "User"),
				"401": refResponse("Unauthorized"),
				"404": refResponse("NotFound"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func tagsPath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "List tags",
			"description": "Retrieve a paginated list of content tags.",
			"tags":        []any{"Tags"},
			"parameters":  paginationParams("tags"),
			"responses": map[string]any{
				"200": paginatedResponse("List of tags", "Tag"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func searchPath() map[string]any {
	qp := queryParam("q", "Search query (required)").(map[string]any)
	qp["required"] = true

	return map[string]any{
		"get": map[string]any{
			"summary":     "Search content",
			"description": "Search published content by title within the API key's tenant.",
			"tags":        []any{"Search"},
			"parameters": []any{
				qp,
				limitParam(20, 100),
				offsetParam(),
			},
			"responses": map[string]any{
				"200": paginatedResponse("Search results", "Content"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func trendingEndpoint(summary, description string) map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     summary,
			"description": description,
			"tags":        []any{"Trending"},
			"parameters": []any{
				limitParam(10, 50),
				map[string]any{
					"name":        "window",
					"in":          "query",
					"schema":      map[string]any{"type": "string", "enum": []any{"24h", "7d", "30d"}, "default": "24h"},
					"description": "Time window for the trending snapshot.",
				},
				map[string]any{
					"name":        "geo",
					"in":          "query",
					"schema":      map[string]any{"type": "string"},
					"description": "Geo code filter for geo-aware trending (ADR-064). Omit for global ranking.",
				},
			},
			"responses": map[string]any{
				"200": map[string]any{
					"description": "Trending snapshot",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"data": map[string]any{
										"type":  "array",
										"items": map[string]any{"$ref": "#/components/schemas/TrendingItem"},
									},
									"computed_at": map[string]any{"type": "string", "format": "date-time"},
								},
							},
						},
					},
				},
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

func mePath() map[string]any {
	return map[string]any{
		"get": map[string]any{
			"summary":     "Get current user profile",
			"description": "Retrieve the authenticated user's own profile. Requires the api.read.me scope.",
			"tags":        []any{"Users"},
			"responses": map[string]any{
				"200": singleResponse("Current user profile", "User"),
				"401": refResponse("Unauthorized"),
				"429": refResponse("RateLimited"),
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Parameter helpers
// ---------------------------------------------------------------------------

func paginationParams(tag string) []any {
	return []any{limitParam(20, 100), offsetParam()}
}

func limitParam(defaultVal, maxVal int) map[string]any {
	return map[string]any{
		"name":        "limit",
		"in":          "query",
		"schema":      map[string]any{"type": "integer", "default": defaultVal, "minimum": 1, "maximum": maxVal},
		"description": "Number of results to return.",
	}
}

func offsetParam() map[string]any {
	return map[string]any{
		"name":        "offset",
		"in":          "query",
		"schema":      map[string]any{"type": "integer", "default": 0, "minimum": 0},
		"description": "Number of results to skip for pagination.",
	}
}

func queryParam(name, description string) any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "string"},
		"description": description,
	}
}

func filterParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "string"},
		"description": description,
	}
}

func uuidPathParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"required":    true,
		"schema":      map[string]any{"type": "string", "format": "uuid"},
		"description": description,
	}
}

func stringPathParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"required":    true,
		"schema":      map[string]any{"type": "string"},
		"description": description,
	}
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func paginatedResponse(description, itemSchema string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/" + itemSchema},
						},
						"total":  map[string]any{"type": "integer"},
						"limit":  map[string]any{"type": "integer"},
						"offset": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
}

func singleResponse(description, schema string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data": map[string]any{"$ref": "#/components/schemas/" + schema},
					},
				},
			},
		},
	}
}

func refResponse(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

// ---------------------------------------------------------------------------
// Component schemas
// ---------------------------------------------------------------------------

func buildPublicSchemas() map[string]any {
	return map[string]any{
		"Space": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string", "format": "uuid"},
				"name":         map[string]any{"type": "string"},
				"slug":         map[string]any{"type": "string"},
				"description":  map[string]any{"type": "string"},
				"member_count": map[string]any{"type": "integer"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
				"updated_at":   map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"Content": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string", "format": "uuid"},
				"title":        map[string]any{"type": "string"},
				"slug":         map[string]any{"type": "string"},
				"content_type": map[string]any{"type": "string", "enum": []any{"post", "article", "guide"}},
				"excerpt":      map[string]any{"type": "string"},
				"author_id":    map[string]any{"type": "string", "format": "uuid"},
				"space_id":     map[string]any{"type": "string", "format": "uuid"},
				"tags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"read_time":    map[string]any{"type": "integer", "description": "Estimated read time in minutes"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
				"updated_at":   map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"User": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string", "format": "uuid"},
				"username":     map[string]any{"type": "string"},
				"display_name": map[string]any{"type": "string"},
				"avatar_url":   map[string]any{"type": "string", "format": "uri"},
				"bio":          map[string]any{"type": "string"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"Tag": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"name":       map[string]any{"type": "string"},
				"slug":       map[string]any{"type": "string"},
				"post_count": map[string]any{"type": "integer"},
			},
		},
		"Comment": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"body":       map[string]any{"type": "string"},
				"author_id":  map[string]any{"type": "string", "format": "uuid"},
				"content_id": map[string]any{"type": "string", "format": "uuid"},
				"parent_id":  map[string]any{"type": "string", "format": "uuid", "description": "Parent comment ID for nested replies"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"TrendingItem": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":    map[string]any{"type": "string", "format": "uuid"},
				"name":  map[string]any{"type": "string"},
				"score": map[string]any{"type": "number", "format": "float", "description": "Trending score"},
				"slug":  map[string]any{"type": "string"},
				"meta":  map[string]any{"type": "object", "description": "Additional metadata varies by entity type"},
			},
		},
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":    map[string]any{"type": "string", "description": "Machine-readable error code"},
						"message": map[string]any{"type": "string", "description": "Human-readable error message"},
						"status":  map[string]any{"type": "integer", "description": "HTTP status code"},
						"docs":    map[string]any{"type": "string", "format": "uri", "description": "Link to error documentation"},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Shared responses
// ---------------------------------------------------------------------------

func buildPublicResponses() map[string]any {
	return map[string]any{
		"Unauthorized": map[string]any{
			"description": "Missing or invalid API key",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		},
		"NotFound": map[string]any{
			"description": "Resource not found",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		},
		"RateLimited": map[string]any{
			"description": "Rate limit exceeded",
			"headers": map[string]any{
				"Retry-After": map[string]any{
					"schema":      map[string]any{"type": "integer"},
					"description": "Seconds to wait before retrying",
				},
				"X-RateLimit-Limit": map[string]any{
					"schema":      map[string]any{"type": "integer"},
					"description": "Maximum requests per minute for this tier",
				},
				"X-RateLimit-Remaining": map[string]any{
					"schema":      map[string]any{"type": "integer"},
					"description": "Remaining requests in the current window",
				},
			},
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		},
	}
}

// extractDomain extracts the domain from a URL (e.g. "https://example.com" -> "example.com").
func extractDomain(url string) string {
	// Remove protocol
	domain := url
	if idx := len("https://"); len(domain) > idx && domain[:idx] == "https://" {
		domain = domain[idx:]
	} else if idx := len("http://"); len(domain) > idx && domain[:idx] == "http://" {
		domain = domain[idx:]
	}
	// Remove trailing path
	for i, c := range domain {
		if c == '/' {
			domain = domain[:i]
			break
		}
	}
	return domain
}
