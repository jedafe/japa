// In-memory store for the TOTP challenge token issued by /auth/login when
// a user with 2FA enabled signs in. The login page stashes the challenge
// token + user email here before redirecting to /verify-totp; the verify-totp
// page reads them to complete the second factor. Kept in memory only (never
// localStorage) to mirror the access-token handling in token.ts.
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)

let _challengeToken: string | null = null;
let _challengeEmail: string | null = null;

export function setChallenge(token: string | null, email: string | null = null): void {
	_challengeToken = token;
	_challengeEmail = email;
}

export function getChallengeToken(): string | null {
	return _challengeToken;
}

export function getChallengeEmail(): string | null {
	return _challengeEmail;
}

export function clearChallenge(): void {
	_challengeToken = null;
	_challengeEmail = null;
}
