package publicapi

import (
	"fmt"
	"log/slog"
	"net/http"
)

// swaggerUIHTML returns the Swagger UI HTML with the given site name.
func swaggerUIHTML(siteName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <title>%s Public API — Swagger UI</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .topbar { display: none !important; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: "/api/v1/docs/openapi.json",
            dom_id: '#swagger-ui',
            presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
            layout: "BaseLayout",
            deepLinking: true,
        });
    </script>
</body>
</html>`, siteName)
}

// HandleSwaggerUI serves the Swagger UI HTML page for the public API.
func HandleSwaggerUI(siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' unpkg.com")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		if _, err := w.Write([]byte(swaggerUIHTML(siteName))); err != nil {
			slog.Warn("swagger ui: write error", "error", err)
		}
	}
}
