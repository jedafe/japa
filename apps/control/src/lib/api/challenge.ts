// In-memory store for the TOTP challenge token issued by /auth/login when
// a user with 2FA enabled signs in. The login page stashes the challenge
// token + user email here before navigating to /verify-totp; the verify-totp
// page reads them to complete the second factor. Kept in memory only (never
// localStorage) to mirror the access-token handling in $lib/api, with a
// sessionStorage backup so a full reload between login and verify does not
// strand the user with an "invalid challenge" error.
//
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)

let _challengeToken: string | null = null;
let _challengeEmail: string | null = null;

const TOKEN_KEY = 'jc_totp_challenge';
const EMAIL_KEY = 'jc_totp_email';

function readSession(key: string): string | null {
	try {
		if (typeof sessionStorage === 'undefined') return null;
		return sessionStorage.getItem(key);
	} catch {
		return null;
	}
}

function writeSession(key: string, value: string | null): void {
	try {
		if (typeof sessionStorage === 'undefined') return;
		if (value === null) sessionStorage.removeItem(key);
		else sessionStorage.setItem(key, value);
	} catch {
		// private-mode / SSR — memory copy still works
	}
}

export function setChallenge(token: string | null, email: string | null = null): void {
	_challengeToken = token;
	_challengeEmail = email;
	writeSession(TOKEN_KEY, token);
	writeSession(EMAIL_KEY, email);
}

export function getChallengeToken(): string | null {
	return _challengeToken ?? readSession(TOKEN_KEY);
}

export function getChallengeEmail(): string | null {
	return _challengeEmail ?? readSession(EMAIL_KEY);
}

export function clearChallenge(): void {
	_challengeToken = null;
	_challengeEmail = null;
	writeSession(TOKEN_KEY, null);
	writeSession(EMAIL_KEY, null);
}
