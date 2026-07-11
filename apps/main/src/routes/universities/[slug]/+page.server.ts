import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Space, CustomFieldValue, ReviewAggregate } from '$lib/types';

interface Follower {
	id: string;
	username: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const spaceResult = await apiFetch<Space>(`/spaces/${params.slug}`, { headers });

	if (!spaceResult.ok || !spaceResult.data) {
		error(404, { code: 'NOT_FOUND', message: 'University not found' });
	}

	const space = spaceResult.data;

	const fieldsResult = await apiFetch<CustomFieldValue[]>(`/custom-field-values/space/${space.id}`, { headers });
	const fields = fieldsResult.ok && fieldsResult.data ? fieldsResult.data : [];

	const reviewsResult = await apiFetch<ReviewAggregate>(
		`/reviews/space/${space.id}/aggregate`,
		{ headers }
	);
	const reviewAggregate = reviewsResult.ok && reviewsResult.data ? reviewsResult.data : null;

	const followersResult = await apiFetch<Follower[]>(
		`/spaces/${space.id}/followers`,
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
