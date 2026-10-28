<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { Modal } from '$lib/components/ui';
	import { Pencil, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import type { Source } from '$lib/types/sources';

	interface Props {
		open: boolean;
		source: Source;
		onClose?: () => void;
		onSave?: (updated: Source) => void;
	}

	let { open = $bindable(), source, onClose, onSave }: Props = $props();

	let saving = $state(false);
	let error = $state('');
	// svelte-ignore state_referenced_locally
	let name = $state(source.name);
	// svelte-ignore state_referenced_locally
	let description = $state(source.description ?? '');
	// svelte-ignore state_referenced_locally
	let sourceUrl = $state(source.source_url ?? '');
	// svelte-ignore state_referenced_locally
	let logoUrl = $state(source.logo_url ?? '');

	$effect(() => {
		if (open) {
			name = source.name;
			description = source.description ?? '';
			sourceUrl = source.source_url ?? '';
			logoUrl = source.logo_url ?? '';
			saving = false;
			error = '';
		}
	});

	async function save() {
		if (saving) return;
		saving = true;
		error = '';

		const body: Record<string, unknown> = {};
		if (name !== source.name) body.name = name;
		if (description !== (source.description ?? '')) body.description = description || null;
		if (sourceUrl !== (source.source_url ?? '')) body.source_url = sourceUrl;
		if (logoUrl !== (source.logo_url ?? '')) body.logo_url = logoUrl || null;

		if (Object.keys(body).length === 0) {
			open = false;
			return;
		}

		const res = await api.patch<unknown>(`/intel/sources/${source.id}`, body);
		if (res.error) {
			error = res.error.message ?? t('sources.edit.error', 'Failed to save changes');
			saving = false;
			return;
		}

		// Fetch the public source to get enriched metadata after edit.
		const fresh = await api.get<Source>(`/sources/${source.slug}`);
		if (fresh.data) {
			onSave?.(fresh.data);
		}
		open = false;
	}

	function close() {
		open = false;
		onClose?.();
	}
</script>

<Modal bind:open onOpenChange={(v) => { if (!v) close(); }} icon={Pencil} title={t('sources.edit.title', 'Edit source')} size="md">
			<div class="flex flex-col gap-3">
				<div class="flex flex-col gap-1">
					<label for="source-name" class="text-sm font-medium text-[var(--color-text)]">{t('sources.edit.name', 'Name')}</label>
					<HintIcon text={t('sources.edit.name_hint', 'The official name of the source.')} />
					<input
						id="source-name"
						type="text"
						bind:value={name}
						class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					/>
				</div>

				<div class="flex flex-col gap-1">
					<label for="source-description" class="text-sm font-medium text-[var(--color-text)]">{t('sources.edit.description', 'Description')}</label>
					<HintIcon text={t('sources.edit.desc_hint', 'What this source covers.')} />
					<textarea
						id="source-description"
						bind:value={description}
						rows={3}
						class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					></textarea>
				</div>

				<div class="flex flex-col gap-1">
					<label for="source-url" class="text-sm font-medium text-[var(--color-text)]">{t('sources.edit.source_url', 'Source URL')}</label>
					<HintIcon text={t('sources.edit.url_hint', 'The main URL the source is fetched from.')} />
					<input
						id="source-url"
						type="url"
						bind:value={sourceUrl}
						class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					/>
					{#if sourceUrl !== (source.source_url ?? '')}
						<HintIcon text={t('sources.edit.reprobe_hint', 'Changing the URL re-runs channel detection for this source.')} />
					{/if}
				</div>

				<div class="flex flex-col gap-1">
					<label for="source-logo" class="text-sm font-medium text-[var(--color-text)]">{t('sources.edit.logo_url', 'Logo URL')}</label>
					<HintIcon text={t('sources.edit.logo_hint', 'Direct image URL for the source logo.')} />
					<input
						id="source-logo"
						type="url"
						bind:value={logoUrl}
						class="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					/>
				</div>

				{#if error}
					<p class="text-sm text-red-600">{error}</p>
				{/if}

				<div class="flex justify-end gap-2 pt-2">
					<button
						type="button"
						onclick={close}
						class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						onclick={save}
						disabled={saving}
						class="inline-flex items-center gap-2 rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if saving}
							<Loader2 size={16} class="animate-spin" />
						{/if}
						{t('common.save', 'Save')}
					</button>
				</div>
			</div>
</Modal>
