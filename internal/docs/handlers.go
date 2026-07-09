package docs

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for documentation HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// ---------------------------------------------------------------------------
// Public Handlers (read-only, RBAC-gated by audience)
// ---------------------------------------------------------------------------

// ListDocPages handles GET /api/v1/docs/{audience}.
// Returns published pages for the given audience section.
func (h *Handlers) ListDocPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	audience := chi.URLParam(r, "audience")
	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	pages, err := h.Service.ListDocPagesByAudience(r.Context(), tenantID, audience)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// GetDocPage handles GET /api/v1/docs/{audience}/{slug}.
// Returns a single published page.
func (h *Handlers) GetDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	audience := chi.URLParam(r, "audience")
	slug := chi.URLParam(r, "slug")

	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	page, err := h.Service.GetDocPageBySlug(r.Context(), tenantID, audience, slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ---------------------------------------------------------------------------
// Admin Handlers (write operations)
// ---------------------------------------------------------------------------

// ListAllDocPages handles GET /api/v1/admin/docs/{audience}.
// Returns all pages (any status) for admin listing.
func (h *Handlers) ListAllDocPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	audience := chi.URLParam(r, "audience")
	if !ValidAudiences[audience] {
		writeError(w, http.StatusBadRequest, "invalid_audience", "Invalid audience section")
		return
	}

	pages, err := h.Service.ListAllDocPagesByAudience(r.Context(), tenantID, audience)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// CreateDocPage handles POST /api/v1/admin/docs.
func (h *Handlers) CreateDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req CreateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if req.Title == "" || req.Slug == "" || req.Audience == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "title, slug, and audience are required")
		return
	}

	if err := h.Service.CreateDocPage(r.Context(), tenantID, req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// UpdateDocPage handles PATCH /api/v1/admin/docs/{id}.
func (h *Handlers) UpdateDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	var req UpdateDocPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "title is required")
		return
	}

	if err := h.Service.UpdateDocPage(r.Context(), tenantID, id, req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteDocPage handles DELETE /api/v1/admin/docs/{id}.
func (h *Handlers) DeleteDocPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	if err := h.Service.DeleteDocPage(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// API Playground & OpenAPI Spec (Section 44 — docs.japaconnect.com)
// ---------------------------------------------------------------------------

// OpenAPIHandler serves the OpenAPI 3.0.3 specification as JSON.
func (h *Handlers) OpenAPIHandler(w http.ResponseWriter, _ *http.Request) {
	// Canonical spec lives in internal/publicapi (the more complete generator);
	// serving it here too avoids two independently hand-maintained specs
	// drifting apart (§6.4 dedupe).
	spec := publicapi.GetPublicOpenAPISpec()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(spec); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// APIPlaygroundHandler serves the interactive API playground page.
func (h *Handlers) APIPlaygroundHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(playgroundHTML)); err != nil {
		slog.Warn("write error", "error", err)
	}
}

// playgroundHTML is a self-contained HTML page for the API playground.
// No external dependencies — all CSS and JS are inline.
const playgroundHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>JapaConnect API Playground</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif;
    background: #0f172a; color: #f1f5f9; line-height: 1.6;
  }
  a { color: #60a5fa; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .header {
    background: #1e293b; border-bottom: 1px solid #334155;
    padding: 16px 24px; display: flex; align-items: center; gap: 16px;
  }
  .header h1 { font-size: 18px; font-weight: 600; }
  .header nav { margin-left: auto; font-size: 14px; display: flex; gap: 16px; }
  .container { max-width: 960px; margin: 0 auto; padding: 24px; }
  .section { background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 20px; margin-bottom: 20px; }
  .section h2 { font-size: 14px; font-weight: 600; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 12px; }
  label { display: block; font-size: 13px; color: #94a3b8; margin-bottom: 4px; }
  input, textarea, select {
    width: 100%; background: #0f172a; border: 1px solid #334155; color: #f1f5f9;
    border-radius: 6px; padding: 8px 12px; font-family: inherit; font-size: 14px;
    outline: none; transition: border-color 0.15s;
  }
  input:focus, textarea:focus, select:focus { border-color: #3b82f6; }
  textarea { font-family: 'SF Mono', 'Fira Code', monospace; min-height: 120px; resize: vertical; }
  .row { display: flex; gap: 12px; margin-bottom: 12px; }
  .row > * { flex: 1; }
  .method-url { display: flex; gap: 8px; margin-bottom: 12px; }
  .method-url select { width: 120px; flex-shrink: 0; }
  .method-url input { flex: 1; }
  .btn {
    background: #3b82f6; color: #fff; border: none; border-radius: 6px;
    padding: 10px 24px; font-size: 14px; font-weight: 600; cursor: pointer;
    transition: background 0.15s;
  }
  .btn:hover { background: #2563eb; }
  .btn:active { background: #1d4ed8; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .response-area { margin-top: 16px; }
  .response-meta { display: flex; gap: 16px; margin-bottom: 8px; font-size: 13px; }
  .status-ok { color: #34d399; }
  .status-err { color: #f87171; }
  .status-pending { color: #94a3b8; }
  .response-body {
    background: #0f172a; border: 1px solid #334155; border-radius: 6px;
    padding: 16px; font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px;
    white-space: pre-wrap; word-break: break-all; max-height: 500px; overflow-y: auto;
  }
  .headers-list { margin-top: 8px; }
  .header-row { display: flex; gap: 8px; margin-bottom: 6px; }
  .header-row input { font-size: 13px; }
  .remove-btn {
    background: none; border: none; color: #64748b; cursor: pointer;
    font-size: 18px; line-height: 1; padding: 0 4px;
  }
  .remove-btn:hover { color: #f87171; }
  .add-header { font-size: 13px; color: #60a5fa; cursor: pointer; background: none; border: none; margin-top: 4px; }
  .add-header:hover { text-decoration: underline; }
  .endpoints { margin-top: 16px; }
  .endpoint-chip {
    display: inline-block; background: #0f172a; border: 1px solid #334155;
    border-radius: 6px; padding: 6px 12px; margin: 4px; font-size: 13px; cursor: pointer;
    transition: border-color 0.15s;
  }
  .endpoint-chip:hover { border-color: #3b82f6; }
  .method-badge {
    display: inline-block; font-weight: 700; font-size: 11px; margin-right: 6px;
    text-transform: uppercase;
  }
  .method-GET { color: #34d399; }
  .method-POST { color: #fbbf24; }
  .method-PUT { color: #60a5fa; }
  .method-DELETE { color: #f87171; }
</style>
</head>
<body>

<div class="header">
  <h1>API Playground</h1>
  <nav>
    <a href="/docs/openapi.json">OpenAPI Spec</a>
    <a href="/">Documentation Home</a>
  </nav>
</div>

<div class="container">
  <div class="section">
    <h2>Authentication</h2>
    <label for="apiKey">API Key</label>
    <input type="text" id="apiKey" placeholder="jc_test_..." autocomplete="off">
  </div>

  <div class="section">
    <h2>Request</h2>
    <div class="method-url">
      <select id="method">
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="DELETE">DELETE</option>
      </select>
      <input type="text" id="url" placeholder="/api/v1/spaces" value="/api/v1/spaces">
    </div>

    <label>Headers</label>
    <div id="headers" class="headers-list">
      <div class="header-row">
        <input type="text" placeholder="Key" value="Content-Type" class="hkey">
        <input type="text" placeholder="Value" value="application/json" class="hval">
        <button class="remove-btn" onclick="this.parentElement.remove()">&times;</button>
      </div>
    </div>
    <button class="add-header" onclick="addHeader()">+ Add header</button>

    <div style="margin-top: 12px;">
      <label for="body">Request Body (JSON)</label>
      <textarea id="body" placeholder='{"key": "value"}'></textarea>
    </div>

    <div style="margin-top: 12px; display: flex; gap: 12px; align-items: center;">
      <button class="btn" id="sendBtn" onclick="sendRequest()">Send Request</button>
      <span id="timing" class="status-pending"></span>
    </div>
  </div>

  <div class="section">
    <h2>Response</h2>
    <div class="response-meta">
      <span id="respStatus" class="status-pending">No request sent yet</span>
      <span id="respTime" class="status-pending"></span>
    </div>
    <div class="response-body" id="respBody">// Response will appear here</div>
  </div>

  <div class="section">
    <h2>Quick Endpoints</h2>
    <div class="endpoints" id="endpoints"></div>
  </div>
</div>

<script>
const BASE = window.location.origin;
const endpoints = [
  { method: 'GET',  path: '/api/v1/spaces',          label: 'List Spaces' },
  { method: 'GET',  path: '/api/v1/content',         label: 'List Content' },
  { method: 'GET',  path: '/api/v1/users/me',        label: 'Get Me' },
  { method: 'POST', path: '/api/v1/content',         label: 'Create Post' },
  { method: 'PUT',  path: '/api/v1/users/me',        label: 'Update Profile' },
  { method: 'DELETE', path: '/api/v1/content/{id}',   label: 'Delete Post' },
];

const epContainer = document.getElementById('endpoints');
endpoints.forEach(ep => {
  const chip = document.createElement('button');
  chip.className = 'endpoint-chip';
  chip.innerHTML = '<span class="method-badge method-' + ep.method + '">' + ep.method + '</span>' + ep.label;
  chip.onclick = () => {
    document.getElementById('method').value = ep.method;
    document.getElementById('url').value = ep.path;
  };
  epContainer.appendChild(chip);
});

function addHeader() {
  const row = document.createElement('div');
  row.className = 'header-row';
  row.innerHTML =
    '<input type="text" placeholder="Key" class="hkey">' +
    '<input type="text" placeholder="Value" class="hval">' +
    '<button class="remove-btn" onclick="this.parentElement.remove()">&times;</button>';
  document.getElementById('headers').appendChild(row);
}

async function sendRequest() {
  const btn = document.getElementById('sendBtn');
  btn.disabled = true;
  btn.textContent = 'Sending...';
  document.getElementById('respStatus').textContent = 'Sending...';
  document.getElementById('respStatus').className = 'status-pending';
  document.getElementById('respBody').textContent = '';
  document.getElementById('timing').textContent = '';
  document.getElementById('respTime').textContent = '';

  const method = document.getElementById('method').value;
  const url = document.getElementById('url').value.trim();
  const apiKey = document.getElementById('apiKey').value.trim();
  const bodyText = document.getElementById('body').value.trim();

  if (!url) {
    document.getElementById('respStatus').textContent = 'Error: URL is required';
    document.getElementById('respStatus').className = 'status-err';
    btn.disabled = false;
    btn.textContent = 'Send Request';
    return;
  }

  const headers = {};
  document.querySelectorAll('.header-row').forEach(row => {
    const k = row.querySelector('.hkey').value.trim();
    const v = row.querySelector('.hval').value.trim();
    if (k) headers[k] = v;
  });
  if (apiKey) headers['X-API-Key'] = apiKey;

  const opts = { method, headers };
  if (bodyText && method !== 'GET') {
    opts.body = bodyText;
  }

  const fullUrl = url.startsWith('http') ? url : BASE + url;
  const t0 = performance.now();

  try {
    const res = await fetch(fullUrl, opts);
    const elapsed = Math.round(performance.now() - t0);
    document.getElementById('timing').textContent = elapsed + ' ms';

    const statusEl = document.getElementById('respStatus');
    statusEl.textContent = res.status + ' ' + res.statusText;
    statusEl.className = res.ok ? 'status-ok' : 'status-err';
    document.getElementById('respTime').textContent = elapsed + ' ms';

    const text = await res.text();
    let formatted = text;
    try {
      formatted = JSON.stringify(JSON.parse(text), null, 2);
    } catch (_) { /* not JSON — display raw */ }
    document.getElementById('respBody').textContent = formatted;
  } catch (err) {
    const elapsed = Math.round(performance.now() - t0);
    document.getElementById('timing').textContent = elapsed + ' ms';
    document.getElementById('respStatus').textContent = 'Network Error';
    document.getElementById('respStatus').className = 'status-err';
    document.getElementById('respBody').textContent = err.message;
  } finally {
    btn.disabled = false;
    btn.textContent = 'Send Request';
  }
}
</script>
</body>
</html>`
