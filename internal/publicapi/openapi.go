package publicapi

// GetPublicOpenAPISpec returns an OpenAPI 3.0.3 spec scoped to the public
// developer API. Only read-only endpoints exposed via X-API-Key auth are
// included. The spec is served at /api/v1/developer/swagger.json.
func GetPublicOpenAPISpec(siteURL, siteName string) map[string]any {
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
		"tags": buildTags(),
		"externalDocs": map[string]any{
			"description": "API documentation",
			"url":         siteURL + "/docs/api",
		},
		"paths": mergePaths(buildPublicPaths(), buildCatalogPublicPaths()),
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

// mergePaths combines two path maps without overwriting existing entries.
func mergePaths(a, b map[string]any) map[string]any {
	merged := make(map[string]any)
	for k, v := range a {
		merged[k] = v
	}

	for k, v := range b {
		merged[k] = v
	}

	return merged
}

// buildTags describes the public API operation groups used by the spec.
func buildTags() []any {
	return []any{
		map[string]any{"name": "Spaces", "description": "Public space discovery and detail"},
		map[string]any{"name": "Content", "description": "Published content and comments"},
		map[string]any{"name": "Users", "description": "Public user profiles and the current user"},
		map[string]any{"name": "Tags", "description": "Content tags and tag detail"},
		map[string]any{"name": "Search", "description": "Search published content"},
		map[string]any{"name": "Trending", "description": "Trending snapshots across entities"},
		map[string]any{"name": "Public", "description": "Public catalog routes that do not require an API key"},
	}
}

// operation builds a single OpenAPI operation object with an operationId,
// tags, required scope (x-required-scope) and the standard response map.
func operation(operationID, summary, description string, tags []any, scope string, parameters []any, responses map[string]any) map[string]any {
	op := map[string]any{
		"operationId":      operationID,
		"summary":          summary,
		"description":      description,
		"tags":             tags,
		"x-required-scope": scope,
		"responses":        responses,
	}
	if len(parameters) > 0 {
		op["parameters"] = parameters
	}

	return op
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
		"/public/trending/content":      trendingEndpoint("listTrendingContent", "Trending content", "Retrieve trending content snapshots scored by recent engagement.", "api.read.content"),
		"/public/trending/tags":         trendingEndpoint("listTrendingTags", "Trending tags", "Retrieve trending tag snapshots scored by recent follower growth.", "api.read.tags"),
		"/public/trending/spaces":       trendingEndpoint("listTrendingSpaces", "Trending spaces", "Retrieve trending space snapshots scored by recent member activity.", "api.read.spaces"),
		"/public/trending/users":        trendingEndpoint("listTrendingUsers", "Trending users", "Retrieve trending user snapshots scored by recent engagement.", "api.read.users.public"),
	}
}

// ---------------------------------------------------------------------------
// Individual path builders
// ---------------------------------------------------------------------------

func spacesPath() map[string]any {
	return map[string]any{
		"get": operation("listSpaces", "List public spaces", "Retrieve a paginated list of public spaces visible to the API key's tenant.", []any{"Spaces"}, "api.read.spaces", paginationParams("spaces"), map[string]any{
			"200": paginatedResponse("List of spaces", "Space"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func spaceBySlugPath() map[string]any {
	return map[string]any{
		"get": operation("getSpaceBySlug", "Get space detail", "Retrieve a single space by its URL slug.", []any{"Spaces"}, "api.read.spaces", []any{stringPathParam("slug", "Space slug")}, map[string]any{
			"200": singleResponse("Space details", "Space"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func contentByIDPath() map[string]any {
	return map[string]any{
		"get": operation("getContentByID", "Get content detail", "Retrieve a single published content item by its unique identifier.", []any{"Content"}, "api.read.content", []any{uuidPathParam("id", "Content ID")}, map[string]any{
			"200": singleResponse("Content details", "Content"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func contentListPath() map[string]any {
	return map[string]any{
		"get": operation("listContent", "List published content", "Retrieve a paginated list of published, public content within the API key's tenant. Drafts, scheduled, and private content are never exposed.", []any{"Content"}, "api.read.content", paginationParams("content"), map[string]any{
			"200": paginatedResponse("List of published content", "Content"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func contentCommentsPath() map[string]any {
	return map[string]any{
		"get": operation("listContentComments", "List content comments", "Retrieve top-level published comments for a content item, ordered by pinned then vote score.", []any{"Content"}, "api.read.content", append([]any{uuidPathParam("id", "Content ID")}, paginationParams("comments")...), map[string]any{
			"200": paginatedResponse("List of comments", "Comment"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func usersListPath() map[string]any {
	return map[string]any{
		"get": operation("listUsers", "List public users", "Retrieve a paginated list of public user profiles. PII-safe: no email, status, last_login, or group membership. Filter by ?q= (username/display_name search) and ?type= (profile_type).", []any{"Users"}, "api.read.users.public", []any{
			queryParam("q", "Search by username or display name"),
			filterParam("type", "Filter by profile type (e.g. individual, organization)"),
			limitParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("List of public user profiles", "User"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func tagBySlugPath() map[string]any {
	return map[string]any{
		"get": operation("getTagBySlug", "Get tag detail", "Retrieve a single tag by its URL slug, including follower and content counts.", []any{"Tags"}, "api.read.tags", []any{stringPathParam("slug", "Tag slug")}, map[string]any{
			"200": singleResponse("Tag details", "Tag"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func userByIDPath() map[string]any {
	return map[string]any{
		"get": operation("getUserByID", "Get user profile", "Retrieve a public user profile by their unique identifier.", []any{"Users"}, "api.read.users.public", []any{uuidPathParam("id", "User ID")}, map[string]any{
			"200": singleResponse("User profile", "User"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func tagsPath() map[string]any {
	return map[string]any{
		"get": operation("listTags", "List tags", "Retrieve a paginated list of content tags.", []any{"Tags"}, "api.read.tags", paginationParams("tags"), map[string]any{
			"200": paginatedResponse("List of tags", "Tag"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func searchPath() map[string]any {
	qp := queryParam("q", "Search query (required)").(map[string]any)
	qp["required"] = true

	return map[string]any{
		"get": operation("searchContent", "Search content", "Search published content by title within the API key's tenant.", []any{"Search"}, "api.read.content", []any{
			qp,
			limitParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("Search results", "Content"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func trendingEndpoint(operationID, summary, description, scope string) map[string]any {
	return map[string]any{
		"get": operation(operationID, summary, description, []any{"Trending"}, scope, []any{
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
		}, map[string]any{
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
		}),
	}
}

func mePath() map[string]any {
	return map[string]any{
		"get": operation("getMe", "Get current user profile", "Retrieve the authenticated user's own profile. Requires the api.read.me scope.", []any{"Users"}, "api.read.me", nil, map[string]any{
			"200": singleResponse("Current user profile", "User"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
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
