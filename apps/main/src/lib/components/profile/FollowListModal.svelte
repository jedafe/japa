<script lang="ts">
	import { Modal } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { X, UserCircle } from 'lucide-svelte';

	// Ref: japaconnect-engineering/sections/31-follow-system.md
	// A single component handles both the followers and following list modal.
	// Fetches the list client-side from /users/{userId}/followers or /following
	// when the dialog opens (lazy load — no SSR fetch for a modal).

	export interface FollowListItem {
		id: string;
		username: string;
		display_name: string;
		avatar_url?: string | null;
		bio?: string | null;
		/** Polymorphic entity type for following lists (followers are always 'user'). */
		entity_type?: string;
	}

	interface RawFollower {
		/** Follow row id — NOT the user id. Use follower_id for user-scoped actions. */
		id: string;
		follower_id?: string;
		username: string;
		display_name?: string;
		avatar_url?: string | null;
		bio?: string | null;
	}

	interface RawFollowing {
		entity_id: string;
		entity_slug: string;
		entity_name: string;
		entity_display_name?: string;
		entity_avatar_url?: string | null;
		bio?: string | null;
		is_following?: boolean;
		entity_type?: string;
	}

	interface Props {
		open: boolean;
		userId: string;
		type: 'followers' | 'following';
		count: number;
		initialItems?: FollowListItem[];
	}

	let { open = $bindable(), userId, type, count, initialItems = [] }: Props = $props();

	let items = $state<FollowListItem[]>([]);
	let loading = $state(false);
	let loaded = $state(false);
	let loadError = $state('');

	const titleKey = $derived(type === 'followers' ? 'profile.followers_label' : 'profile.following_label');
	// Mirrored from /u/[username]/+page.server.ts FOLLOW_LIST_PAGE_SIZE.
	// Pages are looped below so long lists (>100) are not truncated.
	const FOLLOW_LIST_PAGE_SIZE = 100;
	const MAX_LIST_PAGES = 5;

	function mapList(raw: unknown[], listType: 'followers' | 'following'): FollowListItem[] {
		if (listType === 'following') {
			return (raw as RawFollowing[]).map((f) => ({
				id: f.entity_id,
				username: f.entity_slug,
				display_name: f.entity_display_name || f.entity_name || f.entity_slug,
				avatar_url: f.entity_avatar_url,
				bio: f.bio,
				entity_type: f.entity_type
			}));
		}
		return (raw as RawFollower[]).map((f) => ({
			id: f.follower_id ?? f.id,
			username: f.username,
			display_name: f.display_name || f.username,
			avatar_url: f.avatar_url,
			bio: f.bio,
			entity_type: 'user'
		}));
	}

	function itemHref(item: FollowListItem): string {
		switch (item.entity_type) {
			case 'space':
				return `/spaces/${item.username}`;
			case 'tag':
				return `/tags/${item.username}`;
			case 'organization':
				return `/org/${item.username}`;
			case 'source':
				return `/sources/${item.username}`;
			default:
				return `/u/${item.username}`;
		}
	}

	function itemHandle(item: FollowListItem): string {
		switch (item.entity_type) {
			case 'space':
				return t('profile.type.space', 'Space');
			case 'tag':
				return t('profile.type.tag', 'Tag');
			case 'organization':
				return t('profile.type.organization', 'Organization');
			case 'source':
				return t('profile.type.source', 'Source');
			default:
				return `@${item.username}`;
		}
	}

	async function loadList() {
		if (loaded || loading) return;
		loading = true;
		try {
			const all: unknown[] = [];
			const seen = new Set<string>();
			for (let p = 0; p < MAX_LIST_PAGES; p++) {
				const res = await api.get<unknown[]>(
					`/users/${userId}/${type}?page_size=${FOLLOW_LIST_PAGE_SIZE}&page_offset=${p * FOLLOW_LIST_PAGE_SIZE}`
				);
				const pageItems = res.data ?? [];
				let added = 0;
				for (const item of pageItems) {
					const rec = item as { id?: string; entity_id?: string };
					const key = rec.id ?? rec.entity_id ?? JSON.stringify(item);
					if (!seen.has(key)) {
						seen.add(key);
						all.push(item);
						added++;
					}
				}
				if (added === 0 || pageItems.length < FOLLOW_LIST_PAGE_SIZE) break;
			}
			items = mapList(all, type);
			loaded = true;
		} catch (e) {
			items = [];
			loadError = e instanceof Error ? e.message : 'Something went wrong';
			loaded = true;
		} finally {
			loading = false;
		}
	}

	// Reset state when the modal closes so a re-open reuses the preloaded list.
	$effect(() => {
		if (!open) {
			items = [];
			loaded = false;
			loading = false;
			loadError = '';
		} else if (!loaded && !loading) {
			if (initialItems.length > 0 || count === 0) {
				items = initialItems;
				loaded = true;
			} else {
				loadList();
			}
		}
	});

	const initials = (name: string, fallback?: string) =>
		(name?.charAt(0) ?? fallback?.charAt(0) ?? '?').toUpperCase();
</script>

<Modal bind:open hideHeader ariaTitle={t(titleKey, 'Followers')} padded={false} size="md">
	{#snippet children(close)}
		<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
			<h2 class="text-base font-medium text-[var(--color-text)]">
				{t(titleKey, type === 'followers' ? 'Followers' : 'Following')}
				<span class="ms-1.5 text-sm font-normal text-[var(--color-muted)]">{count}</span>
			</h2>
			<button
				type="button"
				onclick={close}
				aria-label={t('common.close', 'Close')}
				class="grid h-7 w-7 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<X size={16} />
			</button>
		</div>

		<div class="flex-1 overflow-y-auto px-2 py-2">
				{#if loading}
					<div class="space-y-2 p-2">
						{#each Array(Math.min(count, 5)) as _, i (i)}
							<div class="flex items-center gap-3 rounded-[var(--radius)] p-2">
								<div class="h-10 w-10 shrink-0 animate-pulse rounded-full bg-[var(--color-surface-subtle)]"></div>
								<div class="flex-1 space-y-1.5">
									<div class="h-3 w-24 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
									<div class="h-2.5 w-16 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
								</div>
							</div>
						{/each}
					</div>
				{:else if loadError}
				<div class="px-4 py-8 text-center">
					<p class="text-sm text-[var(--color-text-muted)]">{loadError}</p>
					<button
						type="button"
						class="mt-3 rounded-[var(--radius-button)] border border-[var(--color-border)] px-3 py-1.5 text-sm hover:bg-[var(--color-surface-2)]"
						onclick={() => { loadError = ''; loaded = false; items = []; }}
					>
						{t('common.retry', 'Try again')}
					</button>
				</div>
			{:else if items.length === 0}
					<div class="flex flex-col items-center gap-2 py-10 text-center">
						<UserCircle size={32} class="text-[var(--color-muted)]" />
						<p class="text-sm text-[var(--color-muted)]">
							{t('profile.no_' + type, type === 'followers' ? 'No followers yet' : 'Not following anyone yet')}
						</p>
					</div>
				{:else}
					<ul class="space-y-0.5">
						{#each items as item (item.id)}
							<li>
								<a
									href={itemHref(item)}
									class="flex items-center gap-3 rounded-[var(--radius)] p-2 transition-colors hover:bg-[var(--color-surface-subtle)]"
									onclick={() => (open = false)}
								>
									{#if item.avatar_url}
										<img
											src={item.avatar_url}
											alt={item.display_name || item.username}
											class="h-10 w-10 shrink-0 rounded-full object-cover"
											loading="lazy"
										/>
									{:else}
										<span
											class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
										>
											{initials(item.display_name, item.username)}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">
											{item.display_name || item.username}
										</span>
										<span class="block truncate text-xs text-[var(--color-muted)]">
											{itemHandle(item)}
										</span>
									</span>
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
	{/snippet}
</Modal>
