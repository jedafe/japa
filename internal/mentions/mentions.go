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
				// Default entity_type to "user" for the "mention" node
				// when absent (frontend may omit it for @-mentions).
				if entityType == "" && node.Type == "mention" {
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
	var filtered []Mention

	for _, m := range mentions {
		// Skip if mentioned user is the post author
		if m.ID == authorID {
			continue
		}

		// Skip if mentioned user's account is erased
		if userChecker != nil {
			erased, err := userChecker.IsErased(ctx, tenantID, m.ID)
			if err != nil {
				continue // skip on error
			}

			if erased {
				continue
			}
		}

		// Skip if there is a block between the author and the mentioned user
		// (either direction). The IsBlocked query checks both directions in
		// a single call, so only one call is needed (BUG-011).
		if blockChecker != nil {
			blocked, err := blockChecker.IsBlocked(ctx, authorID, m.ID)
			if err != nil {
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
