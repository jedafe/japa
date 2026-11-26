<script lang="ts">
	// One row in the Communications Hub stream — avatar (+presence), title +
	// verified badge + timestamp, preview, unread count, status pill and the
	// kind chip (Direct/Services/Mentorship/Support/AI).
	import { t } from '#lib/i18n/index.js';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import RelativeTime from '#lib/components/ui/RelativeTime.svelte';
	import { Headset, Sparkles, Users, Building2 } from 'lucide-svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import { realtime } from '#lib/realtime/realtime.svelte.js';
	import type { HubItem } from './hub.js';

	interface Props {
		item: HubItem;
		active: boolean;
		onSelect: (item: HubItem) => void;
	}

	let { item, active, onSelect }: Props = $props();

	const chipLabel = $derived(
		{
			direct: t('hub.chip_direct', 'Direct'),
			services: t('hub.chip_services', 'Services'),
			mentorship: t('hub.chip_mentorship', 'Mentorship'),
			support: t('hub.chip_support', 'Support'),
			ai: t('hub.chip_ai', 'AI')
		}[item.chip]
	);

	const online = $derived(
		item.onlineUserId !== '' && realtime.onlineUserIds.has(item.onlineUserId)
	);

	const toneClass = $derived(
		{
			info: 'bg-[var(--color-info)]/10 text-[var(--color-info)]',
			success: 'bg-[var(--color-success)]/10 text-[var(--color-success)]',
			warning: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]',
			danger: 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]',
			primary: 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]',
			muted: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'
		}[item.statusTone]
	);
</script>

<li>
	<button
		type="button"
		onclick={() => onSelect(item)}
		aria-current={active ? 'true' : undefined}
		class="flex w-full items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 text-start transition-colors {active
			? 'bg-[var(--color-surface-subtle)]'
			: 'hover:bg-[var(--color-surface-subtle)]'}"
	>
		<span class="relative shrink-0">
			{#if item.avatarUrl}
				<img
					src={remoteImageUrl(item.avatarUrl)}
					alt=""
					class="h-10 w-10 rounded-full object-cover"
				/>
			{:else}
				<span
					class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
				>
					{#if item.avatarIcon === 'headset'}
						<Headset size={17} aria-hidden="true" />
					{:else if item.avatarIcon === 'sparkles'}
						<Sparkles size={17} aria-hidden="true" />
					{:else if item.avatarIcon === 'org'}
						<Building2 size={17} aria-hidden="true" />
					{:else if item.avatarIcon === 'users'}
						<Users size={17} aria-hidden="true" />
					{:else}
						{item.title ? item.title.charAt(0).toUpperCase() : ''}
					{/if}
				</span>
			{/if}
			{#if online}
				<span
					class="absolute bottom-0 end-0 h-3 w-3 rounded-full border-2 border-[var(--color-surface)] bg-[var(--color-success)]"
					aria-hidden="true"
				></span>
			{/if}
		</span>

		<span class="min-w-0 flex-1">
			<span class="flex items-center justify-between gap-2">
				<span class="inline-flex min-w-0 items-center gap-1 text-sm font-medium text-[var(--color-text)]">
					<span class="truncate">{item.title}</span>
					<VerifiedBadge
						verified={item.verified}
						type={item.verificationType}
						scope={item.verifiedScope}
						size={13}
					/>
				</span>
				<span class="flex shrink-0 items-center gap-1.5">
					{#if item.at}
						<span class="text-[11px] text-[var(--color-muted)]"><RelativeTime iso={item.at} /></span>
					{/if}
					{#if item.unread > 0}
						<span
							class="grid min-h-5 min-w-5 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[10px] font-medium text-[var(--color-primary-contrast)]"
							>{item.unread}</span
						>
					{/if}
				</span>
			</span>
			<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">
				{#if item.subtitle}{item.subtitle} · {/if}{item.preview || '—'}
			</span>
			<span class="mt-1 flex items-center gap-1.5">
				<span
					class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-primary-text)]"
					>{chipLabel}</span
				>
				{#if item.statusLabel}
					<span class="rounded-full px-2 py-0.5 text-[10px] font-medium {toneClass}"
						>{item.statusLabel}</span
					>
				{/if}
				{#if item.kind === 'request'}
					<span
						class="rounded-full bg-[var(--color-warning)]/10 px-2 py-0.5 text-[10px] font-medium text-[var(--color-warning)]"
						>{t('hub.status_request', 'Request')}</span
					>
				{/if}
			</span>
		</span>
	</button>
</li>
