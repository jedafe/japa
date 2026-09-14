// Package ai — tools_user.go implements the user tool handlers for the AI
// Agent Executor extended tool catalog (Section 103, Phase 9).
//
// Tools implemented here (Phase 9, tasks 9.06):
//
//   - user.read_profile    — read a user's public profile + public custom fields
//   - user.update_profile  — update own profile fields (self only)
//   - user.read_own_fields — read own private fields (self only)
//
// Privacy: user.read_profile targets a user entity, so it registers a
// PrivacyTargetFunc that extracts the user_id. The executor's privacy gate
// blocks the call when the target user is ai_exempt. user.update_profile
// and user.read_own_fields are self-only — the handler enforces that the
// target user_id matches the calling user's ID. They do not register a
// privacy target because the target is always the caller (self).
//
// The user.read_profile handler omits PII fields (email, last_login_at,
// groups) — it surfaces only the public profile subset.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (User tools)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, task 9.06)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// user.read_profile (task 9.06a)
// ---------------------------------------------------------------------------

// userReadProfileArgs is the argument shape for user.read_profile.
type userReadProfileArgs struct {
	UserID string `json:"user_id"`
}

// UserReadProfileHandler returns a ToolHandler that reads a user's public
// profile. The handler omits PII (email, last_login_at, groups) — only the
// public profile subset is surfaced. The executor's privacy gate blocks
// the call when the target user is ai_exempt.
func UserReadProfileHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Users == nil {
			return errorToolResult("users_unavailable", "users service is not configured"), nil
		}

		var a userReadProfileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		targetUserID, err := scanUUID(a.UserID)
		if err != nil {
			return errorToolResult("invalid_user_id", "user_id is not a valid UUID"), nil
		}

		profile, err := deps.Users.GetProfile(ctx, targetUserID)
		if err != nil {
			return errorToolResult("user_not_found", fmt.Sprintf("user not found: %v", err)), nil
		}

		// Surface only the public profile subset — no PII.
		result := map[string]any{
			"id":             profile.ID,
			"username":       profile.Username,
			"profile_type":   profile.ProfileType,
			"follower_count": profile.FollowerCount,
			"created_at":     profile.CreatedAt,
		}
		if profile.DisplayName != nil {
			result["display_name"] = *profile.DisplayName
		}

		if profile.AvatarURL != nil {
			result["avatar_url"] = *profile.AvatarURL
		}

		if profile.Bio != nil {
			result["bio"] = *profile.Bio
		}

		return successToolResult(map[string]any{
			"profile": result,
		}), nil
	}
}

// UserReadProfilePrivacyTarget extracts the user_id from
// user.read_profile arguments so the executor can check the ai_exempt flag
// before dispatch.
func UserReadProfilePrivacyTarget(ctx context.Context, args json.RawMessage) (PrivacyEntityType, pgtype.UUID, error) {
	var a userReadProfileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("parsing user.read_profile args: %w", err)
	}

	id, err := scanUUID(a.UserID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("invalid user_id: %w", err)
	}

	return PrivacyEntityUser, id, nil
}

// ---------------------------------------------------------------------------
// user.update_profile (task 9.06b)
// ---------------------------------------------------------------------------

// userUpdateProfileArgs is the LLM-produced argument shape for
// user.update_profile. This is self-only — the handler enforces that
// user_id matches the calling user's ID.
type userUpdateProfileArgs struct {
	UserID             string  `json:"user_id"`
	DisplayName        *string `json:"display_name,omitempty"`
	AvatarURL          *string `json:"avatar_url,omitempty"`
	Bio                *string `json:"bio,omitempty"`
	Timezone           *string `json:"timezone,omitempty"`
	OriginCountry      *string `json:"origin_country,omitempty"`
	DestinationCountry *string `json:"destination_country,omitempty"`
}

// UserUpdateProfileHandler returns a ToolHandler that updates the calling
// user's own profile. The handler enforces self-only behavior: the
// user_id in the arguments MUST match the calling user's ID. A mismatch
// returns an error result.
func UserUpdateProfileHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Users == nil {
			return errorToolResult("users_unavailable", "users service is not configured"), nil
		}

		var a userUpdateProfileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		targetUserID, err := scanUUID(a.UserID)
		if err != nil {
			return errorToolResult("invalid_user_id", "user_id is not a valid UUID"), nil
		}

		// Self-only enforcement: the target must be the calling user.
		if targetUserID != userID {
			return errorToolResult("not_self", "user.update_profile can only update the calling user's own profile"), nil
		}

		if err := deps.Users.UpdateProfile(ctx, tenantID, userID, &UserProfileUpdateRequest{
			DisplayName:        a.DisplayName,
			AvatarURL:          a.AvatarURL,
			Bio:                a.Bio,
			Timezone:           a.Timezone,
			OriginCountry:      a.OriginCountry,
			DestinationCountry: a.DestinationCountry,
		}); err != nil {
			return errorToolResult("profile_update_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"user_id": uuidString(userID),
			"updated": true,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// user.read_own_fields (task 9.06c)
// ---------------------------------------------------------------------------

// userReadOwnFieldsArgs is the argument shape for user.read_own_fields.
// This is self-only — the handler enforces that user_id matches the
// calling user's ID.
type userReadOwnFieldsArgs struct {
	UserID string `json:"user_id"`
}

// UserReadOwnFieldsHandler returns a ToolHandler that reads the calling
// user's own custom field values (including private/ai_exempt fields that
// would normally be filtered). This is self-only: the user_id in the
// arguments MUST match the calling user's ID.
//
// Unlike custom_field_values.read (which filters ai_exempt values), this
// tool shows the user their own private fields — the user has the right to
// see their own data. The privacy gate is not registered because the
// target is always self.
func UserReadOwnFieldsHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CustomFields == nil {
			return errorToolResult("custom_fields_unavailable", "custom fields service is not configured"), nil
		}

		var a userReadOwnFieldsArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		targetUserID, err := scanUUID(a.UserID)
		if err != nil {
			return errorToolResult("invalid_user_id", "user_id is not a valid UUID"), nil
		}

		// Self-only enforcement.
		if targetUserID != userID {
			return errorToolResult("not_self", "user.read_own_fields can only read the calling user's own fields"), nil
		}

		// Read the user's own field values. Unlike the generic
		// custom_field_values.read handler, this does NOT filter
		// ai_exempt values — the user can see their own private data.
		cfvs, err := deps.CustomFields.ListFieldValuesByEntity(ctx, "user_profile", userID)
		if err != nil {
			return errorToolResult("own_fields_read_failed", err.Error()), nil
		}

		results := make([]map[string]any, 0, len(cfvs))

		for _, cfv := range cfvs {
			entry := map[string]any{
				"field_id":  uuidString(cfv.FieldID),
				"entity_id": uuidString(cfv.EntityID),
			}
			if cfv.ValueText != nil {
				entry["value_text"] = *cfv.ValueText
			}

			if cfv.ValueBool != nil {
				entry["value_bool"] = *cfv.ValueBool
			}

			if cfv.ValueJson != nil {
				entry["value_json"] = json.RawMessage(cfv.ValueJson)
			}

			if cfv.ValueDate.Valid {
				entry["value_date"] = timestampString(cfv.ValueDate)
			}

			results = append(results, entry)
		}

		return successToolResult(map[string]any{
			"user_id":      uuidString(userID),
			"field_values": results,
			"total":        len(results),
		}), nil
	}
}
