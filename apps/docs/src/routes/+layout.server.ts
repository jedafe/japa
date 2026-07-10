import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/api/client';
import type { User, DocNavTree } from '$lib/api/types';

export const load: LayoutServerLoad = async ({ locals }) => {
	const user = locals.user as User | null;

	let navTree: DocNavTree[] = [];
	try {
		const navResult = await apiFetch<DocNavTree[]>('/docs/nav', {}, fetch);
		if (navResult.data) {
			navTree = navResult.data;
		}
	} catch {
		// Nav tree is non-critical; degrade gracefully
	}

	return {
		user,
		isAuth: !!user,
		navTree,
	};
};
