<script lang="ts">
	import { Download, ArrowLeft, Award } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	let certificate = $derived(data.certificate);
	let downloading = $state(false);

	async function handleDownload() {
		if (!certificate) return;
		downloading = true;
		try {
			const result = await api.downloadCertificate(certificate.id);
			if (result.download_url) {
				window.open(result.download_url, '_blank');
			}
		} catch {
			// Download failed silently
		} finally {
			downloading = false;
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}
</script>

<svelte:head>
	<title>{t('certificate.title')} — JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/learn"
		class="mb-6 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('common.back')}
	</a>

	{#if certificate}
		<div
			class="rounded-xl border p-8 text-center"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)">
				<Award class="h-8 w-8" style="color: var(--color-primary)" />
			</div>

			<h1 class="text-xl font-bold" style="color: var(--color-foreground)">
				{t('certificate.congratulations')}
			</h1>

			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				{t('certificate.earned_for')} <span class="font-medium" style="color: var(--color-foreground)">{certificate.track_title}</span>
			</p>

			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('certificate.issued_on')} {formatDate(certificate.issued_at)}
			</p>

			<button
				onclick={handleDownload}
				disabled={downloading}
				class="mt-6 inline-flex items-center gap-2 rounded-lg px-6 py-3 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				<Download class="h-4 w-4" />
				{downloading ? t('common.loading') : t('certificate.download')}
			</button>
		</div>
	{/if}
</main>
