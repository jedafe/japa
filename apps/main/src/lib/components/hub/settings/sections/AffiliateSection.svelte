<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { createAffiliate, deleteAffiliate } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Share2, MousePointerClick, TrendingUp, Wallet, MessageCircle, AtSign, Link as LinkIcon, Copy, Check } from 'lucide-svelte';
	import type { AffiliateEarnings } from '$lib/types';

	// Ref: japaconnect-engineering/sections/56-user-affiliate-program.md.
	// The backend UserAffiliateLink row returns { id, product_type, code,
	// description, destination_url, click_count, conversion_count, is_active,
	// created_at } — NOT { target_url, label }. The previous interface caused
	// the create form to send the wrong fields (BUG-014) and the list to read
	// non-existent fields.
	interface AffiliateLink {
		id: string;
		product_type: string;
		code: string;
		description?: string | null;
		destination_url: string;
		click_count?: number;
		conversion_count?: number;
		is_active?: boolean;
		created_at?: string;
	}

	interface AffiliateSectionData {
		links?: AffiliateLink[];
		earnings?: AffiliateEarnings | null;
	}

	let { data }: { data: unknown } = $props();
	const sectionData = $derived((data as AffiliateSectionData | null) ?? {});
	let links = $state<AffiliateLink[]>(untrack(() => sectionData.links ?? []));
	const earnings = $derived(sectionData.earnings ?? null);
	let newUrl = $state('');
	let newDescription = $state('');
	let newProductType = $state('pro_subscription');
	let creating = $state(false);
	let error = $state<string | null>(null);

	const productTypes = $derived([
		{ value: 'pro_subscription', label: t('affiliate.product_type_pro_subscription', 'Pro Subscription') },
		{ value: 'course', label: t('affiliate.product_type_course', 'Course') },
		{ value: 'pro_annual', label: t('affiliate.product_type_pro_annual', 'Pro Annual') }
	]);

	async function doCreate() {
		if (!newUrl.trim()) return;
		creating = true;
		error = null;
		const res = await createAffiliate({
			product_type: newProductType,
			destination_url: newUrl.trim(),
			description: newDescription.trim() || undefined
		});
		creating = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) links = [res.data as AffiliateLink, ...links];
		newUrl = '';
		newDescription = '';
	}

	async function doDelete(id: string) {
		const idx = links.findIndex((l) => l.id === id);
		if (idx < 0) return;
		const removed = links[idx];
		await optimisticAction({
			apply: () => (links = links.filter((l) => l.id !== id)),
			rollback: () => (links = [...links.slice(0, idx), removed, ...links.slice(idx)]),
			call: () => deleteAffiliate(id)
		});
	}

	// Phase 34.06 — share buttons for referral links. Builds share URLs for
	// WhatsApp, Twitter/X, and LinkedIn, plus a copy-to-clipboard fallback.
	// Ref: sections/56-user-affiliate-program.md
	function shareUrl(link: AffiliateLink): string {
		return `${link.destination_url}?ref=${link.code}`;
	}

	function shareWhatsApp(link: AffiliateLink) {
		const url = encodeURIComponent(shareUrl(link));
		const text = encodeURIComponent(t('settings.affiliate.share_text', 'Check this out on JapaConnect'));
		window.open(`https://wa.me/?text=${text}%20${url}`, '_blank', 'noopener,noreferrer');
	}

	function shareTwitter(link: AffiliateLink) {
		const url = encodeURIComponent(shareUrl(link));
		const text = encodeURIComponent(t('settings.affiliate.share_text', 'Check this out on JapaConnect'));
		window.open(`https://twitter.com/intent/tweet?text=${text}&url=${url}`, '_blank', 'noopener,noreferrer');
	}

	function shareLinkedIn(link: AffiliateLink) {
		const url = encodeURIComponent(shareUrl(link));
		window.open(`https://www.linkedin.com/sharing/share-offsite/?url=${url}`, '_blank', 'noopener,noreferrer');
	}

	let copiedId = $state<string | null>(null);
	async function copyLink(link: AffiliateLink) {
		try {
			await navigator.clipboard.writeText(shareUrl(link));
			copiedId = link.id;
			setTimeout(() => (copiedId = null), 2000);
		} catch {
			// Clipboard API may be unavailable (e.g. insecure context). No-op —
			// the user can still manually copy from the displayed URL.
		}
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.affiliate')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.affiliate.desc')}</p>
	</header>

	{#if earnings}
		<div class="mb-6 grid grid-cols-3 gap-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<MousePointerClick size={13} /> {t('settings.affiliate.clicks', 'Clicks')}
				</div>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)] [font-variant-numeric:tabular-nums]">{earnings.total_clicks}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<TrendingUp size={13} /> {t('settings.affiliate.conversions', 'Conversions')}
				</div>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)] [font-variant-numeric:tabular-nums]">{earnings.total_conversions}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<Wallet size={13} /> {t('settings.affiliate.earned', 'Earned')}
				</div>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)] [font-variant-numeric:tabular-nums]">{earnings.total_earned}</p>
			</div>
		</div>

		{#if earnings.by_product.length > 0}
			<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.affiliate.by_product', 'Earnings by product')}</h2>
				<ul class="space-y-2">
					{#each earnings.by_product as row (row.product_type)}
						<li class="flex items-center justify-between gap-3 text-sm">
							<span class="capitalize text-[var(--color-text)]">{row.product_type.replace(/_/g, ' ')}</span>
							<span class="flex items-center gap-4 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
								<span>{row.clicks} {t('settings.affiliate.clicks', 'Clicks')}</span>
								<span>{row.conversions} {t('settings.affiliate.conversions', 'Conversions')}</span>
								<span class="font-medium text-[var(--color-text)]">{row.earned}</span>
							</span>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if earnings.recent_events.length > 0}
			<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.affiliate.recent', 'Recent activity')}</h2>
				<ul class="divide-y divide-[var(--color-border)]">
					{#each earnings.recent_events.slice(0, 10) as ev (ev.id)}
						<li class="flex items-center justify-between gap-3 py-2 text-sm">
							<span class="min-w-0">
								<span class="capitalize text-[var(--color-text)]">{ev.event_type.replace(/_/g, ' ')}</span>
								<span class="ml-1 text-xs text-[var(--color-muted)]">· {ev.product_type.replace(/_/g, ' ')}</span>
							</span>
							<span class="flex items-center gap-3 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
								{#if ev.commission_amount}<span class="font-medium text-[var(--color-text)]">{ev.commission_amount}</span>{/if}
								<span>{ev.link_code}</span>
							</span>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	{/if}

	<form class="mb-6 space-y-3" onsubmit={(e) => { e.preventDefault(); doCreate(); }}>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.affiliate.product_type', 'Product type')}</span>
			<select
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={newProductType}
				onchange={(e) => (newProductType = e.currentTarget.value)}
			>
				{#each productTypes as pt (pt.value)}
					<option value={pt.value}>{pt.label}</option>
				{/each}
			</select>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.affiliate.url')}</span>
			<input
				type="url"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				placeholder="https://"
				value={newUrl}
				oninput={(e) => (newUrl = e.currentTarget.value)}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.affiliate.label')}</span>
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={newDescription}
				oninput={(e) => (newDescription = e.currentTarget.value)}
			/>
		</label>
		{#if error}<p class="text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		<button
			type="submit"
			disabled={creating || !newUrl.trim()}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{creating ? t('settings.affiliate.creating') : t('settings.affiliate.create')}
		</button>
	</form>

	{#if links.length === 0}
		<EmptyState icon={Share2} title={t('settings.affiliate.empty_title')} body={t('settings.affiliate.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each links as link (link.id)}
				<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="flex items-center justify-between">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{link.description ?? link.destination_url}</p>
							<p class="truncate text-xs text-[var(--color-muted)]">{link.destination_url}</p>
							<p class="mt-0.5 text-xs text-[var(--color-muted)]">/{link.code} · {link.product_type.replace(/_/g, ' ')}</p>
						</div>
						<button
							type="button"
							class="ml-3 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							onclick={() => doDelete(link.id)}
						>
							{t('settings.affiliate.delete')}
						</button>
					</div>
					<!-- Phase 34.06 — share buttons -->
					<div class="mt-2 flex items-center gap-1.5 border-t border-[var(--color-border)] pt-2">
						<span class="mr-1 text-xs text-[var(--color-muted)]">{t('settings.affiliate.share', 'Share')}:</span>
						<button
							type="button"
							onclick={() => shareWhatsApp(link)}
							class="rounded p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							aria-label={t('settings.affiliate.share_whatsapp', 'Share on WhatsApp')}
							title="WhatsApp"
						>
							<MessageCircle size={15} />
						</button>
						<button
							type="button"
							onclick={() => shareTwitter(link)}
							class="rounded p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							aria-label={t('settings.affiliate.share_twitter', 'Share on X')}
							title="X"
						>
							<AtSign size={15} />
						</button>
						<button
							type="button"
							onclick={() => shareLinkedIn(link)}
							class="rounded p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							aria-label={t('settings.affiliate.share_linkedin', 'Share on LinkedIn')}
							title="LinkedIn"
						>
							<LinkIcon size={15} />
						</button>
						<button
							type="button"
							onclick={() => copyLink(link)}
							class="rounded p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							aria-label={t('settings.affiliate.copy_link', 'Copy link')}
							title={t('settings.affiliate.copy_link', 'Copy link')}
						>
							{#if copiedId === link.id}
								<Check size={15} class="text-[var(--color-success)]" />
							{:else}
								<Copy size={15} />
							{/if}
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
