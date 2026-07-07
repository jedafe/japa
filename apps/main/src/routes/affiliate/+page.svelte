<script lang="ts">
	import { Link2, DollarSign, Copy, Trash2, Plus, Loader2, TrendingUp } from 'lucide-svelte';
	import { api, type AffiliateLink, type AffiliateConversion } from '$lib/api';
	import { onMount } from 'svelte';

	let links = $state<AffiliateLink[]>([]);
	let earnings = $state<{ total_credits: number; conversions: AffiliateConversion[] }>({
		total_credits: 0,
		conversions: []
	});
	let enrolled = $state(false);
	let loading = $state(true);
	let enrolling = $state(false);
	let error = $state('');
	let copiedId = $state('');

	// New-link form
	let showNewLink = $state(false);
	let productType = $state('subscription');
	let productId = $state('');
	let creating = $state(false);

	onMount(loadDashboard);

	async function loadDashboard() {
		loading = true;
		try {
			const [linksResult, earningsResult] = await Promise.allSettled([
				api.getAffiliateLinks(),
				api.getAffiliateEarnings()
			]);
			if (linksResult.status === 'fulfilled') {
				links = linksResult.value.links ?? [];
				enrolled = true;
			}
			if (earningsResult.status === 'fulfilled') {
				earnings = earningsResult.value;
			}
		} catch {
			error = 'Failed to load affiliate data';
		} finally {
			loading = false;
		}
	}

	async function enroll() {
		enrolling = true;
		error = '';
		try {
			await api.enrollAffiliate();
			enrolled = true;
			await loadDashboard();
		} catch {
			error = 'Enrolment failed. The affiliate program may not be open yet.';
		} finally {
			enrolling = false;
		}
	}

	async function createLink(e: Event) {
		e.preventDefault();
		if (!productId.trim()) return;
		creating = true;
		try {
			const link = await api.createAffiliateLink({
				product_type: productType,
				product_id: productId.trim()
			});
			links = [link, ...links];
			productId = '';
			showNewLink = false;
		} catch {
			error = 'Failed to create link';
		} finally {
			creating = false;
		}
	}

	async function deleteLink(id: string) {
		try {
			await api.deleteAffiliateLink(id);
			links = links.filter((l) => l.id !== id);
		} catch {
			// ignore
		}
	}

	async function copyLink(link: AffiliateLink) {
		try {
			await navigator.clipboard.writeText(link.url);
			copiedId = link.id;
			setTimeout(() => (copiedId = ''), 2000);
		} catch {
			// clipboard unavailable
		}
	}
</script>

<svelte:head>
	<title>Affiliate Program | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold" style="color: var(--color-foreground)">
			<Link2 class="h-6 w-6" style="color: var(--color-primary)" /> Affiliate Program
		</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			Earn Credits by referring people to JapaConnect products.
		</p>
	</header>

	{#if error}
		<p class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-destructive, #ef4444) 10%, transparent); color: var(--color-destructive, #ef4444)">
			{error}
		</p>
	{/if}

	{#if loading}
		<div class="flex justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
		</div>
	{:else if !enrolled}
		<div
			class="rounded-2xl border p-8 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<TrendingUp class="mx-auto h-10 w-10" style="color: var(--color-primary)" />
			<h2 class="mt-3 text-lg font-semibold" style="color: var(--color-foreground)">Join the program</h2>
			<p class="mx-auto mt-1 max-w-md text-sm" style="color: var(--color-muted)">
				Generate tracked referral links for JapaConnect products and earn Credits on every
				qualifying conversion.
			</p>
			<button
				onclick={enroll}
				disabled={enrolling}
				class="mt-4 rounded-xl px-6 py-2.5 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background: var(--color-primary)"
			>
				{#if enrolling}<Loader2 class="h-4 w-4 animate-spin" />{:else}Enrol now{/if}
			</button>
		</div>
	{:else}
		<!-- Earnings summary -->
		<div class="mb-6 grid grid-cols-2 gap-4">
			<div class="rounded-xl border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
				<p class="flex items-center gap-1 text-xs" style="color: var(--color-muted)">
					<DollarSign class="h-3.5 w-3.5" /> Total earned
				</p>
				<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">
					{earnings.total_credits.toLocaleString()} <span class="text-sm font-normal">Credits</span>
				</p>
			</div>
			<div class="rounded-xl border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
				<p class="flex items-center gap-1 text-xs" style="color: var(--color-muted)">
					<TrendingUp class="h-3.5 w-3.5" /> Conversions
				</p>
				<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">
					{earnings.conversions.length}
				</p>
			</div>
		</div>

		<!-- Links -->
		<section class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="font-semibold" style="color: var(--color-foreground)">Your referral links</h2>
				<button
					onclick={() => (showNewLink = !showNewLink)}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background: var(--color-primary)"
				>
					<Plus class="h-4 w-4" /> New link
				</button>
			</div>

			{#if showNewLink}
				<form onsubmit={createLink} class="mb-4 flex flex-col gap-2 rounded-xl border p-4 sm:flex-row sm:items-end"
					style="border-color: var(--color-border); background: var(--color-background)">
					<div>
						<label for="ptype" class="mb-1 block text-xs" style="color: var(--color-muted)">Product type</label>
						<select id="ptype" bind:value={productType}
							class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">
							<option value="subscription">Subscription</option>
							<option value="course">Course</option>
							<option value="product">Product</option>
						</select>
					</div>
					<div class="flex-1">
						<label for="pid" class="mb-1 block text-xs" style="color: var(--color-muted)">Product ID</label>
						<input id="pid" bind:value={productId} required placeholder="e.g. pro-annual"
							class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
					</div>
					<button type="submit" disabled={creating}
						class="rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-50" style="background: var(--color-primary)">
						{#if creating}<Loader2 class="h-4 w-4 animate-spin" />{:else}Create{/if}
					</button>
				</form>
			{/if}

			{#if links.length === 0}
				<p class="py-6 text-center text-sm" style="color: var(--color-muted)">
					No referral links yet. Create one to start earning.
				</p>
			{:else}
				<ul class="space-y-2">
					{#each links as link (link.id)}
						<li class="flex items-center gap-3 rounded-lg border p-3" style="border-color: var(--color-border)">
							<div class="min-w-0 flex-1">
								<p class="truncate font-mono text-sm" style="color: var(--color-foreground)">{link.url}</p>
								<p class="text-xs" style="color: var(--color-muted)">
									{link.product_type} · {link.click_count ?? 0} clicks · {link.conversion_count ?? 0} conversions
								</p>
							</div>
							<button onclick={() => copyLink(link)} class="shrink-0 rounded-lg p-2 transition-colors hover:opacity-70" title="Copy" style="color: var(--color-primary)">
								{#if copiedId === link.id}
									<span class="text-xs">Copied!</span>
								{:else}
									<Copy class="h-4 w-4" />
								{/if}
							</button>
							<button onclick={() => deleteLink(link.id)} class="shrink-0 rounded-lg p-2 transition-colors hover:opacity-70" title="Delete" style="color: var(--color-destructive, #ef4444)">
								<Trash2 class="h-4 w-4" />
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</main>
