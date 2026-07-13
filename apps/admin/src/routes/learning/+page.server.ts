import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';

const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

export const load: PageServerLoad = async ({ locals }) => {
	const token = (locals as any)?.token;
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	try {
		const res = await fetch(`${API_BASE}/api/v1/admin/learn/tracks`, { headers });
		if (res.ok) {
			const { data } = await res.json();
			return { initial: data };
		}
		return { initial: null };
	} catch {
		return { initial: null, error: 'Failed to load' };
	}
};
