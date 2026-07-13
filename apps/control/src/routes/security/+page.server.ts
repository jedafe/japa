import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = {};
	if (token) headers.Authorization = `Bearer ${token}`;

	const [securityRes, ipBlocksRes] = await Promise.all([
		fetch(`${API_BASE}/control/security?page=0`, { headers }).catch(() => null),
		fetch(`${API_BASE}/control/security/ip-blocks`, { headers }).catch(() => null),
	]);

	let initial = null;
	if (securityRes?.ok) {
		const body = await securityRes.json();
		initial = body.data ?? body;
	}

	let ipBlocks: unknown[] = [];
	if (ipBlocksRes?.ok) {
		const body = await ipBlocksRes.json();
		ipBlocks = body.data ?? [];
	}

	return { initial, ipBlocks };
};
