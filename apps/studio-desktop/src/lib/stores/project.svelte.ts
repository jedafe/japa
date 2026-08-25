/**
 * Reactive project store (Svelte 5 runes) for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface Project {
	id: string;
	name: string;
	watchedPath: string;
	baseUrl: string;
	apiKey: string;
	autoSync: boolean;
	autoSyncContentTypes: string[];
}

function createProjectStore() {
	let project = $state<Project | null>(null);

	return {
		get project() {
			return project;
		},
		setProject(value: Project) {
			project = value;
		},
		clear() {
			project = null;
		}
	};
}

export const projectStore = createProjectStore();
