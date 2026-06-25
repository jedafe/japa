# Section 15 — Block Editor (TipTap)

**Status:** ✅ Locked

## Decided
ProseMirror JSON storage, `@tiptap/html` SSR rendering on Bun, Redis-cached output keyed by `content_id:updated_at` (no explicit invalidation needed — see `ai-memory/skills/valkey-cache-key-versioning.md`). Extension list: Heading, Bold, Italic, Strike, Underline, lists, Blockquote, CodeBlock with syntax highlighting, Link, Image, HorizontalRule, Placeholder, Embed. Table extension deliberately excluded — the Generic Comparison System covers structured tabular needs.

Trigger mapping (corrected mid-session, see ADR notes): `/` slash commands, `@` user mentions, `#` tags, `[[` content/space interlinking. All reference nodes store `{ id, entity_type, label }`, never raw text — render-time resolves current state, falling back to a "deleted content" placeholder if the referenced entity is gone.

**Mention edge cases (restored post-audit, see ADR-029):** a mentioned user with DM privacy set to `nobody` (Section 10) still receives the mention notification — mentions bypass DM privacy since a mention isn't a new conversation. If the post author has blocked the mentioned user (or vice versa), no notification is sent. If the mentioned user's account has been erased (Section 19), the mention node renders as `@[deleted user]` with no link. A mention inside a private Space's post only notifies the mentioned user if they're a member of that Space — mentions never leak visibility into content a user wouldn't otherwise be permitted to see.

Media referenced by `media_id` pointing at `content_media_fields`, never a raw URL. Internal link previews render from local data; external links get an async OG/oEmbed scrape (`link_preview_fetch` task) with baseline SSRF protection (blocking private IP ranges) regardless of the admin blacklist setting, which defaults to blacklist-mode (block specific bad domains) rather than strict whitelist.

## See Also
ADR-006, ADR-009, ADR-029.
