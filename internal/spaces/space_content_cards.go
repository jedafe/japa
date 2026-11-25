package spaces

import (
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/feed"
)

// spaceContentCard is the card-shaped payload for /spaces/{slug}/content,
// /pinned, and ?q= search — the same surface fields the explore feed cards
// render (excerpt, cover, engagement counts, tags, org badge, journey
// badges, video overlay) plus the moderation columns this page needs
// (status/visibility, the space pin flag lives on PostCard.IsPinned).
type spaceContentCard struct {
	content.PostCard
	Status     string `json:"status"`
	Visibility string `json:"visibility"`
	VoteScore  int32  `json:"vote_score"`
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefI32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

func spaceCardFromListRow(c *gen.ListContentsBySpaceRow) spaceContentCard {
	return spaceContentCard{
		PostCard: content.PostCard{
			ID:                     c.ID,
			ContentType:            c.ContentType,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Excerpt:                feed.CleanExcerpt(c.Excerpt, c.Title),
			BodyHtml:               c.BodyHtml,
			AuthorID:               c.AuthorID,
			AuthorName:             c.AuthorName,
			AuthorUsername:         c.AuthorUsername,
			AuthorAvatarURL:        c.AuthorAvatarUrl,
			AuthorVerified:         c.AuthorVerified,
			AuthorVerificationType: c.AuthorVerificationType,
			OrganizationID:         c.OrganizationID,
			Organization:           content.OrgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoStorageBucket, c.OrgLogoStoragePath, c.OrgVerified, c.OrgVerificationType),
			SpaceID:                c.SpaceID,
			SpaceName:              derefStr(c.SpaceName),
			SpaceSlug:              c.SpaceSlug,
			SpaceAvatarURL:         content.MediaURL(c.SpaceAvatarStorageBucket, c.SpaceAvatarStoragePath),
			HeaderImageID:          c.HeaderImageID,
			FeaturedImageID:        c.FeaturedImageID,
			CoverImageURL:          content.CoverImageURL(c.CoverStorageBucket, c.CoverStoragePath, derefStr(c.VideoThumbnailUrl)),
			ReadTimeMinutes:        c.ReadTimeMinutes,
			ViewCount:              c.ViewCount,
			CommentCount:           c.CommentCount,
			ReactionCount:          c.ReactionCount,
			BookmarkCount:          c.BookmarkCount,
			ShareCount:             c.ShareCount,
			Tags:                   content.SplitTags(c.Tags),
			JourneyStage:           c.JourneyStage,
			VisaRoute:              c.VisaRoute,
			SourceCountry:          c.SourceCountry,
			TargetCountry:          c.TargetCountry,
			GuideCategory:          c.GuideCategory,
			VideoSourceID:          c.VideoSourceID,
			VideoSourceURL:         derefStr(c.VideoSourceUrl),
			VideoSourceType:        derefStr(c.VideoSourceType),
			VideoDurationSecs:      derefI32(c.VideoDurationSecs),
			VideoThumbnailURL:      derefStr(c.VideoThumbnailUrl),
			IsPinned:               c.IsPinned,
			PublishedAt:            c.PublishedAt,
			CreatedAt:              c.CreatedAt,
		},
		Status:     c.Status,
		Visibility: c.Visibility,
		VoteScore:  c.VoteScore,
	}
}

func spaceCardFromPinnedRow(c *gen.ListPinnedContentBySpaceRow) spaceContentCard {
	return spaceContentCard{
		PostCard: content.PostCard{
			ID:                     c.ID,
			ContentType:            c.ContentType,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Excerpt:                feed.CleanExcerpt(c.Excerpt, c.Title),
			BodyHtml:               c.BodyHtml,
			AuthorID:               c.AuthorID,
			AuthorName:             c.AuthorName,
			AuthorUsername:         c.AuthorUsername,
			AuthorAvatarURL:        c.AuthorAvatarUrl,
			AuthorVerified:         c.AuthorVerified,
			AuthorVerificationType: c.AuthorVerificationType,
			OrganizationID:         c.OrganizationID,
			Organization:           content.OrgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoStorageBucket, c.OrgLogoStoragePath, c.OrgVerified, c.OrgVerificationType),
			SpaceID:                c.SpaceID,
			SpaceName:              derefStr(c.SpaceName),
			SpaceSlug:              c.SpaceSlug,
			SpaceAvatarURL:         content.MediaURL(c.SpaceAvatarStorageBucket, c.SpaceAvatarStoragePath),
			HeaderImageID:          c.HeaderImageID,
			FeaturedImageID:        c.FeaturedImageID,
			CoverImageURL:          content.CoverImageURL(c.CoverStorageBucket, c.CoverStoragePath, derefStr(c.VideoThumbnailUrl)),
			ReadTimeMinutes:        c.ReadTimeMinutes,
			ViewCount:              c.ViewCount,
			CommentCount:           c.CommentCount,
			ReactionCount:          c.ReactionCount,
			BookmarkCount:          c.BookmarkCount,
			ShareCount:             c.ShareCount,
			Tags:                   content.SplitTags(c.Tags),
			JourneyStage:           c.JourneyStage,
			VisaRoute:              c.VisaRoute,
			SourceCountry:          c.SourceCountry,
			TargetCountry:          c.TargetCountry,
			GuideCategory:          c.GuideCategory,
			VideoSourceID:          c.VideoSourceID,
			VideoSourceURL:         derefStr(c.VideoSourceUrl),
			VideoSourceType:        derefStr(c.VideoSourceType),
			VideoDurationSecs:      derefI32(c.VideoDurationSecs),
			VideoThumbnailURL:      derefStr(c.VideoThumbnailUrl),
			IsPinned:               c.IsPinned,
			PublishedAt:            c.PublishedAt,
			CreatedAt:              c.CreatedAt,
		},
		Status:     c.Status,
		Visibility: c.Visibility,
		VoteScore:  c.VoteScore,
	}
}

func spaceCardFromSearchRow(c *gen.SearchContentsBySpaceRow) spaceContentCard {
	return spaceContentCard{
		PostCard: content.PostCard{
			ID:                     c.ID,
			ContentType:            c.ContentType,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Excerpt:                feed.CleanExcerpt(c.Excerpt, c.Title),
			BodyHtml:               c.BodyHtml,
			AuthorID:               c.AuthorID,
			AuthorName:             c.AuthorName,
			AuthorUsername:         c.AuthorUsername,
			AuthorAvatarURL:        c.AuthorAvatarUrl,
			AuthorVerified:         c.AuthorVerified,
			AuthorVerificationType: c.AuthorVerificationType,
			OrganizationID:         c.OrganizationID,
			Organization:           content.OrgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoStorageBucket, c.OrgLogoStoragePath, c.OrgVerified, c.OrgVerificationType),
			SpaceID:                c.SpaceID,
			SpaceName:              c.SpaceName,
			SpaceSlug:              c.SpaceSlug,
			SpaceAvatarURL:         content.MediaURL(c.SpaceAvatarStorageBucket, c.SpaceAvatarStoragePath),
			HeaderImageID:          c.HeaderImageID,
			FeaturedImageID:        c.FeaturedImageID,
			CoverImageURL:          content.CoverImageURL(c.CoverStorageBucket, c.CoverStoragePath, derefStr(c.VideoThumbnailUrl)),
			ReadTimeMinutes:        c.ReadTimeMinutes,
			ViewCount:              c.ViewCount,
			CommentCount:           c.CommentCount,
			ReactionCount:          c.ReactionCount,
			BookmarkCount:          c.BookmarkCount,
			ShareCount:             c.ShareCount,
			Tags:                   content.SplitTags(c.Tags),
			JourneyStage:           c.JourneyStage,
			VisaRoute:              c.VisaRoute,
			SourceCountry:          c.SourceCountry,
			TargetCountry:          c.TargetCountry,
			GuideCategory:          c.GuideCategory,
			VideoSourceID:          c.VideoSourceID,
			VideoSourceURL:         derefStr(c.VideoSourceUrl),
			VideoSourceType:        derefStr(c.VideoSourceType),
			VideoDurationSecs:      derefI32(c.VideoDurationSecs),
			VideoThumbnailURL:      derefStr(c.VideoThumbnailUrl),
			IsPinned:               c.IsPinned,
			PublishedAt:            c.PublishedAt,
			CreatedAt:              c.CreatedAt,
		},
		Status:     c.Status,
		Visibility: c.Visibility,
		VoteScore:  c.VoteScore,
	}
}
