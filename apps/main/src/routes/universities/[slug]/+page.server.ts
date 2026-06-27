import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const spaceResult = await apiFetch<any>(`/spaces/${params.slug}`, { headers });

	if (!spaceResult.ok || !spaceResult.data) {
		error(404, { code: 'NOT_FOUND', message: 'University not found' });
	}

	const space = spaceResult.data;

	const fieldsResult = await apiFetch<any[]>(`/custom-field-values/space/${space.id}`, { headers });
	const fields = fieldsResult.ok && fieldsResult.data ? fieldsResult.data : [];

	const reviewsResult = await apiFetch<any>(
		`/reviews/aggregate?entity_type=space&entity_id=${space.id}`,
		{ headers }
	);
	const reviewAggregate = reviewsResult.ok && reviewsResult.data ? reviewsResult.data : null;

	const followersResult = await apiFetch<any[]>(
		`/follows/space/${space.id}/followers`,
		{ headers }
	);
	const followers = followersResult.ok && followersResult.data ? followersResult.data : [];

	return {
		university: space,
		fields,
		reviewAggregate,
		followerCount: followers.length
	};
};
