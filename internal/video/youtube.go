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

// ErrYouTubeAPIKeyMissing is returned when a Data API call is requested but
// no API key has been configured.
var ErrYouTubeAPIKeyMissing = errors.New("YouTube Data API key is not configured")

// httpClient is the minimal HTTP interface used by the provider.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// YouTubeProvider resolves YouTube URLs to Metadata.
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

// CloneWithAPIKey returns a shallow copy of the provider with the API key
// replaced. It reuses the same HTTP client and SSRF configuration.
func (p *YouTubeProvider) CloneWithAPIKey(key string) *YouTubeProvider {
	c := *p
	c.apiKey = key

	return &c
}

// Resolve parses a YouTube URL and resolves metadata first from oEmbed, then
// optionally from the YouTube Data API v3 when an API key is configured.
func (p *YouTubeProvider) Resolve(ctx context.Context, rawURL string) (*Metadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVideoURL, err)
	}

	videoID := extractYouTubeID(u)
	if videoID == "" {
		return nil, ErrNoYouTubeID
	}

	canonicalURL := "https://www.youtube.com/watch?v=" + videoID
	meta := &Metadata{
		SourceType:    SourceYouTube,
		SourceURL:     canonicalURL,
		ExternalID:    videoID,
		ChannelName:   "",
		ChannelURL:    "",
		ChannelLogo:   "",
		ChannelBanner: "",
		AuthorName:    "",
		AuthorHandle:  "",
		AuthorAvatar:  "",
		AuthorDesc:    "",
		EmbedHTML:     fmt.Sprintf(`<iframe width="560" height="315" src="https://www.youtube.com/embed/%s" frameborder="0" allowfullscreen></iframe>`, videoID),
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
func (p *YouTubeProvider) resolveOEmbed(ctx context.Context, rawURL string, meta *Metadata) error {
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
func (p *YouTubeProvider) resolveDataAPI(ctx context.Context, videoID string, meta *Metadata) error {
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
		meta.ChannelURL = "https://www.youtube.com/channel/" + chID
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
func (p *YouTubeProvider) resolveChannel(ctx context.Context, channelID string, meta *Metadata) error {
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

	applyChannelSnippet(item["snippet"], meta)
	applyChannelBranding(item["brandingSettings"], meta)

	return nil
}

// applyChannelSnippet extracts snippet fields into the metadata struct.
func applyChannelSnippet(snippetRaw any, meta *Metadata) {
	snippet, _ := snippetRaw.(map[string]any)
	if len(snippet) == 0 {
		return
	}

	if desc, ok := snippet["description"].(string); ok {
		meta.AuthorDesc = desc
	}

	if country, ok := snippet["country"].(string); ok {
		meta.AuthorLocation = country
	}

	if thumbs, _ := snippet["thumbnails"].(map[string]any); len(thumbs) > 0 {
		meta.ChannelLogo = bestThumbnail(thumbs)
	}
}

// applyChannelBranding extracts banner images into the metadata struct.
func applyChannelBranding(brandingRaw any, meta *Metadata) {
	branding, _ := brandingRaw.(map[string]any)
	if len(branding) == 0 {
		return
	}

	images, _ := branding["image"].(map[string]any)
	if images == nil {
		return
	}

	for _, key := range []string{"bannerExternalUrl", "bannerMobileExtraHdImageUrl", "bannerMobileHdImageUrl"} {
		if banner, ok := images[key].(string); ok && banner != "" {
			meta.ChannelBanner = banner
			return
		}
	}
}

// fetchJSON performs a GET request and decodes the JSON body.
func (p *YouTubeProvider) fetchJSON(ctx context.Context, u string) (map[string]any, error) {
	if p.ssrfEnabled {
		if err := linkpreview.ValidateURL(u); err != nil {
			return nil, fmt.Errorf("ssrf validation failed: %w", err)
		}
	}
	// #nosec G704 — u is constructed from the YouTube Data/OEmbed API base URL
	// and validated URL-escaped identifiers; the client is SSRF-safe.
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

// SearchChannel resolves a YouTube channel URL or name and returns the
// canonical video URLs for the channel's recent uploads.
func (p *YouTubeProvider) SearchChannel(ctx context.Context, query string) ([]string, error) {
	if p.apiKey == "" {
		return nil, ErrYouTubeAPIKeyMissing
	}

	channelID, err := p.resolveChannelID(ctx, query)
	if err != nil {
		return nil, err
	}

	searchURL := fmt.Sprintf("%s/search?part=snippet&channelId=%s&type=video&maxResults=50&order=date&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(channelID), url.QueryEscape(p.apiKey))

	data, err := p.fetchJSON(ctx, searchURL)
	if err != nil {
		return nil, err
	}

	return p.videoURLsFromSearch(data), nil
}

// SearchPlaylist resolves a YouTube playlist URL and returns the canonical
// video URLs for each item in the playlist.
func (p *YouTubeProvider) SearchPlaylist(ctx context.Context, rawURL string) ([]string, error) {
	if p.apiKey == "" {
		return nil, ErrYouTubeAPIKeyMissing
	}

	playlistID := extractYouTubePlaylistID(rawURL)
	if playlistID == "" {
		return nil, fmt.Errorf("%w: no playlist ID found in %s", ErrInvalidVideoURL, rawURL)
	}

	itemsURL := fmt.Sprintf("%s/playlistItems?part=snippet&playlistId=%s&maxResults=50&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(playlistID), url.QueryEscape(p.apiKey))

	data, err := p.fetchJSON(ctx, itemsURL)
	if err != nil {
		return nil, err
	}

	return p.videoURLsFromPlaylistItems(data), nil
}

// SearchKeyword runs a YouTube keyword search and returns the canonical video
// URLs for the matching results.
func (p *YouTubeProvider) SearchKeyword(ctx context.Context, query string) ([]string, error) {
	if p.apiKey == "" {
		return nil, ErrYouTubeAPIKeyMissing
	}

	searchURL := fmt.Sprintf("%s/search?part=snippet&q=%s&type=video&maxResults=50&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(query), url.QueryEscape(p.apiKey))

	data, err := p.fetchJSON(ctx, searchURL)
	if err != nil {
		return nil, err
	}

	return p.videoURLsFromSearch(data), nil
}

// resolveChannelID takes a channel URL or identifier and returns a YouTube
// channel ID suitable for use in search/list calls.
func (p *YouTubeProvider) resolveChannelID(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	u, err := url.Parse(query)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		id, handle := extractYouTubeChannelIDFromURL(u)
		if id != "" {
			return id, nil
		}

		if handle != "" {
			if ch, err := p.lookupChannelByHandle(ctx, handle); err == nil && ch != "" {
				return ch, nil
			}

			return p.fallbackChannelSearch(ctx, handle)
		}
	}

	// Try treating the query as a channel ID directly.
	if strings.HasPrefix(query, "UC") && len(query) > 10 {
		return query, nil
	}

	// Try forHandle (/@handle, /c/name, bare handle).
	handle := strings.TrimPrefix(query, "@")
	if handle != "" {
		if ch, err := p.lookupChannelByHandle(ctx, handle); err == nil && ch != "" {
			return ch, nil
		}

		return p.fallbackChannelSearch(ctx, handle)
	}

	return "", errors.New("no matching YouTube channel found")
}

func (p *YouTubeProvider) lookupChannelByHandle(ctx context.Context, handle string) (string, error) {
	forHandle := handle
	if !strings.HasPrefix(handle, "@") {
		forHandle = "@" + handle
	}

	channelURL := fmt.Sprintf("%s/channels?part=id&forHandle=%s&maxResults=1&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(forHandle), url.QueryEscape(p.apiKey))

	data, err := p.fetchJSON(ctx, channelURL)
	if err != nil {
		return "", err
	}

	items, _ := data["items"].([]any)
	if len(items) == 0 {
		// Some older custom URLs do not expose a handle; try forUsername.
		channelURL = fmt.Sprintf("%s/channels?part=id&forUsername=%s&maxResults=1&key=%s",
			p.dataAPIBaseURL, url.QueryEscape(handle), url.QueryEscape(p.apiKey))

		data, err = p.fetchJSON(ctx, channelURL)
		if err != nil {
			return "", err
		}

		items, _ = data["items"].([]any)
	}

	if len(items) > 0 {
		if item, ok := items[0].(map[string]any); ok {
			if id, ok := item["id"].(string); ok && id != "" {
				return id, nil
			}
		}
	}

	return "", errors.New("no matching YouTube channel found")
}

func (p *YouTubeProvider) fallbackChannelSearch(ctx context.Context, query string) (string, error) {
	searchURL := fmt.Sprintf("%s/search?part=snippet&q=%s&type=channel&maxResults=1&key=%s",
		p.dataAPIBaseURL, url.QueryEscape(query), url.QueryEscape(p.apiKey))

	data, err := p.fetchJSON(ctx, searchURL)
	if err != nil {
		return "", err
	}

	id := p.channelIDFromSearch(data)
	if id == "" {
		return "", errors.New("no matching YouTube channel found")
	}

	return id, nil
}

func (p *YouTubeProvider) channelIDFromSearch(data map[string]any) string {
	items, _ := data["items"].([]any)
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}

		if id, ok := item["id"].(map[string]any); ok {
			if chID, ok := id["channelId"].(string); ok && chID != "" {
				return chID
			}
		}

		snippet, _ := item["snippet"].(map[string]any)
		if chID, ok := snippet["channelId"].(string); ok && chID != "" {
			return chID
		}
	}

	return ""
}

func (p *YouTubeProvider) videoURLsFromSearch(data map[string]any) []string {
	var urls []string

	items, _ := data["items"].([]any)
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}

		videoID := ""

		if id, ok := item["id"].(map[string]any); ok {
			if v, ok := id["videoId"].(string); ok {
				videoID = v
			}
		}

		snippet, _ := item["snippet"].(map[string]any)
		if videoID == "" && snippet != nil {
			if res, ok := snippet["resourceId"].(map[string]any); ok {
				if v, ok := res["videoId"].(string); ok {
					videoID = v
				}
			}

			if v, ok := snippet["videoId"].(string); ok {
				videoID = v
			}
		}

		if videoID != "" {
			urls = append(urls, "https://www.youtube.com/watch?v="+videoID)
		}
	}

	return urls
}

func (p *YouTubeProvider) videoURLsFromPlaylistItems(data map[string]any) []string {
	var urls []string

	items, _ := data["items"].([]any)
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}

		snippet, _ := item["snippet"].(map[string]any)
		if snippet == nil {
			continue
		}

		res, _ := snippet["resourceId"].(map[string]any)
		if res == nil {
			continue
		}

		if videoID, ok := res["videoId"].(string); ok && videoID != "" {
			urls = append(urls, "https://www.youtube.com/watch?v="+videoID)
		}
	}

	return urls
}

func extractYouTubeChannelIDFromURL(u *url.URL) (channelID, handle string) {
	path := strings.TrimPrefix(u.Path, "/")
	segments := strings.Split(path, "/")

	if v := u.Query().Get("channel"); v != "" {
		return v, ""
	}

	if len(segments) > 0 && strings.HasPrefix(segments[0], "UC") && len(segments[0]) > 10 {
		return segments[0], ""
	}

	return segmentToChannelID(segments)
}

func segmentToChannelID(segments []string) (channelID, handle string) {
	if len(segments) == 0 || segments[0] == "" {
		return "", ""
	}

	switch segments[0] {
	case "channel":
		if len(segments) > 1 {
			if strings.HasPrefix(segments[1], "UC") {
				return segments[1], ""
			}

			return "", segments[1]
		}
	case "user", "c":
		if len(segments) > 1 {
			return "", segments[1]
		}
	case "@":
		if len(segments) > 1 {
			return "", segments[1]
		}
	}

	// Treat any other first path segment as a handle (/@handle).
	return "", strings.TrimPrefix(segments[0], "@")
}

func extractYouTubePlaylistID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if list := u.Query().Get("list"); list != "" {
		return list
	}

	path := strings.Trim(u.Path, "/")
	if strings.HasPrefix(path, "playlist?list=") {
		if v, err := url.ParseQuery(strings.TrimPrefix(path, "playlist?")); err == nil {
			return v.Get("list")
		}
	}

	return ""
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
