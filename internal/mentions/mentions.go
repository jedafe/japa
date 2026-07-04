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
type BlockChecker interface {
	IsBlocked(ctx context.Context, blockerID, blockedID pgtype.UUID) (bool, error)
}

// SpaceMemberChecker determines whether a user is a member of a space.
type SpaceMemberChecker interface {
	IsMember(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error)
}

// UserStatusChecker determines whether a user account is active or erased.
type UserStatusChecker interface {
	IsErased(ctx context.Context, userID pgtype.UUID) (bool, error)
}

// ExtractMentions walks a ProseMirror JSON document and returns all
// mention nodes with entity_type == "user".
func ExtractMentions(docJSON []byte) ([]Mention, error) {
	var doc struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string         `json:"type"`
			Attrs   map[string]any `json:"attrs"`
			Content []struct {
				Type  string         `json:"type"`
				Attrs map[string]any `json:"attrs"`
			} `json:"content"`
		} `json:"content"`
	}

	if err := json.Unmarshal(docJSON, &doc); err != nil {
		return nil, err
	}

	var mentions []Mention

	for _, block := range doc.Content {
		for _, node := range block.Content {
			if node.Type == "japaMention" && node.Attrs != nil {
				entityType, _ := node.Attrs["entity_type"].(string)
				if entityType != "user" {
					continue
				}

				idStr, _ := node.Attrs["id"].(string)
				label, _ := node.Attrs["label"].(string)

				var id pgtype.UUID
				if err := id.Scan(idStr); err != nil {
					continue // skip invalid UUIDs
				}

				mentions = append(mentions, Mention{
					ID:         id,
					EntityType: entityType,
					Label:      label,
				})
			}
		}
	}

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
	authorID pgtype.UUID,
	spaceID pgtype.UUID,
	isPrivateSpace bool,
	blockChecker BlockChecker,
	memberChecker SpaceMemberChecker,
	userChecker UserStatusChecker,
) ([]Mention, error) {
	var filtered []Mention

	for _, m := range mentions {
		// Skip if mentioned user is the post author
		if m.ID == authorID {
			continue
		}

		// Skip if mentioned user's account is erased
		if userChecker != nil {
			erased, err := userChecker.IsErased(ctx, m.ID)
			if err != nil {
				continue // skip on error
			}

			if erased {
				continue
			}
		}

		// Skip if post author has blocked the mentioned user
		if blockChecker != nil {
			blocked, err := blockChecker.IsBlocked(ctx, authorID, m.ID)
			if err != nil {
				continue
			}

			if blocked {
				continue
			}

			// Skip if mentioned user has blocked the post author
			blocked, err = blockChecker.IsBlocked(ctx, m.ID, authorID)
			if err != nil {
				continue
			}

			if blocked {
				continue
			}
		}

		// Skip if post is in a private space and mentioned user is not a member
		if isPrivateSpace && memberChecker != nil {
			isMember, err := memberChecker.IsMember(ctx, spaceID, m.ID)
			if err != nil {
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
