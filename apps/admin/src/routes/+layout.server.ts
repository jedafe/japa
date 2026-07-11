import { PUBLIC_JC_API_URL } from '$env/static/public';

export const load = async ({ locals }: { locals: any }) => {
	const token = locals?.token ?? null;
	let permissions: string[] = [];

	if (token) {
		try {
			const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/permissions`, {
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
		permissions
	};
};
