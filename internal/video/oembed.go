package video

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/jedafe/japa/internal/linkpreview"
)

// instagramTokenCtxKey is used to pass an optional Instagram access token
// through the request context without changing the Resolver interface.
type instagramTokenCtxKey struct{}

func withInstagramToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, instagramTokenCtxKey{}, token)
}

func instagramTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(instagramTokenCtxKey{}).(string)

	return token
}

// oembedProvider resolves video URLs for platforms that expose an oEmbed
// endpoint (TikTok, Instagram Reels, etc.).
type oembedProvider struct {
	platform    string
	oembedURL   string
	client      httpClient
	ssrfEnabled bool
	extractID   func(*url.URL, map[string]any) string
}

// newOEmbedProvider returns a generic oEmbed-backed provider.
func newOEmbedProvider(platform, oembedURL string, extractID func(*url.URL, map[string]any) string) *oembedProvider {
	return &oembedProvider{
		platform:    platform,
		oembedURL:   oembedURL,
		client:      newSSRFSafeClient(),
		ssrfEnabled: true,
		extractID:   extractID,
	}
}

// Platform returns the platform key.
func (p *oembedProvider) Platform() string { return p.platform }

// Resolve fetches oEmbed metadata and builds the canonical Metadata.
func (p *oembedProvider) Resolve(ctx context.Context, rawURL string) (*Metadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVideoURL, err)
	}

	oembedURL := fmt.Sprintf("%s?url=%s", p.oembedURL, url.QueryEscape(rawURL))

	if p.platform == SourceInstagramReel {
		if token := instagramTokenFromContext(ctx); token != "" {
			oembedURL += "&access_token=" + url.QueryEscape(token)
		}
	}

	data, err := p.fetchOEmbed(ctx, oembedURL)
	if err != nil {
		return nil, err
	}

	videoID := p.extractID(u, data)
	if videoID == "" {
		return nil, ErrNoYouTubeID
	}

	title, _ := data["title"].(string)
	authorName, _ := data["author_name"].(string)
	authorURL, _ := data["author_url"].(string)
	thumbnailURL, _ := data["thumbnail_url"].(string)
	embedHTML, _ := data["html"].(string)

	canonicalURL, _ := data["url"].(string)
	if canonicalURL == "" {
		canonicalURL = rawURL
	}

	channelName := authorName

	if authorURL != "" {
		if au, err := url.Parse(authorURL); err == nil && au.Path != "" {
			channelName = strings.TrimPrefix(path.Base(strings.TrimSuffix(au.Path, "/")), "@")
		}
	}

	return &Metadata{
		SourceType:   p.platform,
		SourceURL:    canonicalURL,
		ExternalID:   videoID,
		Title:        html.UnescapeString(title),
		Description:  html.UnescapeString(title),
		ThumbnailURL: thumbnailURL,
		DurationSecs: oembedDuration(data),
		EmbedHTML:    embedHTML,
		AuthorName:   authorName,
		ChannelName:  channelName,
		ChannelURL:   authorURL,
		AuthorAvatar: thumbnailURL,
	}, nil
}

// fetchOEmbed performs a GET request to the oEmbed endpoint and decodes JSON.
func (p *oembedProvider) fetchOEmbed(ctx context.Context, u string) (map[string]any, error) {
	return doOEmbedRequest(ctx, p.client, u, p.ssrfEnabled, "")
}

// doOEmbedRequest performs an SSRF-safe GET to an oEmbed endpoint and decodes JSON.
func doOEmbedRequest(ctx context.Context, client httpClient, u string, ssrfEnabled bool, statusPrefix string) (map[string]any, error) {
	if ssrfEnabled {
		if err := linkpreview.ValidateURL(u); err != nil {
			return nil, fmt.Errorf("ssrf validation failed: %w", err)
		}
	}
	// #nosec G704 — u is the oEmbed endpoint plus a URL-escaped user URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%soembed returned status %d", statusPrefix, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, 1<<20)

	var data map[string]any

	dec := json.NewDecoder(limited)
	dec.UseNumber()

	if err := dec.Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}

// oembedDuration extracts duration in seconds when the provider includes it.
func oembedDuration(data map[string]any) int {
	duration, _ := data["duration"].(float64)

	if d, ok := data["duration"].(json.Number); ok {
		if n, err := d.Int64(); err == nil {
			duration = float64(n)
		}
	}

	return int(duration)
}

// extractTikTokID pulls the numeric video ID from a TikTok URL path.
func extractTikTokID(u *url.URL, _ map[string]any) string {
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		if s := segments[i]; s != "" {
			if _, err := strconv.ParseInt(s, 10, 64); err == nil {
				return s
			}
		}
	}

	return path.Base(strings.TrimSuffix(u.Path, "/"))
}

// extractInstagramID pulls the shortcode from an Instagram post/reel URL path.
func extractInstagramID(u *url.URL, _ map[string]any) string {
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := range segments {
		if segments[i] == "p" || segments[i] == "reel" || segments[i] == "reels" {
			if i+1 < len(segments) {
				return segments[i+1]
			}
		}
	}

	return path.Base(strings.TrimSuffix(u.Path, "/"))
}

// NewTikTokProvider returns a TikTok oEmbed resolver.
func NewTikTokProvider() Provider {
	return newOEmbedProvider(SourceTikTok, "https://www.tiktok.com/oembed", extractTikTokID)
}

// NewInstagramProvider returns an Instagram oEmbed resolver for Reels and posts.
func NewInstagramProvider() Provider {
	return newOEmbedProvider(SourceInstagramReel, "https://graph.facebook.com/v25.0/instagram_oembed", extractInstagramID)
}
