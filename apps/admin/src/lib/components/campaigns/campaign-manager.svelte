<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { campaignsAdminApi, api, type Campaign, type CampaignItem } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Play, X, RefreshCw } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';

	type PersonaOption = { id: string; label: string };

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let personas = $state<PersonaOption[]>([]);
	let retryingItemId = $state('');
	let retryError = $state('');
	let itemRefreshKey = $state(0);

	onMount(async () => {
		const res = await api<{ data: { id: string; name: string }[] }>('/admin/studio/personas');
		if (res.ok && res.data) {
			personas = (res.data.data ?? []).map((p: any) => ({ id: p.id, label: p.name ?? p.id }));
		}
	});

	async function retryCampaignItem(item: CampaignItem) {
		retryingItemId = item.id;
		retryError = '';
		const res = await campaignsAdminApi.retryItem(item.id);
		if (!res.ok) {
			retryError = res.error ?? t('campaigns.retry_error', 'Failed to retry item');
		} else {
			itemRefreshKey += 1;
			entityController?.reload();
		}
		retryingItemId = '';
	}

	const fields: FieldDef[] = [
		{ key: 'prompt', label: t('campaigns.prompt', 'Prompt'), type: 'textarea', required: true },
		{ key: 'count', label: t('campaigns.count', 'Count'), type: 'number', required: true },
		{ key: 'content_type', label: t('campaigns.content_type', 'Content Type'), type: 'text' },
		{ key: 'target_space_id', label: t('campaigns.target_space_id', 'Target Space ID'), type: 'text' },
		{ key: 'target_tags', label: t('campaigns.target_tags', 'Target Tags'), type: 'text' },
		{ key: 'auto_provision_personas', label: t('campaigns.auto_provision_personas', 'Auto-provision personas'), type: 'checkbox' },
		{ key: 'auto_assign', label: t('campaigns.auto_assign', 'Auto-assign personas'), type: 'checkbox' },
		{ key: 'run_immediately', label: t('campaigns.run_immediately', 'Run immediately'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('campaigns.title', 'Content Campaigns'),
		description: t('campaigns.description', 'Plan and run multi-persona AI content campaigns.'),
		createLabel: t('campaigns.create', 'New Campaign'),
		createPermission: 'campaign.create',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('campaigns.name', 'Name') },
			{
				key: 'prompt',
				label: t('campaigns.prompt', 'Prompt'),
				render: (v: string) => `<span class="truncate text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'status', label: t('campaigns.status', 'Status'), componentSnippet: StatusCell },
			{ key: 'content_type', label: t('campaigns.content_type', 'Type') },
			{ key: 'requested_count', label: t('campaigns.requested_count', 'Count') },
			{ key: 'provisioned_personas', label: t('campaigns.provisioned', 'Provisioned') },
			{
				key: 'created_at',
				label: t('campaigns.created_at', 'Created'),
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('campaigns.status', 'Status'),
				options: [
					{ value: 'planning', label: t('campaigns.status_planning', 'Planning') },
					{ value: 'planned', label: t('campaigns.status_planned', 'Planned') },
					{ value: 'generating', label: t('campaigns.status_generating', 'Generating') },
					{ value: 'completed', label: t('campaigns.status_completed', 'Completed') },
					{ value: 'failed', label: t('campaigns.status_failed', 'Failed') },
					{ value: 'cancelled', label: t('campaigns.status_cancelled', 'Cancelled') }
				]
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams();
			params.set('limit', String(pageSize));
			params.set('offset', String(page * pageSize));
			if (q) params.set('q', q);
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) {
					params.set('sort', sortKey);
					params.set('dir', sortDir ?? 'desc');
				}
			}
			if (filter.status) params.set('status', filter.status);
			const result = await campaignsAdminApi.list(params.toString());
			if (!result.ok) {
				throw new Error(result.error ?? t('campaigns.load_error', 'Failed to load campaigns'));
			}
			if (result.data) {
				const data = result.data.data ?? [];
				const total = result.data.meta?.total ?? data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(t('campaigns.load_error', 'Failed to load campaigns'));
		},
		create: async (body) => {
			const res = await campaignsAdminApi.create({
				prompt: String(body.prompt ?? '').trim(),
				name: body.name ? String(body.name).trim() : undefined,
				count: Number(body.count ?? 1),
				content_type: body.content_type ? String(body.content_type).trim() : undefined,
				persona_ids: parsePersonaIDs(body.persona_ids),
				auto_provision_personas: !!body.auto_provision_personas,
				auto_assign: !!body.auto_assign,
				target_space_id: body.target_space_id ? String(body.target_space_id).trim() : undefined,
				target_tags: parseTags(body.target_tags),
				run_immediately: !!body.run_immediately
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{
				label: t('campaigns.run', 'Run'),
				icon: Play,
				show: (row: Campaign) => row.status === 'planned',
				onclick: async (row: Campaign) => {
					const res = await campaignsAdminApi.run(row.id);
					if (!res.ok) pageError = res.error;
					else entityController?.reload();
				}
			},
			{
				label: t('campaigns.cancel', 'Cancel'),
				icon: X,
				variant: 'destructive',
				show: (row: Campaign) => !['completed', 'cancelled', 'failed'].includes(row.status),
				onclick: async (row: Campaign) => {
					const res = await campaignsAdminApi.cancel(row.id);
					if (!res.ok) pageError = res.error;
					else entityController?.reload();
				}
			}
		],
		viewSnippet: CampaignView,
		formSnippet: CampaignForm
	};

	function parsePersonaIDs(v: unknown): string[] | undefined {
		if (Array.isArray(v)) return v.filter(Boolean).map(String);
		if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter(Boolean);
		return undefined;
	}

	function parseTags(v: unknown): string[] | undefined {
		if (Array.isArray(v)) return v.filter(Boolean).map(String);
		if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter(Boolean);
		return undefined;
	}
</script>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet StatusCell({ value, row }: { value: string; row?: any })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value === 'completed'
			? 'bg-success/15 text-success'
			: value === 'failed' || value === 'cancelled'
				? 'bg-danger/15 text-danger'
				: value === 'generating'
					? 'bg-warning/15 text-warning'
					: 'bg-muted/15 text-muted'}"
	>
		{value}
	</span>
{/snippet}

{#snippet CampaignForm({ value }: { value: any; editing: boolean })}
	<div class="space-y-4">
		<div>
			<label class="block text-sm font-medium" for="campaign-prompt" style="color: var(--color-foreground)">
				{t('campaigns.prompt', 'Prompt')}
			</label>
			<textarea
				id="campaign-prompt"
				rows="4"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('campaigns.prompt_placeholder', 'Create 20 blog posts for Canadian users about investing in African countries...')}
				bind:value={value.prompt}
			></textarea>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label class="block text-sm font-medium" for="campaign-count" style="color: var(--color-foreground)">
					{t('campaigns.count', 'Count')}
				</label>
				<input
					id="campaign-count"
					type="number"
					min="1"
					max="100"
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={value.count}
				/>
			</div>
			<div>
				<label class="block text-sm font-medium" for="campaign-content-type" style="color: var(--color-foreground)">
					{t('campaigns.content_type', 'Content Type')}
				</label>
				<input
					id="campaign-content-type"
					type="text"
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder="blog, story, guide, discussion, reply"
					bind:value={value.content_type}
				/>
			</div>
		</div>
		<div>
			<label class="block text-sm font-medium" for="campaign-persona-pool" style="color: var(--color-foreground)">
				{t('campaigns.persona_pool', 'Persona Pool (optional)')}
			</label>
			<select
				id="campaign-persona-pool"
				multiple
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.persona_ids}
			>
				{#each personas as persona}
					<option value={persona.id}>{persona.label}</option>
				{/each}
			</select>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('campaigns.persona_pool_hint', 'Hold Ctrl/Cmd to select multiple. Leave empty to use all active personas.')}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label class="block text-sm font-medium" for="campaign-target-space" style="color: var(--color-foreground)">
					{t('campaigns.target_space_id', 'Target Space ID')}
				</label>
				<input
					id="campaign-target-space"
					type="text"
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={value.target_space_id}
				/>
			</div>
			<div>
				<label class="block text-sm font-medium" for="campaign-target-tags" style="color: var(--color-foreground)">
					{t('campaigns.target_tags', 'Target Tags')}
				</label>
				<input
					id="campaign-target-tags"
					type="text"
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder="tag1, tag2"
					bind:value={value.target_tags}
				/>
			</div>
		</div>
		<div class="flex flex-wrap gap-4">
			<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
				<input type="checkbox" bind:checked={value.auto_provision_personas} />
				{t('campaigns.auto_provision_personas', 'Auto-provision personas')}
			</label>
			<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
				<input type="checkbox" bind:checked={value.auto_assign} />
				{t('campaigns.auto_assign', 'Auto-assign personas')}
			</label>
			<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
				<input type="checkbox" bind:checked={value.run_immediately} />
				{t('campaigns.run_immediately', 'Run immediately')}
			</label>
		</div>
	</div>
{/snippet}

{#snippet CampaignView({ row }: { row: Campaign })}
	{#if retryError}
		<div
			class="mb-4 rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{retryError}
		</div>
	{/if}
	{#key itemRefreshKey}
		{#await campaignsAdminApi.get(row.id)}
		<div class="flex items-center gap-2 text-sm" style="color: var(--color-muted)">
			<RefreshCw class="h-4 w-4 animate-spin" />
			{t('common.loading', 'Loading...')}
		</div>
	{:then res}
		{@const viewError = res.ok ? '' : (res.error ?? t('common.error', 'An error occurred'))}
		{@const detail = res.ok ? res.data : undefined}
		{@const campaign = detail?.campaign ?? row}
		{@const items = detail?.items ?? []}

		{#if viewError}
			<div
				class="mb-4 rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{viewError}
			</div>
		{/if}

		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{campaign.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.prompt', 'Prompt')}</p>
				<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{campaign.prompt}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.status', 'Status')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{campaign.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.requested_count', 'Count')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{campaign.requested_count}</p>
				</div>
			</div>
			{#if campaign.target_space_id || (campaign.target_tags && campaign.target_tags.length > 0)}
				<div class="grid grid-cols-2 gap-4">
					{#if campaign.target_space_id}
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.target_space_id', 'Target Space')}</p>
							<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{campaign.target_space_id}</p>
						</div>
					{/if}
					{#if campaign.target_tags && campaign.target_tags.length > 0}
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.target_tags', 'Target Tags')}</p>
							<p class="mt-1 text-sm" style="color: var(--color-foreground)">{campaign.target_tags.join(', ')}</p>
						</div>
					{/if}
				</div>
			{/if}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('campaigns.items', 'Items')}</p>
				{#if items.length === 0}
					<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('campaigns.items_detail_hint', 'Open the campaign to see individual items.')}</p>
				{:else}
					<div class="mt-2 space-y-2">
						{#each items as item (item.id)}
							<div class="rounded-lg border p-3 text-sm" style="border-color: var(--color-border)">
								<div class="flex items-start justify-between gap-2">
									<div class="min-w-0 flex-1">
										<p class="break-words font-medium" style="color: var(--color-foreground)">
											{item.title || item.topic_prompt}
										</p>
										<p class="mt-1 text-xs" style="color: var(--color-muted)">
											{item.persona_name ?? item.persona_id} · {item.content_type}
										</p>
									</div>
									<div class="flex items-center gap-2">
										{@render StatusCell({ value: item.status, row: item })}
										{#if item.status === 'failed'}
											<Button
												size="sm"
												variant="outline"
												disabled={retryingItemId === item.id}
												onclick={() => retryCampaignItem(item)}
											>
												{#if retryingItemId === item.id}
													<RefreshCw class="h-3 w-3 animate-spin" />
													{t('campaigns.retrying', 'Retrying...')}
												{:else}
													{t('campaigns.retry', 'Retry')}
												{/if}
											</Button>
										{/if}
									</div>
								</div>
								<div class="mt-2 grid gap-1 text-xs font-mono" style="color: var(--color-foreground)">
									{#if item.studio_job_id}
										<div>Studio Job ID: {item.studio_job_id}</div>
									{/if}
									{#if item.content_id}
										<div>Content ID: {item.content_id}</div>
									{/if}
									{#if item.review_id}
										<div>Review ID: {item.review_id}</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{:catch err}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{err instanceof Error ? err.message : t('common.error', 'An error occurred')}
		</div>
	{/await}
	{/key}
{/snippet}
