// SSE client for POST /api/v1/ai/chat (ADR-130 unified Japa AI) — parses
// the `data: {json}\n\n` lines the Go handler writes
// (internal/ai/unified_handlers.go HandleUnifiedChat).
//
// Same transport pattern as agentStream.ts: EventSource cannot POST with a
// CSRF header, so we use fetch + a ReadableStream reader. The unified
// endpoint emits the §103 agent event stream plus `conversation` and
// `sources` events.
//
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import type { AgentEvent } from './agentTypes';
import { AgentRequestError } from './agentStream';

/** Per-message tool override — mirrors the Go ToolSelection (internal/ai/prefs.go). */
export interface ToolSelectionOverride {
	/** false forces a pure-chat turn; true stays bounded by prefs/RBAC. */
	enabled?: boolean;
	/** Category whitelist — narrows the resolved catalog when non-empty. */
	categories?: string[];
	/** Tool whitelist — narrows the resolved catalog when non-empty. */
	tools?: string[];
}

/** Options for streamChat. */
export interface StreamChatOptions {
	message: string;
	conversationId?: string;
	/** §103 AI Projects — stamps a newly created conversation into this
	 * project. Ignored when conversationId resolves to an existing thread. */
	projectId?: string;
	model?: string;
	voiceId?: string;
	/** Client context merged into the AI context payload (allowlisted
	 * server-side — e.g. `tone` steers response style). */
	context?: Record<string, unknown>;
	humanize?: boolean;
	humanizeLevel?: string;
	tools?: ToolSelectionOverride;
	/** AbortSignal to cancel the stream (cancels the fetch + the backend loop). */
	signal?: AbortSignal;
}

/**
 * Stream unified chat events from POST /api/v1/ai/chat with
 * `Accept: text/event-stream`. Yields each parsed AgentEvent as it arrives.
 *
 * Pre-check failures (permission, rate limit, invalid model, empty message)
 * come back as a JSON error response — thrown as AgentRequestError before
 * any event is yielded.
 */
export async function* streamChat(
	opts: StreamChatOptions
): AsyncGenerator<AgentEvent, void, unknown> {
	const csrfToken = getCsrfToken();
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		accept: 'text/event-stream',
		// Identify this surface for scoped feature-flag resolution (X-JC-Surface).
		'x-jc-surface': 'main'
	};
	if (csrfToken) headers['x-csrf-token'] = csrfToken;

	const body = JSON.stringify({
		message: opts.message,
		conversation_id: opts.conversationId ?? undefined,
		project_id: opts.projectId ?? undefined,
		model: opts.model || undefined,
		voice_id: opts.voiceId || undefined,
		context: opts.context,
		humanize: opts.humanize,
		humanize_level: opts.humanizeLevel || undefined,
		tools: opts.tools
	});
	const url = buildApiUrl('/ai/chat', PUBLIC_JC_API_URL);

	let res = await fetch(url, {
		method: 'POST',
		headers,
		body,
		credentials: 'include',
		signal: opts.signal
	});

	// CSRF rejection → refresh token once and retry.
	if (res.status === 403) {
		let errBody: { error?: { code?: string } } | null = null;
		try {
			errBody = (await res.clone().json()) as { error?: { code?: string } };
		} catch {
			errBody = null;
		}
		if (errBody?.error?.code === 'csrf_token_invalid') {
			await refreshCsrfToken();
			const retryHeaders = { ...headers, 'x-csrf-token': getCsrfToken() ?? '' };
			res = await fetch(url, {
				method: 'POST',
				headers: retryHeaders,
				body,
				credentials: 'include',
				signal: opts.signal
			});
		}
	}

	// Non-OK and not an event stream → pre-check failure with a JSON error body.
	const contentType = res.headers.get('content-type') ?? '';
	if (!res.ok || !contentType.includes('text/event-stream')) {
		let errBody: { error?: { code?: string; message?: string } } | null = null;
		try {
			errBody = (await res.json()) as { error?: { code?: string; message?: string } };
		} catch {
			errBody = null;
		}
		const err = errBody?.error;
		throw new AgentRequestError(
			res.status,
			err?.code ?? 'ai_error',
			err?.message ?? res.statusText
		);
	}

	const reader = res.body?.getReader();
	if (!reader) {
		throw new AgentRequestError(0, 'no_stream', 'Response body is not readable');
	}

	const decoder = new TextDecoder();
	let buffer = '';

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });

			let delimiterIndex: number;
			while ((delimiterIndex = buffer.indexOf('\n\n')) !== -1) {
				const rawEvent = buffer.slice(0, delimiterIndex);
				buffer = buffer.slice(delimiterIndex + 2);
				const event = parseSseEvent(rawEvent);
				if (event) yield event;
			}
		}
		// Flush any trailing event (the server may close without a final delimiter).
		if (buffer.trim()) {
			const event = parseSseEvent(buffer);
			if (event) yield event;
		}
	} finally {
		reader.releaseLock();
	}
}

/** Parse a raw SSE block into an AgentEvent — same shape as the agent stream. */
function parseSseEvent(raw: string): AgentEvent | null {
	const lines = raw.split('\n');
	for (const line of lines) {
		const trimmed = line.trim();
		if (!trimmed.startsWith('data:')) continue;
		const jsonStr = trimmed.slice(5).trim();
		if (!jsonStr) continue;
		try {
			return JSON.parse(jsonStr) as AgentEvent;
		} catch {
			continue;
		}
	}
	return null;
}
