import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { availableTypes } from '$lib/components/create/contentTypeRegistry';
import type {
	ContentTypeMeta,
	MeSpace,
	CustomFieldGroup,
	BlogSeries,
	ContentRecord
} from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	// /create is auth-gated — a guest has no spaces, drafts, or series to compose with.
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required to create content' });
	}

	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [typesRes, flagsRes, spacesRes, groupsRes, seriesRes] = await Promise.allSettled([
		apiList<ContentTypeMeta>(ctx, '/content-types'),
		apiGet<Record<string, boolean>>(ctx, '/feature-flags'),
		apiList<MeSpace>(ctx, '/me/spaces'),
		apiList<CustomFieldGroup>(ctx, '/custom-field-groups'),
		apiList<BlogSeries>(ctx, '/blog/series')
	]);

	const rawTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
	const flags = flagsRes.status === 'fulfilled' ? flagsRes.value : {};
	const spaces = spacesRes.status === 'fulfilled' ? spacesRes.value.items : [];
	const fieldGroups =
		groupsRes.status === 'fulfilled'
			? groupsRes.value.items.filter((g) => g.entity_type === 'content')
			: [];
	const series = seriesRes.status === 'fulfilled' ? seriesRes.value.items : [];

	let draft: ContentRecord | null = null;
	if (draftId) {
		try {
			// Ownership-scoped: GET /me/drafts/{id} returns the caller's own
			// draft only (tenant + author + status='draft'); 404 otherwise.
			// The public GET /content/{slug} resolves by slug and is not
			// ownership-scoped, so it cannot safely serve the editor restore.
			draft = await apiGet<ContentRecord>(ctx, `/me/drafts/${draftId}`);
		} catch {
			draft = null;
		}
	}

	return {
		types: availableTypes(rawTypes, flags),
		spaces,
		fieldGroups,
		series,
		draft,
		loadError: typesRes.status === 'rejected'
	};
};
