import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { JCEComparisonSummary } from '$lib/types/jce';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const searchParams = event.url.searchParams;

	const params = new URLSearchParams();
	if (searchParams.get('type')) params.set('type', searchParams.get('type')!);
	if (searchParams.get('depth')) params.set('depth', searchParams.get('depth')!);
	if (searchParams.get('status')) params.set('status', searchParams.get('status')!);
	if (searchParams.get('visibility')) params.set('visibility', searchParams.get('visibility')!);
	if (searchParams.get('q')) params.set('q', searchParams.get('q')!);
	const query = params.toString();

	let items: JCEComparisonSummary[] = [];
	try {
		const res = await apiGet<JCEComparisonSummary[]>(ctx, `/jce/comparisons?${query}`);
		items = res ?? [];
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			items = [];
		} else {
			throw err;
		}
	}

	return {
		items,
		filters: {
			type: searchParams.get('type') ?? '',
			depth: searchParams.get('depth') ?? '',
			status: searchParams.get('status') ?? '',
			visibility: searchParams.get('visibility') ?? '',
			q: searchParams.get('q') ?? ''
		}
	};
};
