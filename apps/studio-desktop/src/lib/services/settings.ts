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
	default_content_type: string;
	default_guide_category: string;
	default_persona_id: string;
	default_voice_id: string;
	default_space_id: string;
	default_status: 'draft' | 'in_review' | 'published' | 'scheduled';
	default_visibility: 'public' | 'private' | 'unlisted';
	default_tags: string[];
}

export function defaultSettings(): StudioSettings {
	return {
		projectName: '',
		baseUrl: '',
		apiKey: '',
		projectRoot: '',
		autoSync: false,
		autoSyncContentTypes: [],
		default_content_type: '',
		default_guide_category: '',
		default_persona_id: '',
		default_voice_id: '',
		default_space_id: '',
		default_status: 'draft',
		default_visibility: 'public',
		default_tags: []
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
			autoSyncContentTypes: parsed.autoSyncContentTypes ?? defaults.autoSyncContentTypes,
			default_content_type: parsed.default_content_type ?? defaults.default_content_type,
			default_guide_category: parsed.default_guide_category ?? defaults.default_guide_category,
			default_persona_id: parsed.default_persona_id ?? defaults.default_persona_id,
			default_voice_id: parsed.default_voice_id ?? defaults.default_voice_id,
			default_space_id: parsed.default_space_id ?? defaults.default_space_id,
			default_status: parsed.default_status ?? defaults.default_status,
			default_visibility: parsed.default_visibility ?? defaults.default_visibility,
			default_tags: parsed.default_tags ?? defaults.default_tags
		};
	} catch {
		return defaultSettings();
	}
}

export function saveSettings(settings: StudioSettings): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
}
