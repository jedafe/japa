<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import type { Certificate } from '$lib/api';
	import { api } from '$lib/api';
	import { Award, Download, ExternalLink, GraduationCap, ArrowRight, Loader2 } from 'lucide-svelte';

	let { data } = $props();
	let certificates = $derived(data.certificates as Certificate[]);

	let downloadingId = $state<string | null>(null);

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}

	async function handleDownload(certId: string) {
		downloadingId = certId;
		try {
			const result = await api.downloadCertificate(certId);
			if (result.download_url) {
				window.open(result.download_url, '_blank');
			} else {
				alert(result.message ?? 'PDF not available yet');
			}
		} catch (e) {
			alert(e instanceof Error ? e.message : 'Failed to get download link');
		} finally {
			downloadingId = null;
		}
	}
</script>

<svelte:head>
	<title>My Certificates | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8 sm:px-6">
	<div class="flex items-center gap-3">
		<div class="rounded-lg p-2" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)">
			<Award class="h-6 w-6" style="color: var(--color-primary)" />
		</div>
		<div>
			<h1 class="text-2xl font-bold tracking-tight" style="color: var(--color-foreground)">{t('certificates.my_certificates')}</h1>
			<p class="text-sm" style="color: var(--color-muted)">
				Certificates earned by completing learning tracks
			</p>
		</div>
	</div>

	{#if certificates.length === 0}
		<div class="mt-8 rounded-lg border px-6 py-16 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<GraduationCap class="mx-auto h-16 w-16" style="color: var(--color-muted)" />
			<h2 class="mt-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('certificates.no_certificates_yet')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Complete a learning track to earn your first certificate!
			</p>
			<a
				href="/"
				class="mt-6 inline-flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				Browse tracks
				<ArrowRight class="h-4 w-4" />
			</a>
		</div>
	{:else}
		<div class="mt-8 space-y-4">
			{#each certificates as cert (cert.id)}
				<article class="rounded-lg border p-6 transition-all hover:shadow-md" style="border-color: var(--color-border)">
					<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
						<div class="flex items-start gap-4">
							<div class="rounded-lg p-2.5" style="background-color: color-mix(in srgb, var(--color-success) 10%, transparent)">
								<Award class="h-6 w-6" style="color: var(--color-success)" />
							</div>
							<div>
								<h3 class="font-semibold" style="color: var(--color-foreground)">Certificate #{cert.certificate_number}</h3>
								<p class="mt-1 text-sm" style="color: var(--color-muted)">
									Issued {formatDate(cert.issued_at)}
								</p>
							</div>
						</div>

						<div class="flex items-center gap-2">
							<a
								href="/certificates/{cert.id}"
								class="flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
								style="border-color: var(--color-border); color: var(--color-foreground)"
							>
								<ExternalLink class="h-4 w-4" />
								View
							</a>
							<button
								class="flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
								style="background-color: var(--color-primary)"
								disabled={downloadingId === cert.id}
								onclick={() => handleDownload(cert.id)}
							>
								{#if downloadingId === cert.id}
									<Loader2 class="h-4 w-4 animate-spin" />
									Loading...
								{:else}
									<Download class="h-4 w-4" />
									Download
								{/if}
							</button>
						</div>
					</div>
				</article>
			{/each}
		</div>
	{/if}
</main>
