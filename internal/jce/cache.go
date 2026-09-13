package jce

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ScoreResultKey returns the canonical Valkey key for a cached JapaScore result.
// It includes the content, score version, and profile identity so that
// different profiles do not share the same cached score.
func ScoreResultKey(contentID pgtype.UUID, version int32, profileID string) string {
	if profileID == "" {
		profileID = "none"
	}

	return fmt.Sprintf("jc:jce:score:%s:%d:%s", contentID.String(), version, profileID)
}

// ScoreResultSetKey returns a Valkey set key that tracks all ScoreResultKey
// entries written for a given profile. It is used to invalidate every
// cached score result for a profile when that profile is mutated.
func ScoreResultSetKey(profileID pgtype.UUID) string {
	return fmt.Sprintf("jc:jce:score_by_profile:%s", profileID.String())
}

// ProfileCacheKey returns the canonical Valkey key for a cached comparison profile.
func ProfileCacheKey(userID, profileID pgtype.UUID) string {
	return fmt.Sprintf("jc:jce:profile:%s:%s", userID.String(), profileID.String())
}
