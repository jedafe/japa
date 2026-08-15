// Ref: japaconnect-engineering/sections/33-professional-profiles.md (profile admin dashboard)
// Loads the full profile row (by ID) + members + invitations + relocations +
// documents + verifiers for the admin view. Auth-gated; the backend self-403s
// non-owners/non-admins.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	GeoCountry,
	ProfessionalProfile,
	ProfessionalProfileMember,
	ProfessionalProfileInvitation,
	ProfessionalRelocation,
	ProfessionalDocument,
	ProfessionalVerifier
} from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const id = event.params.id;

	let profile: ProfessionalProfile | null = null;
	try {
		profile = await apiGet<ProfessionalProfile>(ctx, `/profiles/${id}`);
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		throw error(status, { code, message: 'Profile not found' });
	}

	const [membersRes, invitationsRes, relocationsRes, documentsRes, verifiersRes, countriesRes] = await Promise.all([
		apiList<ProfessionalProfileMember>(ctx, `/profiles/${id}/members`).catch(() => ({ items: [] as ProfessionalProfileMember[] })),
		apiList<ProfessionalProfileInvitation>(ctx, `/profiles/${id}/invitations`).catch(() => ({ items: [] as ProfessionalProfileInvitation[] })),
		apiList<ProfessionalRelocation>(ctx, `/profiles/${id}/relocations`).catch(() => ({ items: [] as ProfessionalRelocation[] })),
		apiList<ProfessionalDocument>(ctx, `/profiles/${id}/documents`).catch(() => ({ items: [] as ProfessionalDocument[] })),
		apiList<ProfessionalVerifier>(ctx, `/profiles/${id}/verifiers`).catch(() => ({ items: [] as ProfessionalVerifier[] })),
		// Country list from the spaces country table (Rule 16).
		apiList<GeoCountry>(ctx, '/geo/countries').catch(() => ({ items: [] as GeoCountry[] }))
	]);

	return {
		profile,
		members: membersRes.items,
		invitations: invitationsRes.items,
		relocations: relocationsRes.items,
		documents: documentsRes.items,
		verifiers: verifiersRes.items,
		countries: countriesRes.items
	};
};
