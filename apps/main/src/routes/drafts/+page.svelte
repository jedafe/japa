<script lang="ts">
	import { onMount } from 'svelte';
	import { FileText, Edit2, Trash2, Clock, Calendar } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Draft = {
		id: string;
		title: string;
		content_type: string;
		status: string;
		updated_at: string;
		created_at: string;
		scheduled_at?: string;
	};

	let activeTab = $state<'drafts' | 'scheduled'>('drafts');
	let drafts = $state<Draft[]>([]);
	let scheduled = $state<Draft[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const [draftsData, scheduledData] = await Promise.all([
				api.getDrafts(),
				api.getScheduled()
			]);
			drafts = draftsData;
			scheduled = scheduledData;
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function deleteDraft(id: string) {
		if (!confirm(t('drafts.confirm_delete'))) return;
		try {
			await api.deleteContent(id);
			drafts = drafts.filter((d) => d.id !== id);
			scheduled = scheduled.filter((d) => d.id !== id);
		} catch {
			error = t('common.error');
		}
	}

	function timeAgo(date: string): string {
		const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000);
		if (seconds < 60) return t('drafts.just_now');
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${t('drafts.ago')}`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${t('drafts.ago')}`;
		return `${Math.floor(seconds / 86400)}d ${t('drafts.ago')}`;
	}

	function formatScheduledDate(date: string): string {
		return new Date(date).toLocaleString();
	}
</script>

<svelte:head>
	<title>{t('drafts.title')} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('drafts.title')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('drafts.description')}
		</p>
	</header>

	{#if error}
		<div class="mb-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<!-- Tabs -->
	<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
			style={activeTab === 'drafts' ? 'background-color: var(--color-primary); color: white' : ''}
			onclick={() => { activeTab = 'drafts'; }}
		>
			<FileText class="h-4 w-4" />
			{t('drafts.drafts')} ({drafts.length})
		</button>
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
			style={activeTab === 'scheduled' ? 'background-color: var(--color-primary); color: white' : ''}
			onclick={() => { activeTab = 'scheduled'; }}
		>
			<Calendar class="h-4 w-4" />
			{t('drafts.scheduled')} ({scheduled.length})
		</button>
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each [1, 2, 3] as _}
				<div class="h-20 animate-pulse rounded-xl" style="background-color: var(--color-background)"></div>
			{/each}
		</div>
	{:else if activeTab === 'drafts'}
		{#if drafts.length > 0}
			<div class="space-y-3">
				{#each drafts as draft (draft.id)}
					<div
						class="flex items-center justify-between rounded-xl border p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
						style="border-color: var(--color-border); background: var(--color-surface);"
					>
						<div class="flex items-center gap-3">
							<FileText class="h-8 w-8" style="color: var(--color-muted)" />
							<div>
								<p class="font-medium" style="color: var(--color-foreground)">{draft.title}</p>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span>{draft.content_type}</span>
									<span>&middot;</span>
									<span>{t('drafts.edited')} {timeAgo(draft.updated_at)}</span>
								</div>
							</div>
						</div>
						<div class="flex items-center gap-2">
							<a
								href={`/create?edit=${draft.id}`}
								class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="border-color: var(--color-border); color: var(--color-foreground)"
							>
								<Edit2 class="h-3.5 w-3.5" />
								{t('drafts.edit')}
							</a>
							<button
								onclick={() => deleteDraft(draft.id)}
								class="rounded-lg p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
								style="color: var(--color-danger)"
								title={t('common.delete')}
							>
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					</div>
				{/each}
			</div>
		{:else}
			<div class="py-12 text-center">
				<FileText class="mx-auto h-12 w-12" style="color: var(--color-border)" />
				<p class="mt-4 text-sm" style="color: var(--color-muted)">
					{t('drafts.no_drafts')}
				</p>
				<a
					href="/create"
					class="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
				>
					{t('drafts.create_content')}
				</a>
			</div>
		{/if}
	{:else}
		{#if scheduled.length > 0}
			<div class="space-y-3">
				{#each scheduled as item (item.id)}
					<div
						class="flex items-center justify-between rounded-xl border p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
						style="border-color: var(--color-border); background: var(--color-surface);"
					>
						<div class="flex items-center gap-3">
							<Calendar class="h-8 w-8" style="color: var(--color-primary)" />
							<div>
								<p class="font-medium" style="color: var(--color-foreground)">{item.title}</p>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span>{item.content_type}</span>
									{#if item.scheduled_at}
										<span>&middot;</span>
										<Clock class="h-3 w-3" />
										<span>{formatScheduledDate(item.scheduled_at)}</span>
									{/if}
								</div>
							</div>
						</div>
						<div class="flex items-center gap-2">
							<a
								href={`/create?edit=${item.id}`}
								class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="border-color: var(--color-border); color: var(--color-foreground)"
							>
								<Edit2 class="h-3.5 w-3.5" />
								{t('drafts.edit')}
							</a>
							<button
								onclick={() => deleteDraft(item.id)}
								class="rounded-lg p-1.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
								style="color: var(--color-danger)"
								title={t('common.delete')}
							>
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					</div>
				{/each}
			</div>
		{:else}
			<div class="py-12 text-center">
				<Calendar class="mx-auto h-12 w-12" style="color: var(--color-border)" />
				<p class="mt-4 text-sm" style="color: var(--color-muted)">
					{t('drafts.no_scheduled')}
				</p>
			</div>
		{/if}
	{/if}
</main>
