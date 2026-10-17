<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { contentAdminApi, settingsAdminApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { parseCardStyleValue, buildCardStyleValue, CARD_STYLE_VALUES, type ParsedCardStyle } from '$lib/feed/cardStyleValue';
	import { RefreshCw, Save, Undo2, Check, LayoutGrid } from 'lucide-svelte';

	const canManage = $derived(hasPermission('admin.settings'));

	type ContentType = { value: string; label: string };

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	let contentTypes = $state<ContentType[]>([]);
	let original = $state<ParsedCardStyle>({ default: 'reddit', byType: {} });
	let current = $state<ParsedCardStyle>({ default: 'reddit', byType: {} });

	let isDirty = $derived(
		current.default !== original.default ||
			JSON.stringify(current.byType) !== JSON.stringify(original.byType)
	);

	const useDefaultOption = { value: '', label: t('content_types.use_default', 'Use default') };
	const styleOptions = $derived(CARD_STYLE_VALUES.map((v) => ({ value: v, label: t(`content_types.style_${v}`, v) })));
	const defaultStyleOptions = $derived(CARD_STYLE_VALUES.map((v) => ({ value: v, label: t(`content_types.style_${v}`, v) })));

	async function load() {
		loading = true;
		error = '';
		messageType = '';
		try {
			const [ctRes, settingsRes] = await Promise.all([
				contentAdminApi.listContentTypes(),
				settingsAdminApi.list(),
			]);
			if (ctRes.ok) {
				contentTypes = (ctRes.data as ContentType[]) ?? [];
			}
			if (settingsRes.ok) {
				const cats = settingsRes.data as Array<{
					key: string;
					settings: Array<{ key: string; value: unknown }>;
				}>;
				const feedCat = cats.find((c) => c.key === 'feed');
				const cardStyleSetting = feedCat?.settings.find((s) => s.key === 'card_style');
				const parsed = parseCardStyleValue(cardStyleSetting?.value);
				original = { default: parsed.default, byType: { ...parsed.byType } };
				current = { default: parsed.default, byType: { ...parsed.byType } };
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	function setDefault(v: string) {
		current = { ...current, default: v };
	}

	function setTypeStyle(ct: string, v: string) {
		const nextByType = { ...current.byType };
		if (v === '') delete nextByType[ct];
		else nextByType[ct] = v;
		current = { ...current, byType: nextByType };
	}

	function revertAll() {
		current = { default: original.default, byType: { ...original.byType } };
		error = t('content_types.reverted', 'Changes reverted.');
		messageType = 'success';
	}

	async function save() {
		saving = true;
		error = '';
		messageType = '';
		try {
			const built = buildCardStyleValue(current.default, current.byType);
			const result = await settingsAdminApi.updateCategory('feed', { card_style: built });
			if (result.ok) {
				original = { default: current.default, byType: { ...current.byType } };
				error = t('content_types.saved', 'Card style mapping saved successfully.');
				messageType = 'success';
			} else {
				error = result.error ?? t('common.error', 'Failed to save');
				messageType = 'error';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to save');
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	load();
</script>

<svelte:head>
	<title>{t('content_types.title', 'Content Types')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('content_types.title', 'Content Types')}
	description={t('content_types.description', 'Assign a listing card style per content type')}
/>

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
		<span class="ml-2 text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</span>
	</div>
{:else}
	<!-- Message banner -->
	{#if error}
		<div
			class="mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm"
			style={messageType === 'success'
				? "border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
				: "border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"}
		>
			{#if messageType === 'success'}
				<Check class="h-4 w-4" />
			{/if}
			{error}
		</div>
	{/if}

	<!-- Default card style card -->
	<div class="mb-6 overflow-hidden rounded-xl border" style="border-color: var(--color-border)">
		<div class="flex items-center gap-3 border-b px-5 py-4" style="border-color: var(--color-border)">
			<LayoutGrid class="h-5 w-5" style="color: var(--color-primary)" />
			<div class="flex-1">
				<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
					{t('content_types.default_style', 'Default Card Style')}
				</h3>
				<p class="text-xs mt-0.5" style="color: var(--color-muted)">
					{t('content_types.default_style_desc', 'Applied to all content types that do not have an explicit override.')}
				</p>
			</div>
			<div class="w-48">
				<Select
					options={defaultStyleOptions}
					value={current.default}
					disabled={!canManage}
					onchange={(val: string) => setDefault(val)}
				/>
			</div>
		</div>
	</div>

	<!-- Per-content-type mapping table -->
	<div class="overflow-hidden rounded-xl border" style="border-color: var(--color-border)">
		<div class="border-b px-5 py-4" style="border-color: var(--color-border)">
			<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
				{t('content_types.per_type', 'Card Style per Content Type')}
			</h3>
			<p class="text-xs mt-0.5" style="color: var(--color-muted)">
				{t('content_types.per_type_desc', 'Override the default card style for individual content types.')}
			</p>
		</div>

		{#if contentTypes.length === 0}
			<div class="px-5 py-8 text-center">
				<p class="text-sm" style="color: var(--color-muted)">
					{t('content_types.no_types', 'No content types found.')}
				</p>
			</div>
		{:else}
			<div class="divide-y" style="border-color: var(--color-border)">
				{#each contentTypes as ct (ct.value)}
					{@const currentStyle = current.byType[ct.value] ?? ''}
					{@const isOverride = currentStyle !== ''}
					{@const wasOriginal = original.byType[ct.value] ?? ''}
					{@const dirty = currentStyle !== wasOriginal}
					<div
						class="flex items-center gap-4 px-5 py-3.5"
						style={dirty ? "background: color-mix(in srgb, var(--color-warning) 5%, transparent)" : ''}
					>
						<div class="flex-1">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium" style="color: var(--color-foreground)">{ct.label}</span>
								<span class="text-xs font-mono" style="color: var(--color-muted)">{ct.value}</span>
								{#if dirty}
									<span
										class="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
										style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)"
									>
										{t('settings.badge_dirty', 'modified')}
									</span>
								{/if}
							</div>
							<p class="text-xs mt-0.5" style="color: var(--color-muted)">
								{#if isOverride}
									{t('content_types.override', 'Override')}: {currentStyle}
								{:else}
									{t('content_types.using_default', 'Using default')}: {current.default}
								{/if}
							</p>
						</div>
						<div class="w-48">
							<Select
								options={[useDefaultOption, ...styleOptions]}
								value={currentStyle}
								disabled={!canManage}
								onchange={(val: string) => setTypeStyle(ct.value, val)}
							/>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Save / Revert bar -->
	{#if canManage}
		<div class="mt-4 flex items-center justify-end gap-3">
			<Button variant="outline" size="sm" onclick={revertAll} disabled={saving || !isDirty}>
				<Undo2 class="h-4 w-4" />
				{t('settings.revert', 'Revert')}
			</Button>
			<Button variant="default" size="sm" onclick={save} disabled={saving || !isDirty}>
				{#if saving}
					<RefreshCw class="h-4 w-4 animate-spin" />
					{t('common.saving', 'Saving...')}
				{:else}
					<Save class="h-4 w-4" />
					{t('content_types.save', 'Save Card Styles')}
				{/if}
			</Button>
		</div>
	{/if}
{/if}
