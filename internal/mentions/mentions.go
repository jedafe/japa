// Package mentions extracts mention nodes from ProseMirror JSON and
// determines which users should receive notifications (Section 15).
//
// Mention nodes in TipTap are stored as:
//
//	{"type": "japaMention", "attrs": {"id": "...", "entity_type": "user", "label": "..."}}
//
// The package checks:
//   - DM privacy settings (mentions bypass DM privacy)
//   - Blocked users (no notification if blocked)
//   - Space membership (private space mentions only notify members)
//   - Deleted users (renders as @[deleted user], no notification)
package mentions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/jackc/pgx/v5/pgtype"
)

// Mention represents a parsed mention node from ProseMirror JSON.
type Mention struct {
	ID         pgtype.UUID
	EntityType string
	Label      string
}

// BlockChecker determines whether user A has blocked user B.
// Returns true if the blocker has blocked the blocked.
// Tenant-scoped per JEOS §9 (BUG-006).
type BlockChecker interface {
	IsBlocked(ctx context.Context, tenantID, blockerID, blockedID pgtype.UUID) (bool, error)
}

// SpaceMemberChecker determines whether a user is a member of a space.
// Tenant-scoped per Section 04 (BUG-005).
type SpaceMemberChecker interface {
	IsMember(ctx context.Context, tenantID, spaceID, userID pgtype.UUID) (bool, error)
}

// UserStatusChecker determines whether a user account is active or erased.
type UserStatusChecker interface {
	IsErased(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error)
}

// ExtractMentions walks a ProseMirror JSON document and returns all
// mention nodes with entity_type == "user".
//
// The document may be either a full {type:"doc",content:[...]} document
// or a bare array of top-level nodes (both shapes are produced by the
// backend — see serialize.ts which handles both).
//
// The walker recurses through all nesting levels (tables, lists,
// blockquotes) so mentions inside any block structure are found.
// Node type "mention" is the canonical TipTap Mention extension name;
// "japaMention" is accepted for backward compatibility with older docs.
func ExtractMentions(docJSON []byte) ([]Mention, error) {
	// A ProseMirror node — content is recursive.
	type pmNode struct {
		Type    string         `json:"type"`
		Attrs   map[string]any `json:"attrs"`
		Content []pmNode       `json:"content"`
	}

	// Try unmarshalling as a full document first, then as a bare array.
	var doc pmNode

	var topBlocks []pmNode

	if err := json.Unmarshal(docJSON, &doc); err == nil && doc.Type == "doc" {
		topBlocks = doc.Content
	} else {
		// Try bare array of nodes.
		if err := json.Unmarshal(docJSON, &topBlocks); err != nil {
			return nil, err
		}
	}

	var mentions []Mention

	// Recursive walker — descends into every node's content array.
	var walk func(nodes []pmNode)
	walk = func(nodes []pmNode) {
		for _, node := range nodes {
			// Check this node for a mention.
			if (node.Type == "mention" || node.Type == "japaMention") && node.Attrs != nil {
				entityType, _ := node.Attrs["entity_type"].(string)
				// Default entity_type to "user" when absent (frontend may
				// omit it for @-mentions). Applied to both "mention" and
				// "japaMention" for backward compatibility (Section 15).
				if entityType == "" {
					entityType = "user"
				}

				if entityType == "user" {
					idStr, _ := node.Attrs["id"].(string)
					label, _ := node.Attrs["label"].(string)

					var id pgtype.UUID
					if err := id.Scan(idStr); err == nil {
						mentions = append(mentions, Mention{
							ID:         id,
							EntityType: entityType,
							Label:      label,
						})
					}
				}
			}

			// Recurse into children.
			if len(node.Content) > 0 {
				walk(node.Content)
			}
		}
	}

	walk(topBlocks)

	return mentions, nil
}

// FilterMentions determines which mentioned users should receive notifications.
// It checks:
//   - Whether the mentioned user's account is erased (skip if erased)
//   - Whether the post author has blocked the mentioned user (skip if blocked)
//   - Whether the mentioned user has blocked the post author (skip if blocked)
//   - Whether the post is in a private space and the mentioned user is a member (skip if not member)
func FilterMentions(
	ctx context.Context,
	mentions []Mention,
	tenantID pgtype.UUID,
	authorID pgtype.UUID,
	spaceID pgtype.UUID,
	isPrivateSpace bool,
	blockChecker BlockChecker,
	memberChecker SpaceMemberChecker,
	userChecker UserStatusChecker,
) ([]Mention, error) {
	// Deduplicate by user ID before filtering to avoid redundant DB calls
	// when the same user is mentioned multiple times in one post (BUG-004).
	seen := make(map[pgtype.UUID]bool, len(mentions))

	var unique []Mention

	for _, m := range mentions {
		if !m.ID.Valid {
			continue
		}

		if seen[m.ID] {
			continue
		}

		seen[m.ID] = true

		unique = append(unique, m)
	}

	var filtered []Mention

	for _, m := range unique {
		// Skip if mentioned user is the post author
		if m.ID == authorID {
			continue
		}

		// Skip if mentioned user's account is erased
		if userChecker != nil {
			erased, err := userChecker.IsErased(ctx, tenantID, m.ID)
			if err != nil {
				slog.Warn("mentions: IsErased check failed, skipping mention",
					"user_id", m.ID.String(), "error", err)
				continue
			}

			if erased {
				continue
			}
		}

		// Skip if there is a block between the author and the mentioned user
		// (either direction). The IsBlocked query checks both directions in
		// a single call, so only one call is needed (BUG-011).
		if blockChecker != nil {
			blocked, err := blockChecker.IsBlocked(ctx, tenantID, authorID, m.ID)
			if err != nil {
				slog.Warn("mentions: IsBlocked check failed, skipping mention",
					"user_id", m.ID.String(), "error", err)
				continue
			}

			if blocked {
				continue
			}
		}

		// Skip if post is in a private space and mentioned user is not a member.
		// Tenant-scoped per Section 04 (BUG-005).
		if isPrivateSpace && memberChecker != nil {
			isMember, err := memberChecker.IsMember(ctx, tenantID, spaceID, m.ID)
			if err != nil {
				slog.Warn("mentions: IsMember check failed, skipping mention",
					"user_id", m.ID.String(), "space_id", spaceID.String(), "error", err)
				continue
			}

			if !isMember {
				continue
			}
		}

		filtered = append(filtered, m)
	}

	return filtered, nil
}

// ComputeErasedMentionIDs extracts user mention IDs from a TipTap JSON body
// and returns the UUIDs of those whose accounts have been erased (Section 15 /
// ADR-029). The returned IDs are lowercase UUID strings suitable for a
// frontend `Set<string>` used to render `@[deleted user]` placeholders.
//
// If userChecker is nil, returns nil (no erased users — backward compat).
func ComputeErasedMentionIDs(
	ctx context.Context,
	body []byte,
	tenantID pgtype.UUID,
	userChecker UserStatusChecker,
) ([]string, error) {
	if userChecker == nil {
		return nil, nil
	}

	mentioned, err := ExtractMentions(body)
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); no mentions.
		return nil, nil
	}

	var erased []string

	for _, m := range mentioned {
		if !m.ID.Valid {
			continue
		}

		isErased, err := userChecker.IsErased(ctx, tenantID, m.ID)
		if err != nil {
			slog.Warn("mentions: IsErased check failed in ComputeErasedMentionIDs",
				"user_id", m.ID.String(), "error", err)
			continue
		}

		if isErased {
			erased = append(erased, m.ID.String())
		}
	}

	return erased, nil
}

// mentionSpanRegex matches the serialized mention span produced by both
// the TipTap Mention extension's renderHTML (editor.getHTML()) and the
// frontend SSR serializer (serialize.ts renderNode 'mention' case):
//
//	<span class="mention" data-type="mention" data-id="UUID" data-label="LABEL">@LABEL</span>
//
// The regex captures the data-id attribute value (the user UUID) so it can
// be matched against a set of erased user IDs. Also matches the older
// data-user-id format for backward compatibility.
var mentionSpanRegex = regexp.MustCompile(
	`<span class="mention(?:\s+mention-deleted)?"[^>]*?(?:data-id|data-user-id)="([^"]+)"[^>]*>[^<]*</span>`,
)

// RedactErasedMentionsInHTML post-processes a pre-rendered body_html string,
// replacing mention spans whose data-user-id is in the erasedIDs set with
// the `@[deleted user]` placeholder (Section 15 / ADR-029). This covers the
// blog/content rendering path that uses stored body_html rather than
// re-rendering from TipTap JSON.
func RedactErasedMentionsInHTML(htmlBody string, erasedIDs []string) string {
	if len(erasedIDs) == 0 {
		return htmlBody
	}

	erasedSet := make(map[string]bool, len(erasedIDs))
	for _, id := range erasedIDs {
		erasedSet[id] = true
	}

	return mentionSpanRegex.ReplaceAllStringFunc(htmlBody, func(match string) string {
		sub := mentionSpanRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}

		if erasedSet[sub[1]] {
			return `<span class="mention mention-deleted">@[deleted user]</span>`
		}

		return match
	})
}

// RedactErasedMentionsInBody is a convenience function that combines
// ComputeErasedMentionIDs and RedactErasedMentionsInHTML for the common
// case where a handler has a TipTap JSON body, a pre-rendered body_html,
// and wants both the erased IDs (for the frontend) and a redacted
// body_html in one call.
func RedactErasedMentionsInBody(
	ctx context.Context,
	body []byte,
	bodyHtml string,
	tenantID pgtype.UUID,
	userChecker UserStatusChecker,
) (redactedHTML string, erasedIDs []string, err error) {
	erasedIDs, err = ComputeErasedMentionIDs(ctx, body, tenantID, userChecker)
	if err != nil {
		return bodyHtml, nil, fmt.Errorf("compute erased mentions: %w", err)
	}

	redactedHTML = RedactErasedMentionsInHTML(bodyHtml, erasedIDs)

	return redactedHTML, erasedIDs, nil
}
