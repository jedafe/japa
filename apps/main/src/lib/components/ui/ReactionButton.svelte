<script lang="ts">
	import { Popover } from 'bits-ui';
	import { Heart } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { requireAuthOrRedirect } from '$lib/auth/guest';

	interface ReactionType {
		key: string;
		emoji: string;
		label: string;
		enabled: boolean;
		position: number;
	}

	interface ReactionCount {
		reaction_key: string;
		count: number;
	}

	interface UserReaction {
		reaction_key: string;
	}

	interface Props {
		contentId?: string;
		count?: number;
		authed?: boolean;
		class?: string;
		size?: number;
		side?: 'top' | 'bottom' | 'left' | 'right';
		align?: 'start' | 'center' | 'end';
		ariaLabel?: string;
		/** Called when the heart is toggled and there is no content reaction backend. */
		onToggle?: (next: boolean) => void;
	}

	let {
		contentId,
		count = 0,
		authed = page.data.isAuthed,
		class: classProp,
		size = 16,
		side = 'top',
		align = 'center',
		ariaLabel,
		onToggle
	}: Props = $props();

	const features = $derived((page.data.features as Record<string, boolean> | undefined) ?? {});
	const reactionsEnabled = $derived(contentId ? (features['feature.reactions'] ?? true) : false);

	let reactionTypes = $state<ReactionType[]>([]);
	let activeReactions = $state<Set<string>>(new Set());
	let reactionCounts = $state<Record<string, number>>({});
	let reactionPending = $state<string | null>(null);
	let showReactions = $state(false);
	let simpleActive = $state(false);

	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		(async () => {
			const res = await api.get<ReactionType[]>('/reaction-types');
			if (res.data) {
				reactionTypes = res.data
					.filter((r) => r.enabled)
					.sort((a, b) => a.position - b.position);
			}
		})();
	});

	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		(async () => {
			const res = await api.get<ReactionCount[]>(`/content/${contentId}/reactions`);
			if (res.data) {
				const next: Record<string, number> = {};
				for (const c of res.data) {
					next[c.reaction_key] = c.count;
				}
				reactionCounts = next;
			}
		})();
	});

	$effect(() => {
		if (!authed || !reactionsEnabled || !contentId) return;
		(async () => {
			const res = await api.get<UserReaction[]>(`/content/${contentId}/reactions/me`);
			if (res.data) {
				activeReactions = new Set(res.data.map((r) => r.reaction_key));
			}
		})();
	});

	const primaryReaction = $derived(
		reactionTypes.find((r) => r.key === 'like' || r.key === 'love' || r.key === 'heart') ??
			reactionTypes[0] ?? { key: 'love', emoji: '❤️', label: t('sources.love', 'Love'), enabled: true, position: 0 }
	);
	const primaryKey = $derived(primaryReaction.key);
	const primaryActive = $derived(activeReactions.has(primaryKey));
	const totalReactions = $derived(
		reactionsEnabled ? reactionTypes.reduce((sum, r) => sum + (reactionCounts[r.key] ?? 0), 0) : count
	);

	function countFor(key: string): number {
		return reactionCounts[key] ?? 0;
	}

	async function toggleReaction(reactionKey: string) {
		if (!contentId) {
			const next = !simpleActive;
			simpleActive = next;
			onToggle?.(next);
			return;
		}
		if (reactionPending) return;
		if (!(await requireAuthOrRedirect())) return;

		reactionPending = reactionKey;
		const wasActive = activeReactions.has(reactionKey);

		const next = new Set(activeReactions);
		if (wasActive) {
			next.delete(reactionKey);
		} else {
			next.add(reactionKey);
		}
		activeReactions = next;
		reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + (wasActive ? -1 : 1);

		try {
			if (wasActive) {
				const res = await api.delete(`/content/${contentId}/react?key=${encodeURIComponent(reactionKey)}`);
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.post(`/content/${contentId}/react`, { reaction_key: reactionKey });
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			const rollback = new Set(activeReactions);
			if (wasActive) {
				rollback.add(reactionKey);
			} else {
				rollback.delete(reactionKey);
			}
			activeReactions = rollback;
			reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + (wasActive ? 1 : -1);
		} finally {
			reactionPending = null;
		}
	}

	function formatCount(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
		return String(n);
	}

	const triggerActive = $derived(reactionsEnabled ? primaryActive : simpleActive);
	const triggerIconFill = $derived(triggerActive ? 'currentColor' : 'none');
	const baseClass = $derived(
		classProp ??
			'inline-flex h-9 items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]'
	);
	const triggerClass = $derived(baseClass + (triggerActive ? ' text-[var(--color-primary)]' : ''));
	const simpleClass = $derived(baseClass + (simpleActive ? ' text-[var(--color-primary)]' : ''));
</script>

{#if reactionsEnabled}
	<Popover.Root bind:open={showReactions}>
		<Popover.Trigger
			type="button"
			openOnHover
			openDelay={100}
			closeDelay={200}
			onclick={() => toggleReaction(primaryKey)}
			class={triggerClass}
			aria-label={ariaLabel ?? primaryReaction.label}
			aria-pressed={triggerActive}
			disabled={!!reactionPending}
		>
			<Heart {size} fill={triggerIconFill} aria-hidden="true" />
			<span class="min-w-[1.25ch] tabular-nums">{formatCount(totalReactions)}</span>
		</Popover.Trigger>

		<Popover.Portal>
			<Popover.Content
				{side}
				{align}
				sideOffset={8}
				class="z-50 w-56 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg"
			>
				<div class="mb-1 px-2 py-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
					{t('blog.add_reaction', 'Add reaction')}
				</div>
				<div class="grid grid-cols-2 gap-1">
					{#each reactionTypes as r (r.key)}
						{@const active = activeReactions.has(r.key)}
						{@const cnt = countFor(r.key)}
						<button
							type="button"
							onclick={() => toggleReaction(r.key)}
							class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							aria-label={r.label}
							aria-pressed={active}
							disabled={!!reactionPending}
						>
							<span class="text-lg">{r.emoji}</span>
							<span class="flex-1 truncate text-left">{r.label}</span>
							<span class="text-xs text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">
								{cnt > 0 ? cnt : ''}
							</span>
						</button>
					{/each}
				</div>
			</Popover.Content>
		</Popover.Portal>
	</Popover.Root>
{:else}
	<button
		type="button"
		onclick={() => onToggle?.(!simpleActive)}
		class={simpleClass}
		aria-label={ariaLabel ?? t('sources.love', 'Love')}
		aria-pressed={simpleActive}
	>
		<Heart {size} fill={simpleActive ? 'currentColor' : 'none'} aria-hidden="true" />
		<span class="min-w-[1.25ch] tabular-nums">{formatCount(count)}</span>
	</button>
{/if}
