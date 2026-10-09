<script lang="ts">
	import { t } from '$lib/i18n';
	import { Gift, Copy, Check, Users, MousePointerClick, CheckCircle2, Clock } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import type { ReferralStats, ReferralHistoryEntry } from '$lib/types';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';

	let stats = $state<ReferralStats | null>(null);
	let history = $state<ReferralHistoryEntry[]>([]);
	let loading = $state(true);
	let disabled = $state(false);
	let copied = $state(false);

	const shareUrl = $derived(
		stats ? `${page.url.origin}/?ref=${stats.code}` : ''
	);

	async function loadStats() {
		loading = true;
		const [statsRes, historyRes] = await Promise.all([
			api.get<ReferralStats>('/me/referral'),
			api.get<ReferralHistoryEntry[]>('/me/referral/history')
		]);
		loading = false;
		if (statsRes.error) {
			if (statsRes.error.code === 'referral_disabled') {
				disabled = true;
			}
			return;
		}
		stats = statsRes.data;
		if (!historyRes.error && historyRes.data) {
			history = historyRes.data;
		}
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

	const statusLabel = (status: string) =>
		t(`referral.status_${status}`, status);

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
			<div class="mb-3 grid grid-cols-4 gap-2">
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
					<Clock size={14} class="mx-auto text-[var(--color-muted)]" />
					<span class="mt-1 block text-sm font-medium text-[var(--color-text)]">{stats.pending}</span>
					<span class="block text-xs text-[var(--color-muted)]">{t('referral.pending', 'Pending')}</span>
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

			<!-- Referral history (BUG-015 fix) -->
			{#if history.length > 0}
				<div class="mt-4 border-t border-[var(--color-border)] pt-3">
					<h3 class="mb-2 text-xs font-medium text-[var(--color-muted)]">
						{t('referral.history', 'Referral history')}
					</h3>
					<ul class="space-y-1">
						{#each history.slice(0, 5) as entry}
							<li class="flex items-center justify-between gap-2 text-xs">
								<span class="font-mono text-[var(--color-text)]">{entry.referral_code}</span>
								<span class="flex items-center gap-2">
									<span class="text-[var(--color-muted)]">{statusLabel(entry.status)}</span>
									{#if entry.status === 'rejected'}
										<AppealButton entityType="referral" entityId={entry.id} />
									{/if}
								</span>
							</li>
						{/each}
					</ul>
				</div>
			{:else if !loading}
				<p class="mt-3 text-xs text-[var(--color-muted)]">
					{t('referral.history_empty', 'No referrals yet. Share your link to start earning.')}
				</p>
			{/if}
		{/if}
	</div>
{/if}
