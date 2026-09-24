<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { MoreVertical, Ban, Flag, Share2, Check, Pencil, Trash2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import ReportDialog from '$lib/components/profile/ReportDialog.svelte';
	import SourceEditDialog from './SourceEditDialog.svelte';
	import type { Source } from '$lib/types/sources';

	interface Props {
		source: Source;
		isBlocked?: boolean;
		onBlockChange?: (isBlocked: boolean) => void;
		onEdit?: (updated: Source) => void;
		onDelete?: () => void;
		/** If true, the Block/Unblock item is shown. */
		showBlock?: boolean;
		/** If true, a Delete source item is shown for privileged users. */
		showDelete?: boolean;
		/** If true, the Report item is shown. */
		showReport?: boolean;
		/** If true, the Share item is shown. Item cards set this false — they
		 *  already render an item-level ShareButton in the footer, so the
		 *  menu's source-URL share would be a confusing duplicate. */
		showShare?: boolean;
		/** Optional trigger button classes. */
		triggerClass?: string;
	}

	let {
		source,
		isBlocked = false,
		onBlockChange,
		onEdit,
		onDelete,
		showBlock = true,
		showDelete = false,
		showReport = true,
		showShare = true,
		triggerClass
	}: Props = $props();

	let blockLoading = $state(false);
	let blockError = $state('');
	let reportOpen = $state(false);
	let editOpen = $state(false);
	let deleteLoading = $state(false);
	let deleteError = $state('');
	let shared = $state(false);
	let open = $state(false);
	// svelte-ignore state_referenced_locally
	let userPerms = $state<string[]>(page.data.user?.permissions ?? []);
	// svelte-ignore state_referenced_locally
	let permsLoaded = $state(!!page.data.user?.permissions && page.data.user.permissions.length > 0);

	$effect(() => {
		if (permsLoaded || !page.data.isAuthed) return;
		permsLoaded = true;
		api.get<{ permissions: string[] }>('/me/permissions').then((res) => {
			if (res.data) {
				userPerms = res.data.permissions ?? [];
			}
		});
	});

	const canEdit = $derived(
		!!page.data.isAuthed &&
			(!!userPerms.includes('intel.sources.manage') ||
				!!page.data.user?.groups?.some((g) => g === 'super_admin' || g === 'Super Admin'))
	);

	const canDelete = $derived(canEdit);

	async function toggleBlock() {
		if (blockLoading) return;
		if (!(await requireAuthOrRedirect())) return;

		blockLoading = true;
		blockError = '';
		const method = isBlocked ? 'delete' : 'post';

		try {
			const res = await api[method]<{ is_blocked: boolean }>(`/sources/${source.slug}/block`);
			if (res.error) {
				blockError = res.error.message || t('sources.block.error', 'Block failed');
				open = false;
				return;
			}
			const next = res.data?.is_blocked ?? !isBlocked;
			onBlockChange?.(next);
			open = false;
		} catch (err) {
			blockError = err instanceof Error ? err.message : t('sources.block.error', 'Block failed');
		} finally {
			blockLoading = false;
		}
	}

	async function openReport() {
		if (!(await requireAuthOrRedirect())) return;
		reportOpen = true;
		open = false;
	}

	async function openEdit() {
		if (!(await requireAuthOrRedirect())) return;
		editOpen = true;
		open = false;
	}

	async function deleteSource() {
		if (deleteLoading) return;
		if (!(await requireAuthOrRedirect())) return;

		const confirmed = window.confirm(
			t('sources.delete.confirm', 'Delete this source permanently? This cannot be undone.')
		);
		if (!confirmed) {
			open = false;
			return;
		}

		deleteLoading = true;
		deleteError = '';
		try {
			const res = await api.delete<unknown>(`/intel/sources/${source.id}`);
			if (res.error) {
				deleteError = res.error.message || t('sources.delete.error', 'Delete failed');
				return;
			}
			onDelete?.();
			open = false;
			await goto('/sources');
		} catch (err) {
			deleteError = err instanceof Error ? err.message : t('sources.delete.error', 'Delete failed');
		} finally {
			deleteLoading = false;
		}
	}

	async function share() {
		const url = `${page.url.origin}/sources/${source.slug}`;
		try {
			if (navigator.share) {
				await navigator.share({ title: source.name, url });
			} else if (navigator.clipboard) {
				await navigator.clipboard.writeText(url);
				shared = true;
				setTimeout(() => (shared = false), 2000);
			}
		} catch {
			// user cancelled share or clipboard not available
		}
		open = false;
	}
</script>

<DropdownMenu.Root bind:open>
	<DropdownMenu.Trigger
		type="button"
		class={triggerClass ??
			"rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-[var(--color-text-muted)] transition hover:bg-[var(--color-surface-hover)]"}
		aria-label={t('sources.actions', 'Source actions')}
	>
		<MoreVertical class="h-4 w-4" />
	</DropdownMenu.Trigger>

	<DropdownMenu.Portal>
		<DropdownMenu.Content
			align="end"
			sideOffset={8}
			class="z-50 min-w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
		>
			{#if canEdit}
				<DropdownMenu.Item
					onSelect={openEdit}
					class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
				>
					<Pencil class="h-4 w-4" />
					{t('sources.edit.label', 'Edit')}
				</DropdownMenu.Item>
			{/if}

			{#if showDelete && canDelete}
				<DropdownMenu.Item
					disabled={deleteLoading}
					onSelect={deleteSource}
					class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-red-600 transition hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
				>
					<Trash2 class="h-4 w-4" />
					{deleteLoading ? t('sources.delete.loading', 'Deleting…') : t('sources.delete.label', 'Delete')}
				</DropdownMenu.Item>
			{/if}

			{#if showBlock}
				<DropdownMenu.Item
					disabled={blockLoading}
					onSelect={toggleBlock}
					class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
				>
					<Ban class="h-4 w-4" />
					{isBlocked ? t('sources.block.unblock', 'Unblock source') : t('sources.block.block', 'Block source')}
				</DropdownMenu.Item>
			{/if}

			{#if showReport}
				<DropdownMenu.Item
					onSelect={openReport}
					class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
				>
					<Flag class="h-4 w-4" />
					{t('sources.report', 'Report')}
				</DropdownMenu.Item>
			{/if}

			{#if showShare}
				<DropdownMenu.Item
					onSelect={share}
					class="flex w-full cursor-pointer items-center gap-2 rounded px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
				>
					{#if shared}
						<Check class="h-4 w-4 text-[var(--color-success)]" />
						{t('common.copied', 'Copied')}
					{:else}
						<Share2 class="h-4 w-4" />
						{t('common.share', 'Share')}
					{/if}
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>

{#if blockError}
	<p class="mt-1 text-xs text-red-600">{blockError}</p>
{/if}

{#if deleteError}
	<p class="mt-1 text-xs text-red-600">{deleteError}</p>
{/if}

<ReportDialog bind:open={reportOpen} entityType="source" entityId={source.slug} entityName={source.name} />
<SourceEditDialog bind:open={editOpen} {source} onSave={onEdit} />
