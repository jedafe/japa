// SSE client for POST /api/v1/ai/chat (ADR-130 unified Japa AI + §115
// panel variant) — parses the `data: {json}\n\n` lines the Go handler
// writes (internal/ai/unified_handlers.go HandleUnifiedChat). Deps are
// injected per host app via initAiPanelDeps — read lazily per request so
// the factory is safe to create before init.
//
// Ref: japaconnect-engineering/sections/115-ai-quick-panel.md

import { buildApiUrl } from '@japaconnect/api-url';
import { getAiPanelDeps } from './deps';
import { AgentRequestError, type AgentEvent, type StreamChatOptions } from './types';

/**
 * Stream unified chat events from POST /api/v1/ai/chat with
 * `Accept: text/event-stream`. Yields each parsed AgentEvent as it arrives.
 *
 * Pre-check failures (permission, rate limit, invalid model, empty message)
 * come back as a JSON error response — thrown as AgentRequestError before
 * any event is yielded.
 */
export function createStreamChat() {
	return async function* streamChat(
		opts: StreamChatOptions
	): AsyncGenerator<AgentEvent, void, unknown> {
		const deps = getAiPanelDeps();
		const csrfToken = deps.getCsrfToken();
		const headers: Record<string, string> = {
			'content-type': 'application/json',
			accept: 'text/event-stream',
			'x-jc-surface': deps.surface
		};
		if (csrfToken) headers['x-csrf-token'] = csrfToken;

		const body = JSON.stringify({
			message: opts.message,
			conversation_id: opts.conversationId ?? undefined,
			project_id: opts.projectId ?? undefined,
			anchor: opts.anchor || undefined,
			model: opts.model || undefined,
			byok_key_id: opts.byokKeyId || undefined,
			voice_id: opts.voiceId || undefined,
			context: opts.context,
			humanize: opts.humanize,
			humanize_level: opts.humanizeLevel || undefined,
			tools: opts.tools
		});
		const url = buildApiUrl('/ai/chat', deps.apiBase);

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
				await deps.refreshCsrfToken();
				const retryHeaders = { ...headers, 'x-csrf-token': deps.getCsrfToken() ?? '' };
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
			if (buffer.trim()) {
				const event = parseSseEvent(buffer);
				if (event) yield event;
			}
		} finally {
			reader.releaseLock();
		}
	};
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
