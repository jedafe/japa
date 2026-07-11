<script lang="ts">
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Play, Loader2, Copy, Check } from 'lucide-svelte';
	import { page } from '$app/state';

	let { data } = $props();

	let method = $state('GET');
	let path = $state('/api/v1/me');
	let requestBody = $state('');
	let response = $state('');
	let loading = $state(false);
	let responseStatus = $state<number | null>(null);
	let copied = $state(false);

	const audience = $derived(page.url.pathname.split('/').filter(Boolean)[0] ?? 'users');

	const sampleEndpoints = [
		{ method: 'GET', path: '/api/v1/me', description: 'Get current user profile' },
		{ method: 'GET', path: '/api/v1/posts', description: 'List recent posts' },
		{ method: 'GET', path: '/api/v1/tags', description: 'List trending tags' },
		{ method: 'GET', path: '/api/v1/docs/users', description: 'List user docs' },
		{ method: 'GET', path: '/api/v1/feature-flags', description: 'Get feature flags' },
	];

	let useAuth = $state(true);
	let customHeaders = $state('');

	async function sendRequest() {
		loading = true;
		response = '';
		responseStatus = null;

		const headers: Record<string, string> = {
			'Content-Type': 'application/json',
		};

		if (useAuth && data.token) {
			headers['Authorization'] = `Bearer ${data.token}`;
		}

		if (customHeaders.trim()) {
			try {
				const parsed = JSON.parse(customHeaders);
				Object.assign(headers, parsed);
			} catch {
				// Ignore invalid headers
			}
		}

		try {
			const opts: RequestInit = { method, headers };
			if (method !== 'GET' && requestBody.trim()) {
				opts.body = requestBody;
			}

			const res = await fetch(`${PUBLIC_JC_API_URL}${path}`, opts);
			responseStatus = res.status;
			const body = await res.json();
			response = JSON.stringify(body, null, 2);
		} catch (err) {
			response = JSON.stringify({ error: err instanceof Error ? err.message : 'Request failed' }, null, 2);
			responseStatus = 0;
		} finally {
			loading = false;
		}
	}

	function copyResponse() {
		navigator.clipboard.writeText(response);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<svelte:head>
	<title>API Playground — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-4xl">
	<a href="/{audience}" class="mb-6 inline-flex items-center gap-1 text-sm hover:underline" style="color: var(--color-muted)">
		<ArrowLeft class="h-3 w-3" />
		Back to docs
	</a>

	<h1 class="text-3xl font-bold tracking-tight">API Playground</h1>
	<p class="mt-2 text-sm" style="color: var(--color-muted)">
		Test API endpoints directly from the browser. Authentication is optional.
	</p>

	<!-- Sample endpoints -->
	<div class="mt-6 rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h3 class="mb-3 text-sm font-semibold">Quick Endpoints</h3>
		<div class="flex flex-wrap gap-2">
			{#each sampleEndpoints as ep}
				<button
					class="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border)"
					onclick={() => { method = ep.method; path = ep.path; }}
				>
					<span class="font-mono text-[10px]" style="color: {ep.method === 'GET' ? 'var(--color-success)' : 'var(--color-warning)'}">
						{ep.method}
					</span>
					{ep.path}
				</button>
			{/each}
		</div>
	</div>

	<!-- Request builder -->
	<div class="mt-6 rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="flex items-center gap-3">
			<select
				class="rounded-md border px-3 py-2 text-sm font-medium"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				bind:value={method}
			>
				<option value="GET">GET</option>
				<option value="POST">POST</option>
				<option value="PATCH">PATCH</option>
				<option value="DELETE">DELETE</option>
			</select>
			<input
				type="text"
				class="flex-1 rounded-md border px-3 py-2 font-mono text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="/api/v1/..."
				bind:value={path}
			/>
			<button
				class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={loading}
				onclick={sendRequest}
			>
				{#if loading}
					<Loader2 class="h-4 w-4 animate-spin" />
					Sending...
				{:else}
					<Play class="h-4 w-4" />
					Send
				{/if}
			</button>
		</div>

		<div class="mt-3 flex items-center gap-4">
			<label class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
				<input type="checkbox" bind:checked={useAuth} class="h-3 w-3" />
				Include Authorization header
			</label>
		</div>

		{#if method !== 'GET'}
			<div class="mt-4">
				<label for="req-body" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Request Body (JSON)</label>
				<textarea
					id="req-body"
					class="w-full rounded-md border px-3 py-2 font-mono text-xs"
					rows="6"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="JSON body"
					bind:value={requestBody}
				></textarea>
			</div>
		{/if}
	</div>

	<!-- Response -->
	{#if response}
		<div class="mt-6 rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-center justify-between border-b px-4 py-3" style="border-color: var(--color-border)">
				<div class="flex items-center gap-3">
					<h3 class="text-sm font-semibold">Response</h3>
					{#if responseStatus !== null}
						<span
							class="rounded-full px-2 py-0.5 text-xs font-medium"
							style="background: color-mix(in srgb, {responseStatus >= 200 && responseStatus < 300 ? 'var(--color-success)' : responseStatus >= 400 ? 'var(--color-danger)' : 'var(--color-warning)'} 12%, transparent); color: {responseStatus >= 200 && responseStatus < 300 ? 'var(--color-success)' : responseStatus >= 400 ? 'var(--color-danger)' : 'var(--color-warning)'}"
						>
							{responseStatus}
						</span>
					{/if}
				</div>
				<button
					class="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="color: var(--color-muted)"
					onclick={copyResponse}
				>
					{#if copied}
						<Check class="h-3 w-3" style="color: var(--color-success)" />
						Copied
					{:else}
						<Copy class="h-3 w-3" />
						Copy
					{/if}
				</button>
			</div>
			<pre class="overflow-x-auto p-4 font-mono text-xs leading-relaxed"><code>{response}</code></pre>
		</div>
	{/if}
</div>
