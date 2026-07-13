import { env } from '$env/dynamic/private';
import { getCached } from '$lib/server/cache';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

export const load = async ({ locals }: { locals: any }) => {
	const token = locals?.token ?? null;
	const user = locals?.user ?? null;

	const permissions = token
		? await getCached<string[]>(`permissions:${user?.id}`, 5 * 60 * 1000, async () => {
			try {
				const res = await fetch(`${API_BASE}/api/v1/me/permissions`, {
					headers: { Authorization: `Bearer ${token}` }
				});
				if (res.ok) {
					const { data } = await res.json();
					return data?.permissions ?? [];
				}
			} catch {
				// Permissions are non-critical; degrade gracefully
			}
			return [];
		})
		: [];

	return {
		token,
		user,
		permissions
	};
};
