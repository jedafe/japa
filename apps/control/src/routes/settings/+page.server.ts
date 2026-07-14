import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';
import type { SystemSetting } from '$lib/api/types';

function deriveSettingType(value: unknown): SystemSetting['type'] {
	if (value === true || value === false) return 'bool';
	if (typeof value === 'string') {
		const lower = value.toLowerCase();
		if (lower === 'true' || lower === 'false') return 'bool';
		if (/^\d+(\.\d+)?$/.test(value)) return value.includes('.') ? 'float' : 'int';
	}
	if (typeof value === 'number') return value % 1 === 0 ? 'int' : 'float';
	if (Array.isArray(value) || (typeof value === 'string' && value.includes(','))) return 'csv';
	return 'string';
}

function deriveSetting(raw: { key: string; value: unknown }): SystemSetting {
	const segments = raw.key.split('.');
	const type = deriveSettingType(raw.value);
	// Keep the raw value for display but ensure consistent string representation
	const valueStr = typeof raw.value === 'string' ? raw.value : JSON.stringify(raw.value);
	return {
		key: raw.key,
		value: valueStr,
		type,
		default_value: valueStr,
		description: segments[segments.length - 1] ?? raw.key,
		namespace: segments.slice(0, 2).join('.')
	};
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const res = await fetch(`${API_BASE}/control/system-settings`, {
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json'
			}
		});

		if (!res.ok) {
			return { settings: null, error: `API ${res.status}` };
		}

		const body = await res.json();
		const raw = body?.data ?? body;

		if (!Array.isArray(raw)) {
			return { settings: null, error: 'Unexpected response format' };
		}

		const settings: SystemSetting[] = raw.map((s: any) => deriveSetting(s));

		return { settings, error: null };
	} catch (e) {
		return {
			settings: null,
			error: e instanceof Error ? e.message : 'Network error'
		};
	}
};
