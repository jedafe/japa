<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import {
		Shield,
		Download,
		Trash2,
		ToggleLeft,
		ToggleRight,
		AlertTriangle,
		CheckCircle,
		Loader2,
		Info
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { formatTimestamp } from '$lib/utils/timezone';
	import { onMount } from 'svelte';

	let { data } = $props();

	// svelte-ignore state_referenced_locally
	let consents = $state(data.consents ?? []);
	// svelte-ignore state_referenced_locally
	let isAuthenticated = $state(data.isAuthenticated);

	let exporting = $state(false);
	let deactivating = $state(false);
	let showDeactivateConfirm = $state(false);
	let togglingConsent = $state<string | null>(null);
	let errorMessage = $state('');
	let successMessage = $state('');

	const consentLabels: Record<string, string> = {
		analytics: 'Analytics & Performance',
		marketing: 'Marketing Communications',
		personalization: 'Personalized Content',
		third_party: 'Third-Party Data Sharing',
		essential: 'Essential Platform Cookies'
	};

	function consentIcon(type: string) {
		switch (type) {
			case 'analytics': return '📊';
			case 'marketing': return '📧';
			case 'personalization': return '🎯';
			case 'third_party': return '🔗';
			case 'essential': return '🔒';
			default: return '⚙️';
		}
	}

	async function toggleConsent(consentId: string, currentGranted: boolean) {
		togglingConsent = consentId;
		errorMessage = '';
		successMessage = '';
		try {
			const consentType = consents.find((c) => c.id === consentId)?.consent_type ?? consentId;
			if (currentGranted) {
				await api.revokeConsent(consentType);
			} else {
				await api.updateConsent({ category: consentType, granted: true });
			}

			consents = consents.map((c) =>
				c.id === consentId ? { ...c, granted: !currentGranted } : c
			);
			successMessage = currentGranted ? 'Consent revoked.' : 'Consent granted.';
		} catch {
			errorMessage = 'Failed to update consent. Please try again.';
		} finally {
			togglingConsent = null;
		}
	}

	async function requestDataExport() {
		exporting = true;
		errorMessage = '';
		successMessage = '';
		try {
			await api.requestDataExport();
			successMessage = 'Data export request submitted. You will receive an email when ready.';
		} catch {
			errorMessage = 'Failed to request data export. Please try again.';
		} finally {
			exporting = false;
		}
	}

	async function confirmDeactivate() {
		deactivating = true;
		errorMessage = '';
		try {
			await api.requestAccountDeletion();
			successMessage = 'Account deactivation request submitted. Your account will be reviewed.';
			showDeactivateConfirm = false;
		} catch {
			errorMessage = 'Failed to submit deactivation request. Please try again.';
		} finally {
			deactivating = false;
		}
	}

	function clearMessages() {
		errorMessage = '';
		successMessage = '';
	}
</script>

<svelte:head>
	<title>Privacy & Data | JapaConnect</title>
	<meta name="description" content="Manage your privacy settings, consents, and data on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<!-- Header -->
	<header class="mb-8">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<Shield class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
					Privacy & Data
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					Manage your consents, data, and account.
				</p>
			</div>
		</div>
	</header>

	{#if !isAuthenticated}
		<div
			class="rounded-xl border p-6 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<Shield class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				Sign in to manage your privacy settings and data.
			</p>
			<a
				href="/login"
				class="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				Sign in
			</a>
		</div>
	{:else}
		<!-- Messages -->
		{#if errorMessage}
			<div
				class="mb-6 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				<AlertTriangle class="h-4 w-4 shrink-0" />
				{errorMessage}
				<button onclick={clearMessages} class="ml-auto opacity-60 hover:opacity-100">&times;</button>
			</div>
		{/if}
		{#if successMessage}
			<div
				class="mb-6 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-success, #22c55e); background: color-mix(in srgb, var(--color-success, #22c55e) 8%, transparent); color: var(--color-success, #22c55e)"
			>
				<CheckCircle class="h-4 w-4 shrink-0" />
				{successMessage}
				<button onclick={clearMessages} class="ml-auto opacity-60 hover:opacity-100">&times;</button>
			</div>
		{/if}

	<!-- Consent Management -->
		<section class="mb-8">
			<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<ToggleRight class="h-5 w-5" style="color: var(--color-primary)" />
				Consent Preferences
			</h2>
			{#if consents.length === 0}
				<EmptyState
					icon={Info}
					title="No consent records"
					message="Your consent preferences will appear here once you interact with platform features."
				/>
			{:else}
				<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)">
					{#each consents as consent (consent.id)}
						<div class="flex items-center justify-between gap-4 p-4 transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]">
							<div class="flex items-center gap-3 min-w-0">
								<span class="text-lg">{consentIcon(consent.consent_type)}</span>
								<div class="min-w-0">
									<p class="text-sm font-medium" style="color: var(--color-foreground)">
										{consentLabels[consent.consent_type] ?? consent.consent_type}
									</p>
									<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
										{consent.granted ? 'Granted' : 'Not granted'}
										&middot; {formatTimestamp(consent.updated_at)}
									</p>
								</div>
							</div>
							<button
								onclick={() => toggleConsent(consent.id, consent.granted)}
								disabled={togglingConsent === consent.id || consent.consent_type === 'essential'}
								class="shrink-0 rounded-lg p-2 transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-40"
								aria-label={consent.granted ? 'Revoke consent' : 'Grant consent'}
							>
								{#if togglingConsent === consent.id}
									<Loader2 class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
								{:else if consent.granted}
									<ToggleRight class="h-5 w-5" style="color: var(--color-primary)" />
								{:else}
									<ToggleLeft class="h-5 w-5" style="color: var(--color-muted)" />
								{/if}
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</section>

	<!-- Data Export -->
		<section class="mb-8">
			<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Download class="h-5 w-5" style="color: var(--color-primary)" />
				Data Export
			</h2>
			<div
				class="rounded-xl border p-5"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<p class="text-sm" style="color: var(--color-muted)">
					Request a copy of all personal data we hold about you. The export will be prepared and sent to your registered email address.
				</p>
				<button
					onclick={requestDataExport}
					disabled={exporting}
					class="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if exporting}
						<Loader2 class="h-4 w-4 animate-spin" />
						Requesting...
					{:else}
						<Download class="h-4 w-4" />
						Request Data Export
					{/if}
				</button>
			</div>
		</section>

		<!-- Account Deactivation -->
		<section>
			<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Trash2 class="h-5 w-5" style="color: var(--color-danger)" />
				Account Deactivation
			</h2>
			<div
				class="rounded-xl border p-5"
				style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 4%, transparent)"
			>
				<p class="text-sm" style="color: var(--color-muted)">
					Deactivating your account will hide your profile and content from other users. You can contact support to reactivate within 30 days.
				</p>
				{#if showDeactivateConfirm}
					<div class="mt-4 flex items-center gap-3">
						<button
							onclick={confirmDeactivate}
							disabled={deactivating}
							class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
							style="background-color: var(--color-danger)"
						>
							{#if deactivating}
								<Loader2 class="h-4 w-4 animate-spin" />
								Processing...
							{:else}
								Yes, deactivate my account
							{/if}
						</button>
						<button
							onclick={() => (showDeactivateConfirm = false)}
							class="rounded-lg px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="color: var(--color-muted)"
						>
							Cancel
						</button>
					</div>
				{:else}
					<button
						onclick={() => (showDeactivateConfirm = true)}
						class="mt-4 inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						style="border-color: var(--color-danger); color: var(--color-danger)"
					>
						<Trash2 class="h-4 w-4" />
						Deactivate Account
					</button>
				{/if}
			</div>
		</section>
	{/if}
</main>
