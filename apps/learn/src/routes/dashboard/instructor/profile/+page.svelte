<script lang="ts">
	// Instructor profile edit — Japa University creator economy (ADR-063).
	// CSR ONLY: GET /learn/university/instructor/me, PATCH /learn/university/instructor/me.
	// PATCH payload: display_name?, headline?, bio?, social_links?, payout_method?
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { PenLine, Save, ArrowLeft, AtSign, Share2, Globe, Play, Link } from 'lucide-svelte';
	import type { InstructorProfile, CatalogOption } from '$lib/types';

	let profile = $state<InstructorProfile | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let success = $state<string | null>(null);
	let error = $state<string | null>(null);

	// Form fields
	let displayName = $state('');
	let headline = $state('');
	let bio = $state('');
	let socialLinks = $state<Record<string, string>>({});
	let socialPlatforms = $state<CatalogOption[]>([]);

	// UI metadata for known social platforms (presentation layer, not the option list).
	const socialIconMap: Record<string, typeof Link> = {
		twitter: AtSign,
		linkedin: Share2,
		website: Globe,
		youtube: Play
	};

	const socialLabelMap: Record<string, string> = {
		twitter: 'university.social_twitter_label',
		linkedin: 'university.social_linkedin_label',
		website: 'university.social_website_label',
		youtube: 'university.social_youtube_label'
	};

	// Rule 16: social platform list comes from the backend, not a hardcoded array.
	// Combine backend platforms with any extra keys already stored on the profile.
	const socialFields = $derived(
		(() => {
			const keys = new Set(socialPlatforms.map((p) => p.value));
			for (const k of Object.keys(socialLinks)) {
				if (socialLinks[k]?.trim()) keys.add(k);
			}
			return [...keys].map((key) => ({
				key,
				label: t(socialLabelMap[key]) || socialPlatforms.find((p) => p.value === key)?.label || key,
				icon: socialIconMap[key] || Link
			}));
		})()
	);

	onMount(async () => {
		const [profRes, optsRes] = await Promise.all([
			api.get<InstructorProfile>('/learn/university/instructor/me'),
			api.get<{ difficulty_levels: CatalogOption[]; social_platforms: CatalogOption[] }>(
				'/learn/university/catalog/options'
			)
		]);
		loading = false;

		if (optsRes.data) {
			socialPlatforms = optsRes.data.social_platforms ?? [];
		}

		if (profRes.data) {
			profile = profRes.data;
			displayName = profRes.data.display_name ?? '';
			headline = profRes.data.headline ?? '';
			bio = profRes.data.bio ?? '';
			const links = { ...(profRes.data.social_links ?? {}) };
			for (const p of socialPlatforms) {
				if (!(p.value in links)) links[p.value] = '';
			}
			socialLinks = links;
		}
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		saving = true;
		success = null;
		error = null;

		const social_links: Record<string, string> = {};
		for (const [key, value] of Object.entries(socialLinks)) {
			if (value.trim()) social_links[key] = value.trim();
		}

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
					placeholder={t('university.display_name_placeholder', 'Your public instructor name')}
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
					placeholder={t('university.headline_placeholder', 'e.g. Senior Go Engineer & Course Creator')}
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
					placeholder={t('university.bio_placeholder', 'Tell learners about your teaching experience and expertise…')}
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
							bind:value={socialLinks[field.key]}
							placeholder={t('university.social_url_placeholder', 'https://')}
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
