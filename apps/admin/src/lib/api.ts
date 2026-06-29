const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	try {
		const res = await fetch(`${API_BASE}${path}`, {
			method: opts.method ?? 'GET',
			headers,
			body: opts.body ? JSON.stringify(opts.body) : undefined,
			signal: opts.signal
		});

		if (!res.ok) {
			const text = await res.text().catch(() => '');
			return { ok: false, error: `API ${res.status}: ${text || res.statusText}` };
		}

		const raw = await res.json();
		// Unwrap the backend's { data: ... } envelope so callers get the payload directly.
		const data = raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
		return { ok: true, data };
	} catch (e) {
		return { ok: false, error: e instanceof Error ? e.message : 'Network error' };
	}
}
