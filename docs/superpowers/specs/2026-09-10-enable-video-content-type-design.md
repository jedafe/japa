# Enable the Video Content Type by Default

**Date:** 2026-09-10  
**Scope:** Configuration / feature activation  
**Approach:** A (catalog default + service fallback + migration flip)  
**Refs:** .jeos/sections/106-video-content-and-import.md, .jeos/conventions/system-settings-catalog.md

## Background

Section 106 / ADR-117 introduced `video` as a first-class content type. All schema, API routes, handlers, and frontend composer wiring are in place, but the feature is gated by the public system setting `video.enabled`, which defaults to `false` in the settings catalog and was seeded as `false` for existing tenants. The system has a single tenant and the owner wants the video content type activated now and by default for any future tenants.

## Design

1. **Catalog default flip**  
   Change `video.enabled` in `internal/control/settings_catalog.go` from `DefaultValue: "false"` to `DefaultValue: "true"`. This makes the canonical default match the desired active state and keeps the admin panel and JEOS in sync.

2. **Service fallback flip**  
   Change the `GetBoolOr` fallback in `internal/video/service.go` from `false` to `true`. This ensures any tenant whose `system_settings` row is missing falls back to the same active default.

3. **Existing-tenant migration**  
   Add migration `000649_enable_video_content_type.up.sql` to flip any `system_settings` rows with `key = 'video.enabled'` and `value = 'false'::jsonb` to `'true'::jsonb`. The corresponding `.down.sql` flips them back to `false` for rollback. This activates the feature for the current single tenant immediately.

4. **JEOS sync**  
   Update `.jeos/conventions/system-settings-catalog.md` and `.jeos/sections/106-video-content-and-import.md` to list `video.enabled` default as `true`.

## Verification

- Run backend tests for `internal/video` to confirm the fallback change does not break unit tests.
- Run `make sqlc-gen` to ensure no generated files are hand-edited.
- Check that the new migration does not conflict with existing `video.enabled` rows and that `golang-migrate` status is clean.

## Files to Change

- `internal/control/settings_catalog.go`
- `internal/video/service.go`
- `.jeos/conventions/system-settings-catalog.md`
- `.jeos/sections/106-video-content-and-import.md`
- `migrations/000649_enable_video_content_type.up.sql`
- `migrations/000649_enable_video_content_type.down.sql`
