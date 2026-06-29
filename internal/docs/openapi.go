package docs

// GetOpenAPISpec returns the JapaConnect OpenAPI 3.0.3 specification as a
// nested map suitable for JSON serialization. The spec is derived from
// docs/openapi/api.yaml and embedded here so the /docs/api/openapi.json
// endpoint can serve it without filesystem access.
func GetOpenAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "JapaConnect Public API",
			"version":     "1.0.0",
			"description": "Public API for JapaConnect platform integrations",
			"contact": map[string]any{
				"name":  "JapaConnect API Support",
				"email": "api-support@japaconnect.com",
			},
			"license": map[string]any{"name": "Proprietary"},
		},
		"servers": []any{
			map[string]any{"url": "https://api.japaconnect.com/v1", "description": "Production"},
			map[string]any{"url": "http://localhost:8080/v1", "description": "Local development"},
		},
		"security": []any{
			map[string]any{"ApiKeyAuth": []any{}},
		},
		"paths": map[string]any{
			"/spaces": map[string]any{
				"get": map[string]any{
					"summary":     "List spaces",
					"description": "Retrieve a paginated list of spaces visible to the API key's tenant.",
					"tags":        []any{"Spaces"},
					"parameters": []any{
						map[string]any{
							"name":        "q",
							"in":          "query",
							"schema":      map[string]any{"type": "string"},
							"description": "Search query to filter spaces by name or description.",
						},
						map[string]any{
							"name":        "limit",
							"in":          "query",
							"schema":      map[string]any{"type": "integer", "default": 20, "maximum": 100},
							"description": "Number of results to return.",
						},
						map[string]any{
							"name":        "offset",
							"in":          "query",
							"schema":      map[string]any{"type": "integer", "default": 0},
							"description": "Number of results to skip.",
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of spaces",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"data": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/Space"},
											},
										},
									},
								},
							},
						},
						"401": map[string]any{"$ref": "#/components/responses/Unauthorized"},
						"429": map[string]any{"$ref": "#/components/responses/RateLimited"},
					},
				},
			},
			"/content": map[string]any{
				"get": map[string]any{
					"summary":     "List content",
					"description": "Retrieve a paginated list of published content items.",
					"tags":        []any{"Content"},
					"parameters": []any{
						map[string]any{
							"name":        "q",
							"in":          "query",
							"schema":      map[string]any{"type": "string"},
							"description": "Search query to filter content.",
						},
						map[string]any{
							"name":        "limit",
							"in":          "query",
							"schema":      map[string]any{"type": "integer", "default": 20, "maximum": 100},
							"description": "Number of results to return.",
						},
						map[string]any{
							"name":        "offset",
							"in":          "query",
							"schema":      map[string]any{"type": "integer", "default": 0},
							"description": "Number of results to skip.",
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of content items",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"data": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/Content"},
											},
										},
									},
								},
							},
						},
						"401": map[string]any{"$ref": "#/components/responses/Unauthorized"},
						"429": map[string]any{"$ref": "#/components/responses/RateLimited"},
					},
				},
			},
			"/users/me": map[string]any{
				"get": map[string]any{
					"summary":     "Get authenticated user",
					"description": "Returns the user profile associated with the validated API key.",
					"tags":        []any{"Users"},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Current user profile",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"data": map[string]any{"$ref": "#/components/schemas/User"},
										},
									},
								},
							},
						},
						"401": map[string]any{"$ref": "#/components/responses/Unauthorized"},
						"429": map[string]any{"$ref": "#/components/responses/RateLimited"},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]any{
					"type":        "apiKey",
					"in":          "header",
					"name":        "X-API-Key",
					"description": "API key obtained from the Developer Portal. Keys are prefixed with jc_live_ for production or jc_test_ for sandbox.",
				},
			},
			"schemas": map[string]any{
				"Space": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "string", "format": "uuid"},
						"name":        map[string]any{"type": "string"},
						"slug":        map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"created_at":  map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"Content": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "string", "format": "uuid"},
						"title":        map[string]any{"type": "string"},
						"content_type": map[string]any{"type": "string"},
						"created_at":   map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"User": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string", "format": "uuid"},
						"username": map[string]any{"type": "string"},
						"email":    map[string]any{"type": "string"},
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
								"docs":    map[string]any{"type": "string", "format": "uri", "description": "Link to API documentation for this error"},
							},
						},
					},
				},
			},
			"responses": map[string]any{
				"Unauthorized": map[string]any{
					"description": "Missing or invalid API key",
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
			},
		},
	}
}
