<script lang="ts">
	// Instructor profile edit — Japa University creator economy (ADR-063).
	// CSR ONLY: GET /learn/university/instructor/me, PATCH /learn/university/instructor/me.
	// PATCH payload: display_name?, headline?, bio?, social_links?, payout_method?
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { PenLine, Save, ArrowLeft, AtSign, Share2, Globe, Play } from 'lucide-svelte';
	import type { InstructorProfile } from '$lib/types';

	let profile = $state<InstructorProfile | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let success = $state<string | null>(null);
	let error = $state<string | null>(null);

	// Form fields
	let displayName = $state('');
	let headline = $state('');
	let bio = $state('');
	let socialTwitter = $state('');
	let socialLinkedin = $state('');
	let socialWebsite = $state('');
	let socialYoutube = $state('');

	const socialFields = $derived([
		{ key: 'twitter', label: t('university.social_twitter_label'), icon: AtSign, value: socialTwitter },
		{ key: 'linkedin', label: t('university.social_linkedin_label'), icon: Share2, value: socialLinkedin },
		{ key: 'website', label: t('university.social_website_label'), icon: Globe, value: socialWebsite },
		{ key: 'youtube', label: t('university.social_youtube_label'), icon: Play, value: socialYoutube }
	]);

	$effect(() => {
		(async () => {
			const r = await api.get<InstructorProfile>('/learn/university/instructor/me');
			loading = false;
			if (r.data) {
				profile = r.data;
				displayName = r.data.display_name ?? '';
				headline = r.data.headline ?? '';
				bio = r.data.bio ?? '';
				const links = r.data.social_links ?? {};
				socialTwitter = links.twitter ?? '';
				socialLinkedin = links.linkedin ?? '';
				socialWebsite = links.website ?? '';
				socialYoutube = links.youtube ?? '';
			}
		})();
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		saving = true;
		success = null;
		error = null;
		const social_links: Record<string, string> = {};
		if (socialTwitter) social_links.twitter = socialTwitter;
		if (socialLinkedin) social_links.linkedin = socialLinkedin;
		if (socialWebsite) social_links.website = socialWebsite;
		if (socialYoutube) social_links.youtube = socialYoutube;
		const r = await api.patch<InstructorProfile>('/learn/university/instructor/me', {
			display_name: displayName || undefined,
			headline: headline || undefined,
			bio: bio || undefined,
			social_links: Object.keys(social_links).length > 0 ? social_links : undefined
		});
		saving = false;
		if (r.error) {
			error = r.error.message || t('university.profile_save_failed');
		} else if (r.data) {
			profile = r.data;
			success = t('university.profile_saved');
		}
	}
</script>

<svelte:head>
	<title>{t('university.edit_profile')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<a
			href="/dashboard/instructor"
			class="mb-3 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('university.back_to_dashboard')}
		</a>
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<PenLine size={24} class="text-[var(--color-primary)]" />
			{t('university.edit_profile')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('university.edit_profile_description')}
		</p>
	</header>

	{#if loading}
		<div class="mx-auto max-w-2xl space-y-4">
			{#each Array(4) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-2 h-4 w-32" />
					<LoadingSkeleton class="h-10 w-full" />
				</div>
			{/each}
		</div>
	{:else if !profile || profile.status !== 'approved'}
		<EmptyState
			title={t('university.not_approved_title')}
			description={t('university.not_approved_description')}
		>
			{#snippet action()}
				<a
					href="/dashboard/instructor/apply"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				>
					{t('university.apply')}
				</a>
			{/snippet}
		</EmptyState>
	{:else}
		<form onsubmit={save} class="mx-auto max-w-2xl space-y-5">
			<!-- Display Name -->
			<div>
				<label for="display_name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.display_name_label')}
				</label>
				<input
					id="display_name"
					type="text"
					bind:value={displayName}
					placeholder="Your public instructor name"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Headline -->
			<div>
				<label for="headline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.headline_label')}
				</label>
				<input
					id="headline"
					type="text"
					bind:value={headline}
					placeholder="e.g. Senior Go Engineer & Course Creator"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Bio -->
			<div>
				<label for="bio" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.bio_label')}
				</label>
				<textarea
					id="bio"
					bind:value={bio}
					rows="5"
					placeholder="Tell learners about your teaching experience and expertise…"
					class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				></textarea>
			</div>

			<!-- Social Links -->
			<fieldset class="space-y-3">
				<legend class="mb-2 block text-sm font-medium text-[var(--color-text)]">
					{t('university.social_links_label')}
				</legend>
				{#each socialFields as field (field.key)}
					{@const Icon = field.icon}
					<div>
						<label for="social_{field.key}" class="mb-1 flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)]">
							<Icon size={14} class="text-[var(--color-muted)]" />
							{field.label}
						</label>
						<input
							id="social_{field.key}"
							type="url"
							value={field.value}
							oninput={(e) => {
								const v = e.currentTarget.value;
								if (field.key === 'twitter') socialTwitter = v;
								else if (field.key === 'linkedin') socialLinkedin = v;
								else if (field.key === 'website') socialWebsite = v;
								else if (field.key === 'youtube') socialYoutube = v;
							}}
							placeholder="https://"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
				{/each}
			</fieldset>

			{#if success}
				<p class="text-sm text-[var(--color-progress)]">{success}</p>
			{/if}
			{#if error}
				<p class="text-sm text-[var(--color-destructive)]">{error}</p>
			{/if}

			<button
				type="submit"
				disabled={saving}
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
			>
				<Save size={16} />
				{saving ? t('university.saving') : t('university.save')}
			</button>
		</form>
	{/if}
</div>
