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
		body?: string;
		fetched_at?: string;
		status_code?: number;
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

	const body = $derived<string>(typeof doc?.body === 'string' ? doc.body : '');
	const contentType = $derived<string>(doc?.content_type ?? '');

	const isJson = $derived(
		contentType.includes('json') ||
			(body.trim().startsWith('{') || body.trim().startsWith('['))
	);
	const isHtml = $derived(contentType.includes('html') || body.trim().startsWith('<'));

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
					<dd class="text-foreground truncate font-mono">{doc.url ?? '—'}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.content_type', 'Content-Type')}</dt>
					<dd class="text-foreground font-mono">{contentType || '—'}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.fetched_at', 'Fetched At')}</dt>
					<dd class="text-foreground font-mono">{formatTime(doc.fetched_at)}</dd>
				</div>
				<div>
					<dt class="text-muted-foreground font-medium uppercase">{t('intel.status_code', 'Status')}</dt>
					<dd class="text-foreground font-mono">{doc.status_code ?? '—'}</dd>
				</div>
			</dl>

			{#if isHtml}
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
