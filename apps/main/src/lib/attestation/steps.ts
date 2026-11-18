import { t } from '#lib/i18n/index.js';

/**
 * Shared rendering helpers for Attestation Guide (§97/ADR-105) pathway steps.
 * pathway_steps is a JSONB array of free-form step objects produced by the
 * Content Agent and edited by admins — the helpers below map the canonical
 * keys to friendly labels in a stable order and keep unknown keys visible.
 */

// Canonical document types — mirror the backend's §97 enum (constrained by a
// DB CHECK) so filters only ever produce valid values.
export const DOC_TYPES = [
	{ value: 'birth_certificate', label: 'Birth certificate' },
	{ value: 'degree', label: 'Degree' },
	{ value: 'academic_transcript', label: 'Academic transcript' },
	{ value: 'marriage', label: 'Marriage certificate' },
	{ value: 'death_certificate', label: 'Death certificate' },
	{ value: 'divorce_certificate', label: 'Divorce certificate' },
	{ value: 'single_status', label: 'Single-status certificate' },
	{ value: 'adoption_papers', label: 'Adoption papers' },
	{ value: 'police_clearance', label: 'Police clearance' },
	{ value: 'professional_licence', label: 'Professional licence' },
	{ value: 'power_of_attorney', label: 'Power of attorney' },
	{ value: 'driving_licence', label: 'Driving licence' },
	{ value: 'employment_letter', label: 'Employment letter' },
	{ value: 'company_registration', label: 'Company registration' },
	{ value: 'medical_certificate', label: 'Medical certificate' },
	{ value: 'other', label: 'Other' }
] as const;

export function docTypeLabel(value: string): string {
	return (
		DOC_TYPES.find((d) => d.value === value)?.label ??
		value.replaceAll('_', ' ').replace(/^./, (c) => c.toUpperCase())
	);
}

const STEP_FIELD_LABELS: Record<string, string> = {
	step: 'Step',
	title: 'Title',
	description: 'Details',
	agency: 'Agency',
	what_to_bring: 'What to bring',
	fee_estimate: 'Estimated fee',
	turnaround_days: 'Turnaround',
	url: 'Link',
	official_url: 'Official link',
	note: 'Note'
};

const STEP_FIELD_ORDER = [
	'step',
	'title',
	'agency',
	'what_to_bring',
	'fee_estimate',
	'turnaround_days',
	'url',
	'official_url',
	'note',
	'description'
];

export interface StepField {
	label: string;
	value: string;
	url: string | null;
}

export function isStepObject(step: unknown): step is Record<string, unknown> {
	return step !== null && typeof step === 'object' && !Array.isArray(step);
}

/** Only http(s) URLs are linkified — other schemes render as plain text. */
export function safeUrl(val: unknown): string | null {
	if (typeof val !== 'string') return null;
	const v = val.trim();
	return /^https?:\/\//i.test(v) ? v : null;
}

export function stepValueText(val: unknown): string {
	if (val === null || val === undefined) return '';
	if (typeof val === 'object') return JSON.stringify(val);
	return String(val);
}

/** Ordered, labeled fields for a step object. step/title are surfaced as the
 *  headline by stepHeadline() and excluded here. */
export function stepFields(step: Record<string, unknown>): StepField[] {
	const entries = Object.entries(step).filter(
		([k, v]) => k !== 'step' && k !== 'title' && v !== null && v !== undefined && v !== ''
	);
	entries.sort(([a], [b]) => {
		const ia = STEP_FIELD_ORDER.indexOf(a);
		const ib = STEP_FIELD_ORDER.indexOf(b);
		return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
	});
	return entries.map(([key, val]) => ({
		label:
			STEP_FIELD_LABELS[key] ??
			key.replaceAll('_', ' ').replace(/^./, (c) => c.toUpperCase()),
		value:
			key === 'turnaround_days' && typeof val === 'number'
				? t('attestation_guide.n_days', '{n} days').replace('{n}', String(val))
				: stepValueText(val),
		url: key === 'url' || key === 'official_url' ? safeUrl(val) : null
	}));
}

/** A step's headline: its step/title text, else a numbered fallback. */
export function stepHeadline(step: Record<string, unknown>, index: number): string {
	const s = step.step ?? step.title;
	return typeof s === 'string' && s.trim()
		? s
		: `${t('attestation_guide.step_n', 'Step')} ${index + 1}`;
}
