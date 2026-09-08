package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/linkpreview"
)

const (
	youtubeOEmbedDefaultBase = "https://www.youtube.com/oembed"
	youtubeDataAPIDefault    = "https://www.googleapis.com/youtube/v3"
)

// ErrNoYouTubeID is returned when a YouTube URL cannot be parsed into a video
// identifier.
var ErrNoYouTubeID = errors.New("no YouTube video ID found in URL")

// httpClient is the minimal HTTP interface used by the provider.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// YouTubeProvider resolves YouTube URLs to VideoMetadata.
type YouTubeProvider struct {
	client         httpClient
	oembedBaseURL  string
	dataAPIBaseURL string
	apiKey         string
	ssrfEnabled    bool
}

// ProviderOption configures a YouTubeProvider.
type ProviderOption func(*YouTubeProvider)

// NewYouTubeProvider returns a production-ready YouTube resolver.
func NewYouTubeProvider(opts ...ProviderOption) *YouTubeProvider {
	p := &YouTubeProvider{
		client:         newSSRFSafeClient(),
		oembedBaseURL:  youtubeOEmbedDefaultBase,
		dataAPIBaseURL: youtubeDataAPIDefault,
		ssrfEnabled:    true,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(c httpClient) ProviderOption {
	return func(p *YouTubeProvider) { p.client = c }
}

// WithOEmbedBaseURL overrides the oEmbed endpoint base URL.
func WithOEmbedBaseURL(u string) ProviderOption {
	return func(p *YouTubeProvider) { p.oembedBaseURL = u }
}

// WithDataAPIBaseURL overrides the YouTube Data API base URL.
func WithDataAPIBaseURL(u string) ProviderOption {
	return func(p *YouTubeProvider) { p.dataAPIBaseURL = u }
}

// WithAPIKey enables the YouTube Data API v3 fallback for category and
// channel details.
func WithAPIKey(key string) ProviderOption {
	return func(p *YouTubeProvider) { p.apiKey = key }
}

// WithSSRFForTesting disables SSRF protection so local httptest servers can
// be used. It must never be used in production.
func WithSSRFForTesting(disabled bool) ProviderOption {
	return func(p *YouTubeProvider) { p.ssrfEnabled = !disabled }
}

// Platform returns the platform key.
func (p *YouTubeProvider) Platform() string { return SourceYouTube }

// Resolve parses a YouTube URL and resolves metadata first from oEmbed, then
// optionally from the YouTube Data API v3 when an API key is configured.
func (p *YouTubeProvider) Resolve(ctx context.Context, rawURL string) (*VideoMetadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidVideoURL, err)
	}

	videoID := extractYouTubeID(u)
	if videoID == "" {
		return nil, ErrNoYouTubeID
	}

	canonicalURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	meta := &VideoMetadata{
		SourceType:   SourceYouTube,
		SourceURL:    canonicalURL,
		ExternalID:   videoID,
		ChannelName:  "",
		ChannelURL:   "",
		ChannelLogo:  "",
		ChannelBanner: "",
		AuthorName:   "",
		AuthorHandle: "",
		AuthorAvatar: "",
		AuthorDesc:   "",
		EmbedHTML:    fmt.Sprintf(`<iframe width="560" height="315" src="https://www.youtube.com/embed/%s" frameborder="0" allowfullscreen></iframe>`, videoID),
	}

	if err := p.resolveOEmbed(ctx, rawURL, meta); err != nil {
		// oEmbed failure is non-fatal: we still have the video ID and can
		// embed via the iframe URL. Log and continue.
		slog.WarnContext(ctx, "youtube oembed failed", "url", rawURL, "error", err)
	}

	if p.apiKey != "" {
		if err := p.resolveDataAPI(ctx, videoID, meta); err != nil {
			// Data API is optional enrichment; a failure should not break
			// the resolve flow.
			slog.WarnContext(ctx, "youtube data api failed", "video_id", videoID, "error", err)
		}
	}

	// If oEmbed/Data API did not populate a channel name, fall back to the
	// author name so the channel key still has something to hash.
	if meta.ChannelName == "" {
		meta.ChannelName = meta.AuthorName
	}
	if meta.ChannelName == "" {
		meta.ChannelName = "youtube"
	}

	return meta, nil
}

// extractYouTubeID pulls the video ID from common YouTube URL shapes.
func extractYouTubeID(u *url.URL) string {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")

	if host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/")
	}

	if v := u.Query().Get("v"); v != "" {
		return v
	}

	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(segments) < 2 {
		return ""
	}

	switch segments[0] {
	case "embed", "shorts":
		return segments[1]
	}

	return ""
}

// resolveOEmbed fetches the YouTube oEmbed endpoint and populates title,
// author, thumbnail, and embed HTML.
func (p *YouTubeProvider) resolveOEmbed(ctx context.Context, rawURL string, meta *VideoMetadata) error {
	values := url.Values{}
	values.Set("url", rawURL)
	values.Set("format", "json")
	oembedURL := p.oembedBaseURL + "?" + values.Encode()

	if p.ssrfEnabled {
		if err := linkpreview.ValidateURL(oembedURL); err != nil {
			return fmt.Errorf("ssrf validation failed for oembed: %w", err)
		}
	}

	data, err := p.fetchJSON(ctx, oembedURL)
	if err != nil {
		return err
	}

	if t, ok := data["title"].(string); ok {
		meta.Title = t
	}
	if a, ok := data["author_name"].(string); ok {
		meta.AuthorName = a
		meta.ChannelName = a
		meta.AuthorHandle = sanitizeUsername(a)
	}
	if u, ok := data["author_url"].(string); ok {
		meta.ChannelURL = u
	}
	if t, ok := data["thumbnail_url"].(string); ok {
		meta.ThumbnailURL = t
	}
	if html, ok := data["html"].(string); ok {
		meta.EmbedHTML = html
	}

	return nil
}

// resolveDataAPI enriches metadata from the YouTube Data API v3.
func (p *YouTubeProvider) resolveDataAPI(ctx context.Context, videoID string, meta *VideoMetadata) error {
	videosURL := fmt.Sprintf("%s/videos?part=snippet,contentDetails&id=%s&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(videoID), url.QueryEscape(p.apiKey))

	vData, err := p.fetchJSON(ctx, videosURL)
	if err != nil {
		return err
	}

	items, ok := vData["items"].([]any)
	if !ok || len(items) == 0 {
		return errors.New("no video items returned from data api")
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		return errors.New("invalid video item from data api")
	}

	snippet, _ := item["snippet"].(map[string]any)
	contentDetails, _ := item["contentDetails"].(map[string]any)

	if catID, ok := snippet["categoryId"].(string); ok {
		meta.Category = catID
	}
	if desc, ok := snippet["description"].(string); ok && meta.Description == "" {
		meta.Description = desc
	}
	if chTitle, ok := snippet["channelTitle"].(string); ok {
		meta.ChannelName = chTitle
		if meta.AuthorName == "" {
			meta.AuthorName = chTitle
		}
	}
	if chID, ok := snippet["channelId"].(string); ok {
		meta.ChannelURL = fmt.Sprintf("https://www.youtube.com/channel/%s", chID)
		if err := p.resolveChannel(ctx, chID, meta); err != nil {
			slog.WarnContext(ctx, "youtube channel fetch failed", "channel_id", chID, "error", err)
		}
	}

	if dur, ok := contentDetails["duration"].(string); ok {
		meta.DurationSecs = parseISO8601Duration(dur)
	}

	return nil
}

// resolveChannel fetches channel-level metadata (logo, banner, description).
func (p *YouTubeProvider) resolveChannel(ctx context.Context, channelID string, meta *VideoMetadata) error {
	channelURL := fmt.Sprintf("%s/channels?part=snippet,brandingSettings&id=%s&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(channelID), url.QueryEscape(p.apiKey))

	cData, err := p.fetchJSON(ctx, channelURL)
	if err != nil {
		return err
	}

	items, ok := cData["items"].([]any)
	if !ok || len(items) == 0 {
		return errors.New("no channel items returned from data api")
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		return errors.New("invalid channel item from data api")
	}

	snippet, _ := item["snippet"].(map[string]any)
	branding, _ := item["brandingSettings"].(map[string]any)

	if desc, ok := snippet["description"].(string); ok {
		meta.AuthorDesc = desc
	}
	if country, ok := snippet["country"].(string); ok {
		meta.AuthorLocation = country
	}

	thumbs, _ := snippet["thumbnails"].(map[string]any)
	if len(thumbs) > 0 {
		meta.ChannelLogo = bestThumbnail(thumbs)
	}

	if images, _ := branding["image"].(map[string]any); images != nil {
		if banner, ok := images["bannerExternalUrl"].(string); ok && banner != "" {
			meta.ChannelBanner = banner
		} else if banner, ok := images["bannerMobileExtraHdImageUrl"].(string); ok && banner != "" {
			meta.ChannelBanner = banner
		} else if banner, ok := images["bannerMobileHdImageUrl"].(string); ok && banner != "" {
			meta.ChannelBanner = banner
		}
	}

	return nil
}

// fetchJSON performs a GET request and decodes the JSON body.
func (p *YouTubeProvider) fetchJSON(ctx context.Context, u string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, 1<<20)
	var data map[string]any
	if err := json.NewDecoder(limited).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// bestThumbnail returns the highest-resolution thumbnail URL available.
func bestThumbnail(thumbs map[string]any) string {
	resolutions := []string{"maxres", "standard", "high", "medium", "default"}
	for _, k := range resolutions {
		v, ok := thumbs[k].(map[string]any)
		if !ok {
			continue
		}
		if u, ok := v["url"].(string); ok && u != "" {
			return u
		}
	}
	return ""
}

// newSSRFSafeClient returns an HTTP client that validates redirect hops.
func newSSRFSafeClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if err := linkpreview.ValidateRedirectURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// parseISO8601Duration converts a YouTube duration like PT1H2M3S to seconds.
func parseISO8601Duration(s string) int {
	if s == "" {
		return 0
	}
	s = strings.ToUpper(s)
	re := regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}

	total := 0
	if h, _ := strconv.Atoi(m[1]); h > 0 {
		total += h * 3600
	}
	if m, _ := strconv.Atoi(m[2]); m > 0 {
		total += m * 60
	}
	if s2, _ := strconv.Atoi(m[3]); s2 > 0 {
		total += s2
	}
	return total
}
