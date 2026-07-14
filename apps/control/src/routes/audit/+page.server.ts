import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = {};
	if (token) headers.Authorization = `Bearer ${token}`;

	try {
		const res = await fetch(`${API_BASE}/control/audit-logs?page=0`, { headers });
		if (!res.ok) return { initial: null };
		const body = await res.json();
		const raw = body.data ?? body;
		const entries = Array.isArray(raw) ? raw : [];
		return { initial: { entries, page: 0, total_pages: 1 } };
	} catch {
		return { initial: null };
	}
};
