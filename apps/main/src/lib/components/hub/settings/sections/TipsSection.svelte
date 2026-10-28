<script lang="ts">
	// Tips settings section (§125) — accept-methods toggles, minimum amount,
	// thank-you message, external links CRUD, Open Collective connect.
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import {
		getMyTipSettings, updateMyTipSettings, createTipLink, deleteTipLink,
		connectOC, disconnectOC,
		type TipSettings, type TipExternalLink
	} from '$lib/api/tips';
	import { CheckCircle2, ExternalLink, Loader2, Plus, Trash2 } from 'lucide-svelte';
	import { SelectField } from '$lib/components/ui';

	let { data }: { data?: unknown } = $props();

	let settings = $state<TipSettings | null>(null);
	let links = $state<TipExternalLink[]>([]);
	let loading = $state(true);
	let saving = $state(false);
	let saved = $state(false);
	let error = $state<string | null>(null);

	let linkLabel = $state('');
	let linkUrl = $state('');
	let linkKind = $state<TipExternalLink['kind']>('paypal');
	let ocSlug = $state('');
	let ocToken = $state('');
	let ocBusy = $state(false);

	onMount(async () => {
		const res = await getMyTipSettings();
		loading = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? 'load_failed';
			return;
		}
		settings = res.data.settings;
		links = res.data.external_links ?? [];
		ocSlug = settings?.oc_collective_slug ?? '';
	});

	async function save() {
		if (!settings) return;
		saving = true;
		error = null;
		saved = false;
		const res = await updateMyTipSettings({
			tips_enabled: settings.tips_enabled,
			accept_points: settings.accept_points,
			accept_credits: settings.accept_credits,
			accept_fiat: settings.accept_fiat,
			accept_external: settings.accept_external,
			min_tip_amount: settings.min_tip_amount,
			thank_you_message: settings.thank_you_message
		});
		saving = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		settings = res.data ?? settings;
		saved = true;
		setTimeout(() => (saved = false), 2000);
	}

	async function addLink() {
		if (!linkLabel || !linkUrl) return;
		const res = await createTipLink({ kind: linkKind, label: linkLabel, url: linkUrl });
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) links = [...links, res.data];
		linkLabel = '';
		linkUrl = '';
	}

	async function removeLink(id: string) {
		const res = await deleteTipLink(id);
		if (!res.error) links = links.filter((l) => l.id !== id);
	}

	async function ocConnect() {
		if (!ocSlug) return;
		ocBusy = true;
		const res = await connectOC({ collective_slug: ocSlug, api_token: ocToken || undefined });
		ocBusy = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) settings = res.data;
		ocToken = '';
	}

	async function ocDisconnect() {
		ocBusy = true;
		await disconnectOC();
		ocBusy = false;
		if (settings) {
			settings.oc_collective_slug = null;
			settings.has_oc_token = false;
			settings.oc_webhook_url = null;
		}
	}
</script>

{#if loading}
	<div class="flex justify-center py-8"><Loader2 class="size-6 animate-spin text-muted-foreground" /></div>
{:else if settings}
	<div class="space-y-6">
		{#if error}
			<div class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>
		{/if}
		{#if saved}
			<div class="flex items-center gap-2 rounded-md bg-green-500/10 px-3 py-2 text-sm text-green-600">
				<CheckCircle2 class="size-4" />{t('tips.saved')}
			</div>
		{/if}

		<div class="flex items-center gap-4">
			<a href="/me/tips" class="inline-flex items-center gap-1 text-sm text-primary hover:underline">
				{t('tips.view_history', 'View tip history')} →
			</a>
			<a href="/donate" class="inline-flex items-center gap-1 text-sm text-primary hover:underline">
				{t('tips.support_platform', 'Support the platform')} →
			</a>
		</div>

		<label class="flex items-center justify-between rounded-lg border border-border px-4 py-3">
			<span class="text-sm font-medium">{t('tips.settings_enabled')}</span>
			<input bind:checked={settings.tips_enabled} type="checkbox" class="size-4" onchange={save} />
		</label>

		<div class="grid grid-cols-2 gap-3">
			<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
				<input bind:checked={settings.accept_points} type="checkbox" class="size-4" onchange={save} />{t('tips.points')}
			</label>
			<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
				<input bind:checked={settings.accept_credits} type="checkbox" class="size-4" onchange={save} />{t('tips.credits')}
			</label>
			<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
				<input bind:checked={settings.accept_fiat} type="checkbox" class="size-4" onchange={save} />{t('tips.fiat')}
			</label>
			<label class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
				<input bind:checked={settings.accept_external} type="checkbox" class="size-4" onchange={save} />{t('tips.external_links')}
			</label>
		</div>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">{t('tips.min_amount')}</span>
			<input bind:value={settings.min_tip_amount} type="number" min="0" onchange={save}
				class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">{t('tips.thank_you')}</span>
			<input bind:value={settings.thank_you_message} type="text" maxlength="200" onchange={save}
				class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
		</label>

		<div class="rounded-lg border border-border p-4">
			<h3 class="mb-3 text-sm font-semibold">{t('tips.external_links')}</h3>
			{#each links as link (link.id)}
				<div class="mb-2 flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm">
					<span class="flex items-center gap-2"><ExternalLink class="size-4" />{link.label} — {link.url}</span>
					<button type="button" onclick={() => removeLink(link.id!)} class="text-destructive hover:opacity-70">
						<Trash2 class="size-4" />
					</button>
				</div>
			{/each}
			<div class="mt-3 flex flex-wrap gap-2">
				<SelectField
					value={linkKind}
					onValueChange={(v) => (linkKind = v as TipExternalLink['kind'])}
					options={[
						{ value: 'paypal', label: 'PayPal' },
						{ value: 'kofi', label: 'Ko-fi' },
						{ value: 'buymeacoffee', label: 'Buy Me a Coffee' },
						{ value: 'opencollective', label: 'Open Collective' },
						{ value: 'other', label: 'Other' }
					]}
					class="sm:w-44"
				/>
				<input bind:value={linkLabel} placeholder={t('tips.link_label')} class="w-32 rounded-md border border-border bg-background px-2 py-2 text-sm" />
				<input bind:value={linkUrl} placeholder="https://" class="flex-1 rounded-md border border-border bg-background px-2 py-2 text-sm" />
				<button type="button" onclick={addLink} class="rounded-md bg-primary px-3 py-2 text-primary-foreground"><Plus class="size-4" /></button>
			</div>
		</div>

		<div class="rounded-lg border border-border p-4">
			<h3 class="mb-3 text-sm font-semibold">Open Collective</h3>
			{#if settings.oc_collective_slug}
				<div class="mb-3 flex items-center justify-between text-sm">
					<span>{settings.oc_collective_slug}{settings.has_oc_token ? ` — ${t('tips.oc_token_stored')}` : ''}</span>
					<button type="button" onclick={ocDisconnect} disabled={ocBusy} class="text-destructive hover:opacity-70">{t('tips.oc_disconnect')}</button>
				</div>
				{#if settings.oc_webhook_url}
					<div class="mb-2 flex items-center gap-2 rounded bg-muted px-2 py-1 text-xs text-muted-foreground">
						<span class="min-w-0 flex-1 truncate">{t('tips.oc_webhook')}: …{settings.oc_webhook_url.slice(-12)}</span>
						<button type="button" class="shrink-0 text-primary hover:opacity-70"
							onclick={() => navigator.clipboard.writeText(settings?.oc_webhook_url ?? '')}>
							{t('tips.oc_webhook_copy', 'Copy')}
						</button>
					</div>
				{/if}
			{/if}
			<div class="flex gap-2">
				<input bind:value={ocSlug} placeholder="collective-slug" class="flex-1 rounded-md border border-border bg-background px-2 py-2 text-sm" />
				<input bind:value={ocToken} type="password" placeholder={t('tips.oc_token')} class="flex-1 rounded-md border border-border bg-background px-2 py-2 text-sm" />
				<button type="button" onclick={ocConnect} disabled={ocBusy || !ocSlug}
					class="rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground disabled:opacity-50">
					{t('tips.oc_connect')}
				</button>
			</div>
		</div>
	</div>
{/if}
