<script lang="ts">
	// /donation-settings — platform-level donation configuration (§125).
	// Singleton form over GET/PUT /admin/donation-settings: which methods the
	// platform accepts, minimum, thank-you message, and the platform's
	// Open Collective slug (rendered as an outbound option on /donate).
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { onMount } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { tipsAdminApi } from '$lib/api';
	import { Loader2, CheckCircle2, AlertCircle } from 'lucide-svelte';

	interface PlatformSettings {
		tips_enabled: boolean;
		accept_credits: boolean;
		accept_fiat: boolean;
		accept_external: boolean;
		min_tip_amount: number;
		thank_you_message?: string | null;
		oc_collective_slug?: string | null;
	}

	let settings = $state<PlatformSettings | null>(null);
	let links = $state<{ id?: string; kind: string; label: string; url: string }[]>([]);
	let linkLabel = $state('');
	let linkUrl = $state('');
	let linkKind = $state('paypal');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let saved = $state(false);

	onMount(async () => {
		const res = await tipsAdminApi.getDonationSettings();
		loading = false;
		if (!res.ok) {
			error = res.error;
			return;
		}
		const env = res.data as { settings?: PlatformSettings; external_links?: typeof links };
		links = env?.external_links ?? [];
		settings = env?.settings ?? {
			tips_enabled: false,
			accept_credits: true,
			accept_fiat: true,
			accept_external: false,
			min_tip_amount: 0,
			thank_you_message: null,
			oc_collective_slug: null
		};
	});

	async function save() {
		if (!settings) return;
		saving = true;
		error = null;
		saved = false;
		const res = await tipsAdminApi.updateDonationSettings({
			tips_enabled: settings.tips_enabled,
			accept_credits: settings.accept_credits,
			accept_fiat: settings.accept_fiat,
			accept_external: settings.accept_external,
			min_tip_amount: Number(settings.min_tip_amount) || 0,
			thank_you_message: settings.thank_you_message || null,
			oc_collective_slug: settings.oc_collective_slug ?? null
		});
		saving = false;
		if (!res.ok) {
			error = res.error;
			return;
		}
		saved = true;
		setTimeout(() => (saved = false), 2500);
	}

	async function addLink() {
		if (!linkLabel || !linkUrl) return;
		const res = await tipsAdminApi.createDonationLink({ kind: linkKind, label: linkLabel, url: linkUrl });
		if (!res.ok) {
			error = res.error ?? 'add_link_failed';
			return;
		}
		if (res.data) links = [...links, res.data as (typeof links)[number]];
		linkLabel = '';
		linkUrl = '';
	}

	async function removeLink(id: string) {
		const res = await tipsAdminApi.deleteDonationLink(id);
		if (res.ok) links = links.filter((l) => l.id !== id);
	}
</script>

<div class="mx-auto max-w-2xl p-6">
	<h1 class="mb-1 text-xl font-bold">{t('nav.donation_settings', 'Donation Settings')}</h1>
	<p class="mb-6 text-sm text-muted-foreground">
		{t('donation_settings.desc', 'Platform-level options for /donate — fiat, wallet and external donation methods.')}
	</p>

	{#if loading}
		<div class="flex justify-center py-10"><Loader2 class="size-6 animate-spin text-muted-foreground" /></div>
	{:else}
		{#if error}
			<div class="mb-3 flex items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
				<AlertCircle class="size-4" />{error}
			</div>
		{/if}
		{#if settings}
		{#if saved}
			<div class="mb-3 flex items-center gap-2 rounded-md bg-green-500/10 px-3 py-2 text-sm text-green-600">
				<CheckCircle2 class="size-4" />{t('common.saved', 'Saved')}
			</div>
		{/if}

		<div class="space-y-3">
			<label class="flex items-center justify-between gap-2 flex-wrap rounded-lg border border-border px-4 py-3">
				<span class="text-sm font-medium">{t('donation_settings.enabled', 'Accept platform donations')}</span>
				<input bind:checked={settings.tips_enabled} type="checkbox" class="size-4" />
			</label>

			<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
				<!-- The platform has no wallet — points/credits toggles are
				     meaningless here and are intentionally not offered. -->
				<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
					<input bind:checked={settings.accept_fiat} type="checkbox" class="size-4" />{t('tips.fiat', 'Fiat (card/crypto)')}
				</label>
				<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
					<input bind:checked={settings.accept_external} type="checkbox" class="size-4" />{t('tips.external', 'External links')}
				</label>
			</div>

			<label class="block rounded-lg border border-border px-4 py-3">
				<span class="mb-1 block text-sm font-medium">{t('donation_settings.min', 'Minimum (minor units)')}</span>
				<input bind:value={settings.min_tip_amount} type="number" min="0" step="1"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
			</label>

			<label class="block rounded-lg border border-border px-4 py-3">
				<span class="mb-1 block text-sm font-medium">{t('donation_settings.thank_you', 'Thank-you message')}</span>
				<textarea bind:value={settings.thank_you_message} rows="2" maxlength="500"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"></textarea>
			</label>

			<label class="block rounded-lg border border-border px-4 py-3">
				<span class="mb-1 block text-sm font-medium">{t('donation_settings.oc_slug', 'Open Collective slug')}</span>
				<input bind:value={settings.oc_collective_slug} type="text" placeholder="collective-slug" maxlength="100"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
				<span class="mt-1 block text-xs text-muted-foreground">
					{t('donation_settings.oc_hint', 'Shown as an outbound donation option. The API token is set via Control → Settings.')}
				</span>
			</label>

			<div class="rounded-lg border border-border p-4">
				<h3 class="mb-3 text-sm font-semibold">{t('tips.external', 'External donation links')}</h3>
				{#each links as link (link.id)}
					<div class="mb-2 flex items-center justify-between gap-2 flex-wrap rounded-md border border-border px-3 py-2 text-sm">
						<span class="truncate">{link.label} — {link.url}</span>
						<button type="button" onclick={() => removeLink(link.id!)} class="shrink-0 text-destructive hover:opacity-70">✕</button>
					</div>
				{/each}
				<div class="mt-3 flex flex-wrap gap-2">
					<select bind:value={linkKind} class="rounded-md border border-border bg-background px-2 py-2 text-sm">
						<option value="paypal">PayPal</option>
						<option value="kofi">Ko-fi</option>
						<option value="buymeacoffee">Buy Me a Coffee</option>
						<option value="opencollective">Open Collective</option>
						<option value="other">Other</option>
					</select>
					<input bind:value={linkLabel} placeholder="Label" class="w-32 rounded-md border border-border bg-background px-2 py-2 text-sm" />
					<input bind:value={linkUrl} placeholder="https://" class="flex-1 rounded-md border border-border bg-background px-2 py-2 text-sm" />
					<button type="button" onclick={addLink} class="rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground">+</button>
				</div>
			</div>

			<button type="button" onclick={save} disabled={saving}
				class="w-full rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90 disabled:opacity-50">
				{#if saving}<Loader2 class="inline size-4 animate-spin" />{:else}{t('common.save', 'Save')}{/if}
			</button>
		</div>
		{/if}
	{/if}
</div>
