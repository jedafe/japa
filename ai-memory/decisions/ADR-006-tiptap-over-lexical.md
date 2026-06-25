# ADR-006: TipTap Over Lexical for the Block Editor

**Status:** Accepted

**Context:** Needed a rich text editor for forum posts, blog posts, and pages, with custom node support for mentions, tags, and interlinking, server-side rendering on Bun.

**Decision:** TipTap, storing ProseMirror JSON, rendered via `@tiptap/html` on the Bun SSR server, cached in Redis.

**Rationale:** TipTap's extension model fits the custom node requirements (mention, interlink, tag) cleanly, and `@tiptap/html` gives a clean server-side rendering path without needing a browser-context renderer.

**Consequences:** Page Builder's Rich Text block embeds TipTap rather than building a second rich-text system — see ADR-011. The editor is also the only place rich text gets authored; Page Builder is layout, not a competing editor.
