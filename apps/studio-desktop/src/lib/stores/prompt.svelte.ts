/**
 * Prompt builder form state.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { promptFromDefaults, type PromptFormValues, type PromptTemplate } from '$lib/types/prompt';
import { loadSettings } from '$lib/services/settings';
import {
	loadTemplates,
	saveTemplates,
	addTemplate,
	updateTemplateName,
	deleteTemplate
} from '$lib/services/templates';

function createPromptStore() {
	const settings = loadSettings();
	let values = $state<PromptFormValues>(promptFromDefaults(settings));
	let generated = $state<string>('');
	let templates = $state<PromptTemplate[]>(loadTemplates());

	function reset() {
		const settings = loadSettings();
		values = promptFromDefaults(settings);
		generated = '';
	}

	function setValue<K extends keyof PromptFormValues>(key: K, value: PromptFormValues[K]) {
		values = { ...values, [key]: value };
	}

	function loadTemplate(template: PromptTemplate) {
		values = structuredClone(template.values);
		generated = '';
	}

	function saveAsTemplate(name: string) {
		templates = addTemplate(templates, name, values);
	}

	function renameTemplate(id: string, name: string) {
		templates = updateTemplateName(templates, id, name);
	}

	function removeTemplate(id: string) {
		templates = deleteTemplate(templates, id);
	}

	function refreshTemplates() {
		templates = loadTemplates();
	}

	return {
		get values() {
			return values;
		},
		get generated() {
			return generated;
		},
		get templates() {
			return templates;
		},
		setGenerated(text: string) {
			generated = text;
		},
		setValue,
		reset,
		loadTemplate,
		saveAsTemplate,
		renameTemplate,
		removeTemplate,
		refreshTemplates
	};
}

export const promptStore = createPromptStore();
