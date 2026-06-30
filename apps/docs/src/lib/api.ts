const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export function getToken(): string | null {
	return jwtToken;
}

interface ApiOk<T> {
	data: T;
}

interface ApiErr {
	error: { code: string; message: string };
}

export interface DocPage {
	id: string;
	title: string;
	slug: string;
	audience: string;
	content: Record<string, unknown>;
	ai_generated: boolean;
	review_status: string;
	parent_id: string | null;
	position: number;
	created_at: string;
	updated_at: string;
}

export interface DocPageSummary {
	id: string;
	title: string;
	slug: string;
	audience: string;
	review_status: string;
	parent_id: string | null;
	position: number;
	created_at: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`, {
		...init,
		headers: {
			'Content-Type': 'application/json',
			...(jwtToken ? { Authorization: `Bearer ${jwtToken}` } : {}),
			...init?.headers,
		},
	});

	const body = await res.json() as ApiOk<T> | ApiErr;
	if (!res.ok || 'error' in body) {
		const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
		throw new Error(`${err.code}: ${err.message}`);
	}
	return (body as ApiOk<T>).data;
}

export const api = {
	listDocPages(audience: string): Promise<DocPageSummary[]> {
		return request<DocPageSummary[]>(`/docs/${audience}`);
	},

	getDocPage(audience: string, slug: string): Promise<DocPage> {
		return request<DocPage>(`/docs/${audience}/${slug}`);
	},
};
