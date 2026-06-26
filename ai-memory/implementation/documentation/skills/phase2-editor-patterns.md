# Phase 2 Editor Patterns

**Date:** 2026-06-25
**Phase:** 2
**Purpose:** Implementation patterns learned during Section 15 (Block Editor TipTap), reusable in future sections.

## Pattern: SSR Rendering via Bun Subprocess (TipTap JSON to HTML)

The Go backend renders TipTap JSON to HTML by shelling out to a Bun script rather
than embedding a JavaScript runtime. The Bun script imports TipTap core and
extensions, accepts JSON on stdin, and writes HTML to stdout. Valkey caching
amortizes subprocess overhead by keying on a content hash:

```go
type Renderer struct {
    cache *valkey.Client
    bunPath string
}

func (r *Renderer) Render(ctx context.Context, jsonContent []byte) (string, error) {
    cacheKey := "jc:render:" + sha256hex(jsonContent)
    if cached, err := r.cache.Get(ctx, cacheKey).Result(); err == nil {
        return cached, nil
    }
    cmd := exec.CommandContext(ctx, r.bunPath, "run", "scripts/tiptap-ssr.ts")
    cmd.Stdin = bytes.NewReader(jsonContent)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("rendering tip tap html: %w", err)
    }
    html := stdout.String()
    r.cache.Set(ctx, cacheKey, html, 15*time.Minute)
    return html, nil
}
```

This keeps the Go binary lean and lets the TipTap extension set evolve
independently. The Bun script is a standalone file that can be updated without
recompiling Go. If the Bun binary is not found at runtime, the renderer returns
an error that the handler maps to 503 — graceful degradation for environments
where Bun is not installed.

## Pattern: SSRF Protection (Private IP Blocking + DNS Resolution Check)

The link preview fetcher must fetch arbitrary URLs submitted by users. This is a
classic SSRF attack surface — an attacker could submit `http://169.254.169.254/latest/meta-data/`
to reach cloud metadata, or `http://internal-service:8080/admin` to scan the
internal network. The protection has three layers:

```go
// Layer 1: Block known dangerous domains
func IsBlockedDomain(host string) bool {
    blocked := []string{
        "169.254.169.254",  // AWS/GCP cloud metadata
        "metadata.google.internal",
        "localhost",
        // ...
    }
    for _, b := range blocked {
        if host == b || strings.HasSuffix(host, "."+b) {
            return true
        }
    }
    return false
}

// Layer 2: Resolve DNS and check every IP
func ValidateURL(rawURL string) (*url.URL, error) {
    u, err := url.Parse(rawURL)
    if err != nil { return nil, err }
    if IsBlockedDomain(u.Hostname()) {
        return nil, fmt.Errorf("blocked domain: %s", u.Hostname())
    }
    ips, err := net.LookupIP(u.Hostname())
    if err != nil { return nil, fmt.Errorf("resolving host: %w", err) }
    for _, ip := range ips {
        if IsPrivateIP(ip) {
            return nil, fmt.Errorf("private IP resolved: %s", ip)
        }
    }
    return u, nil
}

// Layer 3: Check against 17 private/reserved CIDR ranges
var privateRanges []*net.IPNet
func init() {
    cidrs := []string{
        "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
        "127.0.0.0/8", "169.254.0.0/16", "100.64.0.0/10",
        // ... 17 total ranges covering IPv4 and IPv6
    }
    for _, cidr := range cidrs {
        _, network, _ := net.ParseCIDR(cidr)
        privateRanges = append(privateRanges, network)
    }
}

func IsPrivateIP(ip net.IP) bool {
    for _, network := range privateRanges {
        if network.Contains(ip) {
            return true
        }
    }
    return false
}
```

The DNS resolution check is critical — blocking by hostname alone is defeated by
DNS rebinding, where an attacker controls a domain that first resolves to a
public IP (passing the check) and then resolves to a private IP when the actual
connection is made. By checking the resolved IPs before connecting, the fetcher
closes this window.

## Pattern: Mention Filtering with Pluggable Checker Interfaces

The mentions package extracts `@` mentions from TipTap JSON and filters them
before notification dispatch. Three cross-section dependencies (blocking, space
membership, user status) are abstracted behind interfaces so the package compiles
and tests without those sections being built:

```go
type BlockChecker interface {
    IsBlocked(ctx context.Context, mentionerID, mentionedID uuid.UUID) (bool, error)
}

type SpaceMemberChecker interface {
    IsMember(ctx context.Context, spaceID, userID uuid.UUID) (bool, error)
}

type UserStatusChecker interface {
    IsActive(ctx context.Context, userID uuid.UUID) (bool, error)
}

func FilterMentions(ctx context.Context, mentions []Mention,
    blocker BlockChecker, memberChecker SpaceMemberChecker,
    statusChecker UserStatusChecker) ([]Mention, error) {
    var filtered []Mention
    for _, m := range mentions {
        blocked, _ := blocker.IsBlocked(ctx, m.MentionerID, m.MentionedID)
        if blocked { continue }
        active, _ := statusChecker.IsActive(ctx, m.MentionedID)
        if !active { continue }
        // Space membership check is optional (some mentions are not space-scoped)
        if m.SpaceID != uuid.Nil {
            member, _ := memberChecker.IsMember(ctx, m.SpaceID, m.MentionedID)
            if !member { continue }
        }
        filtered = append(filtered, m)
    }
    return filtered, nil
}
```

Noop stubs provide permissive defaults until the real implementations are wired:

```go
type NoopBlockChecker struct{}
func (NoopBlockChecker) IsBlocked(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
    return false, nil  // nothing blocked until Section 10
}
```

This pattern — interfaces for cross-section dependencies with noop stubs — is
reusable anywhere a package needs to compile before its dependencies exist. The
real implementations are swapped in by changing the constructor call, not the
interface or the filtering logic.

## Pattern: Link Preview with OG Tag Extraction

The link preview fetcher performs an HTTP GET on a user-submitted URL and
extracts Open Graph metadata from the HTML response. Rather than pulling in a
full HTML parser dependency, a lightweight byte-scanning walker extracts the
four OG tags needed for preview cards:

```go
type Preview struct {
    Title       string `json:"title"`
    Description string `json:"description"`
    Image       string `json:"image"`
    URL         string `json:"url"`
}

func parseOGTags(html []byte) Preview {
    var p Preview
    targets := map[string]*string{
        "og:title":       &p.Title,
        "og:description": &p.Description,
        "og:image":       &p.Image,
        "og:url":         &p.URL,
    }
    // Scan for <meta property="og:..." content="..."> patterns
    // using bytes.Index and bytes.IndexByte — no DOM parser needed
    // for these four well-structured tags
    return p
}
```

The result is cached in Valkey keyed by SHA-256 of the URL. Cache hits return
instantly without an HTTP call. The fetch uses a short timeout (5 seconds) to
prevent slow external sites from blocking the request. If the fetch fails or the
HTML contains no OG tags, the handler returns a minimal preview with just the
URL — the frontend falls back to a plain link display.

## Pattern: Svelte 5 + TipTap Custom Wrapper

No `@tiptap/svelte` package exists — TipTap ships official integrations for
React, Vue, and Angular, but not Svelte. The solution is a custom wrapper using
Svelte 5 runes to manage the editor lifecycle:

```svelte
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Editor } from '@tiptap/core';
  import StarterKit from '@tiptap/starter-kit';

  let { content = '', extensions = [], onUpdate }: Props = $props();
  let editor: Editor | null = $state(null);
  let editorElement: HTMLDivElement | null = $state(null);

  $effect(() => {
    if (editorElement && !editor) {
      editor = new Editor({
        element: editorElement,
        extensions: [StarterKit, ...extensions],
        content: content,
        onUpdate: ({ editor: e }) => {
          onUpdate?.(e.getJSON());
        },
      });
    }
    return () => {
      editor?.destroy();
      editor = null;
    };
  });
</script>

<div bind:this={editorElement}></div>
```

Key points:

- `$state` for the editor instance and DOM element reference — Svelte 5's
  fine-grained reactivity ensures the editor is created exactly once and
  destroyed on unmount
- `$effect` return function handles cleanup — no separate `onDestroy` needed
- `$props()` receives initial content and extension configuration from the
  parent component
- The `onUpdate` callback emits TipTap JSON on every transaction, enabling
  two-way binding with the parent form

This pattern is reusable for any TipTap-based editor in the codebase — the
wrapper is generic enough that Section 16 (Page Builder) can compose multiple
instances with different extension sets.

## Pattern: Popup Components (Keyboard Navigation + Fixed Positioning)

The mention, tag, interlink, and slash-command popups all follow the same
pattern: detect a trigger character, show a fixed-position popup near the cursor,
navigate with keyboard, and insert a node on selection. The shared structure:

```svelte
<script lang="ts">
  let { items = [], onSelect }: PopupProps = $props();
  let selectedIndex = $state(0);
  let popupElement: HTMLDivElement | null = $state(null);

  // Keyboard navigation
  function handleKeydown(event: KeyboardEvent) {
    switch (event.key) {
      case 'ArrowDown':
        selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
        event.preventDefault();
        break;
      case 'ArrowUp':
        selectedIndex = Math.max(selectedIndex - 1, 0);
        event.preventDefault();
        break;
      case 'Enter':
        onSelect?.(items[selectedIndex]);
        event.preventDefault();
        break;
      case 'Escape':
        // Close popup
        break;
    }
  }

  // Scroll selected item into view
  $effect(() => {
    const selected = popupElement?.querySelector('[data-selected]');
    selected?.scrollIntoView({ block: 'nearest' });
  });
</script>

<div bind:this={popupElement} class="fixed z-50 bg-white dark:bg-gray-900 ...">
  {#each items as item, i}
    <button
      data-selected={i === selectedIndex ? '' : undefined}
      onclick={() => onSelect?.(item)}
      class:selected={i === selectedIndex}
    >
      {item.label}
    </button>
  {/each}
</div>
```

Key points:

- Fixed positioning with `z-50` — the popup floats above the editor content
- Keyboard navigation via `ArrowDown`, `ArrowUp`, `Enter`, `Escape` — standard
  accessible popup behavior
- `$effect` scrolls the selected item into view when `selectedIndex` changes
- The popup receives its items as props — the parent component (Editor.svelte)
  is responsible for filtering items based on the typed query
- Mock data in the popup components — real API integration (searching users,
  tags, content) is deferred to Section 45

This pattern is consistent across all four popup types, making it easy to add
new trigger-based popups (e.g., emoji picker on `:`) by reusing the same
structure with different item types and selection handlers.

## Lesson: Always Check for Official Framework Integrations Before Building Custom Wrappers

TipTap does not ship a Svelte integration. Before building a custom wrapper,
check the TipTap documentation and npm registry for `@tiptap/svelte` or
community packages. If none exists, the custom wrapper approach using `$effect`
and `$state` is the correct pattern — but be aware that you are responsible for
lifecycle management (create on mount, destroy on unmount) and extension
configuration, which the official integrations handle automatically for React
and Vue.

## Lesson: SSRF Protection Must Check Resolved IPs, Not Just Hostnames

Blocking by hostname alone is insufficient — DNS rebinding attacks can bypass
hostname checks by resolving to different IPs at different times. Always resolve
DNS before connecting and check every resolved IP against the private range list.
This adds a DNS lookup to every fetch, but the latency is negligible compared to
the HTTP fetch itself, and the security benefit is significant. Include cloud
metadata endpoints (169.254.169.254) in the blocklist even though they are
link-local — they are not in the standard RFC 1918 private ranges but are a
common SSRF target in cloud environments.
