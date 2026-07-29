import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { Certificate, PublicUser } from '$lib/types';

/**
 * Public certificate page at /certificates/[id] — SSR, indexable, shareable.
 * Renders the earned certificate as social proof; links to the recipient's
 * public profile.
 *
 * Ref: japaconnect-engineering/sections/17-payments-learning.md
 *      ADR-060 (certificates badge the public profile and link here)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;

	let cert: Certificate;
	try {
		cert = await apiGet<Certificate>(ctx, `/certificates/${id}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Certificate not found' : 'Failed to load certificate'
		});
	}

	// Best-effort: resolve the recipient's public profile for the header.
	let recipient: PublicUser | null = null;
	try {
		recipient = await apiGet<PublicUser>(ctx, `/users/${cert.user_id}`);
	} catch {
		// The /users/{id} endpoint may not exist; /users/{username} is the
		// canonical one. Leave recipient null — the page renders without it.
		recipient = null;
	}

	return { cert, recipient };
};
