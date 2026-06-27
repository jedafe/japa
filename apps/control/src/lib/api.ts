const API_BASE = 'http://localhost:8080/api/v1';

type FetchOptions = {
	method?: string;
	body?: unknown;
	signal?: AbortSignal;
};

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export async function api<T>(path: string, opts: FetchOptions = {}): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	const res = await fetch(`${API_BASE}${path}`, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body ? JSON.stringify(opts.body) : undefined,
		signal: opts.signal
	});

	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`API ${res.status}: ${text || res.statusText}`);
	}

	return res.json();
}
