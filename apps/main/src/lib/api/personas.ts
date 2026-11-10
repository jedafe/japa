/**
 * Persona API client — delegated authorship (spec §1.8).
 * Feeds the "Interact as" picker: personas the operator may drive via
 * author_id/actor_id overrides. GET /me/personas is gated server-side on
 * content.assign_author — callers without it get a 403, which the store
 * degrades to an empty list.
 */
import { api } from '#lib/api/client.js';

/** GET /me/personas row — slim payload for pickers. */
export interface AssignablePersona {
	/** ai_personas.id */
	id: string;
	/** Backing bot user id — the value sent as author_id/actor_id. */
	user_id: string;
	/** Display name (user_profiles.display_name ?? username). */
	name: string;
	username: string;
	avatar_url?: string | null;
	geo_space_id?: string | null;
	geo_space_name?: string | null;
}

export async function listAssignablePersonas(): Promise<AssignablePersona[]> {
	const res = await api.get<AssignablePersona[]>('/me/personas');
	return res.data ?? [];
}
