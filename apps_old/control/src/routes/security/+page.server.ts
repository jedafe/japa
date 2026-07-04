import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('control_token');
	const headers: Record<string, string> = {};
	if (token) headers.Authorization = `Bearer ${token}`;

	try {
		const res = await fetch(`${API_BASE}/admin/analytics/security?page=0`, { headers });
		if (!res.ok) return { initial: null };
		const body = await res.json();
		return { initial: body.data ?? body };
	} catch {
		return { initial: null };
	}
};
