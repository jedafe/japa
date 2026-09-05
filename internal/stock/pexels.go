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

const pexelsAPI = "https://api.pexels.com/v1"

// PexelsProvider implements StockProvider for Pexels (pexels.com/api).
type PexelsProvider struct {
	apiKey string
	client *http.Client
}

func NewPexelsProvider(apiKey string, timeout time.Duration) *PexelsProvider {
	return &PexelsProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *PexelsProvider) Name() string { return "pexels" }

func (p *PexelsProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pexelsAPI+"/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pexels request: %w", err)
	}

	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("pexels returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		TotalResults int    `json:"total_results"`
		Page         int    `json:"page"`
		PerPage      int    `json:"per_page"`
		NextPage     string `json:"next_page"`
		Photos       []struct {
			ID     int `json:"id"`
			Width  int `json:"width"`
			Height int `json:"height"`
			Src    struct {
				Medium   string `json:"medium"`
				Large    string `json:"large"`
				Original string `json:"original"`
			} `json:"src"`
			Photographer    string `json:"photographer"`
			PhotographerURL string `json:"photographer_url"`
			Alt             string `json:"alt"`
		} `json:"photos"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pexels response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Photos {
		photos = append(photos, StockPhoto{
			ID:              strconv.Itoa(r.ID),
			Provider:        "pexels",
			ThumbnailURL:    r.Src.Medium,
			FullURL:         r.Src.Original,
			Width:           r.Width,
			Height:          r.Height,
			Photographer:    r.Photographer,
			PhotographerURL: r.PhotographerURL,
			Description:     r.Alt,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.TotalResults,
		Page:        raw.Page,
		PerPage:     raw.PerPage,
		HasNextPage: raw.NextPage != "",
	}, nil
}

func (p *PexelsProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pexelsAPI+"/photos/"+photoID, nil)
	if err != nil {
		return nil, fmt.Errorf("building pexels request: %w", err)
	}

	req.Header.Set("Authorization", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels returned %d", resp.StatusCode)
	}

	var raw struct {
		ID     int `json:"id"`
		Width  int `json:"width"`
		Height int `json:"height"`
		Src    struct {
			Medium   string `json:"medium"`
			Large    string `json:"large"`
			Original string `json:"original"`
		} `json:"src"`
		Photographer    string `json:"photographer"`
		PhotographerURL string `json:"photographer_url"`
		Alt             string `json:"alt"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pexels photo: %w", err)
	}

	return &StockPhoto{
		ID:              strconv.Itoa(raw.ID),
		Provider:        "pexels",
		ThumbnailURL:    raw.Src.Medium,
		FullURL:         raw.Src.Original,
		Width:           raw.Width,
		Height:          raw.Height,
		Photographer:    raw.Photographer,
		PhotographerURL: raw.PhotographerURL,
		Description:     raw.Alt,
	}, nil
}

func (p *PexelsProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
	photo, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("pexels download returned %d", resp.StatusCode)
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
		License:         "pexels",
		LicenseURL:      "https://www.pexels.com/license/",
		ProviderPhotoID: photoID,
	}, nil
}
