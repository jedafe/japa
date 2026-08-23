<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Users, FileText, ScrollText, LogIn } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import AchievementWidget from '$lib/components/spaces/AchievementWidget.svelte';
	import SpaceFollowListModal from '$lib/components/spaces/SpaceFollowListModal.svelte';
	import type { Space, SpaceRule } from '$lib/types';

	interface Props {
		space: Space;
		rules: SpaceRule[];
		authed: boolean;
		/** Imgproxy-signed logo URL (from GET /spaces/{slug} logo_url field). */
		logoUrl?: string;
		/** Whether the caller is already a member (hydrated server-side from
		 * the space members list). Best-effort for spaces with >20 members. */
		initialJoined?: boolean;
		/** Bindable follow state — the sidebar sets this to true when auto-
		 * following on join, so the parent page's header Follow button stays
		 * in sync. */
		following?: boolean;
	}
	let { space, rules, authed, logoUrl = '', initialJoined = false, following = $bindable(false) }: Props = $props();

	// Wiki content: the API stores it as base64-encoded JSON with
	// {title, extract, description, thumbnail, content_url}. Decode and
	// extract the text for the sidebar about widget.
	const wikiExtract = $derived.by(() => {
		if (!space.wiki_content) return null;
		try {
			const decoded = typeof space.wiki_content === 'string'
				? decodeBase64Text(space.wiki_content)
				: String(space.wiki_content);
			try {
				const json = JSON.parse(decoded);
				return json?.extract ?? null;
			} catch {
				// Not JSON — treat as raw HTML, strip tags for sidebar preview.
				return decoded.replace(/<[^>]*>/g, '').trim() || null;
			}
		} catch {
			return null;
		}
	});

	// Join state is hydrated from the server (initialJoined) so the button
	// reflects the caller's actual membership on SSR load instead of always
	// defaulting to "Join" — which previously caused the toggle to LEAVE when
	// the user was already a member (e.g. auto-joined via follow, BL-010).
	let joined = $state(untrack(() => initialJoined));
	// pendingJoin is set when the space is request-only and the caller's join
	// request is awaiting admin approval (POST /join returns 202 { pending: true }).
	let pendingJoin = $state(false);
	let pending = $state(untrack(() => false));
	let joinError = $state('');
	let followersOpen = $state(false);

	// Guest "Join" link — sends the guest to /login with a returnTo so they
	// land back on this space after authenticating.
	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	async function toggleJoin() {
		if (!authed || pending) return;
		const was = joined;
		joined = !was;
		pending = true;
		joinError = '';
		if (was) {
			// Already a member → leave.
			const res = await api.delete<{ left: boolean }>(`/spaces/${space.slug}/leave`);
			if (res.error) {
				joined = was; // rollback
				joinError = res.error.message || 'Failed to leave space. Please try again.';
			} else {
				joined = false;
				pendingJoin = false;
			}
		} else {
			// Not a member → join (or submit a pending request for request-only spaces).
			const res = await api.post<{ joined: boolean; pending?: boolean }>(`/spaces/${space.slug}/join`);
			if (res.error) {
				joined = was; // rollback
				joinError = res.error.message || 'Failed to join space. Please try again.';
			} else if (res.data) {
				// Authoritative state from the server: joined may be false with
				// pending=true for request-only spaces (202 response).
				joined = !!res.data.joined;
				pendingJoin = !!res.data.pending && !res.data.joined;

				// Auto-follow on a successful join (best-effort — a follow
				// failure does not roll back the join). Mirrors the
				// SpaceJoinButton behavior on the /spaces list page.
				// Sets the bindable `following` prop so the parent page's
				// header Follow button immediately reflects "Following".
				if (joined && !res.data.pending) {
					following = true;
					void api.post<{ following: boolean }>(`/spaces/${space.slug}/follow`);
				}
			}
		}
		pending = false;
	}
</script>

<div class="space-y-4">
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-start gap-3">
			{#if logoUrl}
				<img
					src={logoUrl}
					alt={space.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-base font-medium text-[var(--color-text-subtle)]"
				>
					{space.name.charAt(0).toUpperCase()}
				</span>
			{/if}
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
				<dd class="text-sm font-medium text-[var(--color-text)]">
				<button
					type="button"
					onclick={() => (followersOpen = true)}
					class="font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
				>
					{space.follower_count}
				</button>
			</dd>
			</div>
		</dl>

		{#if authed}
			{#if joinError}
				<p class="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{joinError}</p>
			{/if}
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
				{:else if pendingJoin}
					<LogIn size={16} /> {t('space.pending', 'Pending')}
				{:else}
					<LogIn size={16} /> {t('space.join', 'Join')}
				{/if}
			</button>
		{:else}
			<a
				href={loginHref}
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
		{:else if wikiExtract}
			<p class="text-sm leading-relaxed text-[var(--color-text-subtle)] line-clamp-4">{wikiExtract}</p>
		{:else}
			<p class="text-sm text-[var(--color-muted)]">{t('space.no_description', 'No description yet.')}</p>
		{/if}
		{#if wikiExtract && space.description}
			<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)] line-clamp-4">{wikiExtract}</p>
		{/if}
	</section>

	<!-- §89 Community Achievement Feed — sidebar widget showing latest
	     achievements filed against this Space (country). Auto-hides when
	     the feature is disabled or the space has no achievements. -->
	<AchievementWidget spaceSlug={space.slug} spaceId={space.id} />

	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<Users size={16} /> {t('space.new_members', 'New members')}
		</h2>
		<p class="text-xs text-[var(--color-muted)]">{t('space.member_count', '{n} members').replace('{n}', String(space.member_count))}</p>
	</section>

	<SpaceFollowListModal bind:open={followersOpen} spaceId={space.id} spaceName={space.name} count={space.follower_count} />
</div>
