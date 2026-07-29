<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Award, Download, ExternalLink } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const cert = $derived(data.cert);
	const recipient = $derived(data.recipient);

	const title = $derived(t('certificates.title', 'Certificate — JapaConnect'));
	const issuedDate = $derived(new Date(cert.issued_at).toLocaleDateString());
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={t('certificates.description', 'A verified certificate issued on JapaConnect.')} />
	<meta property="og:title" content={title} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="p-6">
		<!-- Certificate card -->
		<section class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
			<div class="border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-6 py-8 text-center">
				<span class="mx-auto grid h-16 w-16 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)]">
					<Award size={32} />
				</span>
				<h1 class="mt-4 text-xl font-semibold text-[var(--color-text)]">{t('certificates.heading', 'Certificate of completion')}</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('certificates.issued_to', 'Issued to')}</p>
				{#if recipient}
					<a href="/u/{recipient.username}" class="mt-2 inline-flex items-center gap-1 text-lg font-medium text-[var(--color-primary)] hover:underline">
						{recipient.display_name} <ExternalLink size={14} />
					</a>
				{:else}
					<p class="mt-2 text-lg font-medium text-[var(--color-text)]">{t('certificates.recipient', 'Recipient')}</p>
				{/if}
			</div>

			<dl class="grid grid-cols-2 gap-px bg-[var(--color-border)]">
				<div class="bg-[var(--color-surface)] p-4">
					<dt class="text-xs text-[var(--color-muted)]">{t('certificates.number', 'Certificate number')}</dt>
					<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">{cert.certificate_number}</dd>
				</div>
				<div class="bg-[var(--color-surface)] p-4">
					<dt class="text-xs text-[var(--color-muted)]">{t('certificates.issued_on', 'Issued on')}</dt>
					<dd class="mt-1 text-sm font-medium text-[var(--color-text)]">{issuedDate}</dd>
				</div>
			</dl>

			<div class="flex items-center justify-center gap-3 p-4">
				<a
					href="/api/v1/certificates/{cert.id}/download"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Download size={16} /> {t('certificates.download', 'Download')}
				</a>
				{#if recipient}
					<a
						href="/u/{recipient.username}"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('certificates.view_profile', 'View profile')}
					</a>
				{/if}
			</div>
		</section>
	</div>
</div>
