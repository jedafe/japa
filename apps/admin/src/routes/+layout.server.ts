import { env } from '$env/dynamic/private';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

export const load = async ({ locals }: { locals: any }) => {
	const token = locals?.token ?? null;
	const user = locals?.user ?? null;
	let permissions: string[] = [];

	if (token) {
		try {
			const res = await fetch(`${API_BASE}/api/v1/me/permissions`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				permissions = data?.permissions ?? [];
			}
		} catch {
			permissions = [];
		}
	}

	return {
		token,
		user,
		permissions
	};
};
