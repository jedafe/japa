import type { SessionUser } from '$lib/types';

/**
 * Determine whether the active user can use the inline AI body assistant.
 *
 * Gated by the `inline_ai_assistant` feature flag (defaults to enabled when
 * missing) and the `ai.editor.assist` permission node.
 */
export function canUseAI(
	user: SessionUser | null | undefined,
	features: Record<string, boolean> = {}
): boolean {
	if (features.inline_ai_assistant === false) return false;
	return user?.permissions?.includes('ai.editor.assist') ?? false;
}
