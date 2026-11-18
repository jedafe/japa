import { t } from '#lib/i18n/index.js';
import { currencySymbol } from '#lib/utils/currency.js';

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
	/** Set when the field is a list (e.g. what_to_bring nested repeater) —
	 *  callers render it as bullets instead of the joined `value`. */
	items?: string[];
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
	if (Array.isArray(val)) return val.map(stepValueText).filter(Boolean).join(', ');
	if (typeof val === 'object') return JSON.stringify(val);
	return String(val);
}

/** {min,max,currency?} range objects → "₦5,000–20,000" / "NGN 5–20" / "5–14". */
function stepRangeText(val: Record<string, unknown>): string {
	const cur = typeof val.currency === 'string' ? val.currency.trim() : '';
	const min = stepValueText(val.min).trim();
	const max = stepValueText(val.max).trim();
	const range = min && max ? `${min}–${max}` : min || max;
	if (!cur) return range;
	// ISO codes resolve to their symbol (managed list, then Intl narrow
	// symbol); a stored symbol ("₦") is already display-ready. Symbols sit
	// flush against the range; unresolved codes take a space.
	const sym = /^[a-z]{3}$/i.test(cur) ? currencySymbol(cur) : cur;
	return /^[a-z]{3}$/i.test(sym) ? `${sym} ${range}` : `${sym}${range}`;
}

/** Ordered, labeled fields for a step object. step/title are surfaced as the
 *  headline by stepHeadline() and excluded here. */
export function stepFields(step: Record<string, unknown>): StepField[] {
	const entries = Object.entries(step).filter(
		([k, v]) =>
			k !== 'step' &&
			k !== 'title' &&
			v !== null &&
			v !== undefined &&
			v !== '' &&
			// all-blank lists (e.g. an untouched what_to_bring sub-repeater)
			// count as empty so no dangling label renders
			!(Array.isArray(v) && v.every((x) => stepValueText(x).trim() === ''))
	);
	entries.sort(([a], [b]) => {
		const ia = STEP_FIELD_ORDER.indexOf(a);
		const ib = STEP_FIELD_ORDER.indexOf(b);
		return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
	});
	return entries.map(([key, val]) => {
		// List values (what_to_bring nested repeater) render as bullets via
		// `items`; the joined string stays in `value` for JSON-LD fallbacks.
		const items = Array.isArray(val)
			? val.map((v) => stepValueText(v).trim()).filter(Boolean)
			: undefined;
		// {min,max,...} objects (fee_estimate, turnaround_days ranges) format
		// as "min–max" (with currency prefix when present). A {currency}-only
		// object still detects — otherwise it would leak raw JSON.
		const range =
			!items?.length &&
			isStepObject(val) &&
			('min' in val || 'max' in val || 'currency' in val)
				? stepRangeText(val)
				: undefined;
		// turnaround_days gets an "N days" suffix — numbers, numeric strings
		// (editorial repeater strings), and {min,max} ranges all match.
		const isTurnaround =
			key === 'turnaround_days' &&
			(typeof val === 'number' ||
				range !== undefined ||
				(typeof val === 'string' && /^\d+(\.\d+)?$/.test(val.trim())));
		const daysSuffix = (n: string) =>
			t('attestation_guide.n_days', '{n} days').replace('{n}', n);
		return {
			label:
				STEP_FIELD_LABELS[key] ??
				key.replaceAll('_', ' ').replace(/^./, (c) => c.toUpperCase()),
			value: isTurnaround
				? daysSuffix(range ?? String(val).trim())
				: (range ?? stepValueText(val)),
			url: key === 'url' || key === 'official_url' ? safeUrl(val) : null,
			items
		};
	});
}

/** A step's headline: "Step N" plus its title when the data carries one
 *  (editorial items are auto-numbered — no step field — while seeded
 *  pathways keep their descriptive titles: "Step 1: Notarize…"). */
export function stepHeadline(step: Record<string, unknown>, index: number): string {
	const s = step.step ?? step.title;
	const prefix = `${t('attestation_guide.step_n', 'Step')} ${index + 1}`;
	return typeof s === 'string' && s.trim() ? `${prefix}: ${s.trim()}` : prefix;
}
