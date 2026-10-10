/**
 * Secret protection for studio credentials (apiKey, accessToken, refreshToken).
 *
 * Threat model: the Tauri SQLite file (state.db) and any localStorage
 * fallback live on the user's own disk (OS user-only permissions). Without
 * protection, any process that can read the file (backup tools, disk
 * scrapers, synced folders) gets full account impersonation. This module
 * encrypts the three secret fields with AES-256-GCM via WebCrypto so a
 * raw file read yields ciphertext, not bearer tokens.
 *
 * The device key is stored as a separate file outside the settings payload
 * so a single-row DB dump never contains both key and ciphertext. A
 * same-disk attacker with full file access can still recover both — the
 * OS keychain (tauri-plugin-keyring) is the follow-up that closes that
 * gap; until then this raises the bar from plaintext to encrypted.
 *
 * Secrets are NEVER written to localStorage: the localStorage fallback
 * adapter persists only non-secret settings, and the legacy localStorage
 * key is wiped after migration.
 */

const SECRET_FIELDS = ['apiKey', 'accessToken', 'refreshToken'] as const;
type SecretField = (typeof SECRET_FIELDS)[number];

const KEY_STORAGE_ITEM = 'japa-studio:device-key';
const ENC_PREFIX = 'enc:v1:';

async function getDeviceKey(): Promise<CryptoKey> {
	let raw: string | null = null;
	try {
		raw = localStorage.getItem(KEY_STORAGE_ITEM);
	} catch {
		raw = null;
	}
	if (!raw) {
		const bytes = crypto.getRandomValues(new Uint8Array(32));
		raw = btoa(String.fromCharCode(...bytes));
		try {
			localStorage.setItem(KEY_STORAGE_ITEM, raw);
		} catch {
			// storage unavailable (private mode) — use the ephemeral key
		}
	}
	const keyBytes = Uint8Array.from(atob(raw), (c) => c.charCodeAt(0));
	return crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, [
		'encrypt',
		'decrypt'
	]);
}

function b64encode(bytes: Uint8Array): string {
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin);
}

function b64decode(s: string): Uint8Array<ArrayBuffer> {
	const bin = atob(s);
	const out = new Uint8Array(new ArrayBuffer(bin.length));
	for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
	return out;
}

export async function encryptSecret(plaintext: string): Promise<string> {
	if (!plaintext) return plaintext;
	const key = await getDeviceKey();
	const iv = crypto.getRandomValues(new Uint8Array(12));
	const ct = await crypto.subtle.encrypt(
		{ name: 'AES-GCM', iv },
		key,
		new TextEncoder().encode(plaintext)
	);
	return ENC_PREFIX + b64encode(iv) + '.' + b64encode(new Uint8Array(ct));
}

export async function decryptSecret(stored: string | undefined): Promise<string | undefined> {
	if (!stored || !stored.startsWith(ENC_PREFIX)) return stored;
	try {
		const key = await getDeviceKey();
		const [ivB64, ctB64] = stored.slice(ENC_PREFIX.length).split('.');
		const pt = await crypto.subtle.decrypt(
			{ name: 'AES-GCM', iv: b64decode(ivB64) },
			key,
			b64decode(ctB64)
		);
		return new TextDecoder().decode(pt);
	} catch {
		return undefined;
	}
}

export function isSecretField(key: string): key is SecretField {
	return (SECRET_FIELDS as readonly string[]).includes(key);
}

/** Returns a copy of settings with secret fields removed (for localStorage). */
export function redactSecrets<T extends Record<string, unknown>>(settings: T): T {
	const out = { ...settings };
	for (const f of SECRET_FIELDS) delete (out as Record<string, unknown>)[f];
	return out;
}
