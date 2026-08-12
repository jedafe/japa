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

const pixabayAPI = "https://pixabay.com/api"

// PixabayProvider implements StockProvider for Pixabay (pixabay.com/api/docs).
type PixabayProvider struct {
	apiKey  string
	client  *http.Client
}

func NewPixabayProvider(apiKey string) *PixabayProvider {
	return &PixabayProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PixabayProvider) Name() string { return "pixabay" }

func (p *PixabayProvider) Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("key", p.apiKey)
	params.Set("q", query)
	params.Set("page", strconv.Itoa(page))
	params.Set("per_page", strconv.Itoa(perPage))
	params.Set("image_type", "photo")
	params.Set("safesearch", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pixabayAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pixabay request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("pixabay returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Total     int `json:"total"`
		TotalHits int `json:"totalHits"`
		Hits      []struct {
			ID            int    `json:"id"`
			WebformatURL  string `json:"webformatURL"`
			LargeImageURL string `json:"largeImageURL"`
			ImageWidth    int    `json:"imageWidth"`
			ImageHeight   int    `json:"imageHeight"`
			User          string `json:"user"`
			UserImageURL  string `json:"userImageURL"`
			Tags          string `json:"tags"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pixabay response: %w", err)
	}

	var photos []StockPhoto
	for _, r := range raw.Hits {
		photos = append(photos, StockPhoto{
			ID:              strconv.Itoa(r.ID),
			Provider:        "pixabay",
			ThumbnailURL:    r.WebformatURL,
			FullURL:         r.LargeImageURL,
			Width:           r.ImageWidth,
			Height:          r.ImageHeight,
			Photographer:     r.User,
			PhotographerURL:  fmt.Sprintf("https://pixabay.com/users/%s-%d/", r.User, r.ID),
			Description:     r.Tags,
		})
	}

	return &SearchResult{
		Photos:      photos,
		Total:       raw.Total,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: page*perPage < raw.TotalHits,
	}, nil
}

func (p *PixabayProvider) GetByID(ctx context.Context, photoID string) (*StockPhoto, error) {
	params := url.Values{}
	params.Set("key", p.apiKey)
	params.Set("id", photoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pixabayAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building pixabay request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pixabay get photo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pixabay returned %d", resp.StatusCode)
	}

	var raw struct {
		Hits []struct {
			ID            int    `json:"id"`
			WebformatURL  string `json:"webformatURL"`
			LargeImageURL string `json:"largeImageURL"`
			ImageWidth    int    `json:"imageWidth"`
			ImageHeight   int    `json:"imageHeight"`
			User          string `json:"user"`
			UserImageURL  string `json:"userImageURL"`
			Tags          string `json:"tags"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding pixabay photo: %w", err)
	}

	if len(raw.Hits) == 0 {
		return nil, fmt.Errorf("pixabay photo not found: %s", photoID)
	}
	h := raw.Hits[0]

	return &StockPhoto{
		ID:              strconv.Itoa(h.ID),
		Provider:        "pixabay",
		ThumbnailURL:    h.WebformatURL,
		FullURL:         h.LargeImageURL,
		Width:           h.ImageWidth,
		Height:          h.ImageHeight,
		Photographer:     h.User,
		PhotographerURL:  fmt.Sprintf("https://pixabay.com/users/%s-%d/", h.User, h.ID),
		Description:     h.Tags,
	}, nil
}

func (p *PixabayProvider) Download(ctx context.Context, photoID string) (*DownloadedPhoto, error) {
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
		return nil, fmt.Errorf("pixabay download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading photo data: %w", err)
	}

	return &DownloadedPhoto{
		Data:             data,
		Filename:         photoID + ".jpg",
		Photographer:     photo.Photographer,
		PhotographerURL:  photo.PhotographerURL,
		License:          "pixabay",
		LicenseURL:       "https://pixabay.com/service/license/",
		ProviderPhotoID:  photoID,
	}, nil
}
