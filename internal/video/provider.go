// Package video implements video URL resolution, synthetic channel creation,
// and video source storage for the video content type (§106).
package video

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Platform constants for video sources.
const (
	SourceYouTube       = "youtube"
	SourceTikTok        = "tiktok"
	SourceInstagramReel = "instagram_reel"
	SourceVimeo         = "vimeo"
	SourceUpload        = "upload"
	SourceOther         = "other"
)

// VideoMetadata holds the canonical metadata extracted from a video URL by a
// VideoProvider. It intentionally carries both author-level and channel-level
// fields so synthetic users can be created with rich public profiles while
// still preserving the original uploader information.
type VideoMetadata struct {
	SourceType     string
	SourceURL      string
	ExternalID     string
	Title          string
	Description    string
	Category       string
	ThumbnailURL   string
	DurationSecs   int
	EmbedHTML      string
	AuthorName     string
	AuthorHandle   string
	AuthorAvatar   string
	AuthorDesc     string
	AuthorLocation string
	ChannelName    string
	ChannelURL     string
	ChannelLogo    string
	ChannelBanner  string
}

// ChannelKey returns a stable, tenant-scoped identifier for the external
// channel. It is used as the synthetic user's username and is derived from
// the source platform plus a sanitized channel name.
func (m *VideoMetadata) ChannelKey() string {
	return fmt.Sprintf("%s:%s", m.SourceType, sanitizeUsername(m.ChannelName))
}

// VideoProvider resolves a public video URL into VideoMetadata.
type VideoProvider interface {
	Resolve(ctx context.Context, url string) (*VideoMetadata, error)
	Platform() string
}

// Resolver selects the appropriate VideoProvider for a URL and delegates
// resolution.
type Resolver interface {
	Resolve(ctx context.Context, rawURL string) (*VideoMetadata, error)
}

var (
	// ErrUnsupportedPlatform is returned when no registered provider can
	// handle the URL.
	ErrUnsupportedPlatform = errors.New("unsupported video platform")
	// ErrInvalidVideoURL is returned when a URL is syntactically invalid or
	// does not contain a video identifier.
	ErrInvalidVideoURL = errors.New("invalid video URL")
)

// resolver is the default Resolver implementation.
type resolver struct {
	providers map[string]VideoProvider
}

// NewResolver returns a Resolver backed by the given providers. The caller is
// responsible for passing at least one provider (normally YouTube).
func NewResolver(providers ...VideoProvider) Resolver {
	r := &resolver{providers: make(map[string]VideoProvider, len(providers))}
	for _, p := range providers {
		r.providers[p.Platform()] = p
	}
	return r
}

// NewDefaultResolver returns a Resolver pre-populated with the YouTube
// provider. Callers can override the YouTube oEmbed endpoint and Data API
// key via functional options on the provider if needed.
func NewDefaultResolver() Resolver {
	return NewResolver(NewYouTubeProvider())
}

// Resolve identifies the platform from the URL and dispatches to the matching
// provider.
func (r *resolver) Resolve(ctx context.Context, rawURL string) (*VideoMetadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVideoURL, err)
	}

	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	switch {
	case host == "youtube.com" || host == "youtu.be":
		p, ok := r.providers[SourceYouTube]
		if !ok {
			return nil, ErrUnsupportedPlatform
		}
		return p.Resolve(ctx, rawURL)
	case host == "vimeo.com":
		if p, ok := r.providers[SourceVimeo]; ok {
			return p.Resolve(ctx, rawURL)
		}
		return nil, ErrUnsupportedPlatform
	}

	return nil, ErrUnsupportedPlatform
}

// sanitizeUsername reduces a channel name to a URL-safe string.
func sanitizeUsername(s string) string {
	if s == "" {
		return "_"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	s = re.ReplaceAllString(s, "-")
	s = strings.ToLower(strings.Trim(s, "-"))
	if s == "" {
		return "_"
	}
	return s
}
