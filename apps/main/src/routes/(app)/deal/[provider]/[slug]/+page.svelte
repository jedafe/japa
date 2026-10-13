<script lang="ts">
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { t, i18n } from '$lib/i18n';
	import {
		Ticket,
		BadgeCheck,
		Copy,
		ExternalLink,
		Clock,
		Pencil,
		ChevronRight
	} from 'lucide-svelte';
	import { api, apiBaseUrl } from '$lib/api/client';
	import type { PageData } from './$types';
	import { safeJsonLdScript, safeHref } from '$lib/utils/safeHtml';
	import { currencySymbol } from '$lib/utils/currency';
	import { dealPath, type Coupon } from '$lib/types';
	import DealForm from '$lib/components/deals/DealForm.svelte';

	let { data }: { data: PageData } = $props();

	const coupon = $derived(data.coupon);
	// coupons.details is free-form JSONB — the admin JSON editor can write a
	// scalar, an array, or wrong-typed members (§55). Coerce defensively so a
	// malformed payload can't crash SSR.
	const details = $derived(
		coupon.details && typeof coupon.details === 'object' && !Array.isArray(coupon.details)
			? coupon.details
			: null
	);
	const sections = $derived(Array.isArray(details?.sections) ? details.sections : []);
	const faq = $derived(Array.isArray(details?.faq) ? details.faq : []);
	const trial = $derived(
		details?.trial && typeof details.trial === 'object' && !Array.isArray(details.trial)
			? details.trial
			: null
	);
	// Staff-authored URL rendered into href — guard against unsafe schemes.
	const trialUrl = $derived(safeHref(trial?.url));
	const termsUrl = $derived(safeHref(coupon.terms_url));
	const summary = $derived(
		(typeof details?.summary === 'string' ? details.summary : '') || coupon.description || null
	);
	// The claim CTA is meaningless once the deal has lapsed or before it starts —
	// the /go redirect already soft-lands on the provider's base_url server-side.
	const isExpired = $derived(
		!!coupon.expires_at && new Date(coupon.expires_at).getTime() <= Date.now()
	);
	const isUpcoming = $derived(
		!!coupon.valid_from && new Date(coupon.valid_from).getTime() > Date.now()
	);
	const isClaimable = $derived(!isExpired && !isUpcoming && coupon.status === 'active');
	const providerLabel = $derived(coupon.provider_name ?? t('deal.this_deal', 'this deal'));

	let editing = $state(false);

	const title = $derived(
		t('coupon.title', '{name} — JapaConnect').replace('{name}', coupon.title)
	);
	const description = $derived(
		summary ??
			t('coupon.description', 'Coupon code and discount from {provider}.').replace(
				'{provider}',
				coupon.provider_name ?? ''
			)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'Offer',
			name: coupon.title,
			description,
			url: page.url.href,
		})
	);

	// FAQPage structured data — only from entries carrying BOTH q and a; a
	// Question with an empty name (or no acceptedAnswer text) is schema-invalid
	// (§55). buildDetails drops those at write time; filter again at render so
	// legacy rows don't emit bad markup.
	const faqSchemaEntries = $derived(faq.filter((f) => f.q && f.a));
	const faqJsonLd = $derived(
		faqSchemaEntries.length
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'FAQPage',
					mainEntity: faqSchemaEntries.map((f) => ({
						'@type': 'Question',
						name: f.q,
						acceptedAnswer: { '@type': 'Answer', text: f.a }
					}))
				})
			: null
	);

	function discountLabelFor(c: Coupon): string {
		if (c.discount_type === 'percent') {
			return t('coupons.percent_off', '{n}% off').replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'fixed') {
			return t('coupons.amount_off', '{cur}{n} off')
				.replace('{cur}', currencySymbol(c.currency, data.currencies) || (c.currency ?? ''))
				.replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'free_shipping') {
			return t('coupons.free_shipping', 'Free shipping');
		}
		if (c.discount_value !== undefined && c.discount_value !== null) {
			return String(c.discount_value);
		}
		return '';
	}

	const discountLabel = $derived(discountLabelFor(coupon));

	// Click-through goes through the backend /go redirect so the click is
	// recorded server-side even without JS (Section 55 Deal Pages).
	const goUrl = $derived(`${apiBaseUrl}/deals/${encodeURIComponent(coupon.slug ?? '')}/go`);

	let copied = $state(false);
	let copyError = $state('');

	// recordEvent fires-and-forgets a copy event to the backend (Section 55).
	// Clicks are recorded by the /go redirect — no client-side call needed.
	function recordEvent(eventType: 'click' | 'copy') {
		api.post(`/coupons/${coupon.id}/click`, { event_type: eventType }).catch(() => {});
	}

	async function copyCode() {
		if (!coupon.code) return;
		copyError = '';
		try {
			await navigator.clipboard.writeText(coupon.code);
			copied = true;
			recordEvent('copy');
			setTimeout(() => (copied = false), 2000);
			return;
		} catch {
			// Fall through to the execCommand fallback below.
		}
		try {
			const ta = document.createElement('textarea');
			ta.value = coupon.code;
			ta.setAttribute('readonly', '');
			ta.style.position = 'fixed';
			ta.style.opacity = '0';
			document.body.appendChild(ta);
			ta.select();
			const ok = document.execCommand('copy');
			ta.remove();
			if (!ok) throw new Error('copy failed');
			copied = true;
			recordEvent('copy');
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Copy failed on every path — surface an error instead of
			// recording a copy event that never happened.
			copyError = t('coupons.copy_failed', 'Could not copy the code — please copy it manually.');
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
	{@html safeJsonLdScript(faqJsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex flex-wrap items-center justify-between gap-2">
			<div class="flex items-center gap-1.5 text-sm">
				<a
					href="/deals"
					class="inline-flex items-center gap-1.5 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
				>
					{t('deals.back', 'All deals')}
				</a>
				{#if coupon.provider_slug}
					<span class="text-[var(--color-text-subtle)]">/</span>
					<a
						href="/deal/{coupon.provider_slug}"
						class="inline-flex items-center gap-1.5 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
					>
						{coupon.provider_name}
					</a>
				{/if}
			</div>
			<div class="flex items-center gap-3">
				{#if coupon.updated_at}
					<span class="text-xs text-[var(--color-muted)]">
						{t('deals.last_updated', 'Last updated {date}', {
							date: new Date(coupon.updated_at).toLocaleDateString(i18n.locale)
						})}
					</span>
				{/if}
				{#if data.canManage && !editing}
					<button
						type="button"
						onclick={() => (editing = true)}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Pencil size={14} />
						{t('deal.edit', 'Edit')}
					</button>
				{/if}
			</div>
		</div>
	</div>

	<div class="p-4">
		{#if editing}
			<!-- Ref: §55 — staff inline edit reuses the shared DealForm. -->
			<DealForm
				mode="edit"
				audience="staff"
				coupon={coupon}
				providers={data.providers}
				canManageProviders={data.canManageProviders}
				currencies={data.currencies}
				onsaved={async () => {
					await invalidateAll();
					editing = false;
				}}
				oncancel={() => (editing = false)}
			/>
		{:else}
			<!-- Title + lede -->
			<div class="flex items-start gap-3">
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Ticket size={24} />
				</span>
				<div class="min-w-0 flex-1">
					<h1 class="text-xl font-semibold text-[var(--color-text)]">{coupon.title}</h1>
					{#if summary}
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{summary}</p>
					{/if}
				</div>
			</div>

			<!-- Active offer -->
			<div
				class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
			>
				<div class="flex flex-wrap items-center justify-between gap-2">
					<h2
						class="text-xs font-semibold tracking-wide text-[var(--color-muted)] uppercase"
					>
						{t('deal.active_offer', 'Active offer')}
					</h2>
					<div class="flex items-center gap-2">
						{#if coupon.provider_name}
							<span class="text-sm font-medium text-[var(--color-text)]"
								>{coupon.provider_name}</span
							>
						{/if}
						{#if coupon.is_verified}
							<span
								class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-medium text-[var(--color-primary)]"
							>
								<BadgeCheck size={14} />
								{t('coupons.verified', 'Verified')}
							</span>
						{/if}
					</div>
				</div>

				<div class="mt-4 flex flex-wrap items-center gap-3">
					{#if coupon.code}
						<code
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 font-mono text-lg font-semibold tracking-wide text-[var(--color-text)]"
						>
							{coupon.code}
						</code>
						<button
							type="button"
							onclick={copyCode}
							class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Copy size={16} />
							{copied ? t('coupons.copied', 'Copied!') : t('coupons.copy_code', 'Copy code')}
						</button>
					{/if}
					{#if copyError}
						<p class="mt-2 text-sm text-red-500" role="alert">{copyError}</p>
					{/if}
					{#if discountLabel}
						<span class="text-base font-semibold text-[var(--color-primary)]"
							>{discountLabel}</span
						>
					{/if}
				</div>

				{#if (coupon.referral_url || coupon.provider_base_url) && isClaimable}
					<a
						href={goUrl}
						target="_blank"
						rel="noopener noreferrer nofollow"
						class="mt-5 inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<ExternalLink size={16} />
						{t('coupon.cta', 'Get this deal')}
					</a>
				{/if}
			</div>

			<!-- Secondary offer (trial / onboarding) -->
			{#if trial && (trial.title || trial.body || trialUrl)}
				<div
					class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-5"
				>
					{#if trial.title}
						<h3 class="text-sm font-semibold text-[var(--color-text)]">{trial.title}</h3>
					{/if}
					{#if trial.body}
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{trial.body}</p>
					{/if}
					{#if trialUrl}
						<a
							href={trialUrl}
							target="_blank"
							rel="nofollow noopener"
							class="mt-3 inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<ExternalLink size={14} />
							{t('deal.trial_cta', 'Try it now')}
						</a>
					{/if}
				</div>
			{/if}

			<!-- Why {provider} / How to redeem -->
			<section class="mt-6">
				<h2 class="text-base font-semibold text-[var(--color-text)]">
					{t('deal.why', 'Why {provider}').replace('{provider}', providerLabel)}
				</h2>
				{#if sections.length}
					<ol class="mt-3 list-decimal space-y-3 pl-5">
						{#each sections as section, i (i)}
							<li class="text-sm text-[var(--color-text)]">
								{#if section.title}
									<p class="font-medium">{section.title}</p>
								{/if}
								{#if section.body}
									<p class="mt-0.5 text-[var(--color-text-subtle)]">{section.body}</p>
								{/if}
							</li>
						{/each}
					</ol>
				{:else}
					<h3 class="mt-3 text-sm font-semibold text-[var(--color-text)]">
						{t('deal.how_to_redeem', 'How to redeem')}
					</h3>
					<ol class="mt-2 list-decimal space-y-1.5 pl-5 text-sm text-[var(--color-text-subtle)]">
						<li>{t('deal.redeem_step_1', "Follow the deal link to open the offer on the provider's site.")}</li>
						<li>{t('deal.redeem_step_2', 'Copy the coupon code if one is shown.')}</li>
						<li>{t('deal.redeem_step_3', 'The discount applies at checkout.')}</li>
					</ol>
				{/if}
			</section>

			<!-- Other ways to save -->
			{#if data.related.length}
				<section class="mt-6">
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('deal.other_ways', 'Other ways to save on {provider}').replace(
							'{provider}',
							coupon.provider_name ?? t('deals.internal', 'JapaConnect')
						)}
					</h2>
					<div class="mt-3 grid gap-2 sm:grid-cols-2">
						{#each data.related as rel (rel.id)}
							{@const relLabel = discountLabelFor(rel)}
							<a
								href={dealPath(rel)}
								class="group flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<div class="min-w-0 flex-1">
									<p
										class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{rel.title}
									</p>
									{#if relLabel}
										<span class="text-xs font-medium text-[var(--color-primary)]">
											{relLabel}
										</span>
									{/if}
								</div>
								<ChevronRight
									size={16}
									class="shrink-0 text-[var(--color-muted)] transition-transform group-hover:translate-x-0.5"
								/>
							</a>
						{/each}
					</div>
				</section>
			{/if}

			<!-- FAQ -->
			{#if faqSchemaEntries.length}
				<section class="mt-6">
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('deal.faq', 'Frequently asked questions')}
					</h2>
					<div
						class="mt-3 divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
					>
						{#each faqSchemaEntries as f, i (i)}
							<details class="group">
								<summary
									class="flex cursor-pointer items-center justify-between gap-3 p-4 text-sm font-medium text-[var(--color-text)]"
								>
									{f.q}
									<ChevronRight
										size={16}
										class="shrink-0 text-[var(--color-muted)] transition-transform group-open:rotate-90"
									/>
								</summary>
								<p class="px-4 pb-4 text-sm text-[var(--color-text-subtle)]">{f.a}</p>
							</details>
						{/each}
					</div>
				</section>
			{/if}

			<!-- Terms + expiry -->
			<div class="mt-6 flex flex-wrap items-center gap-4 text-xs text-[var(--color-muted)]">
				{#if isUpcoming}
					<span class="inline-flex items-center gap-1">
						<Clock size={12} />
						{t('coupons.starts', 'Starts {date}').replace(
							'{date}',
							new Date(coupon.valid_from as string).toLocaleDateString(i18n.locale)
						)}
					</span>
				{/if}
				{#if coupon.expires_at}
					<span class="inline-flex items-center gap-1">
						<Clock size={12} />
						{t('coupons.expires', 'Expires {date}').replace(
							'{date}',
							new Date(coupon.expires_at).toLocaleDateString(i18n.locale)
						)}
					</span>
				{/if}
				{#if termsUrl}
					<a
						href={termsUrl}
						target="_blank"
						rel="noopener noreferrer"
						class="underline transition-colors hover:text-[var(--color-text)]"
					>
						{t('coupons.terms', 'Terms & conditions')}
					</a>
				{/if}
			</div>
		{/if}
	</div>
</div>
