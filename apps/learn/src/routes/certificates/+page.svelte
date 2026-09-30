<script lang="ts">
	// Certificates — CSR-only (GET /me/certificates, ADR-060 SSR/CSR boundary).
	// Earned certificates grid, each linking to its public shareable page
	// (/certificates/:id). Per ADR-060 §4, certificates are shareable social proof.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Trophy, ExternalLink, Award } from 'lucide-svelte';
	import type { Certificate } from '$lib/types';

	let certificates = $state<Certificate[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let initialized = $state(false);

	async function load(force = false) {
		// Retry must bypass the guard — the error EmptyState calls load again.
		if (initialized && !force) return;
		initialized = true;
		loading = true;
		error = null;
		const r = await api.get<Certificate[]>('/me/certificates');
		loading = false;
		if (r.error) {
			error = r.error.message || t('common.error');
			certificates = [];
		} else if (r.data) {
			certificates = r.data;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('learn.certificates')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<Trophy size={24} class="text-[var(--level-intermediate)]" />
			{t('learn.certificates')}
		</h1>
	</header>

	{#if loading}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-3 h-20 w-full" />
					<LoadingSkeleton class="h-4 w-3/4" />
					<LoadingSkeleton class="mt-2 h-4 w-1/2" />
				</div>
			{/each}
		</div>
	{:else if error}
		<EmptyState
			title={t('common.error')}
			description={error}
		>
			{#snippet action()}
				<button
					type="button"
					onclick={() => load(true)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.retry')}
				</button>
			{/snippet}
		</EmptyState>
	{:else if certificates.length === 0}
		<EmptyState
			title={t('learn.no_certificates')}
			description={t('learn.no_certificates_description')}
		>
			{#snippet action()}
				<a
					href="/"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('learn.explore')}
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
			{#each certificates as cert (cert.id)}
				<a
					href="/certificates/{cert.id}"
					class="group flex flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-shadow hover:shadow-md"
				>
					<div class="mb-3 flex items-center justify-between">
						<Award size={32} class="text-[var(--level-intermediate)]" />
						<ExternalLink size={16} class="text-[var(--color-muted)] opacity-0 group-hover:opacity-100" />
					</div>
					<p class="text-base font-semibold text-[var(--color-text)]">{cert.title}</p>
					<p class="mt-1 text-sm text-[var(--color-muted)]">
						{t('learn.earned_by')}: {cert.recipient_name}
					</p>
					<p class="mt-2 text-xs text-[var(--color-muted)]">
						{new Date(cert.issued_at).toLocaleDateString()}
					</p>
					{#if cert.certificate_number}
						<p class="mt-1 text-xs text-[var(--color-muted)]">#{cert.certificate_number}</p>
					{/if}
				</a>
			{/each}
		</div>
	{/if}
</div>
