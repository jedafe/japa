// Package customfields provides the platform embed URL resolver for
// JapaConnect custom field values. It identifies the platform from a URL's
// hostname and resolves the embed using either structured URL parsing or the
// platform's oEmbed endpoint.
//
// GitHub gists, Figma files, and Google Maps links are resolved purely from
// URL parsing (no HTTP calls). YouTube embeds the video via an iframe URL and
// additionally enriches metadata from oEmbed. Twitter/X, Vimeo, Spotify, and
// CodePen are resolved entirely from their oEmbed endpoints.
package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Platform identifier constants. Each corresponds to a host pattern recognized
// by identifyPlatform and a key in the per-platform resolver dispatch.
const (
	platformYouTube    = "youtube"
	platformGitHub     = "github"
	platformTwitter    = "twitter"
	platformVimeo      = "vimeo"
	platformSpotify    = "spotify"
	platformFigma      = "figma"
	platformCodePen    = "codepen"
	platformGoogleMaps = "google_maps"
)

// defaultOEmbedURLs maps each oEmbed-backed platform to its real oEmbed
// endpoint. Platforms missing from this map (github, figma, google_maps) are
// resolved purely from URL parsing.
var defaultOEmbedURLs = map[string]string{
	platformYouTube: "https://www.youtube.com/oembed",
	platformTwitter: "https://publish.twitter.com/oembed",
	platformVimeo:   "https://vimeo.com/api/oembed.json",
	platformSpotify: "https://open.spotify.com/oembed",
	platformCodePen: "https://codepen.io/api/oembed",
}

// oEmbed platforms that require the format=json query parameter.
var platformsNeedingFormatJSON = map[string]struct{}{
	platformYouTube: {},
	platformCodePen: {},
}

// EmbedData holds the resolved embed information for a platform URL.
type EmbedData struct {
	// Platform is the canonical platform key (e.g. "youtube", "github").
	Platform string
	// URL is the original URL provided to Resolve.
	URL string
	// EmbedURL is the URL suitable for an iframe src, when the platform
	// supports direct iframe embedding. Empty when not applicable.
	EmbedURL string
	// Title is the title of the embedded content, if available.
	Title string
	// Author is the author or creator name, if available.
	Author string
	// ThumbnailURL is a preview image URL, if available.
	ThumbnailURL string
	// HTML is the raw oEmbed HTML response, when the platform returns one.
	HTML string
	// Raw is the full oEmbed response body, or a small synthetic metadata map
	// for URL-parsed platforms (github, figma, google_maps). Never nil.
	Raw map[string]any
}

// ErrUnsupportedPlatform is returned when the URL's host does not match any
// supported platform, or when an allowedPlatforms filter excludes it.
var ErrUnsupportedPlatform = errors.New("unsupported embed platform")

// EmbedResolver resolves platform URLs into EmbedData.
type EmbedResolver struct {
	client *http.Client
	// oembedBaseURLs overrides the real oEmbed endpoints, keyed by platform.
	// Used by tests to point oEmbed calls at a mock httptest server. When nil
	// or missing a key, defaultOEmbedURLs is consulted.
	oembedBaseURLs map[string]string
}

// NewEmbedResolver creates an EmbedResolver with a 10-second timeout HTTP
// client suitable for production use.
func NewEmbedResolver() *EmbedResolver {
	return &EmbedResolver{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// newEmbedResolverWithOverrides creates an EmbedResolver for tests, replacing
// the real oEmbed endpoints with the provided overrides. Each platform's
// override URL receives the standard ?url={rawURL} query (plus format=json for
// youtube and codepen). Not exported because callers outside the package
// cannot intercept production HTTP endpoints.
func newEmbedResolverWithOverrides(client *http.Client, overrides map[string]string) *EmbedResolver {
	return &EmbedResolver{
		client:         client,
		oembedBaseURLs: overrides,
	}
}

// Resolve identifies the platform from the URL and resolves the embed. If
// allowedPlatforms is non-empty, only platforms in that slice are accepted —
// any other platform, even if the host is recognized, returns
// ErrUnsupportedPlatform.
func (r *EmbedResolver) Resolve(
	ctx context.Context,
	rawURL string,
	allowedPlatforms []string,
) (*EmbedData, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse embed URL %q: %w", rawURL, err)
	}

	host := strings.TrimPrefix(parsed.Hostname(), "www.")

	platform := identifyPlatform(host, parsed.Path)
	if platform == "" {
		return nil, ErrUnsupportedPlatform
	}

	if !platformAllowed(platform, allowedPlatforms) {
		return nil, ErrUnsupportedPlatform
	}

	switch platform {
	case platformYouTube:
		return r.resolveYouTube(ctx, rawURL, parsed)
	case platformGitHub:
		return resolveGitHub(rawURL, parsed), nil
	case platformTwitter, platformVimeo, platformSpotify, platformCodePen:
		return r.resolveOEmbedPlatform(ctx, platform, rawURL)
	case platformFigma:
		return resolveFigma(rawURL), nil
	case platformGoogleMaps:
		return resolveGoogleMaps(rawURL), nil
	default:
		// identifyPlatform returned a value not covered above: treat as
		// unsupported rather than panic on unexpected state.
		return nil, ErrUnsupportedPlatform
	}
}

// identifyPlatform returns the canonical platform key for a host/path pair, or
// an empty string when no platform matches. The "www." prefix on the host
// must already be stripped by the caller.
func identifyPlatform(host, path string) string {
	switch {
	case host == "youtube.com" || host == "youtu.be":
		return platformYouTube
	case host == "github.com" || host == "gist.github.com":
		return platformGitHub
	case host == "twitter.com" || host == "x.com":
		return platformTwitter
	case host == "vimeo.com":
		return platformVimeo
	case host == "open.spotify.com":
		return platformSpotify
	case host == "figma.com":
		return platformFigma
	case host == "codepen.io":
		return platformCodePen
	case isGoogleMaps(host, path):
		return platformGoogleMaps
	default:
		return ""
	}
}

// isGoogleMaps reports whether a host/path pair represents a Google Maps URL.
// Matches both maps.google.com and google.com/maps/*.
func isGoogleMaps(host, path string) bool {
	return host == "maps.google.com" ||
		(host == "google.com" && strings.Contains(path, "/maps"))
}

// platformAllowed reports whether the platform passes the allowedPlatforms
// filter. An empty allowed slice allows every recognized platform.
func platformAllowed(platform string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}

	for _, p := range allowed {
		if p == platform {
			return true
		}
	}

	return false
}

// resolveYouTube resolves a YouTube URL to embed data. The embed iframe URL is
// built from the extracted video ID even when the oEmbed call fails — the
// video remains watchable, only metadata is missing.
func (r *EmbedResolver) resolveYouTube(
	ctx context.Context,
	rawURL string,
	u *url.URL,
) (*EmbedData, error) {
	videoID := extractYouTubeID(u)
	if videoID == "" {
		// The platform is recognized as YouTube but the URL shape does not carry
		// a video ID. This is a parse failure, distinct from an unsupported
		// platform.
		return nil, fmt.Errorf("no YouTube video ID found in %q", rawURL)
	}

	embed := &EmbedData{
		Platform: platformYouTube,
		URL:      rawURL,
		EmbedURL: "https://www.youtube.com/embed/" + videoID,
		Raw:      map[string]any{"video_id": videoID, "type": "video"},
	}

	if err := r.enrichFromOEmbed(ctx, platformYouTube, rawURL, embed); err != nil {
		// YouTube oEmbed failure is non-fatal: the iframe still embeds the
		// video, only title/author/thumbnail are absent. Log and continue.
		slog.WarnContext(
			ctx,
			"youtube oembed fetch failed; embed will lack metadata",
			"err",
			err,
			"url",
			rawURL,
		)
	}

	return embed, nil
}

// extractYouTubeID pulls the video ID from the common YouTube URL shapes:
// youtu.be/{id}, youtube.com/watch?v={id}, youtube.com/embed/{id}, and
// youtube.com/shorts/{id}. Returns "" when no ID can be extracted.
func extractYouTubeID(u *url.URL) string {
	if u.Hostname() == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/")
	}

	if v := u.Query().Get("v"); v != "" {
		return v
	}

	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(segments) < 2 {
		return ""
	}

	first := segments[0]
	if first == "embed" || first == "shorts" {
		return segments[1]
	}

	return ""
}

// resolveGitHub resolves a GitHub URL with no HTTP calls. Gists are detected
// from either gist.github.com/{id} or github.com/{user}/gist/{id}. Repos are
// detected from github.com/{owner}/{repo}. Other GitHub paths still produce an
// embed with the platform set but no parsed metadata.
func resolveGitHub(rawURL string, u *url.URL) *EmbedData {
	embed := &EmbedData{
		Platform: platformGitHub,
		URL:      rawURL,
		Raw:      map[string]any{},
	}

	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	host := u.Hostname()

	isGistHost := host == "gist.github.com"
	isGistPathOnRepoHost := len(segments) >= 3 && segments[1] == "gist"

	if !isGistHost && !isGistPathOnRepoHost {
		return resolveGitHubRepo(embed, segments)
	}

	var gistUser, gistID string

	switch {
	case isGistHost && len(segments) == 1:
		// gist.github.com/{id}
		gistID = segments[0]
	case isGistHost && len(segments) >= 2:
		// gist.github.com/{user}/{id} (a trailing slash adds an empty last
		// segment, hence >= 2 rather than == 2).
		gistUser = segments[0]
		gistID = segments[1]
	case !isGistHost && len(segments) >= 3:
		// github.com/{user}/gist/{id}
		gistUser = segments[0]
		gistID = segments[2]
	}

	switch {
	case gistUser != "" && gistID != "":
		embed.EmbedURL = fmt.Sprintf("https://gist.github.com/%s/%s", gistUser, gistID)
	case gistID != "":
		embed.EmbedURL = "https://gist.github.com/" + gistID
	}

	embed.Raw["type"] = "gist"
	embed.Raw["gist_user"] = gistUser
	embed.Raw["gist_id"] = gistID

	return embed
}

// resolveGitHubRepo fills embed data for a github.com/{owner}/{repo} URL.
func resolveGitHubRepo(embed *EmbedData, segments []string) *EmbedData {
	embed.Raw["type"] = "repo"
	if len(segments) < 2 {
		return embed
	}

	owner := segments[0]
	repo := segments[1]
	embed.Title = owner + "/" + repo
	embed.EmbedURL = embed.URL
	embed.Raw["owner"] = owner
	embed.Raw["repo"] = repo

	return embed
}

// resolveFigma constructs a Figma embed URL with no HTTP calls. The frontend
// loads the embed inside an iframe using embed_host=japaconnect.
func resolveFigma(rawURL string) *EmbedData {
	embedURL := "https://www.figma.com/embed?embed_host=japaconnect&url=" +
		url.QueryEscape(rawURL)

	return &EmbedData{
		Platform: platformFigma,
		URL:      rawURL,
		EmbedURL: embedURL,
		Raw:      map[string]any{},
	}
}

// resolveGoogleMaps stores the original URL as the embed URL. The frontend
// renders the actual map using the platform's Maps API key, so no HTTP call is
// needed here.
func resolveGoogleMaps(rawURL string) *EmbedData {
	return &EmbedData{
		Platform: platformGoogleMaps,
		URL:      rawURL,
		EmbedURL: rawURL,
		Raw:      map[string]any{},
	}
}

// resolveOEmbedPlatform resolves a URL for a platform whose entire embed
// representation comes from its oEmbed endpoint. Returns an error when the
// oEmbed call fails — for these platforms there is no fallback representation.
func (r *EmbedResolver) resolveOEmbedPlatform(
	ctx context.Context,
	platform, rawURL string,
) (*EmbedData, error) {
	embed := &EmbedData{
		Platform: platform,
		URL:      rawURL,
		Raw:      map[string]any{},
	}
	if err := r.enrichFromOEmbed(ctx, platform, rawURL, embed); err != nil {
		return nil, fmt.Errorf("resolve %s: %w", platform, err)
	}

	return embed, nil
}

// enrichFromOEmbed fetches the platform's oEmbed response and populates the
// Title, Author, ThumbnailURL, HTML, and Raw fields on embed. When the
// platform has no oEmbed endpoint (e.g. github, figma, google_maps), this is
// a no-op.
func (r *EmbedResolver) enrichFromOEmbed(
	ctx context.Context,
	platform, rawURL string,
	embed *EmbedData,
) error {
	oembedURL, ok := r.oembedURL(platform, rawURL)
	if !ok {
		return nil
	}

	data, err := r.fetchOEmbed(ctx, oembedURL)
	if err != nil {
		return err
	}

	embed.HTML = getString(data, "html")
	embed.Title = getString(data, "title")
	embed.Author = getString(data, "author_name")
	embed.ThumbnailURL = getString(data, "thumbnail_url")

	// Merge the oEmbed response into embed.Raw rather than replacing it so
	// platform-specific keys set earlier (e.g. YouTube's video_id) survive.
	if embed.Raw == nil {
		embed.Raw = map[string]any{}
	}

	for k, v := range data {
		embed.Raw[k] = v
	}

	return nil
}

// oembedURL builds the full oEmbed request URL for a platform. Returns
// (url, false) when the platform has no oEmbed endpoint (github, figma,
// google_maps). Test overrides take precedence over defaultOEmbedURLs.
func (r *EmbedResolver) oembedURL(platform, rawURL string) (string, bool) {
	base := ""
	if r.oembedBaseURLs != nil {
		base = r.oembedBaseURLs[platform]
	}

	if base == "" {
		base = defaultOEmbedURLs[platform]
	}

	if base == "" {
		return "", false
	}

	values := url.Values{}
	values.Set("url", rawURL)

	if _, needsFormat := platformsNeedingFormatJSON[platform]; needsFormat {
		values.Set("format", "json")
	}

	return base + "?" + values.Encode(), true
}

// fetchOEmbed makes a GET request to the oEmbed endpoint and parses the JSON
// response into a map. The returned map is never nil on success.
func (r *EmbedResolver) fetchOEmbed(
	ctx context.Context,
	oembedURL string,
) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oembedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build oembed request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oembed request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oembed returned non-2xx status %d", resp.StatusCode)
	}

	data := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode oembed json: %w", err)
	}

	return data, nil
}

// getString returns the string value at key in m, or "" when the key is
// missing or the value is not a string.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}

	return ""
}
