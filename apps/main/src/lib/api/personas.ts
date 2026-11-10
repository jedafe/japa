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

/**
 * Admin persona detail — GET /admin/personas/{id}. Only resolvable by
 * viewers holding persona.manage (the route enforces it); the profile
 * page surfaces an Edit affordance when PublicUser.persona_id is set.
 */
export interface PersonaDetail {
	id: string;
	user_id: string;
	disclosure_mode: string;
	autonomy_mode: string;
	voice_id?: string;
	voice_prompt_override?: string | null;
	activity_policy?: Record<string, unknown>;
	status: string;
	backing_username: string;
	profile_name: string;
	profile_slug?: string;
	profile_tagline?: string | null;
	geo_space_id?: string;
	geo_space_name?: string | null;
	avatar_url?: string | null;
	cover_url?: string | null;
	avatar_media_id?: string;
	header_media_id?: string;
	profession?: string | null;
	location?: string | null;
	website_url?: string | null;
	education?: string | null;
	interests?: string[];
	tips_enabled?: boolean;
	points_balance?: number;
}

/** PATCH /admin/personas/{id} body — send only changed fields. */
export interface UpdatePersonaInput {
	name?: string;
	tagline?: string;
	username?: string;
	disclosure_mode?: string;
	status?: string;
	avatar_url?: string;
	avatar_media_id?: string;
	cover_url?: string;
	header_media_id?: string;
	profession?: string;
	location?: string;
	website_url?: string;
	education?: string;
	interests?: string[];
	tips_enabled?: boolean;
	geo_space?: string;
}

export async function getPersona(id: string): Promise<PersonaDetail | null> {
	const res = await api.get<PersonaDetail>(`/admin/personas/${id}`);
	return res.data ?? null;
}

export async function updatePersona(
	id: string,
	body: UpdatePersonaInput
): Promise<{ ok: boolean; error?: string }> {
	const res = await api.patch<PersonaDetail>(`/admin/personas/${id}`, body);
	if (res.error) return { ok: false, error: res.error.message };
	return { ok: true };
}
