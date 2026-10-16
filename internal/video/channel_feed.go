package video

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// YouTube channel feed resolution + polling for the video:sync_channels task
// (§106 channel-sync amendment). Both the channel-ID lookup and the uploads
// feed are unauthenticated — no YouTube Data API key required.

const youtubeChannelFeedBase = "https://www.youtube.com/feeds/videos.xml"

// channelIDPattern matches a YouTube channel ID (24 chars, UC prefix).
const channelIDPattern = `UC[0-9A-Za-z_-]{22}`

var (
	// ytChannelIDJSON matches "channelId":"UC…" / "externalId":"UC…" embedded
	// in the channel page's ytInitialData / meta JSON.
	ytChannelIDJSON = regexp.MustCompile(`"(?:channelId|externalId)"\s*:\s*"(` + channelIDPattern + `)"`)
	// ytChannelIDMeta matches <meta itemprop="identifier" content="UC…">.
	ytChannelIDMeta = regexp.MustCompile(`itemprop="identifier"[^>]*content="(` + channelIDPattern + `)"`)
	// ytChannelIDCanonical matches a canonical /channel/UC… link or URL.
	ytChannelIDCanonical = regexp.MustCompile(`youtube\.com/channel/(` + channelIDPattern + `)`)
)

const channelPageMaxBytes = 4 << 20 // 4 MiB — channel pages are large HTML

// youtubeChannelFeedURL builds the unauthenticated Atom feed URL for a
// channel ID (~15 newest uploads per poll).
func youtubeChannelFeedURL(channelID string) string {
	return youtubeChannelFeedBase + "?channel_id=" + url.QueryEscape(channelID)
}

// isYouTubeHost reports whether rawURL is a youtube.com URL. Only YouTube
// channels expose an uploads Atom feed — TikTok/Instagram/Vimeo channel
// rows are directory listings and are skipped by the sync poller.
func isYouTubeHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")

	return host == "youtube.com" || host == "youtu.be"
}

// channelIDFromURL extracts a UC channel ID directly from URL shapes that
// carry it: youtube.com/channel/UC… or the feeds/videos.xml?channel_id= form.
// Handle-style URLs (/@x, /c/x, /user/x) return "" and need page resolution.
func channelIDFromURL(rawURL string) string {
	if !isYouTubeHost(rawURL) {
		return ""
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if rest, ok := strings.CutPrefix(u.Path, "/channel/"); ok {
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			rest = rest[:i]
		}

		if strings.HasPrefix(rest, "UC") {
			return rest
		}
	}

	if strings.HasPrefix(u.Path, "/feeds/videos.xml") {
		return u.Query().Get("channel_id")
	}

	return ""
}

// resolveYouTubeChannelID resolves a channel page URL to its UC channel ID.
// Direct ID shapes are free; handle/page URLs are resolved by fetching the
// channel page once and extracting the embedded channelId — the value is
// then cached in provider_config so this runs at most once per source.
func resolveYouTubeChannelID(ctx context.Context, client *http.Client, channelURL string) (string, error) {
	if id := channelIDFromURL(channelURL); id != "" {
		return id, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelURL, nil)
	if err != nil {
		return "", fmt.Errorf("video: channel resolve: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-VideoSync/1.0 (+https://japaconnect.com)")
	req.Header.Set("Accept-Language", "en")
	// YouTube redirects EU-region requests through consent.youtube.com;
	// this cookie bypasses the interstitial so the channel page is served.
	req.AddCookie(&http.Cookie{Name: "CONSENT", Value: "YES+cb"})

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("video: channel resolve: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("video: channel resolve: unexpected HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, channelPageMaxBytes))
	if err != nil {
		return "", fmt.Errorf("video: channel resolve: %w", err)
	}

	for _, re := range []*regexp.Regexp{ytChannelIDJSON, ytChannelIDMeta, ytChannelIDCanonical} {
		if m := re.FindSubmatch(body); m != nil {
			return string(m[1]), nil
		}
	}

	return "", fmt.Errorf("video: channel id not found on page %s", channelURL)
}

// atomFeed mirrors the upload feed shape: <feed> with <entry> children each
// carrying a namespaced <yt:videoId>. Only the video ID is needed — full
// metadata comes from the oEmbed resolver at import time.
type atomFeed struct {
	Entries []struct {
		VideoID string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	} `xml:"entry"`
}

// fetchChannelVideoIDs GETs a channel's uploads feed and returns the video
// IDs in feed order (newest first), deduplicated.
func fetchChannelVideoIDs(ctx context.Context, client *http.Client, feedURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("video: channel feed: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-VideoSync/1.0 (+https://japaconnect.com)")
	req.Header.Set("Accept", "application/atom+xml, application/xml, text/xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video: channel feed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("video: channel feed: unexpected HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("video: channel feed: %w", err)
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("video: channel feed parse: %w", err)
	}

	ids := make([]string, 0, len(feed.Entries))
	seen := make(map[string]struct{}, len(feed.Entries))

	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}

		if _, dup := seen[e.VideoID]; dup {
			continue
		}

		seen[e.VideoID] = struct{}{}

		ids = append(ids, e.VideoID)
	}

	return ids, nil
}
