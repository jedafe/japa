// Package ai — tools_academy.go implements the Japa Academy tool handlers
// for the AI Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.02):
//
//   - academy.tracks.list     — list learning tracks
//   - academy.tracks.create    — create a track (admin, HITL)
//   - academy.tracks.update    — update a track
//   - academy.lessons.list     — list lessons
//   - academy.lessons.create   — create a lesson in a module (admin, HITL)
//   - academy.lessons.update   — update a lesson
//   - academy.lessons.reorder  — reorder lessons within a track
//
// Academy operations are on academy.Store, not a Service. Lesson creation
// requires a moduleID — lessons attach to modules, not directly to tracks.
// This is preserved in the handler schema (module_id is required for
// academy.lessons.create).
//
// Privacy: academy data is not user data and has no ai_exempt column, so
// no PrivacyTargetFunc is registered. The executor's RBAC gate enforces
// the required_permission on each tool.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Academy tools)
// Ref: japaconnect-engineering/sections/36-academy-platform.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.02)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// academy.tracks.list (task 9.02a)
// ---------------------------------------------------------------------------

// AcademyTracksListHandler returns a ToolHandler that lists all academy
// tracks for the platform (including drafts — the agent needs to see the
// full catalog for admin operations).
func AcademyTracksListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		tracks, err := deps.Academy.ListTracks(ctx)
		if err != nil {
			return errorToolResult("learn_tracks_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(tracks))
		for _, t := range tracks {
			results = append(results, learnTrackToMap(t))
		}

		return successToolResult(map[string]any{
			"tracks": results,
			"total":  len(results),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.tracks.create (task 9.02b)
// ---------------------------------------------------------------------------

// learnTrackCreateArgs is the LLM-produced argument shape for
// academy.tracks.create.
type learnTrackCreateArgs struct {
	Title                string  `json:"title"`
	Slug                 string  `json:"slug"`
	Description          *string `json:"description,omitempty"`
	Difficulty           string  `json:"difficulty,omitempty"`
	CoverMediaID         string  `json:"cover_media_id,omitempty"`
	IsPublished          bool    `json:"is_published,omitempty"`
	SortOrder            int32   `json:"sort_order,omitempty"`
	Category             *string `json:"category,omitempty"`
	SubscriptionEligible bool    `json:"subscription_eligible,omitempty"`
}

// AcademyTracksCreateHandler returns a ToolHandler that creates an academy
// track. The executor's RBAC gate checks the required_permission
// (academy.admin.manage); the tool config sets requires_hitl = true.
func AcademyTracksCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		var a learnTrackCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		if a.Slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}

		difficulty := a.Difficulty
		if difficulty == "" {
			difficulty = "beginner"
		}

		coverMediaID, err := optionalUUID(a.CoverMediaID)
		if err != nil {
			return errorToolResult("invalid_cover_media_id", "cover_media_id is not a valid UUID"), nil
		}

		track, err := deps.Academy.CreateTrack(ctx, AcademyCreateTrackInput{
			Title:                a.Title,
			Slug:                 a.Slug,
			Description:          a.Description,
			Difficulty:           difficulty,
			CoverMediaID:         coverMediaID,
			IsPublished:          a.IsPublished,
			SortOrder:            a.SortOrder,
			Category:             a.Category,
			SubscriptionEligible: a.SubscriptionEligible,
		})
		if err != nil {
			return errorToolResult("learn_track_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"track_id":     uuidString(track.ID),
			"title":        track.Title,
			"slug":         track.Slug,
			"difficulty":   track.Difficulty,
			"is_published": track.IsPublished,
			"created_at":   timestampString(track.CreatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.tracks.update (task 9.02c)
// ---------------------------------------------------------------------------

// learnTrackUpdateArgs is the LLM-produced argument shape for
// academy.tracks.update.
type learnTrackUpdateArgs struct {
	TrackID              string  `json:"track_id"`
	Title                string  `json:"title"`
	Description          *string `json:"description,omitempty"`
	Difficulty           string  `json:"difficulty,omitempty"`
	CoverMediaID         string  `json:"cover_media_id,omitempty"`
	IsPublished          bool    `json:"is_published,omitempty"`
	SortOrder            int32   `json:"sort_order,omitempty"`
	Category             *string `json:"category,omitempty"`
	SubscriptionEligible *bool   `json:"subscription_eligible,omitempty"`
}

// AcademyTracksUpdateHandler returns a ToolHandler that updates an academy
// track.
func AcademyTracksUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		var a learnTrackUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		trackID, err := scanUUID(a.TrackID)
		if err != nil {
			return errorToolResult("invalid_track_id", "track_id is not a valid UUID"), nil
		}

		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		difficulty := a.Difficulty
		if difficulty == "" {
			difficulty = "beginner"
		}

		coverMediaID, err := optionalUUID(a.CoverMediaID)
		if err != nil {
			return errorToolResult("invalid_cover_media_id", "cover_media_id is not a valid UUID"), nil
		}

		track, err := deps.Academy.UpdateTrack(ctx, trackID, AcademyUpdateTrackInput{
			Title:                a.Title,
			Description:          a.Description,
			Difficulty:           difficulty,
			CoverMediaID:         coverMediaID,
			IsPublished:          a.IsPublished,
			SortOrder:            a.SortOrder,
			Category:             a.Category,
			SubscriptionEligible: a.SubscriptionEligible,
		})
		if err != nil {
			return errorToolResult("learn_track_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"track_id":     uuidString(track.ID),
			"title":        track.Title,
			"slug":         track.Slug,
			"is_published": track.IsPublished,
			"updated_at":   timestampString(track.UpdatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.lessons.list (task 9.02d)
// ---------------------------------------------------------------------------

// AcademyLessonsListHandler returns a ToolHandler that lists all lessons for
// the platform (admin view, including drafts).
func AcademyLessonsListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		lessons, err := deps.Academy.ListLessons(ctx)
		if err != nil {
			return errorToolResult("learn_lessons_list_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(lessons))
		for _, l := range lessons {
			results = append(results, learnLessonToMap(l))
		}

		return successToolResult(map[string]any{
			"lessons": results,
			"total":   len(results),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.lessons.create (task 9.02e)
// ---------------------------------------------------------------------------

// learnLessonCreateArgs is the LLM-produced argument shape for
// academy.lessons.create. module_id is required — lessons attach to modules,
// not directly to tracks.
type learnLessonCreateArgs struct {
	ModuleID        string  `json:"module_id"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	LessonType      string  `json:"lesson_type,omitempty"`
	Content         *string `json:"content,omitempty"`
	MediaID         string  `json:"media_id,omitempty"`
	DurationMinutes *int32  `json:"duration_minutes,omitempty"`
	SortOrder       int32   `json:"sort_order,omitempty"`
	IsPublished     bool    `json:"is_published,omitempty"`
	IsPreview       bool    `json:"is_preview,omitempty"`
}

// AcademyLessonsCreateHandler returns a ToolHandler that creates a lesson in
// a module. The executor's RBAC gate checks the required_permission
// (academy.admin.manage); the tool config sets requires_hitl = true.
func AcademyLessonsCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		var a learnLessonCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		if a.Slug == "" {
			return errorToolResult("missing_slug", "slug is required"), nil
		}

		moduleID, err := scanUUID(a.ModuleID)
		if err != nil || !moduleID.Valid {
			return errorToolResult("missing_or_invalid_module_id", "module_id is required and must be a valid UUID"), nil
		}

		lessonType := a.LessonType
		if lessonType == "" {
			lessonType = "text"
		}

		mediaID, err := optionalUUID(a.MediaID)
		if err != nil {
			return errorToolResult("invalid_media_id", "media_id is not a valid UUID"), nil
		}

		lesson, err := deps.Academy.CreateLesson(ctx, moduleID, AcademyCreateLessonInput{
			Title:           a.Title,
			Slug:            a.Slug,
			LessonType:      lessonType,
			Content:         a.Content,
			MediaID:         mediaID,
			DurationMinutes: a.DurationMinutes,
			SortOrder:       a.SortOrder,
			IsPublished:     a.IsPublished,
			IsPreview:       a.IsPreview,
		})
		if err != nil {
			return errorToolResult("learn_lesson_create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"lesson_id":    uuidString(lesson.ID),
			"module_id":    uuidString(lesson.ModuleID),
			"title":        lesson.Title,
			"slug":         lesson.Slug,
			"lesson_type":  lesson.LessonType,
			"is_published": lesson.IsPublished,
			"created_at":   timestampString(lesson.CreatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.lessons.update (task 9.02f)
// ---------------------------------------------------------------------------

// learnLessonUpdateArgs is the LLM-produced argument shape for
// academy.lessons.update.
type learnLessonUpdateArgs struct {
	LessonID        string  `json:"lesson_id"`
	Title           string  `json:"title"`
	LessonType      string  `json:"lesson_type,omitempty"`
	Content         *string `json:"content,omitempty"`
	MediaID         string  `json:"media_id,omitempty"`
	DurationMinutes *int32  `json:"duration_minutes,omitempty"`
	SortOrder       int32   `json:"sort_order,omitempty"`
	IsPublished     bool    `json:"is_published,omitempty"`
	IsPreview       *bool   `json:"is_preview,omitempty"`
}

// AcademyLessonsUpdateHandler returns a ToolHandler that updates a lesson.
func AcademyLessonsUpdateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		var a learnLessonUpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		lessonID, err := scanUUID(a.LessonID)
		if err != nil {
			return errorToolResult("invalid_lesson_id", "lesson_id is not a valid UUID"), nil
		}

		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}

		lessonType := a.LessonType
		if lessonType == "" {
			lessonType = "text"
		}

		mediaID, err := optionalUUID(a.MediaID)
		if err != nil {
			return errorToolResult("invalid_media_id", "media_id is not a valid UUID"), nil
		}

		lesson, err := deps.Academy.UpdateLesson(ctx, lessonID, AcademyUpdateLessonInput{
			Title:           a.Title,
			LessonType:      lessonType,
			Content:         a.Content,
			MediaID:         mediaID,
			DurationMinutes: a.DurationMinutes,
			SortOrder:       a.SortOrder,
			IsPublished:     a.IsPublished,
			IsPreview:       a.IsPreview,
		})
		if err != nil {
			return errorToolResult("learn_lesson_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"lesson_id":    uuidString(lesson.ID),
			"title":        lesson.Title,
			"slug":         lesson.Slug,
			"is_published": lesson.IsPublished,
			"updated_at":   timestampString(lesson.UpdatedAt),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// academy.lessons.reorder (task 9.02g)
// ---------------------------------------------------------------------------

// learnLessonReorderArgs is the LLM-produced argument shape for
// academy.lessons.reorder.
type learnLessonReorderArgs struct {
	Entries []learnLessonReorderEntry `json:"entries"`
}

type learnLessonReorderEntry struct {
	LessonID  string `json:"lesson_id"`
	SortOrder int32  `json:"sort_order"`
}

// AcademyLessonsReorderHandler returns a ToolHandler that reorders lessons
// by updating their sort_order values in bulk.
func AcademyLessonsReorderHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Academy == nil {
			return errorToolResult("academy_unavailable", "academy store is not configured"), nil
		}

		var a learnLessonReorderArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if len(a.Entries) == 0 {
			return errorToolResult("empty_entries", "at least one reorder entry is required"), nil
		}

		entries := make([]AcademyReorderEntry, 0, len(a.Entries))

		for _, e := range a.Entries {
			id, err := scanUUID(e.LessonID)
			if err != nil {
				return errorToolResult("invalid_lesson_id", fmt.Sprintf("lesson_id %q is not a valid UUID", e.LessonID)), nil
			}

			entries = append(entries, AcademyReorderEntry{
				ID:        id,
				SortOrder: e.SortOrder,
			})
		}

		if err := deps.Academy.ReorderLessons(ctx, entries); err != nil {
			return errorToolResult("learn_lessons_reorder_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"reordered": len(entries),
		}), nil
	}
}

// ---------------------------------------------------------------------------
// Shared converters
// ---------------------------------------------------------------------------

func learnTrackToMap(t AcademyTrack) map[string]any {
	m := map[string]any{
		"id":           uuidString(t.ID),
		"title":        t.Title,
		"slug":         t.Slug,
		"difficulty":   t.Difficulty,
		"is_published": t.IsPublished,
		"sort_order":   t.SortOrder,
		"created_at":   timestampString(t.CreatedAt),
		"updated_at":   timestampString(t.UpdatedAt),
	}
	if t.Description != nil {
		m["description"] = *t.Description
	}

	return m
}

func learnLessonToMap(l AcademyLesson) map[string]any {
	m := map[string]any{
		"id":           uuidString(l.ID),
		"module_id":    uuidString(l.ModuleID),
		"title":        l.Title,
		"slug":         l.Slug,
		"lesson_type":  l.LessonType,
		"sort_order":   l.SortOrder,
		"is_published": l.IsPublished,
		"is_preview":   l.IsPreview,
		"created_at":   timestampString(l.CreatedAt),
		"updated_at":   timestampString(l.UpdatedAt),
	}
	if l.Content != nil {
		m["content"] = *l.Content
	}

	if l.DurationMinutes != nil {
		m["duration_minutes"] = *l.DurationMinutes
	}

	return m
}
