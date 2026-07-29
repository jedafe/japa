<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { Users, FileText, ScrollText, LogIn } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';
	import type { Space, SpaceRule } from '$lib/types';

	interface Props {
		space: Space;
		rules: SpaceRule[];
		authed: boolean;
	}
	let { space, rules, authed }: Props = $props();

	// Join/follow state is seeded from the space's membership counts; the
	// backend does not return the caller's join/follow flag on the space
	// detail endpoint, so we start neutral and let the toggle reflect the
	// optimistic action.
	let joined = $state(false);
	let pending = $state(untrack(() => false));

	async function toggleJoin() {
		if (!authed || pending) return;
		const was = joined;
		joined = !was;
		pending = true;
		const res = was
			? await api.delete(`/spaces/${space.slug}/leave`)
			: await api.post(`/spaces/${space.slug}/join`);
		if (res.error) joined = was; // rollback
		pending = false;
	}
</script>

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="flex items-start gap-3">
		<span
			class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-base font-medium text-[var(--color-text-subtle)]"
		>
			{space.name.charAt(0).toUpperCase()}
		</span>
		<div class="min-w-0 flex-1">
			<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">{space.name}</h2>
			<p class="text-xs capitalize text-[var(--color-muted)]">{space.level} space</p>
		</div>
	</div>

	<dl class="mt-4 grid grid-cols-3 gap-2 text-center">
		<div>
			<dt class="text-xs text-[var(--color-muted)]">{t('space.members', 'Members')}</dt>
			<dd class="text-sm font-medium text-[var(--color-text)]">{space.member_count}</dd>
		</div>
		<div>
			<dt class="text-xs text-[var(--color-muted)]">{t('space.posts', 'Posts')}</dt>
			<dd class="text-sm font-medium text-[var(--color-text)]">{space.post_count}</dd>
		</div>
		<div>
			<dt class="text-xs text-[var(--color-muted)]">{t('space.followers', 'Followers')}</dt>
			<dd class="text-sm font-medium text-[var(--color-text)]">{space.follower_count}</dd>
		</div>
	</dl>

	{#if authed}
		<button
			type="button"
			onclick={toggleJoin}
			disabled={pending}
			aria-busy={pending}
			class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50 {joined
				? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
				: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
		>
			{#if joined}
				<LogIn size={16} /> {t('space.joined', 'Joined')}
			{:else}
				<LogIn size={16} /> {t('space.join', 'Join')}
			{/if}
		</button>
		<div class="mt-2">
			<FollowButton entityType="space" entityId={space.id} entitySlug={space.slug} compact={false} />
		</div>
	{:else}
		<a
			href="/login"
			class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<LogIn size={16} /> {t('space.join', 'Join')}
		</a>
	{/if}
</section>

{#if rules.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<ScrollText size={16} /> {t('space.rules', 'Rules')}
		</h2>
		<ol class="space-y-3">
			{#each rules as rule, i (rule.id)}
				<li class="text-sm">
					<p class="font-medium text-[var(--color-text)]">{i + 1}. {rule.title}</p>
					{#if rule.body}
						<p class="mt-0.5 text-[var(--color-text-subtle)]">{rule.body}</p>
					{/if}
				</li>
			{/each}
		</ol>
	</section>
{/if}

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
		<FileText size={16} /> {t('space.about', 'About')}
	</h2>
	{#if space.description}
		<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">{space.description}</p>
	{:else}
		<p class="text-sm text-[var(--color-muted)]">{t('space.no_description', 'No description yet.')}</p>
	{/if}
	{#if space.wiki_content}
		<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">{space.wiki_content}</p>
	{/if}
</section>

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
		<Users size={16} /> {t('space.new_members', 'New members')}
	</h2>
	<p class="text-xs text-[var(--color-muted)]">{t('space.member_count', '{n} members').replace('{n}', String(space.member_count))}</p>
</section>
