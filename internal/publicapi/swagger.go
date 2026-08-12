package publicapi

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// Swagger UI assets are embedded via go:embed — no CDN dependency, works in
// air-gapped environments, binary impact <10MB (PUB-016). The assets are
// pre-built from the official swagger-ui-dist@5.18.2 package and committed
// to the repo; no build step is needed.

//go:embed swagger-ui
var swaggerUIFS embed.FS

// swaggerSubFS is the embedded swagger-ui directory exposed as an http.FS.
// It is resolved once at init so per-request handlers do not repeat the
// fs.Sub call.
var swaggerSubFS, _ = fs.Sub(swaggerUIFS, "swagger-ui")

// swaggerUIHTML returns the Swagger UI HTML with the given site name.
// Assets are loaded from the same origin (/developer/swagger/assets/*)
// instead of a CDN, so the page works in air-gapped environments and has
// no supply-chain dependency (PUB-016).
func swaggerUIHTML(siteName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <title>%s Public API — Swagger UI</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="icon" type="image/png" href="/developer/swagger/assets/favicon-32x32.png">
    <link rel="stylesheet" type="text/css" href="/developer/swagger/assets/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .topbar { display: none !important; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="/developer/swagger/assets/swagger-ui-bundle.js"></script>
    <script src="/developer/swagger/assets/swagger-ui-standalone-preset.js"></script>
    <script>
        SwaggerUIBundle({
            url: "/docs/openapi.json",
            dom_id: '#swagger-ui',
            presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
            layout: "BaseLayout",
            deepLinking: true,
        });
    </script>
</body>
</html>`, siteName)
}

// HandleSwaggerUI serves the Swagger UI HTML page for the public API.
// The page references embedded assets served by HandleSwaggerUIAssets.
// CSP is locked to 'self' (plus the inline scripts/styles Swagger UI
// requires) per Section 47 — no third-party origins (PUB-016).
func HandleSwaggerUI(siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		if _, err := w.Write([]byte(swaggerUIHTML(siteName))); err != nil {
			slog.Warn("swagger ui: write error", "error", err)
		}
	}
}

// HandleSwaggerUIAssets serves the embedded Swagger UI static assets
// (CSS, JS, favicon) under /developer/swagger/assets/. It must be mounted
// with http.StripPrefix("/developer/swagger/assets") so the embedded file
// names line up with the request paths (PUB-016).
func HandleSwaggerUIAssets() http.Handler {
	fileServer := http.FileServer(http.FS(swaggerSubFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Long-cache immutable static assets; the files are pinned to the
		// swagger-ui-dist version committed to the repo, so a content change
		// ships in a new binary rather than via cache-busting query strings.
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Header().Set("X-Frame-Options", "DENY")

		// Swagger UI ships only CSS/JS/PNG; lock the content types so a
		// misrouted request can never be served as HTML.
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		} else if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "image/png")
		}

		fileServer.ServeHTTP(w, r)
	})
}
