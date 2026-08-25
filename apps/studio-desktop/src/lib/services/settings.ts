/**
 * Local settings persistence.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export const SETTINGS_KEY = 'japa-studio:settings';

export interface StudioSettings {
	projectName: string;
	baseUrl: string;
	apiKey: string;
	projectRoot: string;
	autoSync: boolean;
	autoSyncContentTypes: string[];
}

export function defaultSettings(): StudioSettings {
	return {
		projectName: '',
		baseUrl: '',
		apiKey: '',
		projectRoot: '',
		autoSync: false,
		autoSyncContentTypes: []
	};
}

export function loadSettings(): StudioSettings {
	if (typeof localStorage === 'undefined') {
		return defaultSettings();
	}
	try {
		const raw = localStorage.getItem(SETTINGS_KEY);
		if (!raw) return defaultSettings();
		const parsed = JSON.parse(raw) as Partial<StudioSettings>;
		const defaults = defaultSettings();
		return {
			projectName: parsed.projectName ?? defaults.projectName,
			baseUrl: parsed.baseUrl ?? defaults.baseUrl,
			apiKey: parsed.apiKey ?? defaults.apiKey,
			projectRoot: parsed.projectRoot ?? defaults.projectRoot,
			autoSync: parsed.autoSync ?? defaults.autoSync,
			autoSyncContentTypes: parsed.autoSyncContentTypes ?? defaults.autoSyncContentTypes
		};
	} catch {
		return defaultSettings();
	}
}

export function saveSettings(settings: StudioSettings): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
}
