package video

import (
	"encoding/json"
	"errors"
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
			t.Parallel()

			meta, err := provider.Resolve(t.Context(), tt.url)
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

	_, err := p.Resolve(t.Context(), "https://www.youtube.com/")
	if !errors.Is(err, ErrNoYouTubeID) {
		t.Fatalf("Resolve: err = %v, want ErrNoYouTubeID", err)
	}
}

func TestResolver_Unsupported(t *testing.T) {
	t.Parallel()

	r := NewDefaultResolver()

	_, err := r.Resolve(t.Context(), "https://example.com/video")
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

	meta, err := r.Resolve(t.Context(), "https://www.youtube.com/watch?v=ABC123XYZ")
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
				"title":         "Mock Video",
				"author_name":   "Mock Channel",
				"author_url":    "https://www.youtube.com/@mockchannel",
				"thumbnail_url": "https://example.com/thumb.jpg",
				"html":          `<iframe src="https://www.youtube.com/embed/ABC123XYZ"></iframe>`,
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

	m := &Metadata{SourceType: SourceYouTube, ChannelName: "Awesome Channel!"}
	want := SourceYouTube + ":awesome-channel"

	if got := m.ChannelKey(); got != want {
		t.Errorf("ChannelKey() = %q, want %q", got, want)
	}
}

func TestOEmbedProviderResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		provider    Provider
		endpoint    string
		rawURL      string
		wantSource  string
		wantID      string
		wantTitle   string
		wantChannel string
	}{
		{
			name:        "TikTok",
			provider:    NewTikTokProvider(),
			endpoint:    "/tiktok",
			rawURL:      "https://www.tiktok.com/@scout2015/video/6718335390845095173",
			wantSource:  SourceTikTok,
			wantID:      "6718335390845095173",
			wantTitle:   "Mock TikTok",
			wantChannel: "scout2015",
		},
		{
			name:        "Instagram Reel",
			provider:    NewInstagramProvider(),
			endpoint:    "/instagram",
			rawURL:      "https://www.instagram.com/reel/DFT12345abc",
			wantSource:  SourceInstagramReel,
			wantID:      "DFT12345abc",
			wantTitle:   "Mock Instagram",
			wantChannel: "mockuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newMockOEmbedServer(t)
			provider := mustOEmbedProvider(t, tt.provider)
			provider.client = srv.Client()
			provider.ssrfEnabled = false
			provider.oembedURL = srv.URL + tt.endpoint

			meta, err := provider.Resolve(t.Context(), tt.rawURL)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			if meta.SourceType != tt.wantSource {
				t.Errorf("SourceType = %q, want %q", meta.SourceType, tt.wantSource)
			}

			if meta.ExternalID != tt.wantID {
				t.Errorf("ExternalID = %q, want %q", meta.ExternalID, tt.wantID)
			}

			if meta.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", meta.Title, tt.wantTitle)
			}

			if meta.ChannelName != tt.wantChannel {
				t.Errorf("ChannelName = %q, want %q", meta.ChannelName, tt.wantChannel)
			}
		})
	}
}

func TestResolver_TikTokAndInstagram(t *testing.T) {
	t.Parallel()

	srv := newMockOEmbedServer(t)

	tiktokProvider := mustOEmbedProvider(t, NewTikTokProvider())
	tiktokProvider.client = srv.Client()
	tiktokProvider.ssrfEnabled = false
	tiktokProvider.oembedURL = srv.URL + "/tiktok"

	instagramProvider := mustOEmbedProvider(t, NewInstagramProvider())
	instagramProvider.client = srv.Client()
	instagramProvider.ssrfEnabled = false
	instagramProvider.oembedURL = srv.URL + "/instagram"

	r := NewResolver(tiktokProvider, instagramProvider)

	meta, err := r.Resolve(t.Context(), "https://www.tiktok.com/@scout2015/video/6718335390845095173")
	if err != nil {
		t.Fatalf("Resolve tiktok: %v", err)
	}

	if meta.ExternalID != "6718335390845095173" {
		t.Errorf("TikTok ExternalID = %q, want %q", meta.ExternalID, "6718335390845095173")
	}

	meta, err = r.Resolve(t.Context(), "https://www.instagram.com/reel/DFT12345abc")
	if err != nil {
		t.Fatalf("Resolve instagram: %v", err)
	}

	if meta.ExternalID != "DFT12345abc" {
		t.Errorf("Instagram ExternalID = %q, want %q", meta.ExternalID, "DFT12345abc")
	}
}

func TestInstagramProviderAccessToken(t *testing.T) {
	t.Parallel()

	var gotToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("access_token")

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"title":         "Mock Reel",
			"author_name":   "u",
			"author_url":    "https://www.instagram.com/u",
			"thumbnail_url": "https://example.com/t.jpg",
			"html":          `<blockquote cite="https://www.instagram.com/reel/ABC"></blockquote>`,
		})
	}))

	provider := mustOEmbedProvider(t, NewInstagramProvider())
	provider.client = srv.Client()
	provider.ssrfEnabled = false
	provider.oembedURL = srv.URL

	_, err := provider.Resolve(withInstagramToken(t.Context(), "appid|secret"), "https://www.instagram.com/reel/ABC")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if gotToken != "appid|secret" {
		t.Errorf("access_token = %q, want %q", gotToken, "appid|secret")
	}
}

// mustOEmbedProvider type-asserts a Provider to *oembedProvider for tests.
func mustOEmbedProvider(t *testing.T, p Provider) *oembedProvider {
	t.Helper()

	op, ok := p.(*oembedProvider)
	if !ok {
		t.Fatalf("provider is %T, want *oembedProvider", p)
	}

	return op
}

func newMockOEmbedServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Determine response by the requested URL path; TikTok and Instagram
		// oEmbed requests are identical in shape, so we reuse the same handler.
		var resp map[string]any

		switch r.URL.Path {
		case "/tiktok":
			resp = map[string]any{
				"title":         "Mock TikTok",
				"author_name":   "Scout & Suki",
				"author_url":    "https://www.tiktok.com/@scout2015",
				"thumbnail_url": "https://example.com/tiktok-thumb.jpg",
				"html":          `<blockquote cite="https://www.tiktok.com/@scout2015/video/6718335390845095173"></blockquote>`,
			}
		case "/instagram":
			resp = map[string]any{
				"title":         "Mock Instagram",
				"author_name":   "mockuser",
				"author_url":    "https://www.instagram.com/mockuser",
				"thumbnail_url": "https://example.com/ig-thumb.jpg",
				"html":          `<blockquote cite="https://www.instagram.com/reel/DFT12345abc"></blockquote>`,
			}
		}

		_ = json.NewEncoder(w).Encode(resp)
	}))
}
