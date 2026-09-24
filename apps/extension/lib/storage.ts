import { browser } from 'wxt/browser';
import type { Digest } from './types';

// chrome.storage.local keys. The API key is a bearer secret — it lives only
// in extension storage (never synced, never logged) and is cleared on
// disconnect.
const KEY_API_KEY = 'jc.apiKey';
const KEY_INSTALL_ID = 'jc.installId';
const KEY_DIGEST_CACHE = 'jc.digestCache';

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
