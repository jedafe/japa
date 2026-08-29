// SSE client for POST /api/v1/ai/agent — parses the `data: {json}\n\n` lines
// the Go handler writes (internal/ai/agent_handlers.go HandleAgent).
//
// EventSource cannot be used here because the agent endpoint requires a POST
// body (the prompt) and a CSRF header — EventSource only supports GET with no
// custom headers. We use fetch + a ReadableStream reader instead, which gives
// us full control over method, headers, and credentials.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (SSE Streaming)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, task 6.01)

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import type { AgentEvent } from './agentTypes';

/** Error thrown when the agent pre-check fails (non-SSE response with a JSON error body). */
export class AgentRequestError extends Error {
	constructor(
		public readonly status: number,
		public readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'AgentRequestError';
	}
}

/** Options for streamAgent. */
export interface StreamAgentOptions {
	prompt: string;
	/** Optional existing conversation id (reserved for future multi-turn support). */
	conversationId?: string;
	/** AbortSignal to cancel the stream (cancels the fetch + the backend loop). */
	signal?: AbortSignal;
}

/**
 * Stream agent events from POST /api/v1/ai/agent. Yields each parsed
 * AgentEvent as it arrives. The generator completes when the server closes
 * the stream (the agent loop closes the channel on completion/failure).
 *
 * Pre-check failures (agent disabled, no permission, rate limited, empty
 * prompt) come back as a normal JSON error response with a non-200 status —
 * these are thrown as AgentRequestError before any event is yielded.
 */
export async function* streamAgent(
	opts: StreamAgentOptions
): AsyncGenerator<AgentEvent, void, unknown> {
	const csrfToken = getCsrfToken();
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		accept: 'text/event-stream'
	};
	if (csrfToken) headers['x-csrf-token'] = csrfToken;

	const body = JSON.stringify({
		prompt: opts.prompt,
		conversation_id: opts.conversationId ?? undefined
	});
	const url = buildApiUrl('/ai/agent', PUBLIC_JC_API_URL);

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
			err?.code ?? 'agent_error',
			err?.message ?? res.statusText
		);
	}

	// Parse the SSE stream. The backend writes `data: {json}\n\n` per event.
	// We buffer incoming bytes, split on the `\n\n` delimiter, and parse each
	// `data: ` line as JSON.
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

			// Split on the SSE event delimiter. Any leftover partial chunk
			// stays in the buffer for the next read.
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

/**
 * Parse a single raw SSE event block (one or more lines) into an AgentEvent.
 * The backend only emits `data:` lines; `event:`/`id:`/`retry:` lines are
 * ignored per the SSE spec.
 */
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
			// Malformed JSON — skip rather than crash the whole stream.
			continue;
		}
	}
	return null;
}
