<script lang="ts">
	// Public certificate share page — SSR (ADR-060 §4, shareable social proof).
	import { t } from '$lib/i18n';
	import { Award, GraduationCap, CheckCircle } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const cert = $derived(data.cert);
</script>

<svelte:head>
	<title>{cert.recipient_name} — {cert.track_title} · {t('learn.certificates')}</title>
	<meta name="description" content={`Certificate of completion: ${cert.track_title}`} />
	<link rel="canonical" href={`https://learn.japaconnect.com/certificates/${cert.id}`} />
	<script type="application/ld+json">
		{JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'EducationalOccupationalCredential',
			name: `Certificate of Completion: ${cert.track_title}`,
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
	</script>
</svelte:head>

<div class="flex min-h-[60vh] items-center justify-center px-4 py-12">
	<div class="w-full max-w-2xl rounded-[var(--radius-card)] border-2 border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center md:p-12">
		<div class="mx-auto mb-6 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)]">
			<Award size={32} class="text-[var(--level-intermediate)]" />
		</div>

		<p class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			Certificate of Completion
		</p>

		<p class="mt-4 text-lg text-[var(--color-text-subtle)]">This certifies that</p>
		<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{cert.recipient_name}</p>
		<p class="text-sm text-[var(--color-muted)]">@{cert.recipient_username}</p>

		<p class="mt-6 text-lg text-[var(--color-text-subtle)]">has successfully completed</p>
		<p class="mt-2 text-xl font-semibold text-[var(--color-text)]">{cert.track_title}</p>

		<div class="mt-6 flex items-center justify-center gap-2 text-sm text-[var(--color-muted)]">
			<GraduationCap size={16} />
			<span>JapaConnect Learn</span>
		</div>

		<div class="mt-8 flex items-center justify-center gap-4 border-t border-[var(--color-border)] pt-6 text-sm text-[var(--color-muted)]">
			<div>
				<p class="text-xs">Issued</p>
				<p class="font-medium text-[var(--color-text)]">{new Date(cert.issued_at).toLocaleDateString()}</p>
			</div>
			{#if cert.certificate_number}
				<div>
					<p class="text-xs">Certificate #</p>
					<p class="font-medium text-[var(--color-text)]">{cert.certificate_number}</p>
				</div>
			{/if}
		</div>

		<a
			href="/tracks/{cert.track_slug}"
			class="mt-6 inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			<CheckCircle size={16} class="text-[var(--color-progress)]" />
			View course
		</a>
	</div>
</div>
