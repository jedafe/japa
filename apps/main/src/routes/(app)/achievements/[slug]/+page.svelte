<script lang="ts">
	// Ref: japaconnect-engineering/sections/89-achievement-feed.md
	// Achievement detail page. Renders the full milestone post with metadata,
	// author info (or anonymous display), and Edit/Delete affordances for the
	// author. Mutations go through the browser API client (PATCH/DELETE).
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { api } from '$lib/api/client';
	import {
		ArrowLeft,
		Trophy,
		Plane,
		Briefcase,
		GraduationCap,
		MapPin,
		Cake,
		BadgeCheck,
		Sparkles,
		Pencil,
		Trash2,
		Loader2,
		Heart,
		MessageCircle,
		Eye
	} from 'lucide-svelte';
	import type { AchievementMilestoneType } from '$lib/types';
	import type { IconComponent } from '$lib/components/ui/icons';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const achievement = $derived(data.achievement);

	const title = $derived(`${achievement.title} — ${t('achievements.heading', 'Achievements')}`);
	const description = $derived(achievement.body.slice(0, 160));

	const milestoneIcons: Record<AchievementMilestoneType, IconComponent> = {
		visa_approved: BadgeCheck,
		job_offer: Briefcase,
		university_accepted: GraduationCap,
		arrived: Plane,
		one_year: Cake,
		citizenship: Trophy,
		other: Sparkles
	};

	function milestoneIcon(type: AchievementMilestoneType): IconComponent {
		return milestoneIcons[type] ?? Sparkles;
	}

	const milestoneLabels: Record<AchievementMilestoneType, string> = {
		visa_approved: t('achievements.type_visa_approved', 'Visa approved'),
		job_offer: t('achievements.type_job_offer', 'Job offer'),
		university_accepted: t('achievements.type_university_accepted', 'University accepted'),
		arrived: t('achievements.type_arrived', 'Arrived'),
		one_year: t('achievements.type_one_year', 'One year'),
		citizenship: t('achievements.type_citizenship', 'Citizenship'),
		other: t('achievements.type_other', 'Other')
	};

	const Icon = $derived(milestoneIcon(achievement.milestone_type as AchievementMilestoneType));
	const isAnon = $derived(achievement.is_anonymous);
	const createdDate = $derived(
		new Date(achievement.created_at).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		})
	);

	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

	async function deleteAchievement() {
		if (deleting || !data.canEdit) return;
		if (!confirm(t('achievements.confirm_delete', 'Delete this achievement? This cannot be undone.'))) return;
		deleting = true;
		deleteError = null;
		const res = await api.delete(`/achievements/${achievement.id}`);
		deleting = false;
		if (res.error) {
			deleteError = res.error.message || t('achievements.delete_error', 'Failed to delete achievement.');
			return;
		}
		goto('/achievements');
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={achievement.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<a
		href="/achievements"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={16} /> {t('achievements.back', 'Back to achievements')}
	</a>

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<!-- Milestone badge -->
		<div class="flex flex-wrap items-center gap-2">
			<span class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-3 py-1 text-sm font-semibold text-[var(--color-primary)]">
				<Icon size={15} />
				{milestoneLabels[achievement.milestone_type as AchievementMilestoneType]}
			</span>

		</div>

		<h1 class="mt-3 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
			{achievement.title}
		</h1>

		<!-- Author + metadata -->
		<div class="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-[var(--color-border)] pb-4 text-sm text-[var(--color-text-subtle)]">
			{#if isAnon}
				<span class="inline-flex items-center gap-1.5">
					<MapPin size={14} />
					{t('achievements.anonymous_from', 'Anonymous member from')}
					{achievement.origin_space_name ?? '?'} → {achievement.destination_space_name}
				</span>
			{:else}
				<span class="flex items-center gap-2">
					{#if achievement.author_avatar}
						<img src={achievement.author_avatar} alt="" class="h-8 w-8 rounded-full object-cover" />
					{:else}
						<span class="grid h-8 w-8 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">
							{(achievement.author_name ?? 'M').charAt(0)}
						</span>
					{/if}
					<span class="font-medium text-[var(--color-text)]">{achievement.author_name ?? t('achievements.member', 'Member')}</span>
				</span>
			{/if}
			<span>·</span>
			<span class="inline-flex items-center gap-1">
				<MapPin size={14} /> {achievement.destination_space_name}
			</span>
			{#if achievement.origin_space_name}
				<span>·</span>
				<span>{t('achievements.from', 'from')} {achievement.origin_space_name}</span>
			{/if}
			<span>·</span>
			<span>{createdDate}</span>
		</div>

		<!-- Engagement stats (reactions / comments / views) -->
		<div class="mt-3 flex flex-wrap items-center gap-4 text-xs text-[var(--color-text-subtle)]">
			<span class="inline-flex items-center gap-1" title={t('achievements.reactions', 'Reactions')}>
				<Heart size={14} class="text-[var(--color-primary)]" fill={achievement.reaction_count > 0 ? 'currentColor' : 'none'} />
				{achievement.reaction_count}
			</span>
			<span class="inline-flex items-center gap-1" title={t('achievements.comments', 'Comments')}>
				<MessageCircle size={14} />
				{achievement.comment_count}
			</span>
			<span class="inline-flex items-center gap-1" title={t('achievements.views', 'Views')}>
				<Eye size={14} />
				{achievement.view_count}
			</span>
		</div>

		<!-- Body -->
		<div class="prose mt-5 max-w-none whitespace-pre-wrap text-[var(--color-text)]">
			{achievement.body}
		</div>

		{#if deleteError}
			<p class="mt-4 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{deleteError}
			</p>
		{/if}

		<!-- Footer actions -->
		{#if data.canEdit}
			<div class="mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
				<a
					href={`/achievements/${achievement.slug}/edit`}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Pencil size={14} /> {t('common.edit', 'Edit')}
				</a>
				<button
					type="button"
					onclick={deleteAchievement}
					disabled={deleting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-red-500/30 px-3 py-1.5 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10 disabled:opacity-50"
				>
					{#if deleting}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<Trash2 size={14} />
					{/if}
					{t('common.delete', 'Delete')}
				</button>
			</div>
		{/if}
	</article>
</div>
