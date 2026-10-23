<script lang="ts">
	import { X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { RelativeTime } from '$lib/components/ui';
	import { notificationIconFor } from './notificationType';
	import type { Notification } from '$lib/types';

	interface Props {
		n: Notification;
		onOpen: (n: Notification) => void;
		onDelete: (n: Notification) => void;
	}

	let { n, onOpen, onDelete }: Props = $props();

	const unread = $derived(!n.read_at);
	const Icon = $derived(notificationIconFor(n.notification_type));
</script>

<li
	class="group flex items-stretch border-b border-[var(--color-border)] last:border-b-0 {unread
		? 'bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)]'
		: ''}"
>
	<button
		type="button"
		onclick={() => onOpen(n)}
		class="flex flex-1 items-start gap-3 p-4 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
		aria-label={unread
			? t('notifications.unread_open', 'Open unread notification')
			: t('notifications.open_aria', 'Open notification')}
	>
		<span
			class="mt-0.5 grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"
		>
			<Icon size={18} />
		</span>
		<span class="min-w-0 flex-1">
			<span class="flex items-start gap-2">
				<span class="text-sm font-medium text-[var(--color-text)]">{n.title}</span>
				{#if unread}
					<span
						class="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-[var(--color-primary)]"
						aria-hidden="true"
					></span>
				{/if}
			</span>
			{#if n.body}
				<span class="mt-0.5 block text-sm text-[var(--color-text-subtle)]">{n.body}</span>
			{/if}
			<span class="mt-1 flex items-center gap-2 text-xs text-[var(--color-muted)]">
				<RelativeTime iso={n.created_at} />
				<span aria-hidden="true">·</span>
				<span>{n.type_label}</span>
			</span>
		</span>
	</button>
	<button
		type="button"
		onclick={(e) => {
			e.stopPropagation();
			onDelete(n);
		}}
		class="flex items-center justify-center px-3 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-destructive)] focus-visible:text-[var(--color-destructive)]"
		aria-label={t('common.remove', 'Remove')}
		title={t('common.remove', 'Remove')}
	>
		<X size={16} />
	</button>
</li>
