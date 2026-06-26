package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeOEmbedResponse is the JSON body returned by httptest mock servers. Each
// field mirrors a key from the real oEmbed spec — see the platform docs.
const fakeOEmbedResponse = `{
	"title": "Test",
	"author_name": "Test Author",
	"html": "<iframe src=\"https://example.com/embed\"></iframe>",
	"thumbnail_url": "https://example.com/thumb.jpg"
}`

// newMockOEmbedServer returns a httptest server that echoes the fake oEmbed
// JSON body for any request. Callers must call Close when finished.
func newMockOEmbedServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck,gosec // test-only response writer.
		_, _ = w.Write([]byte(fakeOEmbedResponse))
	}))
}

// newMockResolver returns an EmbedResolver pointing every oEmbed-backed
// platform at the given mock server. URL-parsed platforms (github, figma,
// google_maps) never hit the network.
func newMockResolver(t *testing.T, mockServer *httptest.Server) *EmbedResolver {
	t.Helper()
	overrides := map[string]string{
		platformYouTube: mockServer.URL,
		platformTwitter: mockServer.URL,
		platformVimeo:   mockServer.URL,
		platformSpotify: mockServer.URL,
		platformCodePen: mockServer.URL,
	}
	return newEmbedResolverWithOverrides(http.DefaultClient, overrides)
}

func TestResolve_UnsupportedPlatform(t *testing.T) {
	r := NewEmbedResolver()
	_, err := r.Resolve(context.Background(), "https://example.com/video", nil)
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestResolve_AllowedPlatformFilter(t *testing.T) {
	r := NewEmbedResolver()
	_, err := r.Resolve(
		context.Background(),
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		[]string{platformVimeo},
	)
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: err = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestResolve_AllowedPlatformMatch(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		[]string{platformYouTube},
	)
	if errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: returned ErrUnsupportedPlatform for allowed platform")
	}
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: returned nil embed for allowed platform")
	}
}

func TestResolveYouTube_VariousURLs(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	tests := []struct {
		name    string
		rawURL  string
		wantID  string
		wantEmb string
	}{
		{
			name:    "watch URL",
			rawURL:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantID:  "dQw4w9WgXcQ",
			wantEmb: "https://www.youtube.com/embed/dQw4w9WgXcQ",
		},
		{
			name:    "youtu.be URL",
			rawURL:  "https://youtu.be/dQw4w9WgXcQ",
			wantID:  "dQw4w9WgXcQ",
			wantEmb: "https://www.youtube.com/embed/dQw4w9WgXcQ",
		},
		{
			name:    "embed URL",
			rawURL:  "https://www.youtube.com/embed/dQw4w9WgXcQ",
			wantID:  "dQw4w9WgXcQ",
			wantEmb: "https://www.youtube.com/embed/dQw4w9WgXcQ",
		},
		{
			name:    "shorts URL",
			rawURL:  "https://www.youtube.com/shorts/dQw4w9WgXcQ",
			wantID:  "dQw4w9WgXcQ",
			wantEmb: "https://www.youtube.com/embed/dQw4w9WgXcQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embed, err := r.Resolve(context.Background(), tt.rawURL, nil)
			if err != nil {
				t.Fatalf("Resolve: unexpected error: %v", err)
			}
			if embed == nil {
				t.Fatal("Resolve: returned nil embed")
			}
			if embed.Platform != platformYouTube {
				t.Errorf("Platform = %q, want %q", embed.Platform, platformYouTube)
			}
			if embed.EmbedURL != tt.wantEmb {
				t.Errorf("EmbedURL = %q, want %q", embed.EmbedURL, tt.wantEmb)
			}
			videoID, _ := embed.Raw["video_id"].(string)
			if videoID != tt.wantID {
				t.Errorf("Raw[video_id] = %q, want %q", videoID, tt.wantID)
			}
			if embed.Title != "Test" {
				t.Errorf("Title = %q, want %q (from mock oEmbed)", embed.Title, "Test")
			}
			if embed.Author != "Test Author" {
				t.Errorf("Author = %q, want %q", embed.Author, "Test Author")
			}
		})
	}
}

func TestResolveYouTube_NoVideoID(t *testing.T) {
	r := NewEmbedResolver()
	_, err := r.Resolve(context.Background(), "https://www.youtube.com/feed", nil)
	if err == nil {
		t.Fatal("Resolve: expected error for YouTube URL without video ID")
	}
	if errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: err is ErrUnsupportedPlatform, want descriptive extract error: %v", err)
	}
}

func TestResolveGitHub_Repo(t *testing.T) {
	r := NewEmbedResolver()

	embed, err := r.Resolve(context.Background(), "https://github.com/jedafe/japa", nil)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformGitHub {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformGitHub)
	}
	if embed.Title != "jedafe/japa" {
		t.Errorf("Title = %q, want %q", embed.Title, "jedafe/japa")
	}
	if got, _ := embed.Raw["type"].(string); got != "repo" {
		t.Errorf("Raw[type] = %q, want %q", got, "repo")
	}
	if got, _ := embed.Raw["owner"].(string); got != "jedafe" {
		t.Errorf("Raw[owner] = %q, want %q", got, "jedafe")
	}
	if got, _ := embed.Raw["repo"].(string); got != "japa" {
		t.Errorf("Raw[repo] = %q, want %q", got, "japa")
	}
}

func TestResolveGitHub_Gist(t *testing.T) {
	r := NewEmbedResolver()

	embed, err := r.Resolve(
		context.Background(),
		"https://gist.github.com/user/abc123",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformGitHub {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformGitHub)
	}
	if got, _ := embed.Raw["type"].(string); got != "gist" {
		t.Errorf("Raw[type] = %q, want %q", got, "gist")
	}
	wantEmbedURL := "https://gist.github.com/user/abc123"
	if embed.EmbedURL != wantEmbedURL {
		t.Errorf("EmbedURL = %q, want %q", embed.EmbedURL, wantEmbedURL)
	}
}

func TestResolveGitHub_GistOnRepoHost(t *testing.T) {
	r := NewEmbedResolver()

	// github.com/{user}/gist/{id} is also a gist URL per the GitHub URL shape
	// introduced with the gist-as-pinned-file flow.
	embed, err := r.Resolve(
		context.Background(),
		"https://github.com/user/gist/abc123",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if got, _ := embed.Raw["type"].(string); got != "gist" {
		t.Errorf("Raw[type] = %q, want %q", got, "gist")
	}
	wantEmbedURL := "https://gist.github.com/user/abc123"
	if embed.EmbedURL != wantEmbedURL {
		t.Errorf("EmbedURL = %q, want %q", embed.EmbedURL, wantEmbedURL)
	}
}

func TestResolveFigma(t *testing.T) {
	r := NewEmbedResolver()

	rawURL := "https://www.figma.com/file/abc123/My-Design"
	embed, err := r.Resolve(context.Background(), rawURL, nil)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformFigma {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformFigma)
	}
	if embed.EmbedURL == "" {
		t.Fatal("EmbedURL empty for figma")
	}
	if !contains(embed.EmbedURL, "embed_host=japaconnect") {
		t.Errorf("EmbedURL = %q, expected embed_host=japaconnect", embed.EmbedURL)
	}

	// Figma escapes the original URL via QueryEscape, so decode the ?url=
	// query parameter before comparing to rawURL.
	parsedEmbed, err := url.Parse(embed.EmbedURL)
	if err != nil {
		t.Fatalf("failed to parse embed URL: %v", err)
	}
	escapedURL := parsedEmbed.Query().Get("url")
	gotURL, err := url.QueryUnescape(escapedURL)
	if err != nil {
		t.Fatalf("failed to decode url= parameter: %v", err)
	}
	if gotURL != rawURL {
		t.Errorf("decoded url= parameter = %q, want %q", gotURL, rawURL)
	}
}

func TestResolveGoogleMaps(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{
			name:   "google.com/maps/place",
			rawURL: "https://www.google.com/maps/place/London",
		},
		{
			name:   "maps.google.com host",
			rawURL: "https://maps.google.com/maps?q=London",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewEmbedResolver()
			embed, err := r.Resolve(context.Background(), tt.rawURL, nil)
			if err != nil {
				t.Fatalf("Resolve: unexpected error: %v", err)
			}
			if embed == nil {
				t.Fatal("Resolve: nil embed")
			}
			if embed.Platform != platformGoogleMaps {
				t.Errorf("Platform = %q, want %q", embed.Platform, platformGoogleMaps)
			}
			if embed.EmbedURL != tt.rawURL {
				t.Errorf("EmbedURL = %q, want %q", embed.EmbedURL, tt.rawURL)
			}
		})
	}
}

func TestResolveTwitter_OEmbed(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://twitter.com/jedafe/status/123456789",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformTwitter {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformTwitter)
	}
	if embed.Title != "Test" {
		t.Errorf("Title = %q, want %q", embed.Title, "Test")
	}
	if embed.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", embed.Author, "Test Author")
	}
	if embed.HTML == "" {
		t.Error("HTML empty; mock oEmbed should have set it")
	}
	if !contains(embed.HTML, "iframe") {
		t.Errorf("HTML = %q, expected to contain iframe", embed.HTML)
	}
	if embed.ThumbnailURL != "https://example.com/thumb.jpg" {
		t.Errorf("ThumbnailURL = %q, want %q",
			embed.ThumbnailURL, "https://example.com/thumb.jpg")
	}
}

func TestResolveVimeo_OEmbed(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://vimeo.com/123456789",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformVimeo {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformVimeo)
	}
	if embed.Title != "Test" {
		t.Errorf("Title = %q, want %q", embed.Title, "Test")
	}
	// Also validate that the mock query actually included the raw URL.
}

func TestResolveSpotify_OEmbed(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://open.spotify.com/track/123abc",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformSpotify {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformSpotify)
	}
	if embed.Title != "Test" {
		t.Errorf("Title = %q, want %q", embed.Title, "Test")
	}
}

func TestResolveCodePen_OEmbed(t *testing.T) {
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://codepen.io/jedafe/pen/abc123",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformCodePen {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformCodePen)
	}
	if embed.Title != "Test" {
		t.Errorf("Title = %q, want %q", embed.Title, "Test")
	}
}

func TestResolve_XHostAlias(t *testing.T) {
	// x.com should be treated the same as twitter.com.
	mockSrv := newMockOEmbedServer(t)
	t.Cleanup(mockSrv.Close)
	r := newMockResolver(t, mockSrv)

	embed, err := r.Resolve(
		context.Background(),
		"https://x.com/jedafe/status/123456789",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil {
		t.Fatal("Resolve: nil embed")
	}
	if embed.Platform != platformTwitter {
		t.Errorf("Platform = %q, want %q", embed.Platform, platformTwitter)
	}
}

func TestResolve_OEmbedURLPassed(t *testing.T) {
	// Verify that the requested oEmbed URL carries the raw URL as ?url= so
	// upstream providers receive the canonical target.
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got == "" {
			t.Errorf("oEmbed request %q missing ?url=", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck // test-only response writer.
		_, _ = w.Write([]byte(fakeOEmbedResponse))
	}))
	t.Cleanup(mockSrv.Close)

	r := newMockResolver(t, mockSrv)
	if _, err := r.Resolve(
		context.Background(),
		"https://vimeo.com/123456789",
		nil,
	); err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
}

func TestResolve_InvalidURL(t *testing.T) {
	r := NewEmbedResolver()
	_, err := r.Resolve(context.Background(), "http://[invalid", nil)
	if err == nil {
		t.Fatal("Resolve: expected error for unparseable URL")
	}
	if errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: returned ErrUnsupportedPlatform, want parse error: %v", err)
	}
}

func TestResolve_OEmbedServerError(t *testing.T) {
	// A non-2xx oEmbed response must produce a hard error for platforms like
	// Twitter whose entire representation comes from oEmbed.
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(mockSrv.Close)

	r := newMockResolver(t, mockSrv)
	_, err := r.Resolve(
		context.Background(),
		"https://twitter.com/jedafe/status/123456789",
		nil,
	)
	if err == nil {
		t.Fatal("Resolve: expected error when oEmbed fails for Twitter")
	}
	if errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Resolve: err is ErrUnsupportedPlatform, want oEmbed failure: %v", err)
	}
}

func TestResolve_FigmaHasNoNetworkCall(t *testing.T) {
	// Point every oEmbed endpoint at a server that fails any request; figma
	// must still resolve because it does not make HTTP calls.
	badSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("figma resolution made an unexpected HTTP call")
	}))
	t.Cleanup(badSrv.Close)

	r := newMockResolver(t, badSrv)
	embed, err := r.Resolve(
		context.Background(),
		"https://www.figma.com/file/abc123/Design",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil || embed.Platform != platformFigma {
		t.Fatalf("Resolve: expected figma embed, got %+v", embed)
	}
}

func TestResolve_GitHubHasNoNetworkCall(t *testing.T) {
	// Same reasoning as figma: GitHub is resolved from URL parsing.
	badSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("github resolution made an unexpected HTTP call")
	}))
	t.Cleanup(badSrv.Close)

	r := newMockResolver(t, badSrv)
	embed, err := r.Resolve(
		context.Background(),
		"https://github.com/jedafe/japa",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil || embed.Platform != platformGitHub {
		t.Fatalf("Resolve: expected github embed, got %+v", embed)
	}
}

func TestResolve_GoogleMapsHasNoNetworkCall(t *testing.T) {
	// Google Maps resolution stores the raw URL only.
	badSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("google maps resolution made an unexpected HTTP call")
	}))
	t.Cleanup(badSrv.Close)

	r := newMockResolver(t, badSrv)
	embed, err := r.Resolve(
		context.Background(),
		"https://maps.google.com/maps?q=London",
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if embed == nil || embed.Platform != platformGoogleMaps {
		t.Fatalf("Resolve: expected google_maps embed, got %+v", embed)
	}
}

func TestIdentifyPlatform(t *testing.T) {
	tests := []struct {
		host, path string
		want       string
	}{
		{"youtube.com", "/watch", platformYouTube},
		{"youtu.be", "/abc", platformYouTube},
		{"www.youtube.com", "/", ""}, // www prefix NOT handled here; Resolve strips it
		{"github.com", "/jedafe/japa", platformGitHub},
		{"gist.github.com", "/user/abc", platformGitHub},
		{"twitter.com", "/jedafe/status/1", platformTwitter},
		{"x.com", "/jedafe/status/1", platformTwitter},
		{"vimeo.com", "/123", platformVimeo},
		{"open.spotify.com", "/track/abc", platformSpotify},
		{"figma.com", "/file/abc", platformFigma},
		{"codepen.io", "/jedafe/pen/abc", platformCodePen},
		{"maps.google.com", "/maps", platformGoogleMaps},
		{"google.com", "/maps/place/London", platformGoogleMaps},
		{"google.com", "/search?q=foo", ""},
		{"example.com", "/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host+tt.path, func(t *testing.T) {
			got := identifyPlatform(tt.host, tt.path)
			if got != tt.want {
				t.Errorf("identifyPlatform(%q, %q) = %q, want %q",
					tt.host, tt.path, got, tt.want)
			}
		})
	}
}

func TestPlatformAllowed(t *testing.T) {
	tests := []struct {
		name    string
		platform string
		allowed []string
		want    bool
	}{
		{"empty allowed accepts all", platformYouTube, nil, true},
		{"empty allowed with literal slice", platformYouTube, []string{}, true},
		{"youtube in [youtube]", platformYouTube, []string{platformYouTube}, true},
		{"youtube not in [vimeo]", platformYouTube, []string{platformVimeo}, false},
		{"youtube in multi-allow", platformYouTube,
			[]string{platformVimeo, platformYouTube, platformTwitter}, true},
		{"github not in [vimeo]", platformGitHub, []string{platformVimeo}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := platformAllowed(tt.platform, tt.allowed)
			if got != tt.want {
				t.Errorf("platformAllowed(%q, %v) = %v, want %v",
					tt.platform, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestExtractYouTubeID(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{"watch", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"youtu.be", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"embed", "https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"shorts", "https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"watch with extra params",
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=12s&feature=share",
			"dQw4w9WgXcQ"},
		{"no id", "https://www.youtube.com/feed", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := parseURL(t, tt.rawURL)
			got := extractYouTubeID(u)
			if got != tt.want {
				t.Errorf("extractYouTubeID(%q) = %q, want %q",
					tt.rawURL, got, tt.want)
			}
		})
	}
}

// parseURL is a helper that fails the test when the URL is not parseable.
func parseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}

func TestFetchOEmbed_DecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck // test-only response writer.
		_, _ = w.Write([]byte(fakeOEmbedResponse))
	}))
	t.Cleanup(srv.Close)

	r := &EmbedResolver{client: http.DefaultClient}
	got, err := r.fetchOEmbed(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchOEmbed: %v", err)
	}
	if got == nil {
		t.Fatal("fetchOEmbed: returned nil map for valid response")
	}
	if got["title"] != "Test" {
		t.Errorf("title = %v, want %q", got["title"], "Test")
	}
}

func TestNewEmbedResolver_DefaultClientTimeout(t *testing.T) {
	r := NewEmbedResolver()
	if r == nil {
		t.Fatal("NewEmbedResolver returned nil")
	}
	if r.client == nil {
		t.Fatal("NewEmbedResolver: client is nil")
	}
	if r.client.Timeout <= 0 {
		t.Errorf("client timeout = %v, want positive", r.client.Timeout)
	}
	// ensure overrides start nil so production calls use default endpoints
	if r.oembedBaseURLs != nil {
		t.Errorf("NewEmbedResolver should not set oembedBaseURLs, got %v",
			r.oembedBaseURLs)
	}
}

func TestEmbedData_NonNilRaw(t *testing.T) {
	// Every resolver path must initialize Raw; verify by exercising several
	// no-network paths. Use httptest server that fails on call to confirm no
	// network traffic.
	badSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP call")
	}))
	t.Cleanup(badSrv.Close)

	r := newMockResolver(t, badSrv)

	cases := []string{
		"https://github.com/jedafe/japa",
		"https://gist.github.com/u/abc",
		"https://www.figma.com/file/x/y",
		"https://maps.google.com/maps?q=x",
	}
	for _, raw := range cases {
		embed, err := r.Resolve(context.Background(), raw, nil)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", raw, err)
		}
		if embed.Raw == nil {
			t.Errorf("Raw must never be nil: %s", raw)
		}
	}
}

// Ensure the mock helper can build a map[string]any response equivalent to the
// fake oEmbed body — sanity check on JSON unmarshaling parity.
func TestFakeOEmbedResponse_MatchesMap(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal([]byte(fakeOEmbedResponse), &m); err != nil {
		t.Fatalf("invalid mock JSON: %v", err)
	}
	if m["title"] != "Test" {
		t.Errorf("title = %v, want Test", m["title"])
	}
}

// contains is a tiny local helper to avoid pulling strings.Contains into tests
// (keeps the test file focused on behaviour, not stdlib imports).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}