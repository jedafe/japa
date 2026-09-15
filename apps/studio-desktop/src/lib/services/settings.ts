/**
 * Local settings persistence.
 * Keeps a synchronous in-memory cache that is hydrated from SQLite on first load.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import * as stateDb from '$lib/services/db';

export const SETTINGS_KEY = 'japa-studio:settings';

export type StudioMode = 'operator' | 'member';

export interface StudioSettings {
	projectName: string;
	baseUrl: string;
	apiKey: string;
	projectRoot: string;
	mode: StudioMode;
	autoSync: boolean;
	autoSyncContentTypes: string[];
	default_content_type: string;
	default_guide_category: string;
	default_persona_id: string;
	default_voice_id: string;
	default_space_id: string;
	default_status: string;
	default_visibility: string;
	default_tags: string[];
	contentMinLength?: number;
	// Session tokens obtained by exchanging the API key.
	accessToken?: string;
	refreshToken?: string;
	tokenExpiresAt?: number;
	deviceId?: string;
}

export function defaultSettings(): StudioSettings {
	return {
		projectName: '',
		baseUrl: '',
		apiKey: '',
		projectRoot: '',
		mode: 'operator',
		autoSync: false,
		autoSyncContentTypes: [],
		default_content_type: '',
		default_guide_category: '',
		default_persona_id: '',
		default_voice_id: '',
		default_space_id: '',
		default_status: 'draft',
		default_visibility: 'public',
		default_tags: [],
		contentMinLength: 10
	};
}

function generateDeviceId(): string {
	if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
		return crypto.randomUUID();
	}
	return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

let settingsCache: StudioSettings | null = null;
let initPromise: Promise<void> | null = null;

export async function initSettingsDb(): Promise<void> {
	if (initPromise) return initPromise;
	initPromise = (async () => {
		let fromDb = await stateDb.loadSettings();
		if (fromDb && fromDb.deviceId) {
			settingsCache = fromDb;
			return;
		}

		// Migrate from legacy localStorage or create defaults.
		let legacy: Partial<StudioSettings> | undefined;
		if (typeof localStorage !== 'undefined') {
			try {
				const raw = localStorage.getItem(SETTINGS_KEY);
				if (raw) {
					legacy = JSON.parse(raw) as Partial<StudioSettings>;
				}
			} catch (e) {
				console.error(
					'Failed to migrate legacy settings from localStorage:',
					e instanceof Error ? e.message : e
				);
			}
		}

		const merged = { ...defaultSettings(), ...fromDb, ...legacy };
		if (!merged.deviceId) {
			merged.deviceId = generateDeviceId();
		}
		settingsCache = merged;
		await stateDb.saveSettings(merged);
	})();
	return initPromise;
}

export async function ensureSettingsLoaded(): Promise<void> {
	if (settingsCache) return;
	await initSettingsDb();
}

export function loadSettings(): StudioSettings {
	return settingsCache ?? defaultSettings();
}

export function setSettingsCache(settings: StudioSettings): void {
	settingsCache = settings;
}

export async function saveSettings(settings: StudioSettings): Promise<void> {
	settingsCache = settings;
	await stateDb.saveSettings(settings);
}
