package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// videoChannelSyncer is the processor contract for the video:sync_channels
// task (§106 channel-sync amendment). The video service implements it.
var videoChannelSyncer interface {
	ProcessChannelSync(ctx context.Context) error
}

// RegisterVideoChannelSyncer sets the channel sync processor for the
// video:sync_channels handler.
func RegisterVideoChannelSyncer(p interface {
	ProcessChannelSync(ctx context.Context) error
},
) {
	videoChannelSyncer = p
}

// HandleVideoSyncChannels processes a video:sync_channels task. The payload
// is empty — the processor iterates all due video_channel sources itself.
func HandleVideoSyncChannels(ctx context.Context, _ *asynq.Task) error {
	if videoChannelSyncer == nil {
		return fmt.Errorf("video channel syncer not registered: %w", asynq.SkipRetry)
	}

	return videoChannelSyncer.ProcessChannelSync(ctx)
}
