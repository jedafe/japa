<script lang="ts">
	import { Award, Download, Share2, ArrowLeft, CheckCircle, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	const cert = $derived(data.certificate);

	let downloading = $state(false);
	let downloadMessage = $state('');

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	}

	function handleShare() {
		if (navigator.share) {
			navigator.share({
				title: `Certificate - ${cert?.track_title}`,
				text: `I completed ${cert?.track_title} on JapaConnect Learn!`,
				url: window.location.href
			});
		}
	}

	async function handleDownload() {
		if (!cert) return;
		downloading = true;
		downloadMessage = '';
		try {
			const result = await api.downloadCertificate(cert.id);
			if (result.download_url) {
				window.open(result.download_url, '_blank');
			} else {
				downloadMessage = result.message ?? 'PDF not available yet';
			}
		} catch (e) {
			downloadMessage = e instanceof Error ? e.message : 'Failed to get download link';
		} finally {
			downloading = false;
		}
	}
</script>

<svelte:head>
	<title>{cert ? `Certificate - ${cert.track_title ?? 'Learning Track'}` : 'Certificate'} | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8 sm:px-6">
	<a
		href="/certificates"
		class="mb-6 inline-flex items-center gap-1 text-sm font-medium transition-colors hover:opacity-80"
		style="color: var(--color-primary)"
	>
		<ArrowLeft class="h-4 w-4" />
		My Certificates
	</a>

	{#if data.error}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
			<p style="color: var(--color-muted)">{data.error}</p>
			<a
				href="/certificates"
				class="mt-4 inline-block text-sm font-medium"
				style="color: var(--color-primary)"
			>
				Back to certificates
			</a>
		</div>
	{:else if cert}
		<!-- Certificate Card -->
		<div class="overflow-hidden rounded-xl border shadow-lg" style="border-color: var(--color-border); background-color: var(--color-background)">
			<!-- Header with gradient -->
			<div class="relative px-8 py-10 text-center" style="background: linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 10%, transparent), color-mix(in srgb, var(--color-primary) 5%, transparent))">
				<Award class="mx-auto mb-4 h-16 w-16" style="color: var(--color-primary)" />
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Certificate of Completion</h1>
				<p class="mt-2 text-lg" style="color: var(--color-muted)">{cert.track_title}</p>
			</div>

			<!-- Certificate body -->
			<div class="px-8 py-8 text-center">
				<p class="text-sm" style="color: var(--color-muted)">This certifies that</p>
				<p class="my-3 text-2xl font-bold" style="color: var(--color-foreground)">{cert.username ?? 'Student'}</p>
				<p class="text-sm" style="color: var(--color-muted)">has successfully completed the learning track</p>
				<p class="my-3 text-xl font-semibold" style="color: var(--color-primary)">{cert.track_title ?? 'Learning Track'}</p>

				<div class="my-6 flex items-center justify-center gap-8 text-sm" style="color: var(--color-muted)">
					<div>
						<p class="font-medium" style="color: var(--color-foreground)">Issued</p>
						<p>{formatDate(cert.issued_at)}</p>
					</div>
					<div class="h-8 w-px" style="background-color: var(--color-border)"></div>
					<div>
						<p class="font-medium" style="color: var(--color-foreground)">Certificate No.</p>
						<p class="font-mono">{cert.certificate_number}</p>
					</div>
				</div>

				<div class="flex items-center justify-center gap-2 text-sm" style="color: var(--color-success)">
					<CheckCircle class="h-4 w-4" />
					Verified
				</div>
			</div>

			<!-- Actions -->
			<div class="flex flex-col items-center gap-4 border-t px-8 py-4" style="border-color: var(--color-border)">
				<div class="flex items-center gap-4">
					<button
						class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
						disabled={downloading}
						onclick={handleDownload}
					>
						{#if downloading}
							<Loader2 class="h-4 w-4 animate-spin" />
							Loading...
						{:else}
							<Download class="h-4 w-4" />
							Download PDF
						{/if}
					</button>
					<button
						class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
						style="border-color: var(--color-border); color: var(--color-foreground)"
						onclick={handleShare}
					>
						<Share2 class="h-4 w-4" />
						Share
					</button>
				</div>
				{#if downloadMessage}
					<p class="text-sm" style="color: var(--color-muted)">{downloadMessage}</p>
				{/if}
			</div>
		</div>
	{/if}
</main>
