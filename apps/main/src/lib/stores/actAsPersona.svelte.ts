/**
 * Act-as-persona store — page-scoped identity override for delegated
 * authorship (spec §1.8). The selection is NOT a session switch: auth stays
 * the operator; supported calls attach author_id/actor_id so the persona is
 * the recorded actor. A cookie mirrors the selection so SSR comment loads
 * can forward ?actor_id= and return persona-scoped vote state.
 */
import { listAssignablePersonas, type AssignablePersona } from '#lib/api/personas.js';

const COOKIE = 'act_as_persona';
const isBrowser = () => typeof document !== 'undefined';

class ActAsPersonaStore {
	items = $state<AssignablePersona[]>([]);
	selected = $state<AssignablePersona | null>(null);
	private loaded = false;

	/** Lazy fetch — callers without content.assign_author get a 403 from
	 *  /me/personas, which degrades to an empty list (picker stays hidden). */
	async load() {
		if (this.loaded || !isBrowser()) return;
		this.loaded = true;
		try {
			this.items = await listAssignablePersonas();
		} catch {
			this.items = [];
		}
		this.restoreSelection();
	}

	select(persona: AssignablePersona | null) {
		this.selected = persona;
		if (!isBrowser()) return;
		document.cookie = persona
			? `${COOKIE}=${persona.user_id}; path=/; max-age=${60 * 60 * 24 * 30}; SameSite=Lax`
			: `${COOKIE}=; path=/; max-age=0`;
	}

	/** The user_id to send as author_id/actor_id, or null when acting as
	 *  the operator. */
	get actorId(): string | null {
		return this.selected?.user_id ?? null;
	}

	private restoreSelection() {
		if (!isBrowser() || this.items.length === 0) return;
		const match = document.cookie.match(new RegExp(`(?:^|; )${COOKIE}=([^;]*)`));
		const id = match?.[1];
		if (id) this.selected = this.items.find((p) => p.user_id === id) ?? null;
	}
}

export const actAsPersona = new ActAsPersonaStore();
