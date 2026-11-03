package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const openverseAPI = "https://api.openverse.org/v1"

// OpenverseProvider implements StockProvider for Openverse (api.openverse.org).
// Works without credentials, but anonymous calls are throttled hard
// (~200/day per IP) — a shared backend IP exhausts that almost immediately.
// When clientID/clientSecret are set, requests authenticate via the
// client-credentials grant for the much higher authenticated quota.
type OpenverseProvider struct {
	client       *http.Client
	clientID     string
	clientSecret string

	tokenMu      sync.Mutex
	token        string
	tokenExpiry  time.Time
	tokenBackoff time.Time
}

// NewOpenverseProvider is an exported function. clientID/clientSecret may be
// empty — the provider then calls the API anonymously.
func NewOpenverseProvider(timeout time.Duration, clientID, clientSecret string) *OpenverseProvider {
	return &OpenverseProvider{
		client:       &http.Client{Timeout: timeout},
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Name is an exported method.
func (p *OpenverseProvider) Name() string { return "openverse" }

// accessToken returns a Bearer token via the client-credentials grant,
// caching it until shortly before expiry. Returns "" when no credentials
// are configured; a failed token fetch backs off for a minute and falls
// back to anonymous rather than failing the request outright.
func (p *OpenverseProvider) accessToken(ctx context.Context) string {
	if p.clientID == "" || p.clientSecret == "" {
		return ""
	}

	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.token != "" && time.Now().Before(p.tokenExpiry) {
		return p.token
	}

	if time.Now().Before(p.tokenBackoff) {
		return ""
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openverseAPI+"/auth_tokens/token/", strings.NewReader(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		slog.Warn("openverse: token request failed", "error", err)
		p.tokenBackoff = time.Now().Add(time.Minute)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("openverse: token request rejected", "status", resp.StatusCode)
		p.tokenBackoff = time.Now().Add(time.Minute)
		return ""
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		slog.Warn("openverse: malformed token response", "error", err)
		p.tokenBackoff = time.Now().Add(time.Minute)
		return ""
	}

	expiresIn := tok.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	// Refresh a minute early so a borderline-expired token is never sent.
	p.token = tok.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(expiresIn)*time.Second - time.Minute)

	return p.token
}

// newRequest builds an Openverse API request with the app User-Agent and a
// Bearer token when credentials are configured. Only use it for
// api.openverse.org endpoints — never attach the token to third-party
// download URLs.
func (p *OpenverseProvider) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0")

	if tok := p.accessToken(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	return req, nil
}

// doAPI executes an api.openverse.org request. If the API rejects a cached
// token with 401 (e.g. revoked credentials), the token is dropped and the
// request retried once anonymously rather than failing until token expiry.
// GET-only today — the retry replays a nil body, so a caller with a body
// would need its body re-buffered.
func (p *OpenverseProvider) doAPI(req *http.Request) (*http.Response, error) {
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || req.Header.Get("Authorization") == "" {
		return resp, err
	}
	resp.Body.Close()

	p.tokenMu.Lock()
	p.token = ""
	p.tokenExpiry = time.Time{}
	p.tokenBackoff = time.Now().Add(time.Minute) // don't re-fetch a rejected token
	p.tokenMu.Unlock()

	retry, err := p.newRequest(req.Context(), req.Method, req.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	return p.client.Do(retry)
}

// Search is an exported method.
func (p *OpenverseProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	// Openverse API enforces a maximum page_size of 20.
	if perPage > 20 {
		perPage = 20
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("page_size", strconv.Itoa(perPage))
	params.Set("mature", "false")

	// Trailing slash matters: api.openverse.org 301s "/v1/images" to
	// "/v1/images/" — the redirect costs an extra slow round-trip.
	req, err := p.newRequest(ctx, http.MethodGet,
		openverseAPI+"/images/?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}

	resp, err := p.doAPI(req)
	if err != nil {
		return nil, fmt.Errorf("openverse search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("openverse returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		ResultCount int `json:"result_count"`
		PageCount   int `json:"page_count"`
		Results     []struct {
			ID             string `json:"id"`
			Title          string `json:"title"`
			Thumbnail      string `json:"thumbnail"`
			URL            string `json:"url"`
			Width          int    `json:"width"`
			Height         int    `json:"height"`
			License        string `json:"license"`
			LicenseURL     string `json:"license_url"`
			LicenseVersion string `json:"license_version"`
			Creator        string `json:"creator"`
			CreatorURL     string `json:"creator_url"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding openverse response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Results {
		photos = append(photos, StockPhoto{
			ID:              r.ID,
			Provider:        "openverse",
			ThumbnailURL:    r.Thumbnail,
			FullURL:         r.URL,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:    r.Creator,
			PhotographerURL: r.CreatorURL,
			License:         normalizeLicense(r.License),
			LicenseURL:      r.LicenseURL,
			Description:     r.Title,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.ResultCount,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page < raw.PageCount,
	}, nil
}

// GetByID is an exported method.
func (p *OpenverseProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := p.newRequest(ctx, http.MethodGet,
		openverseAPI+"/images/"+url.PathEscape(photoID)+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}

	resp, err := p.doAPI(req)
	if err != nil {
		return nil, fmt.Errorf("openverse get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openverse returned %d", resp.StatusCode)
	}

	var raw struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Thumbnail  string `json:"thumbnail"`
		URL        string `json:"url"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		License    string `json:"license"`
		LicenseURL string `json:"license_url"`
		Creator    string `json:"creator"`
		CreatorURL string `json:"creator_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding openverse photo: %w", err)
	}

	return &StockPhoto{
		ID:              raw.ID,
		Provider:        "openverse",
		ThumbnailURL:    raw.Thumbnail,
		FullURL:         raw.URL,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:    raw.Creator,
		PhotographerURL: raw.CreatorURL,
		License:         normalizeLicense(raw.License),
		LicenseURL:      raw.LicenseURL,
		Description:     raw.Title,
	}, nil
}

// Download is an exported method.
func (p *OpenverseProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	if err := validateDownloadURL(photo.FullURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openverse download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	// Build a safe filename: strip path separators, fallback to photoID
	// if the description is empty.
	filename := strings.ReplaceAll(photo.Description, " ", "_")
	filename = strings.NewReplacer("/", "_", "\\", "_").Replace(filename)
	filename = strings.TrimSpace(filename)

	if filename == "" {
		filename = photoID
	}

	filename += ".jpg"

	return &DownloadedPhoto{
		Data:            data,
		Filename:        filename,
		Photographer:    photo.Photographer,
		PhotographerURL: photo.PhotographerURL,
		License:         photo.License,
		LicenseURL:      photo.LicenseURL,
		ProviderPhotoID: photoID,
	}, nil
}

// normalizeLicense maps Openverse license names to canonical short codes.
func normalizeLicense(raw string) string {
	switch {
	case strings.Contains(strings.ToLower(raw), "cc0"):
		return "cc0"
	case strings.Contains(strings.ToLower(raw), "by-sa"):
		return "cc-by-sa"
	case strings.Contains(strings.ToLower(raw), "by-nd"):
		return "cc-by-nd"
	case strings.Contains(strings.ToLower(raw), "by-nc"):
		return "cc-by-nc"
	case strings.Contains(strings.ToLower(raw), "by"):
		return "cc-by"
	default:
		return raw
	}
}
