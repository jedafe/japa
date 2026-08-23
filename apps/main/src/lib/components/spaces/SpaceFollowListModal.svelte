<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { X, UserCircle } from 'lucide-svelte';

	// Ref: japaconnect-engineering/sections/31-follow-system.md
	// Modal that lazy-loads GET /spaces/{space_id}/followers when opened.

	interface FollowListItem {
		id: string;
		username: string;
		display_name: string;
		avatar_url?: string | null;
		bio?: string | null;
		is_following?: boolean;
	}

	interface Props {
		open: boolean;
		spaceId: string;
		spaceName?: string;
		count: number;
	}

	let { open = $bindable(), spaceId, spaceName, count }: Props = $props();

	let items = $state<FollowListItem[]>([]);
	let loading = $state(false);
	let loaded = $state(false);

	async function loadList() {
		if (loaded || loading) return;
		loading = true;
		try {
			const res = await api.get<FollowListItem[]>(`/spaces/${spaceId}/followers`);
			items = res.data ?? [];
			loaded = true;
		} catch {
			items = [];
			loaded = true;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (!open) {
			items = [];
			loaded = false;
			loading = false;
		} else {
			loadList();
		}
	});

	const initials = (name: string) => name?.charAt(0)?.toUpperCase() ?? '?';
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Content
			class="z-50 flex max-h-[80vh] w-full max-w-md flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg"
		>
			<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
				<Dialog.Title class="text-base font-medium text-[var(--color-text)]">
					{t('space.followers_label', `Followers of ${spaceName ?? ''}`)}
					<span class="ml-1.5 text-sm font-normal text-[var(--color-muted)]">{count}</span>
				</Dialog.Title>
				<Dialog.Close
					class="grid h-7 w-7 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<X size={16} />
				</Dialog.Close>
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
				{:else if items.length === 0}
					<div class="flex flex-col items-center gap-2 py-10 text-center">
						<UserCircle size={32} class="text-[var(--color-muted)]" />
						<p class="text-sm text-[var(--color-muted)]">
							{t('space.no_followers', 'No followers yet')}
						</p>
					</div>
				{:else}
					<ul class="space-y-0.5">
						{#each items as item (item.id)}
							<li>
								<a
									href="/u/{item.username}"
									class="flex items-center gap-3 rounded-[var(--radius)] p-2 transition-colors hover:bg-[var(--color-surface-subtle)]"
									onclick={() => (open = false)}
								>
									{#if item.avatar_url}
										<img
											src={item.avatar_url}
											alt={item.display_name}
											class="h-10 w-10 shrink-0 rounded-full object-cover"
											loading="lazy"
										/>
									{:else}
										<span
											class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]"
										>
											{initials(item.display_name)}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">
											{item.display_name}
										</span>
										<span class="block truncate text-xs text-[var(--color-muted)]">
											@{item.username}
										</span>
									</span>
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
