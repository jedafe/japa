<script lang="ts">
	// PageSpeed dashboard — backed by GET/POST /admin/pagespeed (PSI proxy;
	// the Google API key stays server-side and reports cache for 30m).
	import { api } from '$lib/api';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { onDestroy } from 'svelte';

	let alive = true;
	onDestroy(() => { alive = false; });

	interface Result {
		target: string;
		url: string;
		strategy: string;
		error?: string;
		scores?: Record<string, number | string>;
	}
	let report = $state<{ generated_at?: string; results?: Result[]; status?: string } | null>(null);
	let error = $state('');
	let loading = $state(true);
	let pending = $state(false);
	let running = $state(false);
	let scanUrl = $state('');
	let scanStrategy = $state('mobile');
	let scanning = $state(false);
	let scanError = $state('');
	interface ScanResult {
		url: string;
		results: Record<string, Record<string, number | string> | { error: string }>;
		status?: string;
	}
	let scanOut = $state<ScanResult | null>(null);

	const CATS = ['performance', 'accessibility', 'best-practices', 'seo'];

	async function load() {
		loading = true;
		error = '';
		const res = await api<typeof report>('/admin/pagespeed');
		if (res.ok && res.data) {
			// 200 {status:"running"} while a run is in flight,
			// {status:"idle"} when nothing has run yet, else the report.
			pending = res.data.status === 'running';
			if (res.data.results) report = res.data;
		} else {
			error = !res.ok ? (res.error ?? 'Failed to load report') : 'Empty report';
		}
		loading = false;
	}

	async function run() {
		running = true;
		pending = true;
		await api('/admin/pagespeed/run', { method: 'POST' });
		// Poll while the backend reports the audit running (~12 min ceiling —
		// JC_PAGESPEED_MAX_TARGETS can push an audit well past a few minutes).
		const started = report?.generated_at;
		for (let i = 0; i < 144; i++) {
			if (!alive) return;
			await new Promise((r) => setTimeout(r, 5000));
			await load();
			if (!pending && report?.generated_at && report.generated_at !== started) break;
		}
		running = false;
		pending = false;
	}

	// Auto-poll when an audit is already running (e.g. opened mid-run).
	let polled = false;
	$effect(() => {
		void load().then(() => {
			if (pending && !polled) {
				polled = true;
				void run();
			}
		});
	});

	async function scan() {
		if (!scanUrl.trim() || scanning) return;
		scanning = true;
		scanError = '';
		scanOut = null;
		const url = /^https?:\/\//i.test(scanUrl.trim()) ? scanUrl.trim() : `https://${scanUrl.trim()}`;
		// Scan is async server-side (202 {status}) — re-POST to poll until the
		// cached result lands (same endpoint, idempotent).
		for (let i = 0; i < 72; i++) {
			if (!alive) { scanning = false; return; }
			const res = await api<ScanResult>('/admin/pagespeed/scan', {
				method: 'POST',
				body: { url, strategy: scanStrategy }
			});
			if (!res.ok) { scanError = res.error ?? 'Scan failed'; break; }
			if (res.data?.results) { scanOut = res.data; break; }
			await new Promise((r) => setTimeout(r, 5000));
		}
		if (!scanOut && !scanError) scanError = 'Scan timed out — try again';
		scanning = false;
	}

	function tone(score: number | string | undefined): string {
		const n = Number(score);
		if (Number.isNaN(n)) return '';
		if (n >= 90) return 'good';
		if (n >= 50) return 'warn';
		return 'bad';
	}
</script>

<svelte:head><title>PageSpeed — Admin</title></svelte:head>

<div class="page">
	<header class="page-head">
		<div>
			<h1>PageSpeed</h1>
			<p class="sub">Google PageSpeed Insights for the public frontends — cached 30 min.</p>
		</div>
		<button class="btn" onclick={run} disabled={running || loading}>
			{running ? 'Audit running…' : 'Run new audit'}
		</button>
	</header>

	{#if loading && !report}
		<p class="muted">Loading report…</p>
	{:else if error}
		<div class="card err">
			<p>{error}</p>
			<p class="muted">Set <code>JC_PAGESPEED_API_KEY</code> on the API deployment (Google Cloud Console → PageSpeed Insights API), then restart.</p>
		</div>
	{:else}
		{#if report}
		<p class="muted">
			Generated {report.generated_at ? new Date(report.generated_at).toLocaleString() : '—'}
			{#if running || pending} — refreshing…{/if}
		</p>
		{/if}
		<div class="card scan-card">
			<h2 class="scan-title">Scan a page</h2>
			<div class="scan-row">
				<input class="input" type="url" placeholder="https://japa.to/…" bind:value={scanUrl}
					onkeydown={(e) => e.key === 'Enter' && scan()} />
				<AutocompleteSelect
					bind:value={scanStrategy}
					class="w-40 shrink-0"
					options={[
						{ value: 'mobile', label: 'mobile' },
						{ value: 'desktop', label: 'desktop' }
					]}
				/>
				<button class="btn" onclick={scan} disabled={scanning || !scanUrl.trim()}>
					{scanning ? 'Scanning…' : 'Scan'}
				</button>
			</div>
			{#if scanError}<p class="err-text">{scanError}</p>{/if}
			{#if scanOut}
				<div class="overflow-x-auto">
				<table>
					<thead><tr><th>Strategy</th>{#each CATS as c}<th>{c}</th>{/each}<th>LCP</th><th>TBT</th><th>CLS</th></tr></thead>
					<tbody>
						{#each Object.entries(scanOut.results) as [strategy, sc]}
							<tr>
								<td>{strategy}</td>
								{#if 'error' in sc}<td colspan="7" class="err-text">{sc.error}</td>
								{:else}
									{#each CATS as c}<td class={tone(sc[c])}>{sc[c] ?? '—'}</td>{/each}
									<td>{sc['largest-contentful-paint'] ?? '—'}</td>
									<td>{sc['total-blocking-time'] ?? '—'}</td>
									<td>{sc['cumulative-layout-shift'] ?? '—'}</td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
				</div>
			{/if}
		</div>

		{#if report}
		<div class="card">
			<div class="overflow-x-auto">
			<table>
				<thead>
					<tr><th>Target</th><th>Strategy</th>{#each CATS as c}<th>{c}</th>{/each}<th>LCP</th><th>TBT</th><th>CLS</th></tr>
				</thead>
				<tbody>
					{#each report.results ?? [] as r}
						<tr>
							<td>{r.target}<div class="muted u">{r.url}</div></td>
							<td>{r.strategy}</td>
							{#if r.error}
								<td colspan="7" class="err-text">{r.error}</td>
							{:else}
								{#each CATS as c}<td class={tone(r.scores?.[c])}>{r.scores?.[c] ?? '—'}</td>{/each}
								<td>{r.scores?.['largest-contentful-paint'] ?? '—'}</td>
								<td>{r.scores?.['total-blocking-time'] ?? '—'}</td>
								<td>{r.scores?.['cumulative-layout-shift'] ?? '—'}</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
			</div>
		</div>
		{:else if pending}
		<p class="muted">Audit is running — results will appear here shortly…</p>
		{:else}
		<p class="muted">No report yet — click "Run new audit" to generate one.</p>
		{/if}
	{/if}
</div>

<style>
	.page { padding: 1.5rem; max-width: 64rem; }
	.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: 1rem; }
	h1 { font-size: 1.5rem; font-weight: 700; margin: 0; }
	.sub { color: var(--color-muted, #71717a); font-size: 0.875rem; margin: 0.25rem 0 0; }
	.muted { color: var(--color-muted, #71717a); font-size: 0.8125rem; }
	.btn { padding: 0.5rem 1rem; border-radius: var(--radius, 0.5rem); background: var(--color-primary, #15803d); color: #fff; font-weight: 600; border: none; cursor: pointer; }
	.btn:disabled { opacity: 0.6; cursor: not-allowed; }
	.card { border: 1px solid var(--color-border, #e4e4e7); border-radius: var(--radius-card, 0.75rem); padding: 1rem; }
	.card.err { border-color: #fca5a5; }
	.err-text { color: #dc2626; font-size: 0.8125rem; }
	table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
	th { text-align: start; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--color-muted, #71717a); padding: 0.375rem 0.5rem; border-bottom: 1px solid var(--color-border, #e4e4e7); }
	td { padding: 0.5rem; border-bottom: 1px solid var(--color-border, #e4e4e7); }
	td.good { color: #15803d; font-weight: 700; }
	td.warn { color: #b45309; font-weight: 700; }
	td.bad { color: #dc2626; font-weight: 700; }
	code { font-family: var(--font-mono, monospace); font-size: 0.8125rem; }
	.scan-card { margin-bottom: 1rem; }
	.scan-title { font-size: 1rem; font-weight: 700; margin: 0 0 0.75rem; }
	.scan-row { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; flex-wrap: wrap; }
	.input { flex: 1; min-width: 16rem; padding: 0.5rem 0.75rem; border: 1px solid var(--color-border, #e4e4e7); border-radius: var(--radius, 0.5rem); font-size: 0.875rem; background: var(--color-bg, #fff); color: inherit; }
	.sel { flex: 0 0 auto; min-width: 7rem; }
	.u { font-size: 0.75rem; word-break: break-all; }
</style>
