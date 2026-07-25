<script lang="ts">
	import { t } from '$lib/i18n';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Award } from 'lucide-svelte';

	interface Certificate {
		id: string;
		title?: string;
		track_slug?: string;
		issued_at?: string;
	}

	let { data }: { data: unknown } = $props();
	const certificates = (data as Certificate[] | null) ?? [];
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.certificates')}</h1>
	</header>

	<DashboardPanel titleKey="dashboard.certificates.title">
		{#if certificates.length === 0}
			<EmptyState icon={Award} title={t('dashboard.certificates.empty_title')} body={t('dashboard.certificates.empty_body')} />
		{:else}
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each certificates as cert (cert.id)}
					<a href="/certificates/{cert.id}" class="block rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4 hover:border-[var(--color-primary)]">
						<div class="mb-2 grid h-10 w-10 place-items-center rounded-full bg-[var(--color-primary)] text-white">
							<Award size={20} />
						</div>
						<p class="text-sm font-medium text-[var(--color-text)]">{cert.title ?? cert.id}</p>
						{#if cert.issued_at}
							<p class="mt-1 text-xs text-[var(--color-muted)]">{t('dashboard.certificates.issued')} {cert.issued_at}</p>
						{/if}
					</a>
				{/each}
			</div>
		{/if}
	</DashboardPanel>
</section>
