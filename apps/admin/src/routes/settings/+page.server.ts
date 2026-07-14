import { fail } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

interface SettingDef {
	key: string;
	label: string;
	type: string;
	value?: unknown;
	options?: string[];
	masked?: boolean;
}

interface SettingCategory {
	key: string;
	label: string;
	settings: SettingDef[];
}

async function fetchSettings(token: string): Promise<SettingCategory[]> {
	const res = await fetch(`${API_BASE}/api/v1/admin/settings`, {
		headers: { Authorization: `Bearer ${token}` }
	});

	if (!res.ok) return [];

	const body = await res.json();
	return (body?.data ?? []) as SettingCategory[];
}

export const load = async ({ locals }: { locals: any }) => {
	const token = locals?.token ?? null;

	if (!token) {
		return { categories: [] as SettingCategory[] };
	}

	const categories = await fetchSettings(token);
	return { categories };
};

export const actions = {
	default: async ({ request, locals }: { request: Request; locals: any }) => {
		const token = locals?.token ?? null;

		if (!token) {
			return fail(401, { success: false, error: 'Unauthorized' });
		}

		const formData = await request.formData();
		const categoryKey = formData.get('_category') as string;

		if (!categoryKey) {
			return fail(400, { success: false, error: 'Missing category' });
		}

		const entries: Record<string, unknown> = {};

		for (const [key, value] of formData.entries()) {
			if (key === '_category') continue;
			if (value === 'true') entries[key] = true;
			else if (value === 'false') entries[key] = false;
			else entries[key] = value;
		}

		try {
			const res = await fetch(`${API_BASE}/api/v1/admin/settings/${categoryKey}`, {
				method: 'PUT',
				headers: {
					Authorization: `Bearer ${token}`,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(entries)
			});

			if (!res.ok) {
				const err = await res.json().catch(() => ({ message: 'Failed to save settings' }));
				return fail(500, { success: false, error: err.message ?? 'Failed to save settings' });
			}

			return { success: true, savedCategory: categoryKey };
		} catch {
			return fail(500, { success: false, error: 'Network error saving settings' });
		}
	}
};
