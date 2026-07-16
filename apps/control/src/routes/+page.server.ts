import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

type ServiceStatus = {
	name: string;
	status: 'healthy' | 'degraded' | 'down' | 'unknown';
	latency_ms: number;
	message?: string;
};

type HealthResponse = {
	infrastructure: ServiceStatus[];
	application: ServiceStatus[];
	background: ServiceStatus[];
	realtime: ServiceStatus[];
};

const UNKNOWN_SERVICES: { group: keyof HealthResponse; name: string }[] = [
	{ group: 'infrastructure', name: 'PostgreSQL' },
	{ group: 'infrastructure', name: 'Valkey' },
	{ group: 'infrastructure', name: 'Meilisearch' },
	{ group: 'infrastructure', name: 'SeaweedFS' },
	{ group: 'application', name: 'API Server' },
	{ group: 'application', name: 'imgproxy' },
	{ group: 'application', name: 'Nginx' },
	{ group: 'background', name: 'Asynq Workers' },
	{ group: 'background', name: 'Queue Depth' },
	{ group: 'realtime', name: 'WebSocket Hub' },
	{ group: 'realtime', name: 'SSE Connections' }
];

function buildDefaultHealthResponse(): HealthResponse {
	const groups: Record<string, ServiceStatus[]> = {
		infrastructure: [],
		application: [],
		background: [],
		realtime: []
	};
	for (const svc of UNKNOWN_SERVICES) {
		groups[svc.group].push({ name: svc.name, status: 'unknown', latency_ms: 0 });
	}
	return groups as unknown as HealthResponse;
}

function transformHealthPayload(raw: Record<string, unknown>): HealthResponse {
	const response = buildDefaultHealthResponse();

	const apiServer = response.application.find((s) => s.name === 'API Server');
	if (apiServer) {
		apiServer.status = 'healthy';
		const uptimeSec = typeof raw.uptime_sec === 'number' ? raw.uptime_sec : 0;
		apiServer.message = `Uptime: ${uptimeSec}s`;
	}

	function extractStatuses(group: Record<string, unknown> | undefined, services: ServiceStatus[], skipApiServer = false) {
		if (!group || typeof group !== 'object') return;
		for (const svc of services) {
			if (skipApiServer && svc.name === 'API Server') continue;
			const key = svc.name.toLowerCase().replace(/[^a-z]/g, '');
			const rawVal = (group as Record<string, Record<string, unknown>>)[key] ?? (group as Record<string, Record<string, unknown>>)[svc.name];
			const status = typeof rawVal === 'object' && rawVal !== null ? rawVal.status : undefined;
			if (typeof status === 'string') {
				svc.status = status === 'ok' || status === 'healthy' ? 'healthy' : status === 'degraded' ? 'degraded' : status === 'down' ? 'down' : 'unknown';
			}
		}
	}

	extractStatuses(raw.infrastructure as Record<string, unknown> | undefined, response.infrastructure);
	extractStatuses(raw.application as Record<string, unknown> | undefined, response.application, true);
	extractStatuses(raw.background as Record<string, unknown> | undefined, response.background);
	extractStatuses(raw.realtime as Record<string, unknown> | undefined, response.realtime);

	return response;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = {};
	if (token) headers.Authorization = `Bearer ${token}`;

	try {
		const res = await fetch(`${API_BASE}/control/health`, { headers });
		if (!res.ok) return { initial: buildDefaultHealthResponse() };
		const body = await res.json();
		const payload = (body.data ?? body) as Record<string, unknown>;

		// If the response already has the full format, pass it through
		if ('infrastructure' in payload && 'application' in payload && 'background' in payload && 'realtime' in payload) {
			return { initial: payload as unknown as HealthResponse };
		}

		// Otherwise transform the simple health check response
		return { initial: transformHealthPayload(payload) };
	} catch {
		return { initial: buildDefaultHealthResponse() };
	}
};
