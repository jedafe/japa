import { browser } from 'wxt/browser';
import type { Digest } from './types';

// chrome.storage.local keys. The API key is a bearer secret — it lives only
// in extension storage (never synced, never logged) and is cleared on
// disconnect.
const KEY_API_KEY = 'jc.apiKey';
const KEY_INSTALL_ID = 'jc.installId';
const KEY_DIGEST_CACHE = 'jc.digestCache';
const KEY_OAUTH_CLIENT = 'jc.oauthClient';
const KEY_OAUTH_SESSION = 'jc.oauthSession';
const KEY_OAUTH_USER = 'jc.oauthUser';

// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
// (Login with JapaConnect — v1.4). OAuth state lives in extension storage
// next to the legacy API key: the registered client_id, the token pair,
// and the last-fetched userinfo claims.

export interface OAuthSession {
	clientID: string;
	accessToken: string;
	refreshToken: string | undefined;
	// expiresAt is epoch ms — access tokens are opaque, expiry is tracked
	// client-side from expires_in.
	expiresAt: number;
	scope: string;
}

// OAuthUser mirrors the /oauth/userinfo claims the extension displays.
export interface OAuthUser {
	sub: string;
	preferred_username?: string;
	name?: string;
	email?: string;
	picture?: string;
}

export interface DigestCache {
	fetchedAt: number;
	digest: Digest;
}

export async function getApiKey(): Promise<string | null> {
	const res = await browser.storage.local.get(KEY_API_KEY);
	return (res[KEY_API_KEY] as string | undefined) ?? null;
}

export async function setApiKey(key: string): Promise<void> {
	await browser.storage.local.set({ [KEY_API_KEY]: key });
}

export async function clearApiKey(): Promise<void> {
	await browser.storage.local.remove([KEY_API_KEY, KEY_DIGEST_CACHE]);
}

export async function getOAuthClientID(): Promise<string | null> {
	const res = await browser.storage.local.get(KEY_OAUTH_CLIENT);
	return (res[KEY_OAUTH_CLIENT] as string | undefined) ?? null;
}

export async function setOAuthClientID(clientID: string): Promise<void> {
	await browser.storage.local.set({ [KEY_OAUTH_CLIENT]: clientID });
}

export async function getOAuthSession(): Promise<OAuthSession | null> {
	const res = await browser.storage.local.get(KEY_OAUTH_SESSION);
	return (res[KEY_OAUTH_SESSION] as OAuthSession | undefined) ?? null;
}

export async function setOAuthSession(session: OAuthSession): Promise<void> {
	await browser.storage.local.set({ [KEY_OAUTH_SESSION]: session });
}

export async function getOAuthUser(): Promise<OAuthUser | null> {
	const res = await browser.storage.local.get(KEY_OAUTH_USER);
	return (res[KEY_OAUTH_USER] as OAuthUser | undefined) ?? null;
}

export async function setOAuthUser(user: OAuthUser): Promise<void> {
	await browser.storage.local.set({ [KEY_OAUTH_USER]: user });
}

export async function clearOAuthSession(): Promise<void> {
	await browser.storage.local.remove([KEY_OAUTH_SESSION, KEY_OAUTH_USER, KEY_DIGEST_CACHE]);
}

// clearAuth drops every credential (OAuth session AND a pasted API key)
// plus the digest cache — the disconnect path does not care which kind
// of credential was live.
export async function clearAuth(): Promise<void> {
	await browser.storage.local.remove([
		KEY_API_KEY,
		KEY_OAUTH_SESSION,
		KEY_OAUTH_USER,
		KEY_DIGEST_CACHE,
	]);
}

// getInstallId returns the stable per-install UUID, generated on first
// read. Sent when minting tokens so each browser gets a distinct,
// revocable key.
export async function getInstallId(): Promise<string> {
	const res = await browser.storage.local.get(KEY_INSTALL_ID);
	const existing = res[KEY_INSTALL_ID] as string | undefined;
	if (existing) return existing;
	const id = crypto.randomUUID();
	await browser.storage.local.set({ [KEY_INSTALL_ID]: id });
	return id;
}

export async function getCachedDigest(): Promise<DigestCache | null> {
	const res = await browser.storage.local.get(KEY_DIGEST_CACHE);
	return (res[KEY_DIGEST_CACHE] as DigestCache | undefined) ?? null;
}

export async function setCachedDigest(digest: Digest): Promise<void> {
	const entry: DigestCache = { fetchedAt: Date.now(), digest };
	await browser.storage.local.set({ [KEY_DIGEST_CACHE]: entry });
}
