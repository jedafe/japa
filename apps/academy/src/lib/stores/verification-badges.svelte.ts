// Badge-type artwork cache (§07 amendment, migration 001013). The public
// /verification-requests/types endpoint already resolves badge_url per
// (scope, value); components look artwork up by an entity's stored
// verification_type and fall back to the generic check when a type has no
// custom image.
import { api } from '#lib/api/client.js';

interface BadgeType {
	scope: string;
	value: string;
	label: string;
	badge_url?: string;
}

let urls = $state<Record<string, string>>({});
let labels = $state<Record<string, string>>({});
let loaded = $state(false);
let pending: Promise<void> | null = null;

export function loadVerificationBadges(): Promise<void> {
	if (loaded) return Promise.resolve();
	if (pending) return pending;
	pending = (async () => {
		const res = await api.get<{ types?: BadgeType[] }>('/verification-requests/types');
		if (!res.error && res.data?.types) {
			const u: Record<string, string> = {};
			const l: Record<string, string> = {};
			for (const t of res.data.types) {
				l[`${t.scope}:${t.value}`] = t.label;
				if (t.badge_url) u[`${t.scope}:${t.value}`] = t.badge_url;
			}
			urls = u;
			labels = l;
		}
		loaded = true;
		pending = null;
	})();
	return pending;
}

/** Resolved badge artwork URL for a (scope, verification_type) pair; null when
 *  the type has no custom image. Triggers the one-time types fetch on first call. */
export function verificationBadgeUrl(scope: string, verificationType?: string | null): string | null {
	if (!verificationType) return null;
	if (!loaded) {
		void loadVerificationBadges();
		return null;
	}
	return urls[`${scope}:${verificationType}`] ?? null;
}

/** Human label for a (scope, verification_type) pair — for badge alt/title text. */
export function verificationBadgeLabel(scope: string, verificationType?: string | null): string | null {
	if (!verificationType) return null;
	if (!loaded) {
		void loadVerificationBadges();
		return null;
	}
	return labels[`${scope}:${verificationType}`] ?? null;
}
