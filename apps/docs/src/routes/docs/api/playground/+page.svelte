<script lang="ts">
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import JsonEditor from '$lib/components/docs/JsonEditor.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { t } from '$lib/i18n';
	import { env } from '$env/dynamic/public';
	import { Plus, X, Send, AlertTriangle, History, KeyRound, AlignLeft } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type HttpMethod = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';

	let apiKey = $state('');
	let useMyKey = $state(false);
	let selectedKeyId = $state<string>('');
	let method = $state<HttpMethod>('GET');
	let url = $state('/api/v1/spaces');
	let body = $state('');
	const bodyPlaceholder = '{"key": "value"}';
	let headers = $state<{ key: string; value: string }[]>([
		{ key: 'Content-Type', value: 'application/json' }
	]);

	let respStatus = $state<number | null>(null);
	let respTime = $state<number | null>(null);
	let respBody = $state('');
	let sending = $state(false);
	let guardError = $state('');

	// Request history (Ref: §47 ROADMAP 04.10) — last 10 requests in
	// component state (not persisted). Click to re-populate the form.
	interface HistoryEntry {
		id: number;
		method: HttpMethod;
		url: string;
		status: number;
		time: number;
	}
	let history = $state<HistoryEntry[]>([]);
	let historyId = 0;

	// When "Use my API Key" is toggled on, clear the manual key field — the
	// authorization header will be injected from the selected key's prefix.
	// Note: the backend only returns key_prefix for existing keys, so the
	// user must copy a full key from /docs/api/keys to use it here. The
	// selector helps them pick which app's key to use.
	let activeKeys = $derived((data.apiKeys ?? []).filter((k: { status: string }) => k.status === 'active'));

	// Clear the manual key field when "Use my API Key" is toggled on.
	// bind:checked already updates useMyKey; this effect handles the
	// side-effect without double-toggling the checkbox state.
	$effect(() => {
		if (useMyKey) apiKey = '';
	});

	// Strip any /api/v1 suffix so apiBase is the origin only; the request
	// URL (from the input or OpenAPI spec paths) already includes /api/v1.
	const apiBase = (env.PUBLIC_JC_API_URL ?? '/api/v1')
		.replace(/\/api\/v1$/, '')
		.replace(/\/$/, '');

	// Security guardrail (Ref: §47 ROADMAP 04.12): the playground may only
	// send requests to the JapaConnect API origin. We derive the allowed
	// origin from PUBLIC_JC_API_URL (stripping any /api/v1 suffix) and reject
	// any absolute URL that does not match it. Relative paths are always
	// allowed because they are resolved against apiBase.
	const allowedEnvOrigin = (() => {
		const raw = (env.PUBLIC_JC_API_URL ?? '').replace(/\/$/, '');
		if (!raw) return '';
		try {
			return new URL(raw).origin;
		} catch {
			return '';
		}
	})();

	function getAllowedOrigin(): string {
		// If PUBLIC_JC_API_URL is relative, fall back to the current page origin
		// so the user can still build absolute URLs in the dev/edge environment.
		if (allowedEnvOrigin) return allowedEnvOrigin;
		if (typeof window !== 'undefined') return window.location.origin;
		return '';
	}

	/** Validate the request URL against the allowed-origin guardrail. */
	function validateTarget(targetUrl: string): string | null {
		if (!targetUrl.startsWith('http')) return null; // relative → safe
		try {
			const parsed = new URL(targetUrl);
			const allowedOrigin = getAllowedOrigin();
			if (!allowedOrigin) {
				return 'Playground requires PUBLIC_JC_API_URL to be configured. Set the env var and restart the dev server.';
			}
			if (parsed.origin !== allowedOrigin) {
				return `Blocked: playground may only send requests to ${allowedOrigin}`;
			}
			return null;
		} catch {
			return 'Blocked: invalid URL';
		}
	}

	// Quick endpoints derived from the OpenAPI spec (first 12 HTTP methods).
	// OpenAPI paths may contain non-method keys (parameters, servers, etc.)
	// which should not be rendered as quick actions.
	const HTTP_METHODS = new Set(['get', 'post', 'put', 'patch', 'delete', 'head', 'options']);
	let quickEndpoints = $derived(
		data.spec
			? Object.entries(data.spec.paths ?? {})
					.flatMap(([path, methods]) =>
						Object.entries(methods)
							.filter(([m]) => HTTP_METHODS.has(m.toLowerCase()))
							.map(([m]) => ({
								method: m.toUpperCase() as HttpMethod,
								path
							}))
					)
					.slice(0, 12)
			: []
	);

	function addHeader() {
		headers = [...headers, { key: '', value: '' }];
	}
	function removeHeader(i: number) {
		headers = headers.filter((_, idx) => idx !== i);
	}

	async function send() {
		guardError = '';
		// Security guardrail: validate the target URL before sending.
		const fullUrl = url.startsWith('http') ? url : `${apiBase}${url}`;
		const blocked = validateTarget(fullUrl);
		if (blocked) {
			guardError = blocked;
			respStatus = null;
			respBody = '';
			return;
		}
		sending = true;
		respStatus = null;
		respTime = null;
		respBody = '';
		const start = performance.now();
		try {
			const reqHeaders: Record<string, string> = {};
			for (const h of headers) {
				if (h.key.trim()) reqHeaders[h.key.trim()] = h.value;
			}
			// DOC-005: public API key auth uses X-API-Key, not Authorization.
			if (apiKey.trim()) reqHeaders['X-API-Key'] = apiKey.trim();
			const res = await fetch(fullUrl, {
				method,
				headers: reqHeaders,
				body: method === 'GET' || method === 'DELETE' ? undefined : body || undefined
			});
			respStatus = res.status;
			respTime = Math.round(performance.now() - start);
			const text = await res.text();
			try {
				respBody = JSON.stringify(JSON.parse(text), null, 2);
			} catch {
				respBody = text;
			}
		} catch (e) {
			respStatus = 0;
			respBody = `Network error: ${e instanceof Error ? e.message : String(e)}`;
		} finally {
			sending = false;
			// Record in history (Ref: §47 ROADMAP 04.10) — keep last 10.
			if (respStatus !== null) {
				historyId++;
				history = [
					{ id: historyId, method, url, status: respStatus, time: respTime ?? 0 },
					...history
				].slice(0, 10);
			}
		}
	}

	function replayHistory(h: HistoryEntry) {
		method = h.method;
		url = h.url;
	}

	function useQuick(q: { method: HttpMethod; path: string }) {
		method = q.method;
		url = q.path;
	}

	/** Pretty-print the JSON body (Ref: §47 ROADMAP 04.07). */
	function formatBody() {
		if (!body.trim()) return;
		try {
			body = JSON.stringify(JSON.parse(body), null, 2);
		} catch {
			// not valid JSON — leave as-is
		}
	}

	let statusClass = $derived(
		respStatus === null
			? 'pending'
			: respStatus >= 200 && respStatus < 300
				? 'ok'
				: 'err'
	);
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	<meta property="og:title" content={data.meta.title} />
	<meta property="og:description" content={data.meta.description} />
	<meta property="og:type" content={data.meta.ogType} />
	<meta property="og:url" content={data.meta.ogUrl} />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	activeAudience="api"
	user={data.user}
	collapsed={data.navCollapsed}
	onToggleCollapse={() => {}}
	variant="full"
>
	<div class="playground">
		<header class="pg-header">
			<div>
				<h1>{t('api.playground_title')}</h1>
				<p class="pg-subtitle">{t('api.playground_subtitle')}</p>
			</div>
			<a href="/docs/api" class="pg-back">← {t('audience.api')}</a>
		</header>

		<div class="pg-warning">
			<AlertTriangle size={16} />
			<span>{t('api.playground_warning')}</span>
		</div>

		{#if guardError}
			<div class="pg-guard-error" role="alert">
				<AlertTriangle size={16} />
				<span>{guardError}</span>
			</div>
		{/if}

		<div class="pg-grid">
			<section class="pg-section">
				<h2>{t('api.api_key')}</h2>
				<div class="key-toggle">
					<label class="toggle-label">
						<input type="checkbox" bind:checked={useMyKey} />
						<KeyRound size={14} />
						<span>{t('api.use_my_key', 'Use my API Key')}</span>
					</label>
					<a href="/docs/api/keys" class="manage-keys-link">{t('api.manage_keys', 'Manage keys')}</a>
				</div>
				{#if useMyKey}
					{#if activeKeys.length === 0}
						<p class="pg-help pg-help-warning">
							{t('api.no_keys_help', 'You have no active API keys. Create one on the API Keys page — the full key is shown only once at creation time.')}
						</p>
						<a href="/docs/api/keys" class="pg-action-btn">
							<KeyRound size={14} />
							{t('api.create_key', 'Create an API key')}
						</a>
					{:else}
						<AutocompleteSelect
							bind:value={selectedKeyId}
							ariaLabel={t('api.select_key', 'Select API key')}
							options={[
								{ value: '', label: t('api.select_key', 'Select an API key…') },
								...activeKeys.map((k) => ({ value: k.id, label: `${k.name} (${k.key_prefix}…)` }))
							]}
						/>
						{#if selectedKeyId}
							{@const selectedKey = activeKeys.find((k) => k.id === selectedKeyId)}
							<div class="pg-key-hint">
								<p class="pg-help">
									{t('api.key_selected_help', 'For security, the full key is only shown when you create it. Copy it from the API Keys page and paste below.')}
								</p>
								{#if selectedKey}
									<p class="pg-key-prefix">
										{t('api.key_prefix_label', 'Selected key prefix')}: <code>{selectedKey.key_prefix}…</code>
									</p>
								{/if}
								<a href="/docs/api/keys" target="_blank" rel="noopener" class="pg-action-btn">
									<KeyRound size={14} />
									{t('api.open_keys_page', 'Open API Keys page')}
								</a>
							</div>
							<input
								type="password"
								bind:value={apiKey}
								placeholder={t('api.api_key_placeholder')}
								autocomplete="off"
							/>
						{:else}
							<p class="pg-help">{t('api.select_key_help', 'Select a key above, then paste the full key value. The selector helps you identify which app it belongs to.')}</p>
						{/if}
					{/if}
				{:else}
					<input
						type="password"
						bind:value={apiKey}
						placeholder={t('api.api_key_placeholder')}
						autocomplete="off"
					/>
				{/if}
				<p class="pg-help">{t('api.api_key_help')}</p>
			</section>

			<section class="pg-section">
				<h2>{t('api.request')}</h2>
				<div class="method-url">
					<AutocompleteSelect
						bind:value={method}
						ariaLabel={t('api.method')}
						options={['GET', 'POST', 'PATCH', 'PUT', 'DELETE'].map((m) => ({ value: m, label: m }))}
						class="pg-method"
					/>
					<input bind:value={url} placeholder="/api/v1/…" aria-label={t('api.endpoint')} />
				</div>

				<span class="field-label">{t('api.headers')}</span>
				<div class="headers">
					{#each headers as h, i (i)}
						<div class="header-row">
							<input bind:value={h.key} placeholder="Key" class="hkey" />
							<input bind:value={h.value} placeholder="Value" class="hval" />
							<button type="button" class="remove" onclick={() => removeHeader(i)} aria-label={t('api.remove_header')}>
								<X size={14} />
							</button>
						</div>
					{/each}
					<button type="button" class="add-header" onclick={addHeader}>
						<Plus size={12} /> {t('api.add_header')}
					</button>
				</div>

				<div class="body-header">
					<label for="pg-body" class="field-label">{t('api.body')}</label>
					{#if method !== 'GET' && method !== 'DELETE'}
						<button type="button" class="format-btn" onclick={formatBody} title={t('api.format_json', 'Format JSON')}>
							<AlignLeft size={12} /> {t('api.format', 'Format')}
						</button>
					{/if}
				</div>
				{#if method !== 'GET' && method !== 'DELETE'}
					<JsonEditor
						value={body}
						placeholder={bodyPlaceholder}
						minHeight={120}
						onChange={(v) => (body = v)}
					/>
				{:else}
					<p class="body-disabled">{t('api.no_body_for_method', 'No body needed for GET/DELETE')}</p>
				{/if}

				<button type="button" class="send-btn" onclick={send} disabled={sending}>
					<Send size={14} />
					{sending ? t('api.sending') : t('api.send')}
				</button>
			</section>

			<section class="pg-section">
				<h2>{t('api.response')}</h2>
				<div class="resp-meta">
					<span class="status-{statusClass}">
						{respStatus === null
							? t('api.no_response')
							: `${t('api.status')}: ${respStatus}`}
					</span>
					{#if respTime !== null}<span class="time">{t('api.time')}: {respTime}ms</span>{/if}
				</div>
				<JsonEditor
					value={respBody || `// ${t('api.no_response')}`}
					readonly={true}
					minHeight={200}
				/>
			</section>

			{#if quickEndpoints.length > 0}
				<section class="pg-section">
					<h2>{t('api.quick_endpoints')}</h2>
					<div class="quick">
						{#each quickEndpoints as q (`${q.method}${q.path}`)}
							<button type="button" class="quick-chip" onclick={() => useQuick(q)}>
								<span class="method-badge method-{q.method}">{q.method}</span>
								<span class="quick-path">{q.path}</span>
							</button>
						{/each}
					</div>
				</section>
			{/if}

			{#if history.length > 0}
				<section class="pg-section">
					<h2><History size={14} /> {t('api.history', 'Request history')}</h2>
					<ul class="history-list">
						{#each history as h (h.id)}
							<li>
								<button type="button" class="history-item" onclick={() => replayHistory(h)}>
									<span class="method-badge method-{h.method}">{h.method}</span>
									<span class="history-url">{h.url}</span>
									<span class="status-{h.status >= 200 && h.status < 300 ? 'ok' : 'err'}">{h.status}</span>
									<span class="time">{h.time}ms</span>
								</button>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</div>
	</div>
</AppShell>

<style>
	.playground {
		max-width: 1100px;
		margin: 0 auto;
		padding: 2rem 1.5rem 4rem;
	}
	.pg-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	.pg-header h1 {
		font-family: var(--font-sans);
		font-size: 1.75rem;
		font-weight: 700;
		margin: 0;
		color: var(--color-text);
	}
	.pg-subtitle {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0.25rem 0 0;
	}
	.pg-back {
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
		white-space: nowrap;
	}
	.pg-back:hover { color: var(--color-primary); }

	.pg-warning {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--method-patch-bg);
		background: var(--method-patch-bg);
		color: var(--method-patch);
		border-radius: var(--radius);
		font-size: 0.8125rem;
		margin-bottom: 1.5rem;
	}
	.pg-guard-error {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--method-delete);
		background: var(--method-delete-bg, rgba(220, 38, 38, 0.1));
		color: var(--method-delete);
		border-radius: var(--radius);
		font-size: 0.8125rem;
		margin-bottom: 1.5rem;
		font-weight: 600;
	}

	.pg-grid {
		display: grid;
		gap: 1.25rem;
	}
	.pg-section {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1.25rem;
	}
	.pg-section h2 {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 0.75rem;
	}
	input {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid var(--color-border);
		color: var(--color-text);
		border-radius: var(--radius);
		padding: 0.5rem 0.625rem;
		font-family: inherit;
		font-size: 0.875rem;
		outline: none;
	}
	input:focus { border-color: var(--color-primary); }
	.field-label { display: block; font-size: 0.75rem; color: var(--color-muted); margin-bottom: 0.25rem; }
	.body-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		margin-top: 0.75rem;
		margin-bottom: 0.25rem;
	}
	.format-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-muted);
		font-size: 0.75rem;
		cursor: pointer;
	}
	.format-btn:hover { color: var(--color-primary); border-color: var(--color-primary); }
	.body-disabled {
		font-size: 0.8125rem;
		color: var(--color-muted);
		padding: 0.75rem;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius);
		margin: 0.25rem 0 0;
	}
	.pg-help {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin: 0.5rem 0 0;
	}
	.key-toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}
	.toggle-label {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.toggle-label input { width: auto; }
	.manage-keys-link {
		font-size: 0.75rem;
		color: var(--color-primary);
		text-decoration: none;
		white-space: nowrap;
	}
	.manage-keys-link:hover { text-decoration: underline; }
	.pg-help-warning {
		color: var(--color-text-subtle);
		background: var(--color-surface-subtle);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
	}
	.pg-key-hint {
		margin: 0.5rem 0;
		padding: 0.625rem 0.75rem;
		background: var(--color-surface-subtle);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
	}
	.pg-key-prefix {
		font-size: 0.75rem;
		color: var(--color-text-subtle);
		margin: 0.375rem 0 0.5rem;
	}
	.pg-key-prefix code {
		font-family: var(--font-mono, monospace);
		font-size: 0.8125rem;
		color: var(--color-text);
		background: var(--color-surface);
		padding: 0.125rem 0.375rem;
		border-radius: 3px;
	}
	.pg-action-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-primary);
		text-decoration: none;
		padding: 0.375rem 0.625rem;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius);
		margin-top: 0.25rem;
		transition: background 0.15s;
	}
	.pg-action-btn:hover {
		background: var(--color-primary);
		color: var(--color-primary-contrast);
	}
	.history-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.history-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.375rem 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		cursor: pointer;
		font-size: 0.75rem;
		text-align: left;
	}
	.history-item:hover { border-color: var(--color-primary); }
	.history-url {
		font-family: var(--font-mono);
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text);
	}
	.method-url {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}
	.method-url :global(.pg-method) { width: 120px; flex-shrink: 0; }
	.headers { display: flex; flex-direction: column; gap: 0.375rem; margin-bottom: 0.75rem; }
	.header-row { display: flex; gap: 0.375rem; }
	.header-row .hkey { flex: 1; }
	.header-row .hval { flex: 2; }
	.remove {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		color: var(--color-muted);
		cursor: pointer;
		flex-shrink: 0;
	}
	.remove:hover { color: var(--method-delete); border-color: var(--method-delete); }
	.add-header {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		background: none;
		border: none;
		color: var(--color-primary);
		font-size: 0.8125rem;
		cursor: pointer;
		padding: 0;
	}
	.add-header:hover { text-decoration: underline; }
	.send-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.75rem;
		padding: 0.5rem 1.25rem;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		border: none;
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.send-btn:hover { background: var(--color-primary-hover); }
	.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	.resp-meta {
		display: flex;
		gap: 1rem;
		font-size: 0.8125rem;
		margin-bottom: 0.5rem;
	}
	.status-ok { color: var(--method-get); font-weight: 600; }
	.status-err { color: var(--method-delete); font-weight: 600; }
	.status-pending { color: var(--color-muted); }
	.time { color: var(--color-muted); }
	.resp-body {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		background: var(--code-bg);
		color: var(--code-text);
		padding: 1rem;
		border-radius: var(--radius);
		overflow-x: auto;
		white-space: pre-wrap;
		word-break: break-all;
		max-height: 420px;
		overflow-y: auto;
		margin: 0;
	}

	.quick { display: flex; flex-wrap: wrap; gap: 0.375rem; }
	.quick-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.25rem 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-bg);
		cursor: pointer;
	}
	.quick-chip:hover { border-color: var(--color-primary); }
	.quick-path {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text);
	}
</style>
