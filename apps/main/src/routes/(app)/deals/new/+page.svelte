<script lang="ts">
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
	// /deals/new — staff create (audience="staff") or organization deal
	// submission (audience="org", create-only — org mode never PATCHes the
	// staff-gated /admin/coupons endpoint).
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ArrowLeft, Ticket, Loader2, CheckCircle2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import DealForm from '$lib/components/deals/DealForm.svelte';
	import { EmptyState } from '$lib/components/ui';
	import { dealPath, type Coupon } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('deals.new_title', 'New deal — JapaConnect'));

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	const labelClass = 'mb-1 block text-sm font-medium text-[var(--color-text)]';

	// Org submission mode (non-staff path only). Single eligible orgs are
	// auto-selected; the picker only renders when there is more than one.
	// untrack: data.orgs only seeds the initial selection.
	let selectedOrgId = $state(untrack(() => data.orgs[0]?.id ?? ''));
	let submissions = $state<Coupon[]>([]);
	let submissionsLoading = $state(false);
	let submitted = $state<Coupon | null>(null);
	// Bumped after each save so DealForm remounts with a blank state — the
	// component seeds its form values once and never resets itself.
	let formKey = $state(0);

	const selectedOrg = $derived(data.orgs.find((o) => o.id === selectedOrgId));

	async function loadSubmissions(orgId: string) {
		submissionsLoading = true;
		try {
			const res = await api.get<Coupon[]>(`/organizations/${orgId}/deals?page_size=50`);
			// Stale-guard: ignore the response if the user switched orgs mid-flight.
			if (selectedOrgId === orgId) submissions = res.data ?? [];
		} finally {
			if (selectedOrgId === orgId) submissionsLoading = false;
		}
	}

	// Load the org's submissions whenever the selection changes (including the
	// initial auto-select on mount). Only the org id is read here, so writes
	// to submissions/submissionsLoading inside loadSubmissions cannot retrigger.
	$effect(() => {
		if (data.isStaff || !selectedOrgId) return;
		loadSubmissions(selectedOrgId);
	});

	function onOrgChange() {
		submitted = null;
		submissions = [];
	}

	function onOrgSaved(coupon: Coupon) {
		submitted = coupon;
		formKey += 1;
		loadSubmissions(selectedOrgId);
	}

	function statusChipClass(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-green-500/10 text-green-600';
			case 'pending':
				return 'bg-amber-500/10 text-amber-600';
			case 'suspended':
				return 'bg-red-500/10 text-red-500';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
		}
	}

	function statusLabel(status: string): string {
		const fallbacks: Record<string, string> = {
			active: 'Active',
			pending: 'Pending',
			suspended: 'Suspended',
			expired: 'Expired'
		};
		return t(`deals.status.${status}`, fallbacks[status] ?? status);
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex" />
	<meta property="og:title" content={title} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div
		class="mx-auto flex max-w-2xl items-center gap-3 border-b border-[var(--color-border)] px-4 py-3"
	>
		<a
			href="/deals"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('deals.back', 'All deals')}
		>
			<ArrowLeft size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('deals.new_deal', 'New deal')}
		</h1>
	</div>

	<div class="mx-auto max-w-2xl space-y-6 p-4">
		{#if data.isStaff}
			<DealForm
				mode="create"
				audience="staff"
				providers={data.providers}
				canManageProviders={data.canManageProviders}
				onsaved={(c) => goto(dealPath(c))}
				oncancel={() => goto('/deals')}
			/>
		{:else if data.orgs.length === 0}
			<EmptyState
				icon={Ticket}
				title={t('deals.no_org', 'Deals are submitted by organizations')}
				description={t(
					'deals.no_org_hint',
					'Create or join an organization with an owner, admin, or editor role to submit deals.'
				)}
				action={{ label: t('organizations.new.submit', 'Create organization'), href: '/org/new' }}
			/>
		{:else}
			{#if submitted}
				<div
					class="flex items-start gap-2 rounded-[var(--radius)] border border-green-500/30 bg-green-500/10 px-3 py-2 text-sm text-green-600"
					role="status"
				>
					<CheckCircle2 size={16} class="mt-0.5 shrink-0" />
					<p>
						{t(
							'deals.submitted_pending',
							'Your deal was submitted for review. It will appear publicly once approved.'
						)}
					</p>
				</div>
			{/if}

			{#if data.orgs.length > 1}
				<div>
					<label for="deal-org" class={labelClass}>{t('deals.org_picker', 'Submitting as')}</label>
					<select
						id="deal-org"
						bind:value={selectedOrgId}
						onchange={onOrgChange}
						class={inputClass}
					>
						{#each data.orgs as org (org.id)}
							<option value={org.id}>{org.name}</option>
						{/each}
					</select>
				</div>
			{:else if selectedOrg}
				<p class="text-sm text-[var(--color-text-subtle)]">
					{t('deals.org_picker', 'Submitting as')}:
					<span class="font-medium text-[var(--color-text)]">{selectedOrg.name}</span>
				</p>
			{/if}

			{#key `${selectedOrgId}:${formKey}`}
				<DealForm mode="create" audience="org" orgId={selectedOrgId} onsaved={onOrgSaved} />
			{/key}

			<section>
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
					{t('deals.your_submissions', 'Your submissions')}
				</h2>
				{#if submissionsLoading}
					<div class="flex items-center gap-2 py-4 text-sm text-[var(--color-text-subtle)]">
						<Loader2 size={16} class="animate-spin" />
						{t('common.loading', 'Loading')}
					</div>
				{:else if submissions.length === 0}
					<p class="py-4 text-sm text-[var(--color-text-subtle)]">
						{t('deals.submissions_empty', 'No submissions yet.')}
					</p>
				{:else}
					<ul>
						{#each submissions as coupon (coupon.id)}
							<li
								class="flex items-center justify-between gap-3 border-b border-[var(--color-border)] py-2.5"
							>
								<div class="min-w-0">
									{#if coupon.status === 'active' && coupon.slug}
										<a
											href={dealPath(coupon)}
											class="truncate text-sm font-medium text-[var(--color-primary)] hover:underline"
										>
											{coupon.title}
										</a>
									{:else}
										<span class="truncate text-sm font-medium text-[var(--color-text)]">
											{coupon.title}
										</span>
									{/if}
									{#if coupon.created_at}
										<p class="text-xs text-[var(--color-text-subtle)]">
											{new Date(coupon.created_at).toLocaleDateString()}
										</p>
									{/if}
								</div>
								<span
									class="inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium {statusChipClass(
										coupon.status
									)}"
								>
									{statusLabel(coupon.status)}
								</span>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}
	</div>
</div>
