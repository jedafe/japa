package auth

import "regexp"

// usernamePattern is the §50 username contract enforced on every
// user-supplied username write (register, PATCH /me/account,
// POST /admin/users): 3–30 characters of ASCII letters, digits, and
// underscore. Stored usernames that predate the rule are never
// re-validated — the check guards writes only.
//
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

// ValidateUsername reports whether a user-supplied username satisfies the
// §50 charset contract. Internally generated usernames (OAuth candidates,
// synthetic users) bypass this — it guards user input only.
func ValidateUsername(username string) bool {
	return usernamePattern.MatchString(username)
}
