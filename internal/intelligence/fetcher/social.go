package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// SocialFetcher is the Phase-2 social adapter (channel design §3):
// per-provider sub-adapters normalize public posts to JSON-lines —
// {"id","title","url","author","created","score","text"} per line — so the
// downstream pipeline (curate/extract + per-item relevance gate) sees a
// uniform itemized body regardless of provider.
//
// provider_config keys:
//
//	{"provider": "reddit|telegram",    // required — allowlisted by
//	                                  // intel.social.enabled_providers
//	 "handle": "r/nigeria|t.me/chan",  // subreddit, user, or channel handle
//	 "endpoint_override": "...",       // explicit URL beats the built one
//	 "ua": "..."}                      // User-Agent override
//
// Providers: reddit (public .json listing — no auth) and telegram (public
// channel preview at t.me/s/{handle}). ToS-respecting means no login
// scraping and no non-public endpoints — a provider not in
// intel.social.enabled_providers answers ErrAdapterDisabled.
type SocialFetcher struct {
	client   *http.Client
	settings AdapterSettings
}

// NewSocialFetcher returns a SocialFetcher. settings may be nil — the
// adapter then reports ErrAdapterDisabled for every fetch.
func NewSocialFetcher(client *http.Client, settings AdapterSettings) *SocialFetcher {
	if client == nil {
		client = NewSafeClient(30*time.Second, defaultMaxBodySize)
	}

	return &SocialFetcher{client: client, settings: settings}
}

// Fetch implements Fetcher.
func (f *SocialFetcher) Fetch(ctx context.Context, src sources.Source) (FetchResult, error) {
	var cfg map[string]any
	_ = json.Unmarshal(src.ProviderConfig, &cfg)

	provider := strings.ToLower(strings.TrimSpace(stringFromCfg(cfg, "provider")))
	if provider == "" {
		return FetchResult{}, errors.New("fetcher: social: provider_config.provider is empty")
	}

	if !f.providerEnabled(ctx, provider) {
		return FetchResult{}, fmt.Errorf("%w: social provider %q not in intel.social.enabled_providers", ErrAdapterDisabled, provider)
	}

	fetchURL := strings.TrimSpace(stringFromCfg(cfg, "endpoint_override"))
	if fetchURL == "" {
		handle := strings.Trim(strings.TrimSpace(stringFromCfg(cfg, "handle")), "@/")
		if handle == "" {
			return FetchResult{}, errors.New("fetcher: social: provider_config.handle is empty")
		}

		var err error

		fetchURL, err = socialEndpoint(provider, handle)
		if err != nil {
			return FetchResult{}, err
		}
	} else if err := validateSocialEndpointOverride(fetchURL); err != nil {
		return FetchResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: social request: %w", err)
	}

	ua := stringFromCfg(cfg, "ua")
	if ua == "" {
		ua = "JapaConnect-Intel/1.0 (+https://japa.to)"
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/html;q=0.9")

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: social %s: %w", provider, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(NewMaxBodyReader(resp.Body, defaultMaxBodySize))
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetcher: social read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("fetcher: social %s: status %d", provider, resp.StatusCode)
	}

	var items []byte

	switch provider {
	case "reddit":
		items, err = normalizeReddit(body)
	case "telegram":
		items, err = normalizeTelegram(body, fetchURL)
	default:
		err = fmt.Errorf("fetcher: social: unsupported provider %q", provider)
	}

	if err != nil {
		return FetchResult{}, err
	}

	return FetchResult{
		HTTPStatus:  resp.StatusCode,
		Headers:     map[string]string{"Content-Type": "application/x-ndjson"},
		ContentType: "application/x-ndjson",
		Body:        items,
		ContentHash: NormalizeForHash(items, "application/x-ndjson"),
	}, nil
}

// providerEnabled checks the intel.social.enabled_providers jsonb allowlist
// (system_settings; default [] — nothing enabled).
func (f *SocialFetcher) providerEnabled(ctx context.Context, provider string) bool {
	if f.settings == nil {
		return false
	}

	raw := f.settings.GetStringOr(ctx, "intel.social.enabled_providers", "[]")

	var providers []string
	if err := json.Unmarshal([]byte(raw), &providers); err != nil {
		return false
	}

	for _, p := range providers {
		if strings.EqualFold(strings.TrimSpace(p), provider) {
			return true
		}
	}

	return false
}

// socialEndpoint builds the provider's public listing URL for a handle.
func socialEndpoint(provider, handle string) (string, error) {
	switch provider {
	case "reddit":
		// r/<sub>, user/<name>, or bare name → subreddit guess is unsafe;
		// require an explicit r/ or user/ prefix. The listing path differs
		// per handle kind: subreddits expose /new.json while user profiles
		// only expose submitted/comments/overview — /user/{name}/new.json
		// is not a valid listing and would 404 every crawl.
		if strings.HasPrefix(handle, "user/") {
			return "https://www.reddit.com/" + handle + "/submitted.json?sort=new&limit=50", nil
		}

		if strings.HasPrefix(handle, "r/") {
			return "https://www.reddit.com/" + handle + "/new.json?limit=50", nil
		}

		return "", fmt.Errorf("fetcher: social: reddit handle %q must be r/<sub> or user/<name>", handle)
	case "telegram":
		return "https://t.me/s/" + url.PathEscape(handle), nil
	default:
		return "", fmt.Errorf("fetcher: social: unsupported provider %q", provider)
	}
}

// normalizeReddit flattens a Reddit listing JSON to JSON-lines.
func normalizeReddit(body []byte) ([]byte, error) {
	var listing struct {
		Data struct {
			Children []struct {
				Data struct {
					ID        string  `json:"id"`
					Title     string  `json:"title"`
					URL       string  `json:"url"`
					Permalink string  `json:"permalink"`
					Author    string  `json:"author"`
					Created   float64 `json:"created_utc"`
					Score     int     `json:"score"`
					Selftext  string  `json:"selftext"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("fetcher: reddit parse: %w", err)
	}

	var b strings.Builder

	for _, child := range listing.Data.Children {
		d := child.Data
		if d.ID == "" {
			continue
		}

		text := d.Selftext
		if len(text) > 500 {
			text = text[:500]
		}

		link := d.URL
		if link == "" && d.Permalink != "" {
			link = "https://www.reddit.com" + d.Permalink
		}

		line, _ := json.Marshal(map[string]any{
			"id":      d.ID,
			"title":   d.Title,
			"url":     link,
			"author":  d.Author,
			"created": time.Unix(int64(d.Created), 0).UTC().Format(time.RFC3339),
			"score":   d.Score,
			"text":    text,
		})
		b.Write(line)
		b.WriteByte('\n')
	}

	return []byte(b.String()), nil
}

// normalizeTelegram scrapes the public channel preview page
// (t.me/s/{handle}) into JSON-lines — one line per rendered message.
func normalizeTelegram(body []byte, pageURL string) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("fetcher: telegram parse: %w", err)
	}

	var b strings.Builder

	doc.Find(".tgme_widget_message").Each(func(_ int, msg *goquery.Selection) {
		link, _ := msg.Find(".tgme_widget_message_date").Attr("href")
		if link == "" {
			link = pageURL
		}

		text := strings.TrimSpace(msg.Find(".tgme_widget_message_text").Text())
		author := strings.TrimSpace(msg.Find(".tgme_widget_message_author").Text())
		date, _ := msg.Find("time").Attr("datetime")

		if text == "" && link == pageURL {
			return // skip empty chrome rows
		}

		line, _ := json.Marshal(map[string]any{
			"id":      link,
			"url":     link,
			"author":  author,
			"created": date,
			"text":    text,
		})
		b.Write(line)
		b.WriteByte('\n')
	})

	return []byte(b.String()), nil
}

// validateSocialEndpointOverride constrains provider_config.endpoint_override:
// http(s) only, no userinfo, no private-literal or numeric-IP hosts — on
// top of the SafeClient dial guard. Social overrides carry no credentials
// (unlike search), so loopback http is accepted for local test gateways;
// production channels use the built-in https provider endpoints.
func validateSocialEndpointOverride(endpoint string) error {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("fetcher: social endpoint_override must be an http(s) URL: %q", endpoint)
	}

	if u.User != nil {
		return errors.New("fetcher: social endpoint_override must not embed credentials")
	}

	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		if u.Scheme != "http" || !isLoopbackOrPrivateLiteral(host) {
			return fmt.Errorf("fetcher: social endpoint_override targets a private address: %q", endpoint)
		}
	}

	return nil
}
