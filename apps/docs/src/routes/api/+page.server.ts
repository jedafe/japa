import { PUBLIC_JC_API_URL } from '$env/static/public';
import type { PageServerLoad } from './$types';

interface OpenAPISchema {
	openapi?: string;
	info?: { title?: string; version?: string; description?: string };
	paths?: Record<string, Record<string, {
		summary?: string;
		description?: string;
		tags?: string[];
		operationId?: string;
		parameters?: Array<{ name: string; in: string; required?: boolean; description?: string; schema?: { type?: string } }>;
		requestBody?: { content?: Record<string, { schema?: Record<string, unknown> }> };
		responses?: Record<string, { description?: string }>;
	}>>;
	tags?: Array<{ name: string; description?: string }>;
}

export const load: PageServerLoad = async ({ fetch }) => {
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/docs/openapi.json`);
		if (!res.ok) {
			return { spec: null, error: 'Failed to load API specification' };
		}
		const spec: OpenAPISchema = await res.json();
		return { spec, error: null };
	} catch {
		return { spec: null, error: 'Could not connect to API server' };
	}
};
