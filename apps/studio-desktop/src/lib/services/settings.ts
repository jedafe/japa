/**
 * Local settings persistence.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export const SETTINGS_KEY = 'japa-studio:settings';

export interface StudioSettings {
	baseUrl: string;
	apiKey: string;
	projectRoot: string;
}

export function loadSettings(): StudioSettings {
	if (typeof localStorage === 'undefined') {
		return { baseUrl: '', apiKey: '', projectRoot: '' };
	}
	try {
		const raw = localStorage.getItem(SETTINGS_KEY);
		if (!raw) return { baseUrl: '', apiKey: '', projectRoot: '' };
		const parsed = JSON.parse(raw) as Partial<StudioSettings>;
		return {
			baseUrl: parsed.baseUrl ?? '',
			apiKey: parsed.apiKey ?? '',
			projectRoot: parsed.projectRoot ?? ''
		};
	} catch {
		return { baseUrl: '', apiKey: '', projectRoot: '' };
	}
}

export function saveSettings(settings: StudioSettings): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
}
