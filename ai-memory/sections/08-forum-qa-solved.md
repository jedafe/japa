# Section 08 — Forum Q&A / Solved

**Status:** ✅ Locked (restored post-audit — see ADR-029; the Solved-status line below survived reconstruction, everything else here did not and has been restored from earlier session material)

## Decided

**Structure:** Nested threads (parent/child replies, not a flat list), with a per-thread Solved status. Solved is also the data source for the QAPage structured-data markup decided in Section 26 — solved threads are eligible for Google's Q&A rich-result treatment, a close-to-free SEO benefit given the data already exists for this purpose.

**Edit window:** Posts and replies are editable for a configurable window after posting (admin-set duration), after which edits are still allowed but flagged as `edited` with a visible timestamp — never silently rewritten.

**Voting:** Forum threads and replies use upvote/downvote (`vote_score`), not emoji reactions — a quality/moderation signal that affects feed ranking. This is deliberately different from the rest of the platform's content (blog posts, events, etc.), which use emoji reactions instead — see Section 14's polymorphic reaction system. One vote per user per thread/reply, enforced the same way reviews enforce one-per-user (Section 29) — changing your vote updates the existing row rather than creating a second one.

**Tag system with moderators:** Tags are first-class entities, not free-text — each has a description, rules, optional cover image, a follower count, and an assignable moderator (or group of moderators). A tag's moderator(s) can be any trusted user or, per the platform's AI-agent permission model (Section 20), an AI agent scoped to that tag. Tag pages aggregate all content across content types carrying that tag, not forum-only.

**Polls:** A poll is an attachable structure on a thread (question, options, single/multi-select, optional close date), not its own content type — vote counts update live via the existing WebSocket `space_activity`/`feed_update` channels (Section 10) rather than requiring a page refresh.

**Anonymous posting:** A per-thread anonymous mode — the author's identity is withheld from other users but always visible to admins/moderators (never truly anonymous from a moderation standpoint). Anonymous threads are excluded from the author's public profile activity but remain attributable internally for abuse handling.

**Bookmarks:** `bookmark_count` on forum threads (and, per Section 14, on every other content type uniformly), with user-side folder organization for saved items — a "Visa Documents," "Canada Stuff"-style personal save-for-later system, not just a flat bookmarked list.

**Reply view counts (admin-switchable):** `reply_views` join table (`reply_id`, `user_id`, `viewed_at`) with a natural per-user dedup via primary key — a DB trigger increments `forum_replies.view_count` on insert. Mode can be switched off; switching off archives `reply_views` rows (via `maintenance:archive_reply_views`, one-time on toggle) rather than deleting them, and the plain `view_count` column becomes the sole source of truth again. The API always returns `view_count` regardless of which mode is active — the frontend never needs to know.

## See Also

ADR-029. Section 10 (WebSocket events for live poll/vote updates), Section 14 (polymorphic reactions/bookmarks for non-forum content), Section 20 (AI agents as tag moderators), Section 26 (QAPage structured data), Section 29 (Ratings & Reviews — same one-vote-per-user enforcement pattern).

