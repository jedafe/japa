<script lang="ts">
	/**
	 * EndpointCard — renders an API endpoint card from a TipTap custom node
	 * (type `api_endpoint`) or from an OpenAPI operation. Shows method badge,
	 * path with colored path params, auth requirement, and collapsible
	 * Request Parameters / Response Schema / Example sections.
	 *
	 * Ref: §44, §47 ROADMAP 03.03.
	 */
	import MethodBadge from './MethodBadge.svelte';
	import { t } from '$lib/i18n';
	import { env } from '$env/dynamic/public';
	import { Copy, ChevronDown } from 'lucide-svelte';

	interface EndpointParam {
		name: string;
		in: 'query' | 'header' | 'path' | 'cookie';
		required?: boolean;
		description?: string;
		type?: string;
	}

	interface Props {
		method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
		path: string;
		description?: string;
		authRequired?: boolean;
		parameters?: EndpointParam[];
		responseSchema?: string;
		example?: string;
	}

	let {
		method,
		path,
		description = '',
		authRequired = false,
		parameters = [],
		responseSchema = '',
		example = ''
	}: Props = $props();

	let showParams = $state(false);
	let showResponse = $state(false);
	let showExample = $state(false);
	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined = undefined;

	$effect(() => {
		return () => clearTimeout(copyTimer);
	});

	// Split path into segments, highlighting {path_param} portions.
	let pathSegments = $derived(
		path.split(/(\{[^}]+\})/g).filter((s) => s.length > 0)
	);

	// API base URL from PUBLIC_JC_API_URL env var (Rule 3 — no hardcoded URLs).
	// In dev this is a relative path (/api/v1); in production it's the full
	// API gateway URL. For curl examples, ensure an absolute URL by joining it
	// with PUBLIC_MAIN_ORIGIN when the API base is relative.
	const apiBaseUrl = env.PUBLIC_JC_API_URL ?? '/api/v1';
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? '').replace(/\/$/, '');
	const curlBaseUrl = apiBaseUrl.startsWith('http')
		? apiBaseUrl
		: mainOrigin
			? `${mainOrigin}${apiBaseUrl}`
			: '';

	let curlExample = $derived(
		example ||
			`curl -X ${method} "${curlBaseUrl}${path}" \\
  -H "Authorization: Bearer $JC_API_KEY"${method !== 'GET' && method !== 'DELETE' ? ' \\\n  -H "Content-Type: application/json"' : ''}`
	);

	async function copyCurl() {
		try {
			await navigator.clipboard.writeText(curlExample);
			copied = true;
			copyTimer = setTimeout(() => (copied = false), 2000);
		} catch {
			// clipboard not available
		}
	}
</script>

<div class="endpoint-card">
	<div class="endpoint-head">
		<MethodBadge {method} />
		<code class="endpoint-path">
			{#each pathSegments as seg}
				{#if /^\{.+\}$/.test(seg)}
					<span class="path-param">{seg}</span>
				{:else}
					{seg}
				{/if}
			{/each}
		</code>
		<span class="auth-badge" class:required={authRequired}>
			{authRequired ? t('api.auth_required', 'API key required') : t('api.public', '[public]')}
		</span>
	</div>

	{#if description}
		<p class="endpoint-desc">{description}</p>
	{/if}

	{#if parameters.length > 0}
		<button type="button" class="collapse-toggle" onclick={() => (showParams = !showParams)}>
			<ChevronDown size={14} class={showParams ? 'rotated' : ''} />
			{t('api.request_params', 'Request Parameters')}
			<span class="count">({parameters.length})</span>
		</button>
		{#if showParams}
			<table class="params-table">
				<thead>
					<tr>
						<th>{t('api.param_name', 'Name')}</th>
						<th>{t('api.param_in', 'In')}</th>
						<th>{t('api.param_type', 'Type')}</th>
						<th>{t('api.param_required', 'Required')}</th>
						<th>{t('api.param_desc', 'Description')}</th>
					</tr>
				</thead>
				<tbody>
					{#each parameters as p}
						<tr>
							<td><code>{p.name}</code></td>
							<td>{p.in}</td>
							<td>{p.type ?? 'string'}</td>
							<td>{p.required ? t('common.yes', 'yes') : t('common.no', 'no')}</td>
							<td>{p.description ?? ''}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	{/if}

	{#if responseSchema}
		<button type="button" class="collapse-toggle" onclick={() => (showResponse = !showResponse)}>
			<ChevronDown size={14} class={showResponse ? 'rotated' : ''} />
			{t('api.response_schema', 'Response Schema')}
		</button>
		{#if showResponse}
			<pre class="schema-block"><code>{responseSchema}</code></pre>
		{/if}
	{/if}

	<div class="example-section">
		<div class="example-head">
			<button type="button" class="collapse-toggle" onclick={() => (showExample = !showExample)}>
				<ChevronDown size={14} class={showExample ? 'rotated' : ''} />
				{t('api.example', 'Example')}
			</button>
			<button type="button" class="copy-curl-btn" onclick={copyCurl} aria-label={t('a11y.copy_code')}>
				{#if copied}<span>{t('common.copied', 'Copied')}</span>{:else}<Copy size={12} /> {t('common.copy_curl', 'Copy cURL')}{/if}
			</button>
		</div>
		{#if showExample}
			<pre class="example-block"><code>{curlExample}</code></pre>
		{/if}
	</div>
</div>

<style>
	.endpoint-card {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		padding: 1.25rem;
		margin: 1.5rem 0;
	}
	.endpoint-head {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}
	.endpoint-path {
		font-family: var(--font-mono);
		font-size: 0.875rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
		flex: 1;
	}
	.path-param {
		color: var(--color-primary);
		font-weight: 600;
	}
	.auth-badge {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
		color: var(--method-get);
		background: var(--method-get-bg);
		white-space: nowrap;
	}
	.auth-badge.required {
		color: var(--method-patch);
		background: var(--method-patch-bg);
	}
	.endpoint-desc {
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		margin: 0.75rem 0 0;
		line-height: 1.5;
	}
	.collapse-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
		padding: 0.75rem 0 0.25rem;
	}
	.collapse-toggle :global(svg) {
		transition: transform 0.15s;
	}
	.collapse-toggle :global(.rotated) {
		transform: rotate(90deg);
	}
	.count {
		color: var(--color-muted);
		font-weight: 400;
	}
	.params-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.8125rem;
		margin-top: 0.5rem;
	}
	.params-table th {
		text-align: left;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		padding: 0.375rem 0.5rem;
		border-bottom: 1px solid var(--color-border);
	}
	.params-table td {
		padding: 0.375rem 0.5rem;
		border-bottom: 1px solid var(--color-border);
		color: var(--color-text);
	}
	.params-table code {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-primary);
	}
	.schema-block, .example-block {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		background: var(--code-bg);
		color: var(--code-text);
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		overflow-x: auto;
		margin: 0.5rem 0 0;
	}
	.example-section { margin-top: 0.25rem; }
	.example-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.copy-curl-btn {
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
	.copy-curl-btn:hover { color: var(--color-primary); border-color: var(--color-primary); }
</style>
