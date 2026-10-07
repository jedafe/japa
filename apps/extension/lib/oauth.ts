import { browser } from 'wxt/browser';
import { SITE_ORIGIN } from './links';
import {
	clearOAuthSession,
	getApiKey,
	getOAuthClientID,
	getOAuthSession,
	getOAuthUser,
	setOAuthClientID,
	setOAuthSession,
	setOAuthUser,
	type OAuthSession,
	type OAuthUser,
} from './storage';

// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
// (Login with JapaConnect — v1.4). The extension is a public OAuth client:
// it self-registers via RFC 7591 DCR, drives the auth-code + PKCE S256 flow
// through chrome.identity.launchWebAuthFlow, and refreshes with the
// rotating refresh_token grant. The issued jcpat_ token is sent as a
// Bearer credential — the public API resolves it like an API key.

// openid unlocks /oauth/userinfo; the api.read.* set mirrors the fixed
// scope set the extension-token mint issues (§47).
const OAUTH_SCOPES = 'openid api.read.digest api.read.content api.read.me api.read.safety';
const OAUTH_CLIENT_NAME = 'JapaConnect Browser Extension';

// Refresh a token this far ahead of expiry so a request never leaves with
// a nearly-dead access token.
const EXPIRY_SKEW_MS = 60_000;

export type Credential = { kind: 'apikey' | 'oauth'; token: string } | null;

export class OAuthError extends Error {
	constructor(
		public code: string,
		message: string,
	) {
		super(message);
	}
}

// LoginCancelledError marks the user closing the auth flow — callers show
// no error for it.
export class LoginCancelledError extends Error {}

function base64url(buf: ArrayBuffer | Uint8Array): string {
	const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

function randomToken(bytes = 32): string {
	return base64url(crypto.getRandomValues(new Uint8Array(bytes)));
}

async function pkcePair(): Promise<{ verifier: string; challenge: string }> {
	const verifier = randomToken(32);
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
	return { verifier, challenge: base64url(digest) };
}

function redirectURI(): string {
	// https://<extension-id>.chromiumapp.org/ — registered verbatim with DCR.
	return browser.identity.getRedirectURL();
}

// ensureClientID returns this install's RFC 7591 client_id, registering on
// first use. Per-install registration gives each browser its own
// connected-app grant and revocation granularity (§120 v1.4).
async function ensureClientID(): Promise<string> {
	const existing = await getOAuthClientID();
	if (existing) return existing;

	const res = await fetch(`${SITE_ORIGIN}/oauth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
		body: JSON.stringify({
			client_name: OAUTH_CLIENT_NAME,
			redirect_uris: [redirectURI()],
			scope: OAUTH_SCOPES,
		}),
	});
	if (!res.ok) {
		throw new OAuthError('registration_failed', `Could not register the extension (${res.status})`);
	}
	const body = (await res.json()) as { client_id?: string };
	if (!body.client_id) {
		throw new OAuthError('registration_failed', 'The server did not issue a client id');
	}
	await setOAuthClientID(body.client_id);
	return body.client_id;
}

async function tokenRequest(clientID: string, params: Record<string, string>): Promise<OAuthSession> {
	const res = await fetch(`${SITE_ORIGIN}/oauth/token`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
		body: new URLSearchParams(params).toString(),
	});
	const body = (await res.json().catch(() => null)) as {
		access_token?: string;
		refresh_token?: string;
		expires_in?: number;
		scope?: string;
		error?: string;
		error_description?: string;
	} | null;
	if (!res.ok || !body?.access_token) {
		throw new OAuthError(
			body?.error ?? 'token_exchange_failed',
			body?.error_description ?? `Token exchange failed (${res.status})`,
		);
	}

	const session: OAuthSession = {
		clientID,
		accessToken: body.access_token,
		refreshToken: body.refresh_token,
		expiresAt: Date.now() + (body.expires_in ?? 3600) * 1000,
		scope: body.scope ?? OAUTH_SCOPES,
	};
	await setOAuthSession(session);
	return session;
}

async function fetchUserinfo(accessToken: string): Promise<OAuthUser | null> {
	const res = await fetch(`${SITE_ORIGIN}/oauth/userinfo`, {
		headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` },
	});
	if (!res.ok) return null;
	const user = (await res.json()) as OAuthUser;
	await setOAuthUser(user);
	return user;
}

// login runs the full connect flow and stores the session + identity.
// Throws LoginCancelledError when the user aborts the flow.
export async function login(): Promise<void> {
	const clientID = await ensureClientID();
	const { verifier, challenge } = await pkcePair();
	const state = randomToken(16);

	const authorize = new URL(`${SITE_ORIGIN}/oauth/authorize`);
	authorize.searchParams.set('response_type', 'code');
	authorize.searchParams.set('client_id', clientID);
	authorize.searchParams.set('redirect_uri', redirectURI());
	authorize.searchParams.set('scope', OAUTH_SCOPES);
	authorize.searchParams.set('state', state);
	authorize.searchParams.set('code_challenge', challenge);
	authorize.searchParams.set('code_challenge_method', 'S256');

	let redirectTo: string | undefined;
	try {
		redirectTo = await browser.identity.launchWebAuthFlow({
			url: authorize.toString(),
			interactive: true,
		});
	} catch {
		throw new LoginCancelledError('Sign-in was cancelled');
	}
	if (!redirectTo) throw new LoginCancelledError('Sign-in was cancelled');

	const params = new URL(redirectTo).searchParams;
	if (params.get('error')) {
		throw new OAuthError(params.get('error')!, params.get('error_description') ?? 'Authorization failed');
	}
	if (params.get('state') !== state || !params.get('code')) {
		throw new OAuthError('invalid_response', 'The authorization response was invalid');
	}

	const session = await tokenRequest(clientID, {
		grant_type: 'authorization_code',
		code: params.get('code')!,
		redirect_uri: redirectURI(),
		client_id: clientID,
		code_verifier: verifier,
	});
	await fetchUserinfo(session.accessToken);
}

let refreshing: Promise<OAuthSession | null> | null = null;

// refreshSession rotates the refresh token. Concurrent callers share one
// in-flight rotation — a second rotate on the same token would hit the
// server's reuse detection. A failed rotation clears the session.
function refreshSession(session: OAuthSession): Promise<OAuthSession | null> {
	if (!session.refreshToken) {
		// No refresh token — the session can't renew; drop it so the user
		// is asked to sign in again.
		return clearOAuthSession().then(() => null);
	}
	refreshing ??= tokenRequest(session.clientID, {
		grant_type: 'refresh_token',
		refresh_token: session.refreshToken,
		client_id: session.clientID,
	})
		.catch(() => null)
		.then(async (s) => {
			if (!s) await clearOAuthSession();
			return s;
		})
		.finally(() => {
			refreshing = null;
		});
	return refreshing;
}

// getValidAccessToken returns a live access token, refreshing when the
// stored one is inside the expiry skew (or when force is set — the 401
// retry path). Returns null when no OAuth session exists or refresh fails.
export async function getValidAccessToken(force = false): Promise<string | null> {
	let session = await getOAuthSession();
	if (!session) return null;
	if (force || session.expiresAt - EXPIRY_SKEW_MS <= Date.now()) {
		session = await refreshSession(session);
	}
	return session?.accessToken ?? null;
}

// getCredential resolves whichever credential the calls should carry:
// an OAuth bearer takes precedence over a pasted extension key.
export async function getCredential(): Promise<Credential> {
	const token = await getValidAccessToken();
	if (token) return { kind: 'oauth', token };
	const key = await getApiKey();
	return key ? { kind: 'apikey', token: key } : null;
}

export { getOAuthUser };

// logout revokes the refresh token (RFC 7009 — kills renewal; the access
// token expires within its TTL) and clears local auth state.
export async function logout(): Promise<void> {
	const session = await getOAuthSession();
	if (session?.refreshToken) {
		try {
			await fetch(`${SITE_ORIGIN}/oauth/revoke`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
				body: new URLSearchParams({ token: session.refreshToken, client_id: session.clientID }).toString(),
			});
		} catch {
			// best-effort — the token dies with its TTL regardless
		}
	}
	await clearOAuthSession();
}
