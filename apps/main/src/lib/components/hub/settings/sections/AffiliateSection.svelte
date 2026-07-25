<script lang="ts">
	import { t } from '$lib/i18n';
	import { createAffiliate, deleteAffiliate } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Share2 } from 'lucide-svelte';

	interface AffiliateLink {
		id: string;
		target_url: string;
		label?: string;
		created_at?: string;
	}

	let { data }: { data: unknown } = $props();
	let links = $state<AffiliateLink[]>((data as AffiliateLink[] | null) ?? []);
	let newUrl = $state('');
	let newLabel = $state('');
	let creating = $state(false);
	let error = $state<string | null>(null);

	async function doCreate() {
		if (!newUrl.trim()) return;
		creating = true;
		error = null;
		const res = await createAffiliate({ target_url: newUrl.trim(), label: newLabel.trim() || undefined });
		creating = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) links = [res.data as AffiliateLink, ...links];
		newUrl = '';
		newLabel = '';
	}

	async function doDelete(id: string) {
		const idx = links.findIndex((l) => l.id === id);
		if (idx < 0) return;
		const removed = links[idx];
		await optimisticAction({
			apply: () => (links = links.filter((l) => l.id !== id)),
			rollback: () => (links = [...links.slice(0, idx), removed, ...links.slice(idx)]),
			call: () => deleteAffiliate(id)
		});
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.affiliate')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.affiliate.desc')}</p>
	</header>

	<form class="mb-6 space-y-3" onsubmit={(e) => { e.preventDefault(); doCreate(); }}>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.affiliate.url')}</span>
			<input
				type="url"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				placeholder="https://"
				value={newUrl}
				oninput={(e) => (newUrl = e.currentTarget.value)}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.affiliate.label')}</span>
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={newLabel}
				oninput={(e) => (newLabel = e.currentTarget.value)}
			/>
		</label>
		{#if error}<p class="text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		<button
			type="submit"
			disabled={creating || !newUrl.trim()}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{creating ? t('settings.affiliate.creating') : t('settings.affiliate.create')}
		</button>
	</form>

	{#if links.length === 0}
		<EmptyState icon={Share2} title={t('settings.affiliate.empty_title')} body={t('settings.affiliate.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each links as link (link.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="min-w-0">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">{link.label ?? link.target_url}</p>
						<p class="truncate text-xs text-[var(--color-muted)]">{link.target_url}</p>
					</div>
					<button
						type="button"
						class="ml-3 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						onclick={() => doDelete(link.id)}
					>
						{t('settings.affiliate.delete')}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>
