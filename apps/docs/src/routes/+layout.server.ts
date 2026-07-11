import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/api/client';
import type { User, DocNavTree } from '$lib/api/types';

const AUDIENCE_PERMISSIONS: Record<string, string> = {
	premium: 'feature.premium',
	organizations: 'feature.organizations',
	moderators: 'moderation.manage',
	admin: 'admin.access',
	superadmin: 'superadmin.access'
};

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

	if (user) {
		const groupId = user.user_group_id ?? '';
		const isAdminOrSuper = groupId === 'admin' || groupId === 'super_admin';
		navTree = navTree.filter((item) => {
			const requiredPerm = AUDIENCE_PERMISSIONS[item.audience];
			if (!requiredPerm) return true;
			if (isAdminOrSuper) return true;
			return groupId === item.audience;
		});
	} else {
		navTree = navTree.filter((item) => !AUDIENCE_PERMISSIONS[item.audience]);
	}

	return {
		user,
		isAuth: !!user,
		navTree,
		token: (locals as any).token ?? null,
	};
};
