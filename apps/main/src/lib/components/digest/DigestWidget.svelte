<script lang="ts">
	// DigestWidget — surfaces the latest community digest on the Space home
	// page (§84, ADR-092). Fetches /digest/latest?dest={spaceId} and renders
	// a compact card with a link to the full digest page.
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Newspaper, ChevronRight } from 'lucide-svelte';

	type DigestEdition = {
		id: string;
		space_id?: string;
		cadence: string;
		status: string;
		scheduled_for?: string;
		published_at?: string;
		created_at: string;
		content?: unknown;
	};

	let { spaceId }: { spaceId: string } = $props();

	let digest = $state<DigestEdition | null>(null);
	let loading = $state(true);
	let errored = $state(false);

	onMount(async () => {
		try {
			const res = await api.get<DigestEdition>(`/digest/latest?dest=${encodeURIComponent(spaceId)}`);
			if (res.data) {
				digest = res.data;
			}
		} catch {
			errored = true;
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<div class="animate-pulse rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="h-4 w-1/3 rounded bg-[var(--color-border)]"></div>
		<div class="mt-3 h-3 w-2/3 rounded bg-[var(--color-border)]"></div>
	</div>
{:else if digest}
	<a
		href="/digest/{digest.id}"
		class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
	>
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<Newspaper size={18} class="text-[var(--color-primary)]" />
				<span class="text-sm font-semibold text-[var(--color-foreground)]">
					{t('digest.widget_title', 'Community Digest')}
				</span>
			</div>
			<ChevronRight size={16} class="text-[var(--color-muted)]" />
		</div>
		<p class="mt-2 text-xs text-[var(--color-text-subtle)]">
			{digest.cadence === 'weekly'
				? t('digest.cadence_weekly', 'Weekly')
				: t('digest.cadence_monthly', 'Monthly')}
			{#if digest.published_at}
				· {new Date(digest.published_at).toLocaleDateString()}
			{/if}
		</p>
	</a>
{:else if !errored}
	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-2">
			<Newspaper size={18} class="text-[var(--color-muted)]" />
			<span class="text-sm font-semibold text-[var(--color-foreground)]">
				{t('digest.widget_title', 'Community Digest')}
			</span>
		</div>
		<p class="mt-2 text-xs text-[var(--color-text-subtle)]">
			{t('digest.widget_empty', 'No digest available yet')}
		</p>
	</div>
{/if}
