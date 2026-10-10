package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitobject "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// GitFetcher is the Phase-2 git adapter (channel design §3): it shallow-
// clones a repository into memory and turns its tracked state into a raw
// document — either the configured file contents (via:"files") or a tag
// manifest (via:"tags"). go-git speaks its own transport, not the shared
// safetransport — SSRF protection is the host check below (private/loopback
// literals are rejected before any connection; DNS-rebinding between the
// check and go-git's own resolution is a residual risk accepted for
// operator/admin-configured channels only).
//
// provider_config keys:
//
//	{"repo":   "https://github.com/org/repo.git", // defaults to entry_url
//	 "branch": "main",                            // default HEAD branch
//	 "via":    "files|tags",                      // default "files"
//	 "paths":  ["data/visa-fees.csv", "docs/"],   // files mode; empty = all
//	 "max_file_bytes": 262144,                    // per-file cap (def 256KB)
//	 "max_files": 200}                            // files-mode cap
//
// files body: one JSON line per tracked file {"path","sha256","content"} —
// the hash covers the whole file set so a commit touching any path is a
// change. tags body: "tag\tsha" manifest lines (sorted), so a new release
// tag is a manifest change.
type GitFetcher struct{}

// NewGitFetcher returns a GitFetcher.
func NewGitFetcher() *GitFetcher { return &GitFetcher{} }

const (
	gitDefaultMaxFileBytes = 256 * 1024
	gitDefaultMaxFiles     = 200
)

// Fetch implements Fetcher.
func (f *GitFetcher) Fetch(ctx context.Context, src sources.Source) (FetchResult, error) {
	var cfg map[string]any
	_ = json.Unmarshal(src.ProviderConfig, &cfg)

	repoURL := strings.TrimSpace(stringFromCfg(cfg, "repo"))
	if repoURL == "" {
		repoURL = strings.TrimSpace(src.SourceUrl)
	}
	if err := validateGitRemote(repoURL); err != nil {
		return FetchResult{}, err
	}

	via := strings.ToLower(strings.TrimSpace(stringFromCfg(cfg, "via")))
	if via == "releases" {
		// GitHub-style releases ride on tags for our purposes — a release
		// without a tag has no git identity anyway. The releases API is
		// out of Phase-2 scope.
		slog.Warn("fetcher: git: via=releases not supported, falling back to tags", "repo", repoURL)
		via = "tags"
	}
	if via == "" {
		via = "files"
	}

	var body []byte
	var err error
	switch via {
	case "tags":
		body, err = f.fetchTags(ctx, repoURL)
	case "files":
		body, err = f.fetchFiles(ctx, repoURL, cfg)
	default:
		err = fmt.Errorf("fetcher: git: unknown via %q", via)
	}
	if err != nil {
		return FetchResult{}, err
	}

	return FetchResult{
		HTTPStatus:  200,
		Headers:     map[string]string{"Content-Type": "application/x-ndjson"},
		ContentType: "application/x-ndjson",
		Body:        body,
		ContentHash: NormalizeForHash(body, "application/x-ndjson"),
	}, nil
}

// validateGitRemote rejects non-http(s) schemes, embedded userinfo, and
// private-literal hosts — the insert-time SSRF guard since go-git does its
// own dial (no SafeTransport Control hook). Hostnames that resolve to
// private IPs at clone time remain a residual: go-git cannot take a custom
// dialer, so operator-supplied repo URLs are trusted past the literal
// check. Alt-IP decimal/hex/octal forms bypass net.ParseIP and are
// normalized before the literal check.
func validateGitRemote(repoURL string) error {
	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("fetcher: git: repo %q must be an http(s) URL", repoURL)
	}
	if u.User != nil {
		return fmt.Errorf("fetcher: git: repo URL must not embed credentials")
	}
	host := u.Hostname()
	if sources.IsPrivateIPLiteral(host) || isNumericIPPrivate(host) {
		return fmt.Errorf("fetcher: git: repo host %q is a private IP", host)
	}
	return nil
}

// isNumericIPPrivate catches decimal/hex/octal IPv4 forms (2130706433,
// 0x7f.0.0.1, 0177.0.0.1) that net.ParseIP rejects but the dialer accepts.
func isNumericIPPrivate(host string) bool {
	ip := parseNumericIPv4(host)
	if ip == nil {
		return false
	}
	return sources.IsPrivateIP(ip)
}

// parseNumericIPv4 parses decimal/hex/octal IPv4 forms into a net.IP.
// Returns nil when the host is not a numeric IPv4 form.
func parseNumericIPv4(host string) net.IP {
	if host == "" || strings.Contains(host, ":") {
		return nil
	}

	parts := strings.Split(host, ".")
	if len(parts) != 1 && len(parts) != 4 {
		return nil
	}

	parsePart := func(s string, bits int) (uint64, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}
		base := 10
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			base = 16
			s = s[2:]
		} else if len(s) > 1 && strings.HasPrefix(s, "0") {
			base = 8
			s = s[1:]
		}
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseUint(s, base, bits)
		if err != nil {
			return 0, false
		}
		return n, true
	}

	if len(parts) == 1 {
		n, ok := parsePart(parts[0], 32)
		if !ok {
			return nil
		}
		return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}

	var b [4]byte
	for i, p := range parts {
		n, ok := parsePart(p, 8)
		if !ok {
			return nil
		}
		b[i] = byte(n)
	}
	return net.IPv4(b[0], b[1], b[2], b[3])
}

// fetchTags lists remote tags without cloning — ls-remote equivalent.
func (f *GitFetcher) fetchTags(ctx context.Context, repoURL string) ([]byte, error) {
	rem := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{Name: "origin", URLs: []string{repoURL}})

	refs, err := rem.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetcher: git ls-remote: %w", err)
	}

	var lines []string
	for _, ref := range refs {
		if !ref.Name().IsTag() {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\t%s", ref.Name().Short(), ref.Hash().String()))
	}
	sort.Strings(lines)

	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// fetchFiles shallow-clones the repo into memory (depth 1, single branch,
// no tags) and emits one JSON line per tracked file matching the configured
// path prefixes.
func (f *GitFetcher) fetchFiles(ctx context.Context, repoURL string, cfg map[string]any) ([]byte, error) {
	branch := strings.TrimSpace(stringFromCfg(cfg, "branch"))
	maxFileBytes := intFromCfg(cfg, "max_file_bytes", gitDefaultMaxFileBytes)
	maxFiles := intFromCfg(cfg, "max_files", gitDefaultMaxFiles)

	opts := &git.CloneOptions{
		URL:           repoURL,
		Depth:         1,
		SingleBranch:  true,
		Tags:          git.NoTags,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
	}
	if branch == "" {
		opts.ReferenceName = "" // remote HEAD
	}

	repo, err := git.CloneContext(ctx, memory.NewStorage(), memfs.New(), opts)
	if err != nil {
		return nil, fmt.Errorf("fetcher: git clone: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("fetcher: git head: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("fetcher: git commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("fetcher: git tree: %w", err)
	}

	paths := stringSliceCfg(cfg, "paths")
	maxFileBytes64 := int64(maxFileBytes)

	var b strings.Builder
	count := 0
	err = tree.Files().ForEach(func(file *gitobject.File) error {
		if count >= maxFiles {
			return errGitStopWalk
		}
		if len(paths) > 0 && !gitPathMatch(file.Name, paths) {
			return nil
		}
		if file.Size > maxFileBytes64 {
			return nil
		}
		content, rerr := file.Contents()
		if rerr != nil {
			return nil // unreadable blob is not fatal
		}
		sum := sha256.Sum256([]byte(content))
		line, _ := json.Marshal(map[string]any{
			"path":    file.Name,
			"sha256":  hex.EncodeToString(sum[:]),
			"content": content,
		})
		b.Write(line)
		b.WriteByte('\n')
		count++
		return nil
	})
	if err != nil && !errors.Is(err, errGitStopWalk) {
		return nil, fmt.Errorf("fetcher: git walk: %w", err)
	}

	return []byte(b.String()), nil
}

var errGitStopWalk = fmt.Errorf("git: file cap reached")

// gitPathMatch reports whether name is under any configured path prefix
// (directory prefix or exact file match).
func gitPathMatch(name string, prefixes []string) bool {
	for _, p := range prefixes {
		p = strings.TrimPrefix(strings.TrimSpace(p), "/")
		if p == "" {
			continue
		}
		if name == p || strings.HasPrefix(name, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

// stringSliceCfg reads a []string provider_config value.
func stringSliceCfg(cfg map[string]any, key string) []string {
	raw, ok := cfg[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
