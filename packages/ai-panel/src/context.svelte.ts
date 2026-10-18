import type { AiPageContext } from './types';

// §115 — page-context registry. The host app's layout extractor publishes
// the current page's entity context here; AiPanel reads it for the
// context chip + the stream's context payload. Cleared on navigation so a
// stale entity context can't leak across pages.
let current = $state<AiPageContext | null>(null);

export const aiPageContext = {
	get current() {
		return current;
	}
};

export function setAiPageContext(ctx: AiPageContext | null): void {
	current = ctx;
}
