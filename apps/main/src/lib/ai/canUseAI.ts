import type { SessionUser } from '$lib/types';

/**
 * Determine whether the active user can use the inline AI body assistant.
 *
 * Gated by the `feature.ai_content_gen` flag — the same reachability gate the
 * backend applies to /ai/assist/* — plus the legacy `inline_ai_assistant` key
 * and the `ai.editor.assist` permission node. Flag keys missing from the map
 * default to enabled (an absent flag can't be evaluated client-side).
 */
export function canUseAI(
	user: SessionUser | null | undefined,
	features: Record<string, boolean> = {}
): boolean {
	if (features['feature.ai_content_gen'] === false || features.inline_ai_assistant === false) {
		return false;
	}
	return user?.permissions?.includes('ai.editor.assist') ?? false;
}
