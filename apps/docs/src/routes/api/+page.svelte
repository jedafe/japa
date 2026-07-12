<script lang="ts">
	import { page } from '$app/state';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { Input, Badge } from '$lib/components/ui';

	let { data } = $props();

	type HttpMethod = 'get' | 'post' | 'put' | 'patch' | 'delete';
	const METHOD_COLORS: Record<HttpMethod, string> = {
		get: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200',
		post: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
		put: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
		patch: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
		delete: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
	};

	let selectedTag = $state('');
	let searchQuery = $state('');

	type Endpoint = { method: HttpMethod; path: string; summary: string; description: string; parameters: Array<{ name: string; in: string; required?: boolean; description?: string }>; tags: string[] };

	let endpoints = $derived.by(() => {
		if (!data.spec?.paths) return [];
		const result: Endpoint[] = [];
		for (const [path, methods] of Object.entries(data.spec.paths)) {
			for (const [method, detail] of Object.entries(methods)) {
				if (['get', 'post', 'put', 'patch', 'delete'].includes(method)) {
					result.push({
						method: method as HttpMethod,
						path,
						summary: detail.summary ?? '',
						description: detail.description ?? '',
						parameters: detail.parameters ?? [],
						tags: detail.tags ?? []
					});
				}
			}
		}
		return result;
	});

	let filteredEndpoints = $derived(
		endpoints.filter((ep) => {
			const matchesTag = !selectedTag || ep.tags.includes(selectedTag);
			const matchesSearch = !searchQuery ||
				ep.path.toLowerCase().includes(searchQuery.toLowerCase()) ||
				ep.summary.toLowerCase().includes(searchQuery.toLowerCase());
			return matchesTag && matchesSearch;
		})
	);

	let groupedByTag = $derived.by(() => {
		const groups: Record<string, Endpoint[]> = {};
		for (const ep of filteredEndpoints) {
			const tag = ep.tags[0] ?? 'Other';
			if (!groups[tag]) groups[tag] = [];
			groups[tag].push(ep);
		}
		return groups;
	});

	let tags = $derived(data.spec?.tags?.map((t) => t.name) ?? []);
</script>

<svelte:head>
	<title>API Reference — JapaConnect Docs</title>
</svelte:head>

<div class="max-w-4xl">
	<h1 class="mb-2 text-3xl font-bold tracking-tight">API Reference</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		{#if data.spec?.info}
			{data.spec.info.title ?? 'JapaConnect API'} v{data.spec.info.version ?? ''}
			{#if data.spec.info.description}
				— {data.spec.info.description}
			{/if}
		{:else}
			JapaConnect REST API endpoints. All routes are prefixed with <code class="rounded bg-gray-100 px-1 py-0.5 text-xs dark:bg-gray-800">/api/v1</code> unless noted.
		{/if}
	</p>

	{#if data.error}
		<div class="mb-6 rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
			<p class="font-medium">Could not load OpenAPI spec</p>
			<p class="mt-1">{data.error}. Showing static endpoint reference.</p>
		</div>
	{/if}

	<!-- Filters -->
	<div class="mb-6 flex flex-wrap gap-3">
		<Input
			type="text"
			placeholder="Search endpoints..."
			bind:value={searchQuery}
			class="max-w-xs"
		/>
		<select
			class="rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={selectedTag}
		>
			<option value="">All Categories</option>
			{#each tags as tag}
				<option value={tag}>{tag}</option>
			{/each}
		</select>
	</div>

	<!-- Endpoints -->
	{#if data.spec?.paths}
		{#each Object.entries(groupedByTag) as [tag, eps]}
			<section class="mb-8">
				<h2 class="mb-3 text-lg font-semibold" style="color: var(--color-foreground)">{tag}</h2>
				<div class="space-y-2">
					{#each eps as ep}
						<details class="group rounded-lg border" style="border-color: var(--color-border)">
							<summary class="flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-black/[0.02] dark:hover:bg-white/[0.02]">
								<Badge class="font-bold uppercase {METHOD_COLORS[ep.method]}">
									{ep.method}
								</Badge>
								<code class="text-sm font-mono">{ep.path}</code>
								{#if ep.summary}
									<span class="ml-auto text-xs" style="color: var(--color-muted)">{ep.summary}</span>
								{/if}
							</summary>
							<div class="border-t px-4 py-3" style="border-color: var(--color-border)">
								{#if ep.description}
									<p class="mb-3 text-sm" style="color: var(--color-muted)">{ep.description}</p>
								{/if}
								{#if ep.parameters.length > 0}
									<h4 class="mb-2 text-xs font-semibold uppercase" style="color: var(--color-muted)">Parameters</h4>
									<div class="mb-3 overflow-x-auto rounded border" style="border-color: var(--color-border)">
										<table class="w-full text-xs">
											<thead>
												<tr style="border-color: var(--color-border)">
													<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Name</th>
													<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">In</th>
													<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Required</th>
													<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Description</th>
												</tr>
											</thead>
											<tbody>
												{#each ep.parameters as param}
													<tr class="border-t" style="border-color: var(--color-border)">
														<td class="px-3 py-2 font-mono">{param.name}</td>
														<td class="px-3 py-2">{param.in}</td>
														<td class="px-3 py-2">{param.required ? 'Yes' : 'No'}</td>
														<td class="px-3 py-2" style="color: var(--color-muted)">{param.description ?? '—'}</td>
													</tr>
												{/each}
											</tbody>
										</table>
									</div>
								{/if}
								<p class="text-xs" style="color: var(--color-muted)">
									Requires authentication via session cookie or <code class="rounded bg-gray-100 px-1 dark:bg-gray-800">Authorization: Bearer &lt;token&gt;</code> header.
								</p>
							</div>
						</details>
					{/each}
				</div>
			</section>
		{/each}

		{#if filteredEndpoints.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No endpoints match your search.</p>
		{/if}
	{:else}
		<div class="space-y-6">
			<section>
				<h2 class="mb-3 text-lg font-semibold">Authentication</h2>
				<div class="rounded-lg border p-4 text-sm" style="border-color: var(--color-border)">
					<p class="mb-2">All endpoints require authentication unless marked <code class="rounded bg-gray-100 px-1 text-xs dark:bg-gray-800">[public]</code>.</p>
					<p class="mb-1 font-medium">Session-based:</p>
					<p class="mb-3 text-xs" style="color: var(--color-muted)">Login via <code>POST /auth/login</code> sets an httpOnly session cookie.</p>
					<p class="mb-1 font-medium">API Key:</p>
					<p class="text-xs" style="color: var(--color-muted)"><code>Authorization: Bearer &lt;api_key&gt;</code> header. Manage keys at <a href="/api-keys" class="underline" style="color: var(--color-primary)">/api-keys</a>.</p>
				</div>
			</section>

			<section>
				<h2 class="mb-3 text-lg font-semibold">Base URL</h2>
				<div class="rounded-lg border p-4 text-sm" style="border-color: var(--color-border)">
					<code class="text-xs">{PUBLIC_JC_API_URL}/api/v1</code>
				</div>
			</section>

			<section>
				<h2 class="mb-3 text-lg font-semibold">Rate Limits</h2>
				<div class="rounded-lg border p-4 text-sm" style="border-color: var(--color-border)">
					<p>Standard: 100 requests/minute per IP. Authenticated: 300 requests/minute per user.</p>
				</div>
			</section>

			<section>
				<h2 class="mb-3 text-lg font-semibold">Error Format</h2>
				<div class="rounded-lg border p-4 text-sm" style="border-color: var(--color-border)">
					<pre class="text-xs">{"{ \"error\": { \"code\": \"not_found\", \"message\": \"...\", \"status\": 404 }"}</pre>
				</div>
			</section>
		</div>
	{/if}
</div>
