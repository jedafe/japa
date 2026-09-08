# Video Content Type, Import, and Channels — Design Spec

> **Canonical JEOS:** `.jeos/sections/106-video-content-and-import.md` and `.jeos/adr/ADR-117-video-content-and-import.md`

---

## Goal

Add `video` as a first-class JapaConnect content type, allow users to import videos from YouTube / TikTok / Instagram Reels / any oEmbed platform by pasting a URL, and attribute the imported video to a synthetic user that represents the real external author. The synthetic user’s public profile becomes the **video channel**. Real authors can later claim or takedown their channel.

## Core model

```
contents (video post) ──video_source_id──► video_sources (canonical metadata)
       │                                      │
       │                                      │
   author = creator                      author = synthetic/real user
       │                                 (the channel)
       ▼                                      ▼
   user A's profile                   /u/[synthetic-username]
                                      with Videos tab
```

- **Video post** = `contents` row, `content_type = 'video'`, authored by the user who creates it. Body is TipTap JSON. `canonical_url` is the JapaConnect post URL.
- **Video source** = `video_sources` row with the external platform metadata: `source_url`, `title`, `description`, `category` (primary tag), `thumbnail`, `duration`, and `author_user_id`.
- **Channel** = the `users`/`user_profiles` row that owns `video_sources`. A synthetic user is auto-created on first import.

## Key user flows

### 1. Create a video post from `/create`

- User selects **Video** from the type selector (same dev.to `/new` shell as Blog).
- Two source modes: **Upload file** or **Import URL**.
- **Import URL:** paste a URL → backend resolves → auto-fills title, description, category, thumbnail. A synthetic user is created/updated for the real author.
- On publish, a `contents` row is created with `author_id = current_user`, `video_source_id = ...`, and `featured_image_id` = thumbnail.
- The post body contains a TipTap `video` block.

### 2. Embed a video in a Blog/Story post

- In the composer, user pastes a URL via the universal import command.
- The system resolves it and inserts a TipTap `video` block.
- The `video_sources` row is authored by the synthetic user; the Blog/Story `contents` row is authored by the user.
- The video appears on the synthetic user’s channel profile.

### 3. Bulk import a YouTube channel / playlist / keyword

- Authorized users (super_admin, assigned usergroup, real author) use the `/studio` page.
- Input: channel URL, playlist URL, or keyword.
- The `video:import_bulk` Asynq task fetches the list via YouTube Data API v3.
- For each video: create/update synthetic author, create `video_sources`, fetch thumbnail, create `contents` row.
- Phase 2 only. Phase 1 ships single import.

### 4. Claim or takedown a channel

- Real author joins and goes to the channel page.
- Submits a **claim** or **takedown** request via a Form Builder form (`video.claim_form_slug`).
- Admin reviews in the queue.
- **Approved claim:** `video_sources.author_user_id` is migrated to the real user; synthetic user is hidden/merged.
- **Approved takedown:** `video_sources.status` becomes `removed`; embeds show a takedown notice.

## Phased approach

| Phase | Scope |
|---|---|
| **Phase 1** | `video` content type, single universal import, synthetic channel, thumbnail-as-featured-image, video feed cards, `/create` integration. |
| **Phase 2** | Bulk YouTube import + `/studio` content management dashboard. |
| **Phase 3** | AI-generated drafts from video metadata (HITL-gated, optional). |

## Front-end surfaces

- `/create?type=video` — video composer (upload or import).
- `/video/[slug]` — single video post (embed + Watch on [Platform] link).
- `/u/[username]/videos` — channel videos tab.
- `/studio` (Phase 2) — content management + bulk import.

## Back-end overview

- New tables: `video_sources`, `video_channel_claims`, `video_import_jobs`.
- New columns: `contents.video_source_id`, `users.is_synthetic`, `users.synthetic_source`, `user_profiles.location`, `user_profiles.banner_media_id`.
- New Asynq tasks: `video:import_bulk`, `video:fetch_thumbnail`, `video:process_upload`, `video:generate_summary`.
- New RBAC nodes: `video.create`, `video.import.single`, `video.import.bulk`, `video.import.manage`, `video.channel.claim`, `video.channel.claim.review`.
- New system settings: `video.enabled`, `video.import.youtube_api_key`, `video.import.max_items_per_run`, `video.import.allowed_platforms`, `video.ai_summary.enabled`, `video.claim_form_slug`.

## Implementation note

TikTok and Instagram Reels lack stable public APIs. The resolver is built as a `VideoProvider` interface: YouTube gets full support (oEmbed + Data API); other platforms start with oEmbed/iframe and gain richer metadata via platform-specific adapters as they become available.

## See also

- `.jeos/sections/106-video-content-and-import.md` (full schema, routes, and spec)
- `.jeos/adr/ADR-117-video-content-and-import.md` (decisions and trade-offs)
