package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openverseAPI = "https://api.openverse.org/v1"

// OpenverseProvider implements StockProvider for Openverse (api.openverse.org).
// No API key required — this is the zero-friction default provider.
type OpenverseProvider struct {
	client *http.Client
}

// NewOpenverseProvider is an exported function.
func NewOpenverseProvider(timeout time.Duration) *OpenverseProvider {
	return &OpenverseProvider{
		client: &http.Client{Timeout: timeout},
	}
}

// Name is an exported method.
func (p *OpenverseProvider) Name() string { return "openverse" }

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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		openverseAPI+"/images?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		openverseAPI+"/images/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building openverse request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0")

	resp, err := p.client.Do(req)
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
