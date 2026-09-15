/**
 * Local state persistence.
 * Uses SQLite inside Tauri and falls back to localStorage in browsers / Vitest.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import type { StudioSettings } from './settings';
import type { FileState } from '$lib/types/queue';
import type { SyncLogEntry } from '$lib/stores/sync-log.svelte';

const DB_PATH = 'sqlite:state.db';
const PROJECT_ID = 'default';

const SETTINGS_KEY = 'japa-studio:settings';
const FILE_STATES_KEY = 'japa-studio:file-states';
const SYNC_LOG_KEY = 'japa-studio:sync-log';
const EDITOR_DRAFTS_KEY = 'japa-studio:editor-drafts';

export interface EditorDraft {
	body: string;
	frontmatter: string;
	isDirty: boolean;
	updatedAt: string;
}

export interface StorageAdapter {
	loadSettings(): Promise<StudioSettings | null>;
	saveSettings(settings: StudioSettings): Promise<void>;
	loadFileStates(): Promise<FileState[]>;
	saveAllFileStates(states: FileState[]): Promise<void>;
	saveFileState(file: FileState): Promise<void>;
	deleteFileState(relativePath: string): Promise<void>;
	loadSyncLog(): Promise<SyncLogEntry[]>;
	appendSyncLog(entry: SyncLogEntry): Promise<void>;
	clearSyncLog(): Promise<void>;
	loadEditorDraft(relativePath: string): Promise<EditorDraft | null>;
	saveEditorDraft(relativePath: string, draft: EditorDraft): Promise<void>;
}

let adapter: StorageAdapter | null = null;
let adapterPromise: Promise<StorageAdapter> | null = null;

function isTauri(): boolean {
	return typeof window !== 'undefined' && '__TAURI__' in window;
}

export async function getStorageAdapter(): Promise<StorageAdapter> {
	if (adapter) return adapter;
	if (adapterPromise) return adapterPromise;
	adapterPromise = createStorageAdapter();
	adapter = await adapterPromise;
	return adapter;
}

export function setStorageAdapter(custom: StorageAdapter): void {
	adapter = custom;
	adapterPromise = Promise.resolve(custom);
}

async function createStorageAdapter(): Promise<StorageAdapter> {
	if (isTauri()) {
		const { default: Database } = await import('@tauri-apps/plugin-sql');
		const db = await Database.load(DB_PATH);
		return new TauriDbAdapter(db);
	}
	return new LocalStorageAdapter();
}

export async function initDb(): Promise<void> {
	await getStorageAdapter();
}

export async function loadSettings(): Promise<StudioSettings | null> {
	return (await getStorageAdapter()).loadSettings();
}

export async function saveSettings(settings: StudioSettings): Promise<void> {
	return (await getStorageAdapter()).saveSettings(settings);
}

export async function loadFileStates(): Promise<FileState[]> {
	return (await getStorageAdapter()).loadFileStates();
}

export async function saveAllFileStates(states: FileState[]): Promise<void> {
	return (await getStorageAdapter()).saveAllFileStates(states);
}

export async function saveFileState(file: FileState): Promise<void> {
	return (await getStorageAdapter()).saveFileState(file);
}

export async function deleteFileState(relativePath: string): Promise<void> {
	return (await getStorageAdapter()).deleteFileState(relativePath);
}

export async function loadSyncLog(): Promise<SyncLogEntry[]> {
	return (await getStorageAdapter()).loadSyncLog();
}

export async function appendSyncLog(entry: SyncLogEntry): Promise<void> {
	return (await getStorageAdapter()).appendSyncLog(entry);
}

export async function clearSyncLog(): Promise<void> {
	return (await getStorageAdapter()).clearSyncLog();
}

export async function loadEditorDraft(relativePath: string): Promise<EditorDraft | null> {
	return (await getStorageAdapter()).loadEditorDraft(relativePath);
}

export async function saveEditorDraft(
	relativePath: string,
	draft: EditorDraft
): Promise<void> {
	return (await getStorageAdapter()).saveEditorDraft(relativePath, draft);
}

// --- Tauri SQLite adapter ---

class TauriDbAdapter implements StorageAdapter {
	private db: any; // Database from @tauri-apps/plugin-sql

	constructor(db: any) {
		this.db = db;
	}

	async loadSettings(): Promise<StudioSettings | null> {
		const rows = (await this.db.select(
			`SELECT * FROM projects WHERE id = $1`,
			[PROJECT_ID]
		)) as Array<Record<string, unknown>>;
		if (!rows.length) return null;
		return rowToSettings(rows[0]);
	}

	async saveSettings(settings: StudioSettings): Promise<void> {
		await this.db.execute(
			`INSERT OR REPLACE INTO projects (
				id, name, watched_path, base_url, api_key, mode, auto_sync,
				auto_sync_content_types, default_content_type, default_guide_category,
				default_persona_id, default_voice_id, default_space_id, default_status,
				default_visibility, default_tags, access_token, refresh_token,
				token_expires_at, device_id, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, datetime('now'))`,
			[
				PROJECT_ID,
				settings.projectName,
				settings.projectRoot,
				settings.baseUrl,
				settings.apiKey,
				settings.mode,
				settings.autoSync ? 1 : 0,
				JSON.stringify(settings.autoSyncContentTypes ?? []),
				settings.default_content_type,
				settings.default_guide_category,
				settings.default_persona_id,
				settings.default_voice_id,
				settings.default_space_id,
				settings.default_status,
				settings.default_visibility,
				JSON.stringify(settings.default_tags ?? []),
				settings.accessToken ?? null,
				settings.refreshToken ?? null,
				settings.tokenExpiresAt ?? null,
				settings.deviceId ?? null
			]
		);
	}

	async loadFileStates(): Promise<FileState[]> {
		const rows = (await this.db.select(
			`SELECT * FROM file_states WHERE project_id = $1 ORDER BY relative_path`,
			[PROJECT_ID]
		)) as Array<Record<string, unknown>>;
		return rows.map(rowToFileState);
	}

	async saveAllFileStates(states: FileState[]): Promise<void> {
		await this.db.execute(`DELETE FROM file_states WHERE project_id = $1`, [PROJECT_ID]);
		for (const file of states) {
			await this.saveFileState(file);
		}
	}

	async saveFileState(file: FileState): Promise<void> {
		await this.db.execute(
			`INSERT OR REPLACE INTO file_states (
				project_id, relative_path, content_id, content_type, slug, title, sync_state,
				last_hash, last_published_hash, last_published_at, last_error, space_id,
				persona_id, guide_category, status, visibility, modified_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			[
				PROJECT_ID,
				file.relativePath,
				file.contentId ?? null,
				file.contentType,
				file.slug,
				file.title,
				file.syncState,
				file.lastHash,
				file.lastPublishedHash || null,
				file.lastPublishedAt || null,
				file.lastError || null,
				file.spaceId ?? null,
				file.personaId ?? null,
				file.guideCategory ?? null,
				file.status ?? null,
				file.visibility ?? null,
				file.modifiedAt ?? null
			]
		);
	}

	async deleteFileState(relativePath: string): Promise<void> {
		await this.db.execute(
			`DELETE FROM file_states WHERE project_id = $1 AND relative_path = $2`,
			[PROJECT_ID, relativePath]
		);
	}

	async loadSyncLog(): Promise<SyncLogEntry[]> {
		const rows = (await this.db.select(
			`SELECT * FROM sync_log WHERE project_id = $1 ORDER BY created_at DESC LIMIT 1000`,
			[PROJECT_ID]
		)) as Array<Record<string, unknown>>;
		return rows.map(rowToSyncLog);
	}

	async appendSyncLog(entry: SyncLogEntry): Promise<void> {
		await this.db.execute(
			`INSERT INTO sync_log (id, project_id, relative_path, action, content_id, error, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			[
				entry.id,
				PROJECT_ID,
				entry.relativePath,
				entry.action,
				entry.contentId ?? null,
				entry.error ?? null,
				entry.createdAt
			]
		);
	}

	async clearSyncLog(): Promise<void> {
		await this.db.execute(`DELETE FROM sync_log WHERE project_id = $1`, [PROJECT_ID]);
	}

	async loadEditorDraft(relativePath: string): Promise<EditorDraft | null> {
		const rows = (await this.db.select(
			`SELECT body, frontmatter, is_dirty, updated_at FROM editor_drafts
			 WHERE project_id = $1 AND relative_path = $2`,
			[PROJECT_ID, relativePath]
		)) as Array<Record<string, unknown>>;
		if (!rows.length) return null;
		return rowToDraft(rows[0]);
	}

	async saveEditorDraft(relativePath: string, draft: EditorDraft): Promise<void> {
		await this.db.execute(
			`INSERT OR REPLACE INTO editor_drafts (
				project_id, relative_path, body, frontmatter, is_dirty, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6)`,
			[PROJECT_ID, relativePath, draft.body, draft.frontmatter, draft.isDirty ? 1 : 0, draft.updatedAt]
		);
	}
}

// --- localStorage fallback adapter ---

class LocalStorageAdapter implements StorageAdapter {
	async loadSettings(): Promise<StudioSettings | null> {
		return readJson<StudioSettings>(SETTINGS_KEY);
	}

	async saveSettings(settings: StudioSettings): Promise<void> {
		writeJson(SETTINGS_KEY, settings);
	}

	async loadFileStates(): Promise<FileState[]> {
		return readJson<FileState[]>(FILE_STATES_KEY) ?? [];
	}

	async saveAllFileStates(states: FileState[]): Promise<void> {
		writeJson(FILE_STATES_KEY, states);
	}

	async saveFileState(file: FileState): Promise<void> {
		const states = await this.loadFileStates();
		const index = states.findIndex((f) => f.relativePath === file.relativePath);
		if (index >= 0) {
			states[index] = file;
		} else {
			states.push(file);
		}
		await this.saveAllFileStates(states);
	}

	async deleteFileState(relativePath: string): Promise<void> {
		const states = (await this.loadFileStates()).filter((f) => f.relativePath !== relativePath);
		await this.saveAllFileStates(states);
	}

	async loadSyncLog(): Promise<SyncLogEntry[]> {
		return readJson<SyncLogEntry[]>(SYNC_LOG_KEY) ?? [];
	}

	async appendSyncLog(entry: SyncLogEntry): Promise<void> {
		const entries = [entry, ...(await this.loadSyncLog())].slice(0, 1000);
		writeJson(SYNC_LOG_KEY, entries);
	}

	async clearSyncLog(): Promise<void> {
		writeJson(SYNC_LOG_KEY, []);
	}

	async loadEditorDraft(relativePath: string): Promise<EditorDraft | null> {
		const map = readJson<Record<string, EditorDraft>>(EDITOR_DRAFTS_KEY) ?? {};
		return map[relativePath] ?? null;
	}

	async saveEditorDraft(relativePath: string, draft: EditorDraft): Promise<void> {
		const map = readJson<Record<string, EditorDraft>>(EDITOR_DRAFTS_KEY) ?? {};
		map[relativePath] = draft;
		writeJson(EDITOR_DRAFTS_KEY, map);
	}
}

// --- helpers ---

function readJson<T>(key: string): T | null {
	if (typeof localStorage === 'undefined') return null;
	try {
		const raw = localStorage.getItem(key);
		if (!raw) return null;
		return JSON.parse(raw) as T;
	} catch (e) {
		console.error(`Failed to read ${key} from localStorage:`, e instanceof Error ? e.message : e);
		return null;
	}
}

function writeJson<T>(key: string, value: T): void {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(key, JSON.stringify(value));
	} catch (e) {
		console.error(`Failed to write ${key} to localStorage:`, e instanceof Error ? e.message : e);
	}
}

function rowToSettings(row: Record<string, unknown>): StudioSettings {
	return {
		projectName: String(row.name ?? ''),
		baseUrl: String(row.base_url ?? ''),
		apiKey: String(row.api_key ?? ''),
		projectRoot: String(row.watched_path ?? ''),
		mode: (row.mode as StudioSettings['mode']) ?? 'operator',
		autoSync: Boolean(row.auto_sync ?? 0),
		autoSyncContentTypes: parseJson(row.auto_sync_content_types as string, []),
		default_content_type: String(row.default_content_type ?? ''),
		default_guide_category: String(row.default_guide_category ?? ''),
		default_persona_id: String(row.default_persona_id ?? ''),
		default_voice_id: String(row.default_voice_id ?? ''),
		default_space_id: String(row.default_space_id ?? ''),
		default_status: (row.default_status as StudioSettings['default_status']) ?? 'draft',
		default_visibility: (row.default_visibility as StudioSettings['default_visibility']) ?? 'public',
		default_tags: parseJson(row.default_tags as string, []),
		accessToken: row.access_token ? String(row.access_token) : undefined,
		refreshToken: row.refresh_token ? String(row.refresh_token) : undefined,
		tokenExpiresAt: row.token_expires_at ? Number(row.token_expires_at) : undefined,
		deviceId: row.device_id ? String(row.device_id) : undefined
	};
}

function rowToFileState(row: Record<string, unknown>): FileState {
	const modifiedAt = row.modified_at ? String(row.modified_at) : undefined;
	const lastPublishedAt = String(row.last_published_at ?? '');
	const status = row.status ? String(row.status) : undefined;
	const syncState = String(row.sync_state ?? 'new') as FileState['syncState'];
	return {
		relativePath: String(row.relative_path ?? ''),
		contentType: String(row.content_type ?? ''),
		slug: String(row.slug ?? ''),
		title: String(row.title ?? ''),
		lastHash: String(row.last_hash ?? ''),
		lastPublishedHash: String(row.last_published_hash ?? ''),
		lastPublishedAt,
		syncState,
		lastError: String(row.last_error ?? ''),
		contentId: row.content_id ? String(row.content_id) : undefined,
		spaceId: row.space_id ? String(row.space_id) : undefined,
		personaId: row.persona_id ? String(row.persona_id) : undefined,
		guideCategory: row.guide_category ? String(row.guide_category) : undefined,
		status,
		visibility: row.visibility ? String(row.visibility) : undefined,
		modifiedAt,
		// Display aliases used by the queue table (Section 105).
		lastModified: modifiedAt,
		lastPublished: lastPublishedAt,
		japaStatus: status,
		localStatus: syncState
	};
}

function rowToSyncLog(row: Record<string, unknown>): SyncLogEntry {
	return {
		id: String(row.id ?? ''),
		relativePath: String(row.relative_path ?? ''),
		action: String(row.action ?? 'failed') as SyncLogEntry['action'],
		contentId: row.content_id ? String(row.content_id) : undefined,
		error: row.error ? String(row.error) : undefined,
		createdAt: String(row.created_at ?? '')
	};
}

function rowToDraft(row: Record<string, unknown>): EditorDraft {
	return {
		body: String(row.body ?? ''),
		frontmatter: String(row.frontmatter ?? '{}'),
		isDirty: Boolean(row.is_dirty ?? 0),
		updatedAt: String(row.updated_at ?? '')
	};
}

function parseJson<T>(value: string | null | undefined, fallback: T): T {
	if (!value) return fallback;
	try {
		return JSON.parse(value) as T;
	} catch {
		return fallback;
	}
}
