// §87 — Japa Stage helpers for the public profile journey card/timeline.
// Derives an ordered milestone list from the member's current public stage.
import type { PublicJapaStage } from '$lib/types';

// Canonical progression order (ADR-122). 'returned' is an alternate terminal
// state — handled separately rather than placed in the linear order.
export const JOURNEY_ORDER = [
	'considering',
	'planning',
	'preparing',
	'applied',
	'approved',
	'traveled',
	'relocated',
	'settled'
] as const;

export type JourneyStepStatus = 'done' | 'current' | 'next';

export interface JourneyStep {
	stage: string;
	status: JourneyStepStatus;
}

/** Ordered milestone list for the timeline: stages before the current one are
 *  done, the current stage is in progress, later stages are upcoming.
 *  'returned' marks every step done. */
export function journeySteps(japaStage: PublicJapaStage | null): JourneyStep[] {
	const stage = japaStage?.stage;
	if (!stage) return [];
	const idx = (JOURNEY_ORDER as readonly string[]).indexOf(stage);
	return JOURNEY_ORDER.map((s, i) => ({
		stage: s,
		status: stage === 'returned' || (idx >= 0 && i < idx) ? 'done' : i === idx ? 'current' : 'next'
	}));
}

/** Progress bar percentage — position of the current stage in the order. */
export function journeyPercent(japaStage: PublicJapaStage | null): number {
	const stage = japaStage?.stage;
	if (!stage) return 0;
	if (stage === 'returned') return 100;
	const idx = (JOURNEY_ORDER as readonly string[]).indexOf(stage);
	if (idx < 0) return 0;
	return Math.round(((idx + 1) / JOURNEY_ORDER.length) * 100);
}

/** i18n key suffix for the journey title, per ADR-122 intent. */
export function journeyIntentKey(intent?: string): string {
	switch (intent) {
		case 'study':
		case 'work':
		case 'family':
		case 'visit':
		case 'business':
			return intent;
		case 'relocation':
		default:
			return 'relocation';
	}
}
