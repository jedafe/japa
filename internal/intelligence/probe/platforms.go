package probe

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// platformDetector recognizes platform hosts from the URL alone — no page
// content needed, which is why it also runs when the entry fetch returned an
// HTTP error. Host signatures are ground truth: their proposals bypass the
// category-method intersection (see bypassMethods / methodAllowed).
type platformDetector struct{}

// videoProviderHosts maps a supported video platform host to its provider
// key — the same provider set as sources.isVideoProviderHost (§106:
// YouTube, TikTok, Instagram, Vimeo), duplicated here because that helper is
// unexported and probe must not reach into sources internals.
var videoProviderHosts = map[string]string{
	"youtube.com":   "youtube",
	"youtu.be":      "youtube",
	"tiktok.com":    "tiktok",
	"instagram.com": "instagram",
	"vimeo.com":     "vimeo",
}

// platformDisplayNames are proposal-label names for provider keys.
var platformDisplayNames = map[string]string{
	"youtube":   "YouTube",
	"tiktok":    "TikTok",
	"instagram": "Instagram",
	"vimeo":     "Vimeo",
}

// Detect implements Detector.
func (platformDetector) Detect(_ context.Context, page *FetchedPage, out *Report) {
	u, err := url.Parse(page.FinalURL)
	if err != nil || u.Hostname() == "" {
		return
	}

	host := normalizeHost(u.Hostname())

	if platform, ok := videoProviderHosts[host]; ok {
		detectVideoPlatform(out, page.FinalURL, platform)

		return
	}

	switch {
	case host == "medium.com" || strings.HasSuffix(host, ".medium.com"):
		detectMedium(out, u)
	case host == "github.com":
		detectGitHub(out, u)
	case host == "reddit.com" || strings.HasSuffix(host, ".reddit.com"):
		detectReddit(out, u)
	case host == "t.me" || host == "telegram.me":
		detectTelegram(out, u)
	}
}

// detectVideoPlatform emits the video_channel proposal: pipeline curate,
// crawl_method "channel_sync" — a routing marker, NOT a fetchable method.
// The materializer routes it to the §106 video_channel source flow; the
// platform is recorded in provider_config.
func detectVideoPlatform(out *Report, entryURL, platform string) {
	out.addKind(platform)
	out.addMethod(MethodChannelSync, weightChannelSync, "video platform host "+platform)

	name := platformDisplayNames[platform]
	if name == "" {
		name = platform
	}

	out.Channels = append(out.Channels, ChannelProposal{
		Label:       name + " channel (video sync)",
		EntryURL:    entryURL,
		CrawlMethod: MethodChannelSync,
		Pipeline:    string(sources.PipelineCurate),
		Confidence:  weightChannelSync,
		ProviderConfig: map[string]any{
			"platform": platform,
			"via":      MethodChannelSync,
		},
	})
}

// detectMedium proposes the platform's conventional /feed endpoint —
// Medium exposes RSS at /feed[/ @user | /tag/…] for every publication and
// profile. For a profile URL (@user path) the feed keeps the user segment.
func detectMedium(out *Report, u *url.URL) {
	out.addKind("medium")

	feedURL := u.Scheme + "://" + u.Host + "/feed"
	if seg := firstPathSegment(u); strings.HasPrefix(seg, "@") {
		feedURL += "/" + seg
	}

	if !slices.Contains(out.FeedURLs, feedURL) {
		out.FeedURLs = append(out.FeedURLs, feedURL)
	}

	out.addMethod(string(sources.CrawlMethodRSS), weightFeed, "medium platform feed")
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "Medium feed",
		EntryURL:    feedURL,
		CrawlMethod: string(sources.CrawlMethodRSS),
		Confidence:  weightFeed,
	})
}

// detectGitHub proposes a git channel for /{owner}/{repo} URLs. The git
// adapter (spec §3) watches releases/tags/files via provider_config.
func detectGitHub(out *Report, u *url.URL) {
	repo := githubRepoPath(u.Path)
	if repo == "" {
		// github.com without a repo path — still a platform signature.
		out.addKind("github")

		return
	}

	out.addKind("github")
	out.addMethod(string(sources.CrawlMethodGit), weightGit, "github repo "+repo)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "GitHub repository " + repo,
		EntryURL:    "https://github.com/" + repo,
		CrawlMethod: string(sources.CrawlMethodGit),
		Confidence:  weightGit,
		ProviderConfig: map[string]any{
			"provider": "github",
			"repo":     repo,
			"via":      "releases",
		},
	})
}

// githubRepoPath returns "owner/repo" for repository URLs, stripping a
// trailing .git suffix and any sub-path (/tree/…, /releases, /issues).
func githubRepoPath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	// Reserved top-level namespaces are never repo owners.
	switch parts[0] {
	case "settings", "explore", "topics", "marketplace", "pricing",
		"features", "login", "signup", "orgs", "organizations", "search":
		return ""
	}

	repo := strings.TrimSuffix(parts[1], ".git")

	return parts[0] + "/" + repo
}

// detectReddit proposes a social channel on the subreddit/user .json
// listing endpoint (reddit appends .json to any listing URL).
func detectReddit(out *Report, u *url.URL) {
	out.addKind("reddit")

	jsonURL, handle := redditJSONEndpoint(u)
	if jsonURL == "" {
		return
	}

	out.addMethod(string(sources.CrawlMethodSocial), weightSocial, "reddit "+handle)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "Reddit " + handle,
		EntryURL:    jsonURL,
		CrawlMethod: string(sources.CrawlMethodSocial),
		Confidence:  weightSocial,
		ProviderConfig: map[string]any{
			"provider": "reddit",
			"handle":   handle,
		},
	})
}

// redditJSONEndpoint maps /r/{sub} and /user/{name} URLs to their .json
// listing endpoint plus a display handle.
func redditJSONEndpoint(u *url.URL) (endpoint, handle string) {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}

	switch parts[0] {
	case "r":
		return "https://www.reddit.com/r/" + parts[1] + "/.json", "r/" + parts[1]
	case "user":
		return "https://www.reddit.com/user/" + parts[1] + "/.json", "u/" + parts[1]
	}

	return "", ""
}

// detectTelegram proposes a social channel for t.me/{handle} URLs — the
// public web preview the social adapter reads lives at /s/{handle}.
func detectTelegram(out *Report, u *url.URL) {
	handle := firstPathSegment(u)
	if handle == "s" {
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) >= 2 {
			handle = segs[1]
		} else {
			handle = ""
		}
	}

	if handle == "" {
		return
	}

	out.addKind("telegram")
	out.addMethod(string(sources.CrawlMethodSocial), weightSocial, "telegram @"+handle)
	out.Channels = append(out.Channels, ChannelProposal{
		Label:       "Telegram @" + handle,
		EntryURL:    "https://t.me/s/" + handle,
		CrawlMethod: string(sources.CrawlMethodSocial),
		Confidence:  weightSocial,
		ProviderConfig: map[string]any{
			"provider": "telegram",
			"handle":   "@" + handle,
		},
	})
}

// normalizeHost mirrors sources.isVideoProviderHost's normalization:
// lowercase, strip www. and m. prefixes.
func normalizeHost(host string) string {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	host = strings.TrimPrefix(host, "m.")

	return host
}

// firstPathSegment returns the first non-empty path segment.
func firstPathSegment(u *url.URL) string {
	for _, seg := range strings.Split(u.Path, "/") {
		if seg != "" {
			return seg
		}
	}

	return ""
}
