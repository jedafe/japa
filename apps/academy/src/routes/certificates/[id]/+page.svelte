<script lang="ts">
	// Public certificate share page — SSR (ADR-060 §4, shareable social proof).
	import { t } from '$lib/i18n';
	import { safeJsonLd } from '$lib/seo';
	import { api } from '$lib/api/client';
	import { Award, GraduationCap, CheckCircle, Download } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const cert = $derived(data.cert);

	let downloading = $state(false);
	let downloadError = $state('');

	async function downloadCertificate() {
		downloading = true;
		downloadError = '';
		try {
			const r = await api.get<{ download_url?: string; message?: string }>(`/certificates/${cert.id}/download`);
			if (r.error) {
				downloadError = r.error.message;
				return;
			}
			if (r.data?.download_url) {
				window.open(r.data.download_url, '_blank');
			} else if (r.data?.message) {
				downloadError = r.data.message;
			}
		} catch (e) {
			downloadError = e instanceof Error ? e.message : t('common.error');
		} finally {
			downloading = false;
		}
	}
</script>

<svelte:head>
	<title>{cert.recipient_name} — {cert.title} · {t('academy.certificates')}</title>
	<meta name="description" content={`Certificate of completion: ${cert.title}`} />
	<!-- Public share page (GET /certificates/{id} is unauthenticated social
	     proof) — indexable by design. No privacy flag exists on the backend
	     Certificate model; if one is added, gate this to noindex. -->
	<meta name="robots" content="index, follow" />
	<link rel="canonical" href={`https://academy.japaconnect.com/certificates/${cert.id}`} />
	<meta property="og:title" content={`${cert.recipient_name} — ${cert.title}`} />
	<meta property="og:description" content={`Certificate of completion: ${cert.title}`} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={`https://academy.japaconnect.com/certificates/${cert.id}`} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={`${cert.recipient_name} — ${cert.title}`} />
	<meta name="twitter:description" content={`Certificate of completion: ${cert.title}`} />
	{@html safeJsonLd({
			'@context': 'https://schema.org',
			'@type': 'EducationalOccupationalCredential',
			name: `Certificate of Completion: ${cert.title}`,
			credentialCategory: 'Course Completion',
			recognizedBy: {
				'@type': 'Organization',
				name: 'JapaConnect'
			},
			hasCredential: {
				'@type': 'EducationalOccupationalCredential',
				identifier: cert.certificate_number ?? cert.id
			}
		})}
</svelte:head>

<div class="flex min-h-[60vh] items-center justify-center px-4 py-12">
	<div class="w-full max-w-2xl rounded-[var(--radius-card)] border-2 border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center md:p-12">
		<div class="mx-auto mb-6 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)]">
			<Award size={32} class="text-[var(--level-intermediate)]" />
		</div>

		<p class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			{t('academy.certificate_of_completion')}
		</p>

		<p class="mt-4 text-lg text-[var(--color-text-subtle)]">{t('academy.this_certifies_that')}</p>
		<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{cert.recipient_name}</p>
		<p class="text-sm text-[var(--color-muted)]">@{cert.recipient_username}</p>

		<p class="mt-6 text-lg text-[var(--color-text-subtle)]">{t('academy.has_successfully_completed')}</p>
		<p class="mt-2 text-xl font-semibold text-[var(--color-text)]">{cert.title}</p>

		<div class="mt-6 flex items-center justify-center gap-2 text-sm text-[var(--color-muted)]">
			<GraduationCap size={16} />
			<span>{t('app.name')}</span>
		</div>

		<div class="mt-8 flex items-center justify-center gap-4 border-t border-[var(--color-border)] pt-6 text-sm text-[var(--color-muted)]">
			<div>
				<p class="text-xs">{t('academy.certificate_issued_label', 'Issued')}</p>
				<p class="font-medium text-[var(--color-text)]">{new Date(cert.issued_at).toLocaleDateString()}</p>
			</div>
			{#if cert.certificate_number}
				<div>
					<p class="text-xs">{t('academy.certificate_number_label', 'Certificate #')}</p>
					<p class="font-medium text-[var(--color-text)]">{cert.certificate_number}</p>
				</div>
			{/if}
		</div>

		<div class="mt-6 flex flex-wrap items-center justify-center gap-3">
			<a
				href="/tracks/{cert.track_slug}"
				class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<CheckCircle size={16} class="text-[var(--color-progress)]" />
				{t('academy.view_course', 'View course')}
			</a>
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				onclick={downloadCertificate}
				disabled={downloading}
			>
				<Download size={16} />
				{downloading ? t('common.loading', 'Loading…') : t('academy.download_certificate', 'Download')}
			</button>
		</div>

		{#if downloadError}
			<p class="mt-3 text-sm text-[var(--color-muted)]">{downloadError}</p>
		{/if}
	</div>
</div>
