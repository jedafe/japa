import type { PageLoad } from './$types';
import { apiFetch } from '$lib/api/client';
import { landingPalettes, type Block } from '@japaconnect/landing-blocks';

interface LandingPage {
	id: string;
	title: string;
	slug: string;
	page_type: string;
	status: string;
	blocks: Block[] | string;
	theme_id?: string;
	updated_at: string;
}

interface Theme {
	id: string;
	name: string;
	slug: string;
}

interface GeoCountry {
	id: string;
	name: string;
	geo_code: string;
}

// parseBlocks normalises the blocks payload which may be stored as a JSON
// string or as a parsed array, into a typed Block[].
function parseBlocks(raw: LandingPage['blocks']): Block[] {
	if (Array.isArray(raw)) return raw as Block[];
	if (typeof raw === 'string') {
		try {
			const parsed = JSON.parse(raw);
			return Array.isArray(parsed) ? (parsed as Block[]) : [];
		} catch {
			return [];
		}
	}
	return [];
}

export const load: PageLoad = async ({ params, fetch }) => {
	const { slug } = params;

	// Fetch the page by slug. The admin pages endpoint returns an array when
	// filtered by query params, so we take the first match.
	const listResult = await apiFetch<LandingPage[]>(
		`/admin/pages?slug=${encodeURIComponent(slug)}`,
		{},
		fetch
	);

	let page: LandingPage;
	if (listResult.data && listResult.data.length > 0) {
		page = { ...listResult.data[0], blocks: parseBlocks(listResult.data[0].blocks) };
	} else {
		// Page not found — create a new one seeded with the homepage palette.
		const createResult = await apiFetch<LandingPage>(
			'/admin/pages',
			{
				method: 'POST',
				body: JSON.stringify({
					title: slug.charAt(0).toUpperCase() + slug.slice(1),
					slug,
					page_type: 'landing',
					blocks: JSON.stringify(landingPalettes['homepage']?.defaultSeed ?? []),
				}),
			},
			fetch
		);

		if (createResult.data) {
			page = { ...createResult.data, blocks: parseBlocks(createResult.data.blocks) };
		} else {
			// Fallback — return an empty draft page so the editor can still render.
			page = {
				id: '',
				title: slug,
				slug,
				page_type: 'landing',
				status: 'draft',
				blocks: [],
				updated_at: new Date().toISOString(),
			};
		}
	}

	// Fetch available themes for the theme selector.
	const themesResult = await apiFetch<Theme[]>('/admin/pagebuilder/themes', {}, fetch);
	const themes = themesResult.data ?? [];

	// Fetch supported geo countries for the preview-country selector.
	// Ref: Rule 16 — no hardcoded country lists; countries come from the API.
	const countriesResult = await apiFetch<GeoCountry[]>('/geo/countries', {}, fetch);
	const countries = countriesResult.data ?? [];

	// Determine palette (default to homepage, fall back to first available).
	const palette =
		landingPalettes['homepage'] ?? landingPalettes[Object.keys(landingPalettes)[0]];

	return {
		page,
		palette,
		themes,
		countries,
	};
};
