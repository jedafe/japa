<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { FileText, Copy, Check } from 'lucide-svelte';

	// RawContentPane renders the raw fetched document for an intel record.
	// The raw document is fetched by the parent and passed in as `document`.
	// It may be HTML, plain text, or JSON; the pane auto-detects and renders
	// accordingly with a copy-to-clipboard affordance.

	interface RawDocument {
		id?: string;
		url?: string;
		content_type?: string;
		// Backend returns body as base64 (body_b64), http_status, and
		// retrieved_at per route catalog §2229. The prior interface used
		// body/status_code/fetched_at which the backend never sent.
		body_b64?: string;
		http_status?: number;
		retrieved_at?: string;
		[hash: string]: unknown;
	}

	let {
		document: doc = null as RawDocument | null,
		loading = false
	}: {
		document?: RawDocument | null;
		loading?: boolean;
	} = $props();

	let copied = $state(false);

	// Decode the base64-encoded body as UTF-8. The backend sends body_b64 to
	// preserve binary content (PDF bytes, etc.). atob() alone returns a
	// Latin-1 string that mangles multi-byte UTF-8 — decode via TextDecoder
	// instead. Binary bodies stay non-empty but are flagged via isBinary.
	const body = $derived.by<string>(() => {
		const b64 = doc?.body_b64;
		if (!b64 || typeof b64 !== 'string') return '';
		try {
			const bin = atob(b64);
			const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
			return new TextDecoder('utf-8', { fatal: false }).decode(bytes);
		} catch {
			return '';
		}
	});
	const contentType = $derived<string>(doc?.content_type ?? '');

	const isBinary = $derived(
		/^(application\/(pdf|octet-stream|zip|msword|vnd\.))|image\/|audio\/|video\//i.test(contentType)
	);

	const isJson = $derived(
		contentType.includes('json') ||
			(body.trim().startsWith('{') || body.trim().startsWith('['))
	);
	const isHtml = $derived(
		contentType.includes('html') || /^\s*<(html|!doctype)\b/i.test(body)
	);

	// Pretty-print JSON when the body parses; otherwise fall back to raw.
	const rendered = $derived.by(() => {
		if (!body) return '';
		if (isJson) {
			try {
				return JSON.stringify(JSON.parse(body), null, 2);
			} catch {
				return body;
			}
		}
		return body;
	});

	async function copyBody() {
		if (!body) return;
		try {
			await navigator.clipboard.writeText(body);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// clipboard unavailable
		}
	}

	function formatTime(ts?: string): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}
</script>

<div class="border-border-card bg-background-alt shadow-card flex h-full flex-col rounded-xl">
	<div class="border-border-card flex items-center justify-between border-b px-4 py-3">
		<div class="flex items-center gap-2">
			<FileText class="text-muted-foreground size-4" />
			<h3 class="text-foreground text-sm font-semibold">
				{t('intel.raw_content', 'Raw Content')}
			</h3>
		</div>
		{#if body}
			<button
				type="button"
				class="text-muted-foreground hover:bg-muted hover:text-foreground flex items-center gap-1 rounded-md px-2 py-1 text-xs transition-colors"
				onclick={copyBody}
			>
				{#if copied}
					<Check class="size-3.5 text-emerald-500" />
					{t('common.copied', 'Copied')}
				{:else}
					<Copy class="size-3.5" />
					{t('common.copy', 'Copy')}
				{/if}
			</button>
		{/if}
	</div>

	<div class="flex-1 overflow-y-auto p-4">
		{#if loading}
			<div class="space-y-2">
				{#each Array(6) as _}
					<div class="bg-muted h-4 w-3/4 animate-pulse rounded"></div>
				{/each}
			</div>
		{:else if !doc}
			<div class="text-muted-foreground py-8 text-center text-sm">
				<FileText class="mx-auto size-8 opacity-30" />
				<p class="mt-2">{t('intel.no_raw_content', 'No raw document available')}</p>
			</div>
		{:else}
			<dl class="mb-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.field_url', 'URL')}</dt>
					<dd class="text-foreground truncate font-mono" title={doc.url ?? undefined}>{doc.url ?? '—'}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.content_type', 'Content-Type')}</dt>
					<dd class="text-foreground font-mono">{contentType || '—'}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.fetched_at', 'Fetched At')}</dt>
					<dd class="text-foreground font-mono">{formatTime(doc.retrieved_at)}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.status_code', 'Status')}</dt>
					<dd class="text-foreground font-mono">{doc.http_status ?? '—'}</dd>
				</div>
			</dl>

			{#if isBinary}
				<div class="border-border-card bg-background rounded-lg border p-3">
					<p class="text-muted-foreground text-xs">
						{t('intel.binary_content', 'Binary content ({type}) — {size} characters decoded. Open the source URL to view the original file.').replace('{type}', contentType || 'unknown').replace('{size}', String(body.length))}
					</p>
				</div>
			{:else if isHtml}
				<div class="border-border-card bg-background max-h-96 overflow-auto rounded-lg border p-3">
					<pre class="text-foreground whitespace-pre-wrap break-words text-xs leading-relaxed">{rendered}</pre>
				</div>
			{:else if isJson}
				<div class="border-border-card bg-background max-h-96 overflow-auto rounded-lg border p-3">
					<pre class="text-foreground whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{rendered}</pre>
				</div>
			{:else}
				<div class="border-border-card bg-background max-h-96 overflow-auto rounded-lg border p-3">
					<pre class="text-foreground whitespace-pre-wrap break-words text-xs leading-relaxed">{body}</pre>
				</div>
			{/if}
		{/if}
	</div>
</div>
