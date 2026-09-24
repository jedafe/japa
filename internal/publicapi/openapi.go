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
			"description": "Public API for the " + siteName + " platform. Requires API key authentication via the X-API-Key header, except where an operation declares bearerAuth (the session-authenticated extension-token mint) or x-anonymous-feeds (unauthenticated explore feeds).",
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
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "Session access token — used only by the /me/extension-tokens mint routes, which are web-session authenticated rather than API-key authenticated.",
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
		map[string]any{"name": "Visa & Immigration", "description": "Visa routes, processing timelines, credential pathways, and language tests"},
		map[string]any{"name": "Education", "description": "Scholarships and universities"},
		map[string]any{"name": "Destinations", "description": "Aggregate destination briefs"},
		map[string]any{"name": "Feeds", "description": "Public explore feeds and the key owner's personalized feed"},
		map[string]any{"name": "Safety", "description": "Confirmed scam reports and the scam alert schema"},
		map[string]any{"name": "Developer", "description": "Session-authenticated developer tooling (extension token mint)"},
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

		// Agent Data API — visa & immigration (spec 2026-09-23-agent-data-api).
		// Chi registers /public/visa-routes/compare before {id}; order in this
		// map is documentation-only but mirrors the router for readability.
		"/public/visa-routes":                 visaRoutesPath(),
		"/public/visa-routes/{id}":            visaRouteByIDPath(),
		"/public/visa-routes/compare":         compareVisaRoutesPath(),
		"/public/visa-timelines/stats":        visaTimelineStatsPath(),
		"/public/visa-timelines/stats/trend":  visaTimelineTrendPath(),
		"/public/credentials":                 credentialPathwaysPath(),
		"/public/credentials/{id}":            credentialPathwayByIDPath(),
		"/public/credentials/professions":     credentialProfessionsPath(),
		"/public/language-tests":              languageTestsPath(),
		"/public/language-tests/{slug}":       languageTestBySlugPath(),
		"/public/language-tests/requirements": languageTestRequirementsPath(),

		// Agent Data API — education + destination brief.
		"/public/scholarships":              scholarshipsPath(),
		"/public/scholarships/{id}":         scholarshipByIDPath(),
		"/public/scholarships/statuses":     scholarshipStatusesPath(),
		"/public/scholarships/levels":       scholarshipLevelsPath(),
		"/public/universities":              universitiesPath(),
		"/public/universities/{slug}":       universityBySlugPath(),
		"/public/destinations/{slug}/brief": destinationBriefPath(),

		// Agent Data API Phase 2 — feeds, markdown, safety, semantic search
		// (spec 2026-09-24-agent-data-api-phase2). Chi registers the static
		// tag/space segments before {name} and scam-alerts/schema before
		// scam-alerts/{id}; map order mirrors the router for readability.
		"/public/feeds/tag/{tag}":       tagFeedPath(),
		"/public/feeds/space/{slug}":    spaceFeedPath(),
		"/public/feeds/{name}":          exploreFeedPath(),
		"/public/me/feed":               myFeedPath(),
		"/public/me/digest":             digestPath(),
		"/public/content/{id}/markdown": contentMarkdownPath(),
		"/public/scam-alerts":           scamAlertsPath(),
		"/public/scam-alerts/schema":    scamAlertSchemaPath(),
		"/public/scam-alerts/{id}":      scamAlertByIDPath(),
		"/public/search/semantic":       semanticSearchPath(),

		// Browser extension (Section 47 v1.3 — Browser-Extension Digest).
		// /me/extension-tokens is session-authenticated (bearerAuth), not
		// API-key authenticated — it exists so a signed-in user can mint the
		// narrow key an extension install consumes.
		"/me/extension-tokens": extensionTokensPath(),
	}
}

// ---------------------------------------------------------------------------
// Individual path builders
// ---------------------------------------------------------------------------

func spacesPath() map[string]any {
	return map[string]any{
		"get": operation("listSpaces", "List public spaces", "Retrieve a paginated list of public spaces visible to the API key's platform.", []any{"Spaces"}, "api.read.spaces", paginationParams("spaces"), map[string]any{
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
		"get": operation("listContent", "List published content", "Retrieve a paginated list of published, public content within the API key's platform. Drafts, scheduled, and private content are never exposed.", []any{"Content"}, "api.read.content", paginationParams("content"), map[string]any{
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
		"get": operation("searchContent", "Search content", "Search published content by title within the API key's platform.", []any{"Search"}, "api.read.content", []any{
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
// Agent Data API path builders (spec 2026-09-23-agent-data-api)
// Query parameters mirror internal/publicapi/handlers_moat.go and brief.go —
// pagination is page_size/offset via parsePageSize/parseOffset, not limit.
// ---------------------------------------------------------------------------

// visaTag is the operation tag for visa & immigration endpoints.
var visaTag = []any{"Visa & Immigration"}

// educationTag is the operation tag for education endpoints.
var educationTag = []any{"Education"}

// withRequiredTier marks an operation as gated behind a minimum unified
// user tier — mirrors RequireTier middleware on premium routes.
func withRequiredTier(op map[string]any, tier string) map[string]any {
	op["x-required-tier"] = tier
	return op
}

func visaRoutesPath() map[string]any {
	return map[string]any{
		"get": operation("listVisaRoutes", "List visa routes", "Retrieve visa route pathways for a destination space. Filter by profession, education level, and years of experience. Gated by the visa_routes.enabled feature flag.", visaTag, "api.read.visa", []any{
			uuidQueryParam("dest", "Destination space ID filter"),
			queryParam("profession", "Profession filter"),
			queryParam("education", "Education level filter"),
			intQueryParam("experience_years", "Years of experience filter"),
			pageSizeParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("List of visa routes", "VisaRoute"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func visaRouteByIDPath() map[string]any {
	return map[string]any{
		"get": operation("getVisaRoute", "Get visa route detail", "Retrieve a single visa route by its unique identifier.", visaTag, "api.read.visa", []any{uuidPathParam("id", "Visa route ID")}, map[string]any{
			"200": singleResponse("Visa route details", "VisaRoute"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func compareVisaRoutesPath() map[string]any {
	ids := queryParam("ids", "Comma-separated visa route IDs to compare (required, maximum 5)").(map[string]any)
	ids["required"] = true

	return map[string]any{
		"get": withRequiredTier(operation("compareVisaRoutes", "Compare visa routes", "Compare up to 5 visa routes side by side. Premium endpoint — requires a pro or business subscription.", visaTag, "api.read.visa", []any{ids}, map[string]any{
			"200": dataListResponse("Compared visa routes", "VisaRoute"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"429": refResponse("RateLimited"),
		}), "pro"),
	}
}

func visaTimelineStatsPath() map[string]any {
	return map[string]any{
		"get": operation("getVisaTimelineStats", "Visa processing-time stats", "Retrieve crowd-sourced visa processing-time statistics for an origin/destination pair. Gated by the visa_timeline.enabled feature flag.", visaTag, "api.read.visa", []any{
			uuidQueryParam("origin", "Origin space ID filter"),
			uuidQueryParam("dest", "Destination space ID filter"),
			queryParam("visa", "Visa category filter"),
			queryParam("consulate", "Consulate filter"),
			monthsParam(0, "Look-back window in months (1-60). Omit for all time."),
		}, map[string]any{
			"200": dataListResponse("Processing-time statistics", "VisaTimelineStat"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func visaTimelineTrendPath() map[string]any {
	return map[string]any{
		"get": withRequiredTier(operation("getVisaTimelineTrend", "Visa processing-time trend", "Retrieve the month-over-month processing-time trend for a destination. Premium endpoint — requires a pro or business subscription.", visaTag, "api.read.visa", []any{
			uuidQueryParam("dest", "Destination space ID filter"),
			queryParam("visa", "Visa category filter"),
			queryParam("consulate", "Consulate filter"),
			monthsParam(12, "Look-back window in months (1-60)."),
		}, map[string]any{
			"200": dataListResponse("Processing-time trend points", "VisaTrendPoint"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"429": refResponse("RateLimited"),
		}), "pro"),
	}
}

func credentialPathwaysPath() map[string]any {
	return map[string]any{
		"get": operation("listCredentialPathways", "List credential pathways", "Retrieve credential recognition pathways. Filter by destination space, issuing space, or profession. Gated by the credential_checker.enabled feature flag.", visaTag, "api.read.visa", []any{
			uuidQueryParam("dest", "Destination space ID filter"),
			uuidQueryParam("issuing", "Issuing space ID filter"),
			uuidQueryParam("profession", "Profession ID filter"),
			pageSizeParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("List of credential pathways", "CredentialPathway"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func credentialPathwayByIDPath() map[string]any {
	return map[string]any{
		"get": operation("getCredentialPathway", "Get credential pathway detail", "Retrieve a single credential pathway by its unique identifier.", visaTag, "api.read.visa", []any{uuidPathParam("id", "Credential pathway ID")}, map[string]any{
			"200": singleResponse("Credential pathway details", "CredentialPathway"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func credentialProfessionsPath() map[string]any {
	return map[string]any{
		"get": operation("listCredentialProfessions", "List credential professions", "Retrieve the professions that have credential recognition pathways. Gated by the credential_checker.enabled feature flag.", visaTag, "api.read.visa", nil, map[string]any{
			"200": dataListResponse("List of professions", "CredentialProfession"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func languageTestsPath() map[string]any {
	return map[string]any{
		"get": operation("listLanguageTests", "List language tests", "Retrieve supported language proficiency tests (IELTS, TOEFL, etc.). Gated by the language_test_hub.enabled feature flag.", visaTag, "api.read.visa", nil, map[string]any{
			"200": dataListResponse("List of language tests", "LanguageTest"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func languageTestBySlugPath() map[string]any {
	return map[string]any{
		"get": operation("getLanguageTest", "Get language test detail", "Retrieve a single language test by its URL slug.", visaTag, "api.read.visa", []any{stringPathParam("slug", "Language test slug")}, map[string]any{
			"200": singleResponse("Language test details", "LanguageTest"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func languageTestRequirementsPath() map[string]any {
	dest := uuidQueryParam("dest", "Destination space ID (required)").(map[string]any)
	dest["required"] = true

	return map[string]any{
		"get": operation("listLanguageTestRequirements", "List language test requirements", "Retrieve language test score requirements for a destination space, optionally filtered by visa category.", visaTag, "api.read.visa", []any{
			dest,
			queryParam("visa", "Visa category filter"),
		}, map[string]any{
			"200": dataListResponse("List of test requirements", "LanguageTestRequirement"),
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func scholarshipsPath() map[string]any {
	return map[string]any{
		"get": operation("findScholarships", "Search scholarships", "Search the scholarship finder. Filters mirror the web scholarship search. Gated by the scholarship_finder.enabled feature flag.", educationTag, "api.read.education", []any{
			queryParam("country", "Country filter"),
			queryParam("level", "Study level filter"),
			queryParam("q", "Free-text search"),
			queryParam("sort", "Sort order"),
			queryParam("deadline_from", "Earliest deadline (date)"),
			queryParam("deadline_to", "Latest deadline (date)"),
			boolQueryParam("include_expired", "Include scholarships past their deadline"),
			numberQueryParam("min_amount", "Minimum award amount"),
			numberQueryParam("max_amount", "Maximum award amount"),
			boolQueryParam("renewable", "Filter by renewable awards"),
			pageSizeParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("Scholarship search results", "Scholarship"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func scholarshipByIDPath() map[string]any {
	return map[string]any{
		"get": operation("getScholarship", "Get scholarship detail", "Retrieve a single scholarship by its guide ID.", educationTag, "api.read.education", []any{uuidPathParam("id", "Scholarship guide ID")}, map[string]any{
			"200": singleResponse("Scholarship details", "Scholarship"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func scholarshipStatusesPath() map[string]any {
	return map[string]any{
		"get": operation("listScholarshipStatuses", "List scholarship statuses", "Retrieve the valid scholarship status options.", educationTag, "api.read.education", nil, map[string]any{
			"200": dataListResponse("List of status options", "Option"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func scholarshipLevelsPath() map[string]any {
	return map[string]any{
		"get": operation("listScholarshipLevels", "List study levels", "Retrieve the valid study-level options for scholarship filtering.", educationTag, "api.read.education", nil, map[string]any{
			"200": dataListResponse("List of level options", "Option"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func universitiesPath() map[string]any {
	return map[string]any{
		"get": operation("listUniversities", "List universities", "Retrieve a paginated list of public university spaces.", educationTag, "api.read.education", []any{
			queryParam("q", "Free-text search"),
			queryParam("country", "Country filter"),
			queryParam("sort", "Sort order"),
			pageSizeParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("List of universities", "University"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func universityBySlugPath() map[string]any {
	return map[string]any{
		"get": operation("getUniversity", "Get university detail", "Retrieve a single public university space by its URL slug.", educationTag, "api.read.education", []any{stringPathParam("slug", "University slug")}, map[string]any{
			"200": singleResponse("University details", "University"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func destinationBriefPath() map[string]any {
	return map[string]any{
		"get": withRequiredTier(operation("getDestinationBrief", "Get destination brief", "Retrieve an aggregate brief for a destination space: visa routes, processing timelines, universities, scholarships, and scam alerts composed in parallel. Sections that fail are listed in sections_unavailable — the brief is best-effort. Premium endpoint — requires a pro or business subscription.", []any{"Destinations"}, "api.read.brief", []any{
			stringPathParam("slug", "Destination space slug"),
			map[string]any{
				"name":        "include",
				"in":          "query",
				"schema":      map[string]any{"type": "string"},
				"description": "Comma-separated sections to include: visa, timelines, universities, scholarships, scam_alerts. Omit for all sections.",
			},
		}, map[string]any{
			"200": singleResponse("Destination brief", "DestinationBrief"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}), "pro"),
	}
}

// ---------------------------------------------------------------------------
// Agent Data API Phase 2 path builders (spec 2026-09-24-agent-data-api-phase2)
// Query parameters mirror internal/publicapi/handlers_feeds.go — pagination
// is limit/offset via parseLimit/parseOffset; the tag feed is cursor-based.
// ---------------------------------------------------------------------------

// feedsTag is the operation tag for feed endpoints.
var feedsTag = []any{"Feeds"}

// safetyTag is the operation tag for scam-alert endpoints.
var safetyTag = []any{"Safety"}

// withFeatureFlag marks an operation as gated behind a system feature flag —
// mirrors features.Gate middleware, which answers 404 feature_disabled when
// the flag is off.
func withFeatureFlag(op map[string]any, flag string) map[string]any {
	op["x-feature-flag"] = flag
	return op
}

// feedSortParam documents the ?sort query parameter shared by the feed
// endpoints (feed.Sort* modes). Handlers fall back to the feed's default
// ranking on unknown values.
func feedSortParam() map[string]any {
	return map[string]any{
		"name":        "sort",
		"in":          "query",
		"schema":      map[string]any{"type": "string", "enum": []any{"latest", "top", "rising", "relevant"}},
		"description": "Sort order override. Unknown values fall back to the feed's default ranking.",
	}
}

func exploreFeedPath() map[string]any {
	op := operation("getExploreFeed", "Get explore feed", "Retrieve a public explore feed. The feed name selects the default ranking (popular → top, latest → latest, rising → rising); a valid ?sort value overrides it. Anonymous requests (no X-API-Key) are permitted for name=popular only — IP rate-limited at a loose tier; latest and rising always require api.read.content.", feedsTag, "api.read.content", []any{
		map[string]any{
			"name":        "name",
			"in":          "path",
			"required":    true,
			"schema":      map[string]any{"type": "string", "enum": []any{"popular", "latest", "rising"}},
			"description": "Explore feed name.",
		},
		feedSortParam(),
		limitParam(20, 100),
		offsetParam(),
	}, map[string]any{
		"200": feedListResponse("Explore feed items", "PublicFeedItem"),
		"400": refResponse("BadRequest"),
		"401": refResponse("Unauthorized"),
		"429": refResponse("RateLimited"),
	})
	// Feeds an unauthenticated caller may request (IP rate-limited). Only
	// "popular" — the browser extension's anonymous new-tab feed.
	op["x-anonymous-feeds"] = []any{"popular"}
	return map[string]any{"get": op}
}

func tagFeedPath() map[string]any {
	return map[string]any{
		"get": operation("getTagFeed", "Get tag feed", "Retrieve published, public content for a tag slug. Cursor-paginated: pass the previous response's next_cursor value in ?cursor to fetch the next page.", feedsTag, "api.read.content", []any{
			stringPathParam("tag", "Tag slug"),
			map[string]any{
				"name":        "cursor",
				"in":          "query",
				"schema":      map[string]any{"type": "string"},
				"description": "Opaque pagination cursor returned as next_cursor by the previous page.",
			},
			limitParam(20, 100),
		}, map[string]any{
			"200": cursorListResponse("Tag feed items", "PublicFeedItem"),
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func spaceFeedPath() map[string]any {
	return map[string]any{
		"get": operation("getSpaceFeed", "Get space feed", "Retrieve the feed for a public, active space. Member-only items resolve using the API key owner's membership, exactly as the web space feed does.", feedsTag, "api.read.content", []any{
			stringPathParam("slug", "Space slug"),
			feedSortParam(),
			limitParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": feedListResponse("Space feed items", "PublicFeedItem"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func myFeedPath() map[string]any {
	return map[string]any{
		"get": operation("getMyFeed", "Get for-you feed", "Retrieve the API key owner's ranked for-you feed. The user identity comes from the API key, never a query parameter.", feedsTag, "api.read.me", []any{
			feedSortParam(),
			limitParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": feedListResponse("For-you feed items", "PublicFeedItem"),
			"401": refResponse("Unauthorized"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func contentMarkdownPath() map[string]any {
	return map[string]any{
		"get": operation("getContentMarkdown", "Get content as markdown", "Retrieve a published, public content item rendered as text/markdown with a front-matter block (title, author, space, canonical URL). GET /public/content/{id} returns the same representation when the request sends Accept: text/markdown.", []any{"Content"}, "api.read.content", []any{uuidPathParam("id", "Content ID")}, map[string]any{
			"200": map[string]any{
				"description": "Markdown representation of the content item",
				"content": map[string]any{
					"text/markdown": map[string]any{
						"schema": map[string]any{"type": "string"},
					},
				},
			},
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func scamAlertsPath() map[string]any {
	return map[string]any{
		"get": withFeatureFlag(operation("listScamAlerts", "List scam alerts", "Retrieve confirmed scam reports, optionally filtered by scam type and destination space. Only status=confirmed reports are ever exposed. Gated by the scam_alert.enabled feature flag.", safetyTag, "api.read.safety", []any{
			queryParam("scam_type", "Scam type filter — see GET /public/scam-alerts/schema for valid values"),
			uuidQueryParam("destination_space_id", "Destination space ID filter"),
			limitParam(20, 100),
			offsetParam(),
		}, map[string]any{
			"200": paginatedResponse("List of confirmed scam reports", "PublicScamReport"),
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}), "scam_alert.enabled"),
	}
}

func scamAlertSchemaPath() map[string]any {
	return map[string]any{
		"get": withFeatureFlag(operation("getScamAlertSchema", "Get scam alert schema", "Retrieve the canonical scam_type and platform option lists used to filter and interpret scam reports. Gated by the scam_alert.enabled feature flag.", safetyTag, "api.read.safety", nil, map[string]any{
			"200": singleResponse("Scam alert schema options", "ScamAlertSchema"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}), "scam_alert.enabled"),
	}
}

func scamAlertByIDPath() map[string]any {
	return map[string]any{
		"get": withFeatureFlag(operation("getScamAlert", "Get scam alert detail", "Retrieve a single confirmed scam report by its unique identifier. Gated by the scam_alert.enabled feature flag.", safetyTag, "api.read.safety", []any{uuidPathParam("id", "Scam report ID")}, map[string]any{
			"200": singleResponse("Scam report details", "PublicScamReport"),
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
		}), "scam_alert.enabled"),
	}
}

func semanticSearchPath() map[string]any {
	q := queryParam("q", "Search query (required)").(map[string]any)
	q["required"] = true

	return map[string]any{
		"get": withRequiredTier(operation("semanticSearch", "Semantic search", "Search indexed content and spaces by embedding similarity (pgvector). Results are cached briefly — cache hits return cached=true. Returns 503 search_unavailable when no embedding provider is configured. Premium endpoint — requires a pro or business subscription.", []any{"Search"}, "api.read.search", []any{
			q,
			map[string]any{
				"name":        "lang",
				"in":          "query",
				"schema":      map[string]any{"type": "string", "default": "en"},
				"description": "Content language filter (ISO language code).",
			},
			queryParam("space", "Restrict results to a space slug"),
			limitParam(20, 50),
		}, map[string]any{
			"200": map[string]any{
				"description": "Semantic search results",
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/SemanticSearchResult"},
					},
				},
			},
			"400": refResponse("BadRequest"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"404": refResponse("NotFound"),
			"429": refResponse("RateLimited"),
			"503": refResponse("ServiceUnavailable"),
		}), "pro"),
	}
}

// ---------------------------------------------------------------------------
// Browser extension path builders (Section 47 v1.3 — Browser-Extension Digest)
// ---------------------------------------------------------------------------

func digestPath() map[string]any {
	return map[string]any{
		"get": operation("getDigest", "Get personalized digest", "Retrieve the aggregate new-tab digest for the API key owner: for-you feed, destination section (resolved from the key owner's destination_country profile field — null when unset or non-public), destination scam alerts, intel items, and the popular community feed. Optional sections degrade to null/empty on failure; a for-you feed outage fails the request. Responses are cached briefly per viewer (cached=true on a hit). Built for the browser extension; any key with api.read.digest may call it.", feedsTag, "api.read.digest", nil, map[string]any{
			"200": singleResponse("Personalized digest", "Digest"),
			"401": refResponse("Unauthorized"),
			"403": refResponse("Forbidden"),
			"429": refResponse("RateLimited"),
		}),
	}
}

func extensionTokensPath() map[string]any {
	sessionAuth := []any{map[string]any{"bearerAuth": []any{}}}
	return map[string]any{
		"get": map[string]any{
			"operationId": "listExtensionTokens",
			"summary":     "List extension tokens",
			"description": "List the session user's active browser-extension tokens (api_keys rows whose name carries the extension: prefix). Revoked tokens are omitted. Revocation uses DELETE /me/api-keys/{id}.",
			"tags":        []any{"Developer"},
			"security":    sessionAuth,
			"responses": map[string]any{
				"200": dataListResponse("Active extension tokens", "ApiKey"),
				"401": refResponse("Unauthorized"),
			},
		},
		"post": map[string]any{
			"operationId": "createExtensionToken",
			"summary":     "Mint an extension token",
			"description": "Mint a narrow-scoped API key for a browser-extension install. Scopes are fixed server-side (api.read.digest, api.read.content, api.read.me, api.read.safety — no search, no agent.use). The full key is returned once — store it in the extension's local storage. install_id is optional; when omitted the server generates a name suffix. The requested rate-limit tier is standard but is capped by the account's unified user tier.",
			"tags":        []any{"Developer"},
			"security":    sessionAuth,
			"requestBody": map[string]any{
				"required": false,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"install_id": map[string]any{"type": "string", "format": "uuid", "description": "Optional extension-generated install identifier; becomes the key name suffix"},
							},
						},
					},
				},
			},
			"responses": map[string]any{
				"201": singleResponse("Minted extension token — full key shown once", "ApiKeyCreated"),
				"400": refResponse("BadRequest"),
				"401": refResponse("Unauthorized"),
				"403": refResponse("Forbidden"),
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

// pageSizeParam documents the page_size query parameter used by the moat
// handlers (parsePageSize). The older endpoints above document the same
// pagination as "limit" — kept for backwards compatibility of the spec.
func pageSizeParam(defaultVal, maxVal int) map[string]any {
	return map[string]any{
		"name":        "page_size",
		"in":          "query",
		"schema":      map[string]any{"type": "integer", "default": defaultVal, "minimum": 1, "maximum": maxVal},
		"description": "Number of results to return per page.",
	}
}

func uuidQueryParam(name, description string) any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "string", "format": "uuid"},
		"description": description,
	}
}

func intQueryParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "integer"},
		"description": description,
	}
}

func boolQueryParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "boolean"},
		"description": description,
	}
}

func numberQueryParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"schema":      map[string]any{"type": "number"},
		"description": description,
	}
}

// monthsParam documents the months look-back window used by the timeline
// endpoints (clamped to 1-60 in the handlers; 0 means unset).
func monthsParam(defaultVal int, description string) map[string]any {
	schema := map[string]any{"type": "integer", "minimum": 1, "maximum": 60}
	if defaultVal > 0 {
		schema["default"] = defaultVal
	}
	return map[string]any{
		"name":        "months",
		"in":          "query",
		"schema":      schema,
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

// feedListResponse is the {data, limit, offset} envelope used by the feed
// endpoints — feeds have no cheap total count, so it is omitted.
func feedListResponse(description, itemSchema string) map[string]any {
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
						"limit":  map[string]any{"type": "integer"},
						"offset": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
}

// cursorListResponse is the {data, next_cursor, limit} envelope used by the
// cursor-paginated tag feed. next_cursor is an empty string when there is no
// next page.
func cursorListResponse(description, itemSchema string) map[string]any {
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
						"next_cursor": map[string]any{"type": "string", "description": "Pass as ?cursor to fetch the next page. Empty when there is no next page."},
						"limit":       map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
}

// dataListResponse is the {data: [...]} envelope used by non-paginated
// list endpoints (stats, trends, option lists, comparisons).
func dataListResponse(description, itemSchema string) map[string]any {
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
		// Agent Data API DTOs — field shapes mirror the domain DTOs in
		// internal/visaroutes, internal/visatimeline, internal/credentialchecker,
		// internal/languagetesthub, internal/scholarships, and brief.go.
		"VisaRoute": map[string]any{
			"type":        "object",
			"description": "A visa route pathway (visaroutes.RouteDTO).",
		},
		"VisaTimelineStat": map[string]any{
			"type":        "object",
			"description": "Crowd-sourced visa processing-time statistics (visatimeline.ProcessingStatDTO).",
		},
		"VisaTrendPoint": map[string]any{
			"type":        "object",
			"description": "A single month-over-month processing-time data point (visatimeline.TrendPointDTO).",
		},
		"CredentialPathway": map[string]any{
			"type":        "object",
			"description": "A credential recognition pathway (credentialchecker.PathwayDTO / PathwayDetailDTO).",
		},
		"CredentialProfession": map[string]any{
			"type":        "object",
			"description": "A profession with credential pathways (credentialchecker.ProfessionDTO).",
		},
		"LanguageTest": map[string]any{
			"type":        "object",
			"description": "A language proficiency test (languagetesthub.TestDTO).",
		},
		"LanguageTestRequirement": map[string]any{
			"type":        "object",
			"description": "A language test score requirement for a destination (languagetesthub.RequirementDTO).",
		},
		"Scholarship": map[string]any{
			"type":        "object",
			"description": "A scholarship listing (scholarships.ScholarshipResult).",
		},
		"Option": map[string]any{
			"type":        "object",
			"description": "A labeled filter option (scholarships.Option).",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
				"label": map[string]any{"type": "string"},
			},
		},
		"University": map[string]any{
			"type":        "object",
			"description": "A public university space.",
		},
		"DestinationBrief": map[string]any{
			"type":        "object",
			"description": "Aggregate destination brief — best-effort composition of visa, timeline, university, scholarship, and scam-alert sections.",
			"properties": map[string]any{
				"destination":          map[string]any{"type": "object"},
				"summary":              map[string]any{"type": "string"},
				"sections":             map[string]any{"type": "object"},
				"sections_unavailable": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"generated_at":         map[string]any{"type": "string", "format": "date-time"},
			},
		},
		// Agent Data API Phase 2 DTOs — field shapes mirror the public DTOs
		// in internal/publicapi/phase2.go.
		"PublicFeedItem": map[string]any{
			"type":        "object",
			"description": "A stable feed-card shape for API consumers (publicapi.PublicFeedItem).",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string"},
				"title":         map[string]any{"type": "string"},
				"slug":          map[string]any{"type": "string"},
				"content_type":  map[string]any{"type": "string"},
				"excerpt":       map[string]any{"type": "string"},
				"space_id":      map[string]any{"type": "string"},
				"space_slug":    map[string]any{"type": "string"},
				"space_name":    map[string]any{"type": "string"},
				"author_name":   map[string]any{"type": "string"},
				"author_handle": map[string]any{"type": "string"},
				"vote_score":    map[string]any{"type": "integer"},
				"comment_count": map[string]any{"type": "integer"},
				"view_count":    map[string]any{"type": "integer"},
				"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"published_at":  map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"PublicScamReport": map[string]any{
			"type":        "object",
			"description": "A confirmed scam report. Moderation internals (reporter, reviewer, moderator notes, AI provenance) are excluded from the public contract (publicapi.PublicScamReport).",
			"properties": map[string]any{
				"id":                      map[string]any{"type": "string", "format": "uuid"},
				"scam_type":               map[string]any{"type": "string"},
				"destination_space_name":  map[string]any{"type": "string"},
				"destination_space_code":  map[string]any{"type": "string"},
				"origin_space_name":       map[string]any{"type": "string"},
				"origin_space_code":       map[string]any{"type": "string"},
				"origin_community":        map[string]any{"type": "string"},
				"platform_used":           map[string]any{"type": "string"},
				"approximate_date":        map[string]any{"type": "string"},
				"amount_range":            map[string]any{"type": "string"},
				"description":             map[string]any{"type": "string"},
				"reported_to_authorities": map[string]any{"type": "boolean"},
				"is_pinned":               map[string]any{"type": "boolean"},
				"created_at":              map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"ScamAlertSchema": map[string]any{
			"type":        "object",
			"description": "Canonical scam_type and platform option lists (scamalert.SchemaOptions).",
			"properties": map[string]any{
				"scam_types": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Option"}},
				"platforms":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Option"}},
			},
		},
		"SemanticSearchResult": map[string]any{
			"type":        "object",
			"description": "Semantic search response envelope (publicapi.SemanticSearchResult).",
			"properties": map[string]any{
				"data":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SemanticHit"}},
				"query":    map[string]any{"type": "string"},
				"language": map[string]any{"type": "string"},
				"space":    map[string]any{"type": "string", "nullable": true, "description": "Space slug the search was restricted to, or null"},
				"cached":   map[string]any{"type": "boolean", "description": "True when the result was served from the response cache"},
			},
		},
		"SemanticHit": map[string]any{
			"type":        "object",
			"description": "One retrieved chunk in a semantic search result (publicapi.PublicSemanticHit).",
			"properties": map[string]any{
				"entity_type":  map[string]any{"type": "string"},
				"entity_id":    map[string]any{"type": "string"},
				"content_type": map[string]any{"type": "string"},
				"snippet":      map[string]any{"type": "string"},
				"similarity":   map[string]any{"type": "number", "format": "float"},
				"tier":         map[string]any{"type": "string"},
				"title":        map[string]any{"type": "string", "nullable": true, "description": "Best-effort resolved entity title; null when unresolvable"},
				"href":         map[string]any{"type": "string", "nullable": true, "description": "Best-effort resolved entity URL path; null when unresolvable"},
			},
		},
		// Browser extension digest DTOs — field shapes mirror
		// internal/publicapi/digest.go (PublicDigest et al.).
		"Digest": map[string]any{
			"type":        "object",
			"description": "Aggregate new-tab digest for the API key owner (publicapi.PublicDigest).",
			"properties": map[string]any{
				"for_you":      map[string]any{"$ref": "#/components/schemas/DigestFeedSection"},
				"destination":  map[string]any{"$ref": "#/components/schemas/DigestDestination", "nullable": true, "description": "Null when the owner has no destination_country or it resolves to a non-public space"},
				"community":    map[string]any{"$ref": "#/components/schemas/DigestCommunity"},
				"generated_at": map[string]any{"type": "string", "format": "date-time"},
				"cached":       map[string]any{"type": "boolean", "description": "True when served from the per-viewer digest cache"},
			},
		},
		"DigestFeedSection": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/PublicFeedItem"}},
				"source": map[string]any{"type": "string", "description": "Feed source identifier (e.g. for_you)"},
			},
		},
		"DigestDestination": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"space":       map[string]any{"$ref": "#/components/schemas/DigestSpaceRef"},
				"scam_alerts": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/PublicScamReport"}},
				"intel":       map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/IntelDigestItem"}},
			},
		},
		"DigestSpaceRef": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":  map[string]any{"type": "string"},
				"name":  map[string]any{"type": "string"},
				"level": map[string]any{"type": "string"},
			},
		},
		"IntelDigestItem": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"kind":       map[string]any{"type": "string", "enum": []any{"visa_route", "job"}},
				"title":      map[string]any{"type": "string"},
				"summary":    map[string]any{"type": "string"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"DigestCommunity": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"popular": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/PublicFeedItem"}},
			},
		},
		"ApiKey": map[string]any{
			"type":        "object",
			"description": "API key metadata (publicapi.APIKey). The full secret is never present on list/read responses — only at creation/rotation.",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "format": "uuid"},
				"name":            map[string]any{"type": "string"},
				"key_prefix":      map[string]any{"type": "string"},
				"scopes":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"rate_limit_tier": map[string]any{"type": "string"},
				"status":          map[string]any{"type": "string"},
				"request_count":   map[string]any{"type": "integer"},
				"last_used_at":    map[string]any{"type": "string", "format": "date-time"},
				"expires_at":      map[string]any{"type": "string", "format": "date-time"},
				"created_at":      map[string]any{"type": "string", "format": "date-time"},
				"updated_at":      map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"ApiKeyCreated": map[string]any{
			"type":        "object",
			"description": "Freshly minted API key (publicapi.CreateAPIKeyResponse). The key field contains the full secret exactly once — it is never retrievable again.",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "format": "uuid"},
				"key":             map[string]any{"type": "string", "description": "Full API key secret — shown only once"},
				"name":            map[string]any{"type": "string"},
				"key_prefix":      map[string]any{"type": "string"},
				"scopes":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"rate_limit_tier": map[string]any{"type": "string"},
				"created_at":      map[string]any{"type": "string", "format": "date-time"},
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
		"BadRequest": map[string]any{
			"description": "Invalid or missing request parameters",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		},
		"Unauthorized": map[string]any{
			"description": "Missing or invalid API key",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		},
		"Forbidden": map[string]any{
			"description": "Insufficient scope or subscription tier for this endpoint",
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
		"ServiceUnavailable": map[string]any{
			"description": "The endpoint is temporarily unavailable (e.g. semantic search without a configured embedding provider)",
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
