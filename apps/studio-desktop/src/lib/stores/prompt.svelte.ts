/**
 * Prompt builder form state.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { promptFromDefaults, type PromptFormValues } from '$lib/types/prompt';
import { loadSettings } from '$lib/services/settings';

function createPromptStore() {
	const settings = loadSettings();
	let values = $state<PromptFormValues>(promptFromDefaults(settings));
	let generated = $state<string>('');

	function reset() {
		const settings = loadSettings();
		values = promptFromDefaults(settings);
		generated = '';
	}

	function setValue<K extends keyof PromptFormValues>(key: K, value: PromptFormValues[K]) {
		values = { ...values, [key]: value };
	}

	return {
		get values() {
			return values;
		},
		get generated() {
			return generated;
		},
		setGenerated(text: string) {
			generated = text;
		},
		setValue,
		reset
	};
}

export const promptStore = createPromptStore();
