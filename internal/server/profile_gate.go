package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/users"
)

// requireVisibleProfileUser gates public profile surfaces on BOTH account
// status and the owner's profile_visibility (§19/§50):
//
//   - 400 when the URL param is not a UUID
//   - 404 when the user is missing or non-active (banned/suspended/deleted)
//   - 403 profile_private when profile_visibility restricts this viewer —
//     'members' denies guests, 'followers' denies everyone but the owner
//     and their followers
//
// This is the single enforcement point for "the profile and everything
// hanging off it is hidden" — every profile tab route wraps its handler
// with it. The tabs use "id"; the followers/following routes use "user_id".
func requireVisibleProfileUser(usersSvc *users.Service, paramKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var userID pgtype.UUID
			if err := userID.Scan(chi.URLParam(r, paramKey)); err != nil {
				writeProfileGateError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
				return
			}

			writeProfileAccessResult(w, usersSvc.CheckProfileAccessByID(r.Context(), userID), next, r)
		})
	}
}

// requireVisibleProfileByUsername is the username-keyed twin for profile
// surfaces that key on {username} — badges, japa-stage, tip-config,
// feed.xml. It resolves public_username aliases and former handles through
// the same query the profile endpoint uses, so a rename cannot bypass the
// owner's visibility setting.
func requireVisibleProfileByUsername(usersSvc *users.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username := httpx.URLParamDecoded(r, "username")
			if username == "" {
				writeProfileGateError(w, http.StatusBadRequest, "invalid_username", "Username is required")
				return
			}

			writeProfileAccessResult(w, usersSvc.CheckProfileAccessByUsername(r.Context(), username), next, r)
		})
	}
}

// writeProfileAccessResult maps the shared access-check outcome to the
// gate's error contract, or serves the wrapped handler when access is
// granted.
func writeProfileAccessResult(w http.ResponseWriter, err error, next http.Handler, r *http.Request) {
	switch {
	case err == nil:
		next.ServeHTTP(w, r)
	case errors.Is(err, pgx.ErrNoRows):
		writeProfileGateError(w, http.StatusNotFound, "user_not_found", "User not found")
	case errors.Is(err, users.ErrProfilePrivate):
		writeProfileGateError(w, http.StatusForbidden, "profile_private", "This profile is private")
	default:
		writeProfileGateError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve profile")
	}
}

func writeProfileGateError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
