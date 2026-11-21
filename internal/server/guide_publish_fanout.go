package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
)

// guidePublishFanout multiplexes the content.GuidePublishHook across every
// tool that auto-provisions a domain row from an editorial guide. Each
// implementation early-returns on categories it does not own, so a single
// published guide only ever lands on one tool. Wired in server.go where
// attestation used to be the sole hook (§97 pattern extended to §109
// eca and §94 credential guides).
type guidePublishFanout struct {
	hooks []content.GuidePublishHook
}

// OnGuidePublished forwards the event to every registered tool hook.
// A hook returning an error fails the publish only for that tool —
// earlier hooks that already provisioned stay provisioned (idempotent
// re-publish re-links them on the next publish attempt).
func (f guidePublishFanout) OnGuidePublished(ctx context.Context, contentID pgtype.UUID, guideCategory string, fieldValues []byte) error {
	for _, h := range f.hooks {
		if h == nil {
			continue
		}
		if err := h.OnGuidePublished(ctx, contentID, guideCategory, fieldValues); err != nil {
			return err
		}
	}

	return nil
}
