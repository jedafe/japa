// Shared helpers for the Community Achievement Feed (§89, ADR-097).
//
// milestoneTypes is the spec-defined enum set, derived from the
// AchievementMilestoneType type so there is a single source of truth.
// milestoneOptions() builds the {value, label} pairs for <select> elements,
// with labels resolved via the i18n $t() function passed by the caller.
//
// Ref: japaconnect-engineering/sections/89-achievement-feed.md

import type { AchievementMilestoneType } from '$lib/types';

/** Spec-defined milestone type enum (§89 Schema). Single source of truth. */
export const milestoneTypes: AchievementMilestoneType[] = [
	'visa_approved',
	'job_offer',
	'university_accepted',
	'arrived',
	'one_year',
	'citizenship',
	'other'
];

/**
 * Build {value, label} option pairs for milestone <select> elements.
 * The label translator is injected so this module stays free of i18n
 * store coupling — callers pass their `$t` function.
 */
export function milestoneOptions(
	translate: (key: string, fallback?: string) => string
): { value: AchievementMilestoneType; label: string }[] {
	return milestoneTypes.map((value) => ({
		value,
		label: translate(`achievements.type_${value}`, value.replace(/_/g, ' '))
	}));
}
