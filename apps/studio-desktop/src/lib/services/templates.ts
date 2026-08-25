/**
 * Local prompt template persistence.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import type { PromptFormValues, PromptTemplate } from '$lib/types/prompt';

export const TEMPLATES_KEY = 'japa-studio:prompt-templates';

export function loadTemplates(): PromptTemplate[] {
	if (typeof localStorage === 'undefined') return [];
	try {
		const raw = localStorage.getItem(TEMPLATES_KEY);
		if (!raw) return [];
		const parsed = JSON.parse(raw) as PromptTemplate[];
		if (!Array.isArray(parsed)) return [];
		return parsed;
	} catch {
		return [];
	}
}

export function saveTemplates(templates: PromptTemplate[]): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(TEMPLATES_KEY, JSON.stringify(templates));
}

function randomId(): string {
	return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export function addTemplate(
	templates: PromptTemplate[],
	name: string,
	values: PromptFormValues
): PromptTemplate[] {
	const next = [
		...templates,
		{
			id: randomId(),
			name: name.trim(),
			values: structuredClone(values),
			created_at: new Date().toISOString()
		}
	];
	saveTemplates(next);
	return next;
}

export function updateTemplateName(
	templates: PromptTemplate[],
	id: string,
	name: string
): PromptTemplate[] {
	const next = templates.map((t) => (t.id === id ? { ...t, name: name.trim() } : t));
	saveTemplates(next);
	return next;
}

export function deleteTemplate(templates: PromptTemplate[], id: string): PromptTemplate[] {
	const next = templates.filter((t) => t.id !== id);
	saveTemplates(next);
	return next;
}
