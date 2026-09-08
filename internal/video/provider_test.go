package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYouTubeProviderResolve(t *testing.T) {
	t.Parallel()

	mock := newMockYouTubeServer(t)
	t.Cleanup(mock.Close)

	provider := NewYouTubeProvider(
		WithHTTPClient(http.DefaultClient),
		WithOEmbedBaseURL(mock.oembedURL),
		WithDataAPIBaseURL(mock.dataURL),
		WithAPIKey("test-key"),
		WithSSRFForTesting(true),
	)

	tests := []struct {
		name      string
		url       string
		wantTitle string
		wantVideo string
		wantChan  string
		wantCat   string
		wantDur   int
	}{
		{
			name:      "watch url",
			url:       "https://www.youtube.com/watch?v=ABC123XYZ",
			wantTitle: "Mock Video",
			wantVideo: "ABC123XYZ",
			wantChan:  "Mock Channel",
			wantCat:   "22",
			wantDur:   305,
		},
		{
			name:      "short url",
			url:       "https://youtu.be/ABC123XYZ",
			wantTitle: "Mock Video",
			wantVideo: "ABC123XYZ",
			wantChan:  "Mock Channel",
			wantCat:   "22",
			wantDur:   305,
		},
		{
			name:      "shorts url",
			url:       "https://www.youtube.com/shorts/ABC123XYZ",
			wantTitle: "Mock Video",
			wantVideo: "ABC123XYZ",
			wantChan:  "Mock Channel",
			wantCat:   "22",
			wantDur:   305,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := provider.Resolve(context.Background(), tt.url)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if meta.SourceType != SourceYouTube {
				t.Errorf("SourceType = %q, want %q", meta.SourceType, SourceYouTube)
			}
			if meta.ExternalID != tt.wantVideo {
				t.Errorf("ExternalID = %q, want %q", meta.ExternalID, tt.wantVideo)
			}
			if meta.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", meta.Title, tt.wantTitle)
			}
			if meta.ChannelName != tt.wantChan {
				t.Errorf("ChannelName = %q, want %q", meta.ChannelName, tt.wantChan)
			}
			if meta.Category != tt.wantCat {
				t.Errorf("Category = %q, want %q", meta.Category, tt.wantCat)
			}
			if meta.DurationSecs != tt.wantDur {
				t.Errorf("DurationSecs = %d, want %d", meta.DurationSecs, tt.wantDur)
			}
			if meta.ChannelURL == "" {
				t.Error("ChannelURL is empty")
			}
		})
	}
}

func TestYouTubeProviderResolve_NoVideoID(t *testing.T) {
	t.Parallel()

	p := NewYouTubeProvider()
	_, err := p.Resolve(context.Background(), "https://www.youtube.com/")
	if !errors.Is(err, ErrNoYouTubeID) {
		t.Fatalf("Resolve: err = %v, want ErrNoYouTubeID", err)
	}
}

func TestResolver_Unsupported(t *testing.T) {
	t.Parallel()

	r := NewDefaultResolver()
	_, err := r.Resolve(context.Background(), "https://example.com/video")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestResolver_Valid(t *testing.T) {
	t.Parallel()

	mock := newMockYouTubeServer(t)
	t.Cleanup(mock.Close)

	p := NewYouTubeProvider(
		WithHTTPClient(http.DefaultClient),
		WithOEmbedBaseURL(mock.oembedURL),
		WithSSRFForTesting(true),
	)

	r := NewResolver(p)
	meta, err := r.Resolve(context.Background(), "https://www.youtube.com/watch?v=ABC123XYZ")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if meta.ExternalID != "ABC123XYZ" {
		t.Errorf("ExternalID = %q, want ABC123XYZ", meta.ExternalID)
	}
}

// mockYouTubeServer hosts the oEmbed and Data API endpoints used by tests.
type mockYouTubeServer struct {
	*httptest.Server
	oembedURL string
	dataURL   string
}

func newMockYouTubeServer(t *testing.T) *mockYouTubeServer {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/oembed" {
			resp := map[string]any{
				"title":        "Mock Video",
				"author_name":  "Mock Channel",
				"author_url":   "https://www.youtube.com/@mockchannel",
				"thumbnail_url": "https://example.com/thumb.jpg",
				"html":         `<iframe src="https://www.youtube.com/embed/ABC123XYZ"></iframe>`,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/videos" {
			resp := map[string]any{
				"items": []any{
					map[string]any{
						"snippet": map[string]any{
							"title":        "Mock Video",
							"description":  "Mock description",
							"channelId":    "CHANNEL123",
							"channelTitle": "Mock Channel",
							"categoryId":   "22",
							"thumbnails": map[string]any{
								"default": map[string]any{"url": "https://example.com/default.jpg"},
							},
						},
						"contentDetails": map[string]any{
							"duration": "PT5M5S",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/channels" {
			resp := map[string]any{
				"items": []any{
					map[string]any{
						"snippet": map[string]any{
							"description": "A channel for tests",
							"country":     "US",
							"thumbnails": map[string]any{
								"default": map[string]any{"url": "https://example.com/channel-logo.jpg"},
							},
						},
						"brandingSettings": map[string]any{
							"image": map[string]any{
								"bannerExternalUrl": "https://example.com/channel-banner.jpg",
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	return &mockYouTubeServer{
		Server:    srv,
		oembedURL: srv.URL + "/oembed",
		dataURL:   srv.URL,
	}
}

func TestChannelKey(t *testing.T) {
	t.Parallel()

	m := &VideoMetadata{SourceType: SourceYouTube, ChannelName: "Awesome Channel!"}
	want := fmt.Sprintf("%s:awesome-channel", SourceYouTube)
	if got := m.ChannelKey(); got != want {
		t.Errorf("ChannelKey() = %q, want %q", got, want)
	}
}
