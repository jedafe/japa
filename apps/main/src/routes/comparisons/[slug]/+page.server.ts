import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface FieldGroup {
	id: string;
	name: string;
	sort_order: number;
}

interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description: string;
	field_groups: FieldGroup[];
}

interface Comparison {
	id: string;
	title: string;
	slug: string;
	item_count: number;
	vote_count: number;
	created_at: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const typeResult = await apiFetch<{ data: ComparisonType }>(
		`/comparison/types/${params.slug}`,
		{ headers }
	);

	if (!typeResult.ok || !typeResult.data?.data) {
		throw { status: 404, message: 'Comparison type not found' };
	}

	const compResult = await apiFetch<{ data: Comparison[] }>(
		`/comparison?type_slug=${params.slug}&limit=50`,
		{ headers }
	);

	return {
		type: typeResult.data.data,
		comparisons: compResult.ok && compResult.data?.data ? compResult.data.data : []
	};
};
