import { env } from '$env/dynamic/private';

export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080/api/v1';

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

export async function listDocPages(audience: string): Promise<DocPageSummary[]> {
    const res = await fetch(`${API_BASE}/docs/${audience}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const body = await res.json();
    return body.data ?? body ?? [];
}

export async function getDocPage(audience: string, slug: string): Promise<DocPage> {
    const res = await fetch(`${API_BASE}/docs/${audience}/${slug}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const body = await res.json();
    return body.data ?? body;
}
