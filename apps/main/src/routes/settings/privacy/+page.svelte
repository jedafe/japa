<script lang="ts">
	import { ArrowLeft, Shield, Download, Trash2, Loader2, CheckCircle2 } from 'lucide-svelte';
	import { api, getToken, type ConsentCategory } from '$lib/api';
	import { onMount } from 'svelte';

	let consents = $state<ConsentCategory[]>([]);
	let loading = $state(true);
	let error = $state('');
	let savingCategory = $state<string | null>(null);

	// Data export
	let exportLoading = $state(false);
	let exportSuccess = $state('');
	let exportError = $state('');

	// Account deletion
	let showDeleteConfirm = $state(false);
	let deleteConfirmText = $state('');
	let deleteLoading = $state(false);
	let deleteError = $state('');

	const consentDescriptions: Record<string, string> = {
		analytics: 'Allow us to collect anonymous usage data to improve the platform.',
		marketing: 'Receive personalized recommendations and promotional content.',
		third_party: 'Allow sharing of your data with trusted third-party partners.',
		cookies: 'Allow non-essential cookies for enhanced functionality.'
	};

	const requiredCategories = ['essential', 'security'];

	onMount(loadConsents);

	async function loadConsents() {
		loading = true;
		error = '';
		try {
			consents = await api.getConsents();
		} catch {
			error = 'Failed to load consent preferences.';
		} finally {
			loading = false;
		}
	}

	async function toggleConsent(category: string, granted: boolean) {
		savingCategory = category;
		try {
			const result = await api.updateConsent({ category, granted });
			consents = consents.map((c) =>
				c.category === category ? { ...c, granted: result.granted, granted_at: result.granted ? new Date().toISOString() : undefined } : c
			);
		} catch {
			// revert on error — keep old state
		} finally {
			savingCategory = null;
		}
	}

	async function handleDataExport() {
		exportLoading = true;
		exportError = '';
		exportSuccess = '';
		try {
			await api.requestDataExport();
			exportSuccess = 'Your data export has been requested. You will receive an email when it is ready.';
		} catch (err) {
			exportError = err instanceof Error ? err.message : 'Failed to request data export.';
		} finally {
			exportLoading = false;
		}
	}

	async function handleAccountDeletion() {
		if (deleteConfirmText !== 'DELETE') return;
		deleteLoading = true;
		deleteError = '';
		try {
			await api.requestAccountDeletion();
			document.cookie = 'jc_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 UTC';
			window.location.href = '/';
		} catch (err) {
			deleteError = err instanceof Error ? err.message : 'Failed to request account deletion.';
		} finally {
			deleteLoading = false;
		}
	}

	function isRequired(category: string): boolean {
		return requiredCategories.includes(category);
	}

	function getDescription(category: string): string {
		return consentDescriptions[category] || `Consent for ${category} data processing.`;
	}

	function getConsentState(category: string): boolean {
		return consents.find((c) => c.category === category)?.granted ?? false;
	}
</script>

<svelte:head>
	<title>Privacy Settings | JapaConnect</title>
	<meta name="description" content="Manage your privacy and data consent preferences on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/settings"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Settings
	</a>

	<h1 class="mb-2 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Privacy & Data</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		Control how your data is used and manage your privacy preferences.
	</p>

	<!-- Consent Categories -->
	<section class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
			<Shield class="h-5 w-5" style="color: var(--color-primary)" />
			Consent Preferences
		</h2>

		{#if loading}
			<div class="space-y-3">
				{#each [1, 2, 3, 4] as _}
					<div class="h-16 animate-pulse rounded-lg" style="background-color: var(--color-background)"></div>
				{/each}
			</div>
		{:else if error}
			<div class="rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{error}
			</div>
		{:else}
			<div class="space-y-3">
				{#each Object.entries(consentDescriptions) as [category, description]}
					{@const required = isRequired(category)}
					{@const granted = getConsentState(category)}
					<div
						class="flex items-center justify-between rounded-lg border p-4"
						style="border-color: var(--color-border); background-color: var(--color-background)"
					>
						<div class="flex-1 pr-4">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium capitalize" style="color: var(--color-foreground)">
									{category.replace(/_/g, ' ')}
								</span>
								{#if required}
									<span
										class="rounded-full px-1.5 py-0.5 text-[10px] font-medium"
										style="background-color: color-mix(in srgb, var(--color-warning) 12%, transparent); color: var(--color-warning)"
									>
										Required
									</span>
								{:else}
									<span
										class="rounded-full px-1.5 py-0.5 text-[10px] font-medium"
										style="background-color: color-mix(in srgb, var(--color-success, #16a34a) 12%, transparent); color: var(--color-success, #16a34a)"
									>
										Optional
									</span>
								{/if}
							</div>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">{description}</p>
						</div>
						{#if required}
							<div
								class="flex h-6 w-10 items-center justify-center rounded-full text-xs font-medium"
								style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)"
							>
								On
							</div>
						{:else}
							<button
								onclick={() => toggleConsent(category, !granted)}
								disabled={savingCategory === category}
								class="relative h-6 w-10 shrink-0 rounded-full transition-colors"
								style="background-color: {granted ? 'var(--color-primary)' : 'var(--color-border)'}"
								aria-label="Toggle {category} consent"
							>
								<span
									class="absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform"
									style="left: {granted ? '18px' : '2px'}"
								></span>
								{#if savingCategory === category}
									<Loader2 class="absolute inset-0 m-auto h-3 w-3 animate-spin" style="color: white" />
								{/if}
							</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- Data Export -->
	<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<h2 class="mb-2 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
			<Download class="h-5 w-5" style="color: var(--color-primary)" />
			Export My Data
		</h2>
		<p class="mb-4 text-sm" style="color: var(--color-muted)">
			Download a copy of all your personal data stored on JapaConnect. This may take up to 48 hours to process.
		</p>

		{#if exportSuccess}
			<div class="mb-3 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-success, #16a34a); background-color: color-mix(in srgb, var(--color-success, #16a34a) 8%, transparent); color: var(--color-success, #16a34a)">
				<CheckCircle2 class="h-4 w-4" />
				{exportSuccess}
			</div>
		{/if}
		{#if exportError}
			<div class="mb-3 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{exportError}
			</div>
		{/if}

		<button
			onclick={handleDataExport}
			disabled={exportLoading}
			class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border); color: var(--color-foreground)"
		>
			{#if exportLoading}
				<Loader2 class="h-4 w-4 animate-spin" />
				Processing...
			{:else}
				<Download class="h-4 w-4" />
				Request Data Export
			{/if}
		</button>
	</section>

	<!-- Account Deletion -->
	<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-danger); background: var(--color-surface);">
		<h2 class="mb-2 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-danger)">
			<Trash2 class="h-5 w-5" />
			Delete My Account
		</h2>
		<p class="mb-4 text-sm" style="color: var(--color-muted)">
			Permanently delete your account and all associated data. This action cannot be undone.
		</p>

		{#if !showDeleteConfirm}
			<button
				onclick={() => (showDeleteConfirm = true)}
				class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="border-color: var(--color-danger); color: var(--color-danger)"
			>
				Request Account Deletion
			</button>
		{:else}
			<div class="space-y-3">
				{#if deleteError}
					<div class="rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
						{deleteError}
					</div>
				{/if}
				<p class="text-sm" style="color: var(--color-danger)">
					Type <strong>DELETE</strong> to confirm:
				</p>
				<input
					type="text"
					bind:value={deleteConfirmText}
					placeholder="DELETE"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-danger)]"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
				<div class="flex gap-2">
					<button
						onclick={handleAccountDeletion}
						disabled={deleteLoading || deleteConfirmText !== 'DELETE'}
						class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-danger)"
					>
						{#if deleteLoading}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<Trash2 class="h-4 w-4" />
						{/if}
						Permanently Delete
					</button>
					<button
						onclick={() => { showDeleteConfirm = false; deleteConfirmText = ''; deleteError = ''; }}
						class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						Cancel
					</button>
				</div>
			</div>
		{/if}
	</section>
</main>
