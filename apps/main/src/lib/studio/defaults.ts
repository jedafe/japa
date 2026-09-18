/**
 * Project-level defaults for the web prompts tab, persisted to localStorage.
 * Separate key from the desktop app's `japa-studio:settings` so the two
 * surfaces never collide on shared machines.
 * Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §6
 */

export interface StudioWebDefaults {
	default_content_type?: string;
	default_guide_category?: string;
	default_space_id?: string;
	default_status?: string;
	default_visibility?: string;
	default_tags?: string[];
	default_persona_id?: string;
	default_voice_id?: string;
}

const STORAGE_KEY = 'japa-studio:web-settings';

export function loadStudioDefaults(): StudioWebDefaults {
	if (typeof localStorage === 'undefined') return {};
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return {};
		const parsed = JSON.parse(raw) as StudioWebDefaults;
		return parsed && typeof parsed === 'object' ? parsed : {};
	} catch {
		return {};
	}
}

export function saveStudioDefaults(defaults: StudioWebDefaults): void {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
	} catch {
		// Quota or privacy-mode failures are non-fatal — defaults are a convenience.
	}
}
