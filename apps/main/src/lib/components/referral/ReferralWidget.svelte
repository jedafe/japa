<script lang="ts">
	import { t } from '$lib/i18n';
	import { Gift, Copy, Check, Users, MousePointerClick, CheckCircle2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import type { ReferralStats } from '$lib/types';

	let stats = $state<ReferralStats | null>(null);
	let loading = $state(true);
	let disabled = $state(false);
	let copied = $state(false);

	const shareUrl = $derived(
		stats ? `${page.url.origin}/?ref=${stats.code}` : ''
	);

	async function loadStats() {
		loading = true;
		const res = await api.get<ReferralStats>('/me/referral');
		loading = false;
		if (res.error) {
			if (res.error.code === 'referral_disabled') {
				disabled = true;
			}
			return;
		}
		stats = res.data;
	}

	async function copyLink() {
		if (!shareUrl) return;
		try {
			await navigator.clipboard.writeText(shareUrl);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard may be blocked; ignore.
		}
	}

	$effect(() => {
		loadStats();
	});
</script>

{#if !disabled}
	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<Gift size={16} class="text-[var(--color-primary)]" />
			{t('referral.title', 'Refer & earn')}
		</h2>

		{#if loading}
			<div class="h-16 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
		{:else if stats}
			<!-- Stats row -->
			<div class="mb-3 grid grid-cols-3 gap-2">
				<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-2 text-center">
					<Users size={14} class="mx-auto text-[var(--color-muted)]" />
					<span class="mt-1 block text-sm font-medium text-[var(--color-text)]">{stats.total}</span>
					<span class="block text-xs text-[var(--color-muted)]">{t('referral.total', 'Total')}</span>
				</div>
				<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-2 text-center">
					<CheckCircle2 size={14} class="mx-auto text-[var(--color-muted)]" />
					<span class="mt-1 block text-sm font-medium text-[var(--color-text)]">{stats.credited}</span>
					<span class="block text-xs text-[var(--color-muted)]">{t('referral.credited', 'Credited')}</span>
				</div>
				<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-2 text-center">
					<MousePointerClick size={14} class="mx-auto text-[var(--color-muted)]" />
					<span class="mt-1 block text-sm font-medium text-[var(--color-text)]">{stats.click_count}</span>
					<span class="block text-xs text-[var(--color-muted)]">{t('referral.clicks', 'Clicks')}</span>
				</div>
			</div>

			<!-- Share link -->
			<div class="flex items-center gap-2">
				<input
					type="text"
					value={shareUrl}
					readonly
					class="min-w-0 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xs text-[var(--color-text)]"
				/>
				<button
					type="button"
					onclick={copyLink}
					class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
				>
					{#if copied}
						<Check size={14} /> {t('common.copied', 'Copied!')}
					{:else}
						<Copy size={14} /> {t('common.copy', 'Copy')}
					{/if}
				</button>
			</div>
		{/if}
	</div>
{/if}
