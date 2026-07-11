<script lang="ts">
	import { api, type Webhook, type WebhookDelivery } from '$lib/api';
	import {
		Webhook as WebhookIcon,
		Plus,
		Trash2,
		Eye,
		AlertTriangle,
		Loader2,
		X,
		CheckCircle2,
		XCircle,
		Clock,
		Key
	} from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let { data } = $props();

	let webhooks = $derived(data.webhooks);
	let showCreate = $state(false);
	let errorMessage = $state('');
	let creating = $state(false);

	// Create form state
	let newUrl = $state('');
	let newSecret = $state('');
	let selectedEvents = $state<string[]>([]);

	// Deliveries modal state
	let viewingWebhook = $state<Webhook | null>(null);
	let deliveries = $state<WebhookDelivery[]>([]);
	let deliveriesLoading = $state(false);

	let availableEvents = $state<{ key: string; label: string; description: string }[]>([]);

	onMount(async () => {
		try {
			const events = await api.listWebhookEvents();
			if (Array.isArray(events)) {
				availableEvents = events;
			}
		} catch {
			availableEvents = [
				{ key: 'content.published', label: 'Content Published', description: 'When content is published' },
				{ key: 'content.updated', label: 'Content Updated', description: 'When content is updated' },
				{ key: 'content.deleted', label: 'Content Deleted', description: 'When content is deleted' },
				{ key: 'claim.approved', label: 'Claim Approved', description: 'When an ownership claim is approved' },
				{ key: 'claim.rejected', label: 'Claim Rejected', description: 'When an ownership claim is rejected' },
				{ key: 'review.created', label: 'Review Created', description: 'When a new review is created' },
				{ key: 'organization.verified', label: 'Organization Verified', description: 'When an organization is verified' },
				{ key: 'payment.succeeded', label: 'Payment Succeeded', description: 'When a payment succeeds' },
				{ key: 'payment.failed', label: 'Payment Failed', description: 'When a payment fails' }
			];
		}
	});

	async function loadWebhooks() {
		errorMessage = '';
		try {
			webhooks = await api.listWebhooks();
		} catch {
			errorMessage = t('webhooks.load_error');
		}
	}

	async function createWebhook() {
		if (!newUrl.trim() || selectedEvents.length === 0) return;
		creating = true;
		errorMessage = '';
		try {
			await api.createWebhook({
				url: newUrl.trim(),
				events: selectedEvents,
				secret: newSecret.trim() || undefined
			});
			showCreate = false;
			newUrl = '';
			newSecret = '';
			selectedEvents = [];
			await loadWebhooks();
		} catch {
			errorMessage = t('webhooks.create_error');
		} finally {
			creating = false;
		}
	}

	async function deleteWebhook(id: string) {
		if (!confirm(t('webhooks.delete_confirm'))) return;
		errorMessage = '';
		try {
			await api.deleteWebhook(id);
			webhooks = webhooks.filter((w) => w.id !== id);
		} catch {
			errorMessage = t('webhooks.delete_error');
		}
	}

	async function viewDeliveries(webhook: Webhook) {
		viewingWebhook = webhook;
		deliveriesLoading = true;
		deliveries = [];
		try {
			deliveries = await api.getWebhookDeliveries(webhook.id);
		} catch {
			errorMessage = t('webhooks.deliveries_error');
		} finally {
			deliveriesLoading = false;
		}
	}

	function toggleEvent(event: string) {
		if (selectedEvents.includes(event)) {
			selectedEvents = selectedEvents.filter((e) => e !== event);
		} else {
			selectedEvents = [...selectedEvents, event];
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function truncate(str: string | undefined, len: number): string {
		if (!str) return '';
		return str.length > len ? str.slice(0, len) + '...' : str;
	}
</script>

<svelte:head>
	<title>{t('webhooks.title')} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<WebhookIcon class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
					{t('webhooks.title')}
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					{t('webhooks.subtitle')}
				</p>
			</div>
		</div>
	</header>

	<nav class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border); background: var(--color-surface)">
		<a
			href="/dashboard/developer"
			class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-muted)"
		>
			<Key class="h-3.5 w-3.5" />
			API Keys
		</a>
		<a
			href="/dashboard/webhooks"
			class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium"
			style="background-color: var(--color-primary); color: white"
		>
			<WebhookIcon class="h-3.5 w-3.5" />
			{t('webhooks.title')}
		</a>
	</nav>

	{#if errorMessage}
		<div
			class="mb-6 flex items-center gap-2 rounded-lg border p-3 text-sm"
			style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
		>
			<AlertTriangle class="h-4 w-4 flex-shrink-0" />
			{errorMessage}
		</div>
	{/if}

	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('webhooks.my_webhooks')}</h2>
		<button
			class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
			onclick={() => (showCreate = true)}
		>
			<Plus class="h-4 w-4" />
			{t('webhooks.add')}
		</button>
	</div>

	{#if webhooks.length === 0}
		<div
			class="mt-4 rounded-xl border p-8 text-center"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<WebhookIcon class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t('webhooks.empty')}. {t('webhooks.empty_hint')}
			</p>
		</div>
	{:else}
		<div class="mt-4 space-y-3">
			{#each webhooks as webhook (webhook.id)}
				<div
					class="rounded-xl border p-4 transition-all hover:shadow-sm"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<p
									class="truncate font-mono text-sm font-medium"
									style="color: var(--color-foreground)"
								>
									{webhook.url}
								</p>
								<span
									class="rounded-full px-2 py-0.5 text-xs font-medium"
									style="background: color-mix(in srgb, {webhook.status === 'active'
										? '#22c55e'
										: '#ef4444'} 12%, transparent); color: {webhook.status === 'active'
										? '#16a34a'
										: '#dc2626'}"
								>
									{webhook.status}
								</span>
							</div>
							<div class="mt-2 flex flex-wrap gap-1">
								{#each webhook.events as event}
									<span
										class="rounded px-1.5 py-0.5 text-xs"
										style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)"
									>
										{event}
									</span>
								{/each}
							</div>
							<p class="mt-2 text-xs" style="color: var(--color-muted)">
								{t('webhooks.created')} {formatDate(webhook.created_at)}
							</p>
						</div>
						<div class="flex shrink-0 items-center gap-1">
							<button
								class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-primary)"
								onclick={() => viewDeliveries(webhook)}
							>
								<Eye class="h-3.5 w-3.5" />
								{t('webhooks.deliveries')}
							</button>
							<button
								class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-danger)"
								onclick={() => deleteWebhook(webhook.id)}
							>
								<Trash2 class="h-3.5 w-3.5" />
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</main>

<!-- Create Webhook Modal -->
{#if showCreate}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="mx-4 max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-xl p-6 shadow-xl"
			style="background: var(--color-surface); border: 1px solid var(--color-border)"
		>
			<div class="flex items-center justify-between">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
					{t('webhooks.add')}
				</h3>
				<button
					class="rounded-lg p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={() => (showCreate = false)}
				>
					<X class="h-5 w-5" style="color: var(--color-muted)" />
				</button>
			</div>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				{t('webhooks.subtitle')}
			</p>

			<div class="mt-4 space-y-4">
				<div>
					<label for="webhook-url" class="text-xs font-medium" style="color: var(--color-foreground)"
						>{t('webhooks.endpoint_url')}</label
					>
					<input
						id="webhook-url"
						bind:value={newUrl}
						placeholder="https://example.com/webhook"
						type="url"
						class="mt-1 w-full rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
				</div>

				<div>
					<label for="webhook-secret" class="text-xs font-medium" style="color: var(--color-foreground)"
						>{t('webhooks.secret')}</label
					>
					<input
						id="webhook-secret"
						bind:value={newSecret}
						placeholder={t('webhooks.secret_hint')}
						type="text"
						class="mt-1 w-full rounded-lg border px-3 py-2 text-sm outline-none transition-colors focus:ring-2"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
					/>
				</div>

				<div>
					<label for="webhook-events" class="text-xs font-medium" style="color: var(--color-foreground)"
						>{t('webhooks.events')}</label
					>
					<div id="webhook-events" class="mt-2 flex flex-wrap gap-2" role="group" aria-label={t('webhooks.events')}>
						{#each availableEvents as event}
							<button
								type="button"
								class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
								style="background: {selectedEvents.includes(event.key)
									? 'var(--color-primary)'
									: 'var(--color-background)'}; color: {selectedEvents.includes(event.key)
									? 'white'
									: 'var(--color-foreground)'}; border: 1px solid {selectedEvents.includes(event.key)
									? 'transparent'
									: 'var(--color-border)'}"
								onclick={() => toggleEvent(event.key)}
								title={event.description}
							>
								{event.label}
							</button>
						{/each}
					</div>
					{#if selectedEvents.length === 0}
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{t('webhooks.events_hint')}
						</p>
					{/if}
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-lg px-4 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-muted)"
					onclick={() => (showCreate = false)}
				>
					{t('webhooks.cancel')}
				</button>
				<button
					class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={creating || !newUrl.trim() || selectedEvents.length === 0}
					onclick={createWebhook}
				>
					{#if creating}
						<Loader2 class="h-4 w-4 animate-spin" />
					{/if}
					{creating ? t('webhooks.creating') : t('webhooks.create')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Deliveries Modal -->
{#if viewingWebhook}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="mx-4 max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-xl p-6 shadow-xl"
			style="background: var(--color-surface); border: 1px solid var(--color-border)"
		>
			<div class="flex items-center justify-between">
				<div>
					<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
						{t('webhooks.delivery_log')}
					</h3>
					<p
						class="mt-0.5 truncate font-mono text-xs"
						style="color: var(--color-muted)"
					>
						{viewingWebhook.url}
					</p>
				</div>
				<button
					class="rounded-lg p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={() => (viewingWebhook = null)}
				>
					<X class="h-5 w-5" style="color: var(--color-muted)" />
				</button>
			</div>

			{#if deliveriesLoading}
				<div class="mt-4 space-y-3">
					{#each Array(5) as _}
						<div
							class="h-16 animate-pulse rounded-lg"
							style="background-color: var(--color-background)"
						></div>
					{/each}
				</div>
			{:else if deliveries.length === 0}
				<div class="mt-6 rounded-lg border p-6 text-center" style="border-color: var(--color-border)">
					<Clock class="mx-auto h-6 w-6" style="color: var(--color-muted)" />
					<p class="mt-2 text-sm" style="color: var(--color-muted)">
						{t('webhooks.no_deliveries')}
					</p>
				</div>
			{:else}
				<div class="mt-4 space-y-2">
					{#each deliveries as delivery (delivery.id)}
						<div
							class="rounded-lg border p-3"
							style="border-color: var(--color-border); background: var(--color-background)"
						>
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2">
									{#if delivery.status_code >= 200 && delivery.status_code < 300}
										<CheckCircle2 class="h-4 w-4 shrink-0" style="color: #16a34a" />
									{:else}
										<XCircle class="h-4 w-4 shrink-0" style="color: #dc2626" />
									{/if}
									<span class="text-xs font-medium" style="color: var(--color-foreground)">
										{delivery.event_type}
									</span>
								</div>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span class="font-mono">{delivery.status_code}</span>
									<span>&middot;</span>
									<span>{formatDate(delivery.delivered_at)}</span>
								</div>
							</div>
							{#if delivery.response_body}
								<pre
									class="mt-2 overflow-x-auto rounded px-2 py-1 text-xs"
									style="background: var(--color-surface); color: var(--color-muted)"
								>{truncate(delivery.response_body, 200)}</pre>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			<div class="mt-4 flex justify-end">
				<button
					class="rounded-lg px-4 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-muted)"
					onclick={() => (viewingWebhook = null)}
				>
					{t('webhooks.close')}
				</button>
			</div>
		</div>
	</div>
{/if}
