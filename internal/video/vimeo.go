package video

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const vimeoOEmbedEndpoint = "https://vimeo.com/api/oEmbed.json"

// VimeoProvider resolves Vimeo video URLs via the official oEmbed endpoint.
type VimeoProvider struct {
	client      httpClient
	oembedURL   string
	ssrfEnabled bool
}

// NewVimeoProvider returns a Vimeo resolver with SSRF-safe defaults.
func NewVimeoProvider() *VimeoProvider {
	return &VimeoProvider{
		client:      newSSRFSafeClient(),
		oembedURL:   vimeoOEmbedEndpoint,
		ssrfEnabled: true,
	}
}

// Platform returns the platform key.
func (p *VimeoProvider) Platform() string { return SourceVimeo }

// Resolve parses a Vimeo URL and resolves metadata from oEmbed.
func (p *VimeoProvider) Resolve(ctx context.Context, rawURL string) (*Metadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVideoURL, err)
	}

	videoID := extractVimeoVideoID(u)
	if videoID == "" {
		videoID = path.Base(strings.TrimSuffix(u.Path, "/"))
	}

	oembedURL := fmt.Sprintf("%s?url=%s", p.oembedURL, url.QueryEscape(rawURL))

	data, err := p.fetchOEmbed(ctx, oembedURL)
	if err != nil {
		return nil, err
	}

	return metadataFromVimeoOEmbed(data, videoID)
}

// metadataFromVimeoOEmbed builds Metadata from a Vimeo oEmbed JSON response.
func metadataFromVimeoOEmbed(data map[string]any, fallbackID string) (*Metadata, error) {
	videoID := extractVimeoVideoIDFromOEmbed(data, fallbackID)
	if videoID == "" {
		return nil, ErrNoYouTubeID
	}

	duration := vimeoDuration(data)

	title, _ := data["title"].(string)
	description, _ := data["description"].(string)
	thumbnailURL, _ := data["thumbnail_url"].(string)
	embedHTML, _ := data["html"].(string)
	authorName, _ := data["author_name"].(string)
	authorURL, _ := data["author_url"].(string)

	canonicalURL, _ := data["url"].(string)
	if canonicalURL == "" {
		canonicalURL = "https://vimeo.com/" + videoID
	}

	channelName := authorName

	if authorURL != "" {
		if au, err := url.Parse(authorURL); err == nil && au.Path != "" {
			channelName = path.Base(strings.TrimSuffix(au.Path, "/"))
		}
	}

	return &Metadata{
		SourceType:   SourceVimeo,
		SourceURL:    canonicalURL,
		ExternalID:   videoID,
		Title:        html.UnescapeString(title),
		Description:  html.UnescapeString(description),
		ThumbnailURL: thumbnailURL,
		DurationSecs: int(duration),
		EmbedHTML:    embedHTML,
		AuthorName:   authorName,
		ChannelName:  channelName,
		ChannelURL:   authorURL,
		AuthorAvatar: thumbnailURL,
	}, nil
}

// extractVimeoVideoIDFromOEmbed prefers oEmbed video_id fields over the URL fallback.
func extractVimeoVideoIDFromOEmbed(data map[string]any, fallback string) string {
	if id, ok := data["video_id"].(float64); ok {
		return strconv.FormatInt(int64(id), 10)
	}

	if id, ok := data["video_id"].(json.Number); ok {
		return id.String()
	}

	if id, ok := data["video_id"].(string); ok {
		return id
	}

	return fallback
}

// vimeoDuration extracts duration in seconds from a Vimeo oEmbed payload.
func vimeoDuration(data map[string]any) float64 {
	duration, _ := data["duration"].(float64)

	if d, ok := data["duration"].(json.Number); ok {
		if n, err := d.Int64(); err == nil {
			duration = float64(n)
		}
	}

	return duration
}

// fetchOEmbed performs a GET request to the oEmbed endpoint and decodes JSON.
func (p *VimeoProvider) fetchOEmbed(ctx context.Context, u string) (map[string]any, error) {
	return doOEmbedRequest(ctx, p.client, u, p.ssrfEnabled, "vimeo ")
}

// extractVimeoVideoID pulls the video ID from common Vimeo URL patterns.
func extractVimeoVideoID(u *url.URL) string {
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		if s := segments[i]; s != "" {
			if _, err := strconv.Atoi(s); err == nil {
				return s
			}
		}
	}

	return ""
}
