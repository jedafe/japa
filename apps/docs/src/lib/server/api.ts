import { env } from '$env/dynamic/private';

export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

/** Base URL with /api/v1 prefix for API routes. */
export const API_V1_BASE = `${API_BASE}/api/v1`;

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

export interface DocNavNode {
    id: string;
    title: string;
    slug: string;
    audience: string;
    children?: DocNavNode[];
}

export interface DocSearchResult {
    id: string;
    title: string;
    slug: string;
    audience: string;
    excerpt: string;
}

export async function listDocPages(audience: string, token?: string): Promise<DocPageSummary[]> {
    const res = await fetch(`${API_V1_BASE}/docs/${audience}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {}
    });
    if (!res.ok) {
        const err = new Error(`HTTP ${res.status}`) as Error & { status: number };
        err.status = res.status;
        throw err;
    }
    const body = await res.json();
    return body.data ?? body ?? [];
}

export async function getDocPage(audience: string, slug: string, token?: string): Promise<DocPage> {
    const res = await fetch(`${API_V1_BASE}/docs/${audience}/${slug}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {}
    });
    if (!res.ok) {
        const err = new Error(`HTTP ${res.status}`) as Error & { status: number };
        err.status = res.status;
        throw err;
    }
    const body = await res.json();
    return body.data ?? body;
}

export async function getDocNav(audience: string, token?: string): Promise<DocNavNode[]> {
    const res = await fetch(`${API_V1_BASE}/docs/nav?audience=${audience}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {}
    });
    if (!res.ok) {
        const err = new Error(`HTTP ${res.status}`) as Error & { status: number };
        err.status = res.status;
        throw err;
    }
    const body = await res.json();
    return body.data ?? body ?? [];
}

export async function searchDocs(query: string, token?: string): Promise<DocSearchResult[]> {
    const res = await fetch(`${API_V1_BASE}/docs/search?q=${encodeURIComponent(query)}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {}
    });
    if (!res.ok) {
        const err = new Error(`HTTP ${res.status}`) as Error & { status: number };
        err.status = res.status;
        throw err;
    }
    const body = await res.json();
    return body.data ?? body ?? [];
}
