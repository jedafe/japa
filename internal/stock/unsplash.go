package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const unsplashAPI = "https://api.unsplash.com"

// UnsplashProvider implements StockProvider for Unsplash (unsplash.com/developers).
type UnsplashProvider struct {
	accessKey string
	client    *http.Client
}

// NewUnsplashProvider creates an Unsplash provider with the given access key
// and HTTP client timeout.
func NewUnsplashProvider(accessKey string, timeout time.Duration) *UnsplashProvider {
	return &UnsplashProvider{
		accessKey: accessKey,
		client:    &http.Client{Timeout: timeout},
	}
}

func (p *UnsplashProvider) Name() string { return "unsplash" }

func (p *UnsplashProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		unsplashAPI+"/search/photos?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building unsplash request: %w", err)
	}

	req.Header.Set("Authorization", "Client-ID "+p.accessKey)
	req.Header.Set("Accept-Version", "v1")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("unsplash returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
		Results    []struct {
			ID     string `json:"id"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Urls   struct {
				Thumb string `json:"thumb"`
				Full  string `json:"full"`
				Raw   string `json:"raw"`
			} `json:"urls"`
			Description *string `json:"description"`
			User        struct {
				Name     string `json:"name"`
				Username string `json:"username"`
				Links    struct {
					HTML string `json:"html"`
				} `json:"links"`
			} `json:"user"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding unsplash response: %w", err)
	}

	var photos []StockPhoto

	for _, r := range raw.Results {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}

		photos = append(photos, StockPhoto{
			ID:              r.ID,
			Provider:        "unsplash",
			ThumbnailURL:    r.Urls.Thumb,
			FullURL:         r.Urls.Full,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:    r.User.Name,
			PhotographerURL: r.User.Links.HTML + "?utm_source=japaconnect&utm_medium=referral",
			Description:     desc,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.Total,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page < raw.TotalPages,
	}, nil
}

func (p *UnsplashProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		unsplashAPI+"/photos/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building unsplash request: %w", err)
	}

	req.Header.Set("Authorization", "Client-ID "+p.accessKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unsplash get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsplash returned %d", resp.StatusCode)
	}

	var raw struct {
		ID     string `json:"id"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Urls   struct {
			Thumb string `json:"thumb"`
			Full  string `json:"full"`
			Raw   string `json:"raw"`
		} `json:"urls"`
		Description *string `json:"description"`
		User        struct {
			Name     string `json:"name"`
			Username string `json:"username"`
			Links    struct {
				HTML string `json:"html"`
			} `json:"links"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding unsplash photo: %w", err)
	}

	desc := ""
	if raw.Description != nil {
		desc = *raw.Description
	}

	return &StockPhoto{
		ID:              raw.ID,
		Provider:        "unsplash",
		ThumbnailURL:    raw.Urls.Thumb,
		FullURL:         raw.Urls.Full,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:    raw.User.Name,
		PhotographerURL: raw.User.Links.HTML + "?utm_source=japaconnect&utm_medium=referral",
		Description:     desc,
	}, nil
}

func (p *UnsplashProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

	// Unsplash requires a download tracking call per API guidelines.
	// Fire-and-forget, but with a bounded timeout, the API key in the
	// Authorization header (not the URL), and proper response-body cleanup.
	go func() {
		trackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(trackCtx, http.MethodGet,
			unsplashAPI+"/photos/"+photoID+"/download", nil)
		if err != nil {
			return
		}

		req.Header.Set("Authorization", "Client-ID "+p.accessKey)

		resp, err := p.client.Do(req)
		if err != nil {
			return
		}

		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photo.FullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:            data,
		Filename:        photoID + ".jpg",
		Photographer:    photo.Photographer,
		PhotographerURL: photo.PhotographerURL,
		License:         "unsplash",
		LicenseURL:      "https://unsplash.com/license",
		ProviderPhotoID: photoID,
	}, nil
}
