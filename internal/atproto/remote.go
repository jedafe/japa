// Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
// Remote Bluesky actor resolution. Remote actors are provenance-only rows in
// atproto_remote_actors — never users rows. Profile data is fetched from the
// configured public AppView (JC_ATPROTO_APPVIEW_URL) once and refreshed at most
// every remoteActorRefreshInterval; the AppView URL is config-pinned so the
// fetcher cannot be steered at arbitrary hosts.
package atproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

const remoteActorRefreshInterval = 24 * time.Hour

// Notifier is the narrow notification seam the inbound consumer needs — the
// concrete implementation is notifications.Service.CreateNotification.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// SetNotifier injects the notifications seam for inbound events.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// resolveRemoteActor returns the provenance row for a remote DID, refreshing
// the cached profile from the AppView when missing or stale. A failed fetch
// still returns the bare DID-keyed row so attribution never blocks processing.
func (s *Service) resolveRemoteActor(ctx context.Context, did string) (gen.AtprotoRemoteActor, error) {
	q := gen.New(s.db.Writer)

	if existing, err := q.GetAtprotoRemoteActorByDID(ctx, did); err == nil {
		if time.Since(existing.FetchedAt.Time) < remoteActorRefreshInterval {
			return existing, nil
		}
	}

	var handle, displayName, avatarURL *string
	if prof, err := s.fetchRemoteActorProfile(ctx, did); err == nil {
		if prof.Handle != "" {
			handle = &prof.Handle
		}
		if prof.DisplayName != "" {
			displayName = &prof.DisplayName
		}
		if prof.Avatar != "" {
			avatarURL = &prof.Avatar
		}
	}

	return q.UpsertAtprotoRemoteActor(ctx, gen.UpsertAtprotoRemoteActorParams{
		Did:         did,
		Handle:      handle,
		DisplayName: displayName,
		AvatarUrl:   avatarURL,
	})
}

type appviewProfile struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

// fetchRemoteActorProfile reads app.bsky.actor.getProfile from the configured
// AppView. Errors are non-fatal to callers — the actor still gets a DID-keyed
// row so events are never dropped over a profile fetch.
func (s *Service) fetchRemoteActorProfile(ctx context.Context, did string) (*appviewProfile, error) {
	base := s.cfg.AppViewURL
	if base == "" {
		return nil, errors.New("atproto: appview URL not configured")
	}
	u, err := url.Parse(base + "/xrpc/app.bsky.actor.getProfile")
	if err != nil {
		return nil, fmt.Errorf("atproto: bad appview URL: %w", err)
	}
	u.RawQuery = url.Values{"actor": []string{did}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req = req.WithContext(fetchCtx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("atproto: appview getProfile status %d", resp.StatusCode)
	}
	var prof appviewProfile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&prof); err != nil {
		return nil, err
	}
	return &prof, nil
}
