/**
 * Prompt builder form state.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { DEFAULT_PROMPT_VALUES, type PromptFormValues } from '$lib/types/prompt';

function createPromptStore() {
	let values = $state<PromptFormValues>(structuredClone(DEFAULT_PROMPT_VALUES));
	let generated = $state<string>('');

	function reset() {
		values = structuredClone(DEFAULT_PROMPT_VALUES);
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
