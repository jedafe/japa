<script lang="ts">
	// /reviews — migrated to the EntityPage scaffold. Moderation entity (no create):
	// View (slide-over), Moderate (page-local Modal → PATCH status), Delete (confirm → DELETE).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { api, moderationAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Shield, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Review = {
		id: string;
		entity_type: string;
		entity_id: string;
		entity_name: string;
		rating: number;
		status: string;
		created_at: string;
	};

	const canModerate = $derived(hasPermission('reviews.moderate'));
	const canDelete = $derived(hasPermission('reviews.moderate'));

	// ── Page-local state ────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Load canonical review status and entity-type options from the moderation catalog (Rule 16).
	let reviewStatusOptions = $state<{ value: string; label: string }[]>([
		{ value: 'published', label: t('reviews.status_published', 'Published') },
		{ value: 'flagged', label: t('reviews.status_flagged', 'Flagged') },
		{ value: 'removed', label: t('reviews.status_removed', 'Removed') }
	]);
	let reviewEntityTypeOptions = $state<{ value: string; label: string }[]>([
		{ value: 'organization', label: t('reviews.type_organization', 'Organization') },
		{ value: 'university', label: t('reviews.type_university', 'University') },
		{ value: 'service', label: t('reviews.type_service', 'Service') },
		{ value: 'mentor', label: t('reviews.type_mentor', 'Mentor') }
	]);

	$effect(() => {
		void (async () => {
			const res = await moderationAdminApi.getOptions();
			if (res.ok && res.data) {
				if (res.data.statuses?.length) {
					reviewStatusOptions = res.data.statuses;
				}
				if (res.data.content_types?.length) {
					reviewEntityTypeOptions = res.data.content_types;
				}
			}
		})();
	});

	// Moderate state-action modal (PATCH status: published/flagged/removed)
	let showModerateModal = $state(false);
	let moderateTarget = $state<Review | null>(null);
	let moderateStatus = $state('published');
	let moderateReason = $state('');

	// Delete state-action confirm (DELETE — hard delete)
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Review | null>(null);

	function openModerate(review: Review) {
		moderateTarget = review;
		moderateStatus = review.status ?? 'published';
		moderateReason = '';
		showModerateModal = true;
	}

	function openDelete(review: Review) {
		deleteTarget = review;
		showDeleteConfirm = true;
	}

	async function confirmModerate() {
		if (!moderateTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await api(`/admin/reviews/${moderateTarget.id}`, {
				method: 'PATCH',
				body: { status: moderateStatus, reason: moderateReason || undefined }
			});
			if (result.ok) {
				showModerateModal = false;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await api(`/admin/reviews/${deleteTarget.id}`, { method: 'DELETE' });
			if (result.ok) {
				showDeleteConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Bulk moderate (set all selected to a status) ────────────────────────
	let bulkModerating = $state(false);

	async function bulkModerate(ids: Set<string>, status: string) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkModerating) return; // in-flight guard
		bulkModerating = true;
		pageError = '';
		try {
			await Promise.all(
				idArr.map((id) =>
					api(`/admin/reviews/${id}`, { method: 'PATCH', body: { status } })
				)
			);
			// EntityPage auto-reloads + clears selection after this returns.
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to moderate reviews');
		} finally {
			bulkModerating = false;
		}
	}

	async function bulkModeratePublished(ids: Set<string>) {
		await bulkModerate(ids, 'published');
	}
	async function bulkModerateFlagged(ids: Set<string>) {
		await bulkModerate(ids, 'flagged');
	}
	async function bulkModerateRemoved(ids: Set<string>) {
		await bulkModerate(ids, 'removed');
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults, so View/Moderate/Delete are included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canModerate) {
			acts.push({
				label: t('reviews.moderate', 'Moderate'),
				icon: Shield,
				onclick: (row: Review) => openModerate(row)
			});
		}
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Review) => openDelete(row)
			});
		}
		return acts;
	});

	// `fields` is required by EntityPage even on readonly pages (used for edit-form seeding).
	const fields = $derived<FieldDef[]>([
		{ key: 'status', label: t('reviews.status', 'Status'), type: 'select', options: reviewStatusOptions }
	]);

	// Config built as a $derived so rowActions/canModerate updates propagate (mirrors /users).
	const config = $derived<EntityPageConfig>({
		title: t('reviews.title', 'Reviews'),
		description: t('reviews.description', 'Moderate user-submitted reviews across the platform.'),
		readonly: true,
		deletePermission: 'reviews.moderate',
		pageSize: 25,
		columns: [
			{ key: 'entity_name', label: t('reviews.entity_name', 'Entity'), sortable: true },
			{ key: 'entity_type', label: t('reviews.entity_type', 'Entity Type'), sortable: true },
			{
				key: 'rating',
				label: t('reviews.rating', 'Rating'),
				sortable: true,
				render: (v: number) => `${v}/5`
			},
			{
				key: 'status',
				label: t('reviews.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const cls = v === 'published' ? 'bg-success/15 text-success' : v === 'flagged' ? 'bg-warning/15 text-warning' : 'bg-danger/15 text-danger';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('reviews.created', 'Created'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters: [
			{
				key: 'status',
				label: t('reviews.status', 'Status'),
				options: reviewStatusOptions
			},
			{
				key: 'entity_type',
				label: t('reviews.entity_type', 'Entity Type'),
				options: reviewEntityTypeOptions
			}
		],
		actions: rowActions,
		bulkActions: canModerate
			? [
					{ label: t('reviews.bulk_publish', 'Set Published'), onclick: bulkModeratePublished },
					{ label: t('reviews.bulk_flag', 'Set Flagged'), onclick: bulkModerateFlagged },
					{ label: t('reviews.bulk_remove', 'Set Removed'), variant: 'destructive', onclick: bulkModerateRemoved }
				]
			: undefined,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.status) params.set('status', filter.status);
			if (filter.entity_type) params.set('entity_type', filter.entity_type);
			if (q) params.set('q', q);
			const result = await api<{ reviews: Review[]; total: number }>(`/admin/reviews?${params.toString()}`);
			if (result.ok) {
				const reviews = result.data?.reviews ?? [];
				const total = result.data?.total ?? 0;
				return {
					data: reviews,
					total,
					totalPages: Math.max(1, Math.ceil(total / pageSize))
				};
			}
			throw new Error(result.error);
		},
		viewSnippet: ReviewView
	});
</script>

<svelte:head>
	<title>{t('reviews.title', 'Reviews')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Moderate state-action modal ───────────────────────────────────────── -->
<Modal bind:open={showModerateModal} title={t('reviews.moderate', 'Moderate Review')}>
	{#if moderateTarget}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">
				{t('reviews.moderate_help', 'Set the moderation status for this review.')}
			</p>
			<div>
				<label for="moderateStatus" class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('reviews.status', 'Status')}
				</label>
				<Select
					options={[
						{ value: 'published', label: t('reviews.status_published', 'Published') },
						{ value: 'flagged', label: t('reviews.status_flagged', 'Flagged') },
						{ value: 'removed', label: t('reviews.status_removed', 'Removed') }
					]}
					bind:value={moderateStatus}
				/>
			</div>
			<div>
				<label for="moderateReason" class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('reviews.moderate_reason', 'Reason (optional)')}
				</label>
				<textarea
					id="moderateReason"
					rows={2}
					bind:value={moderateReason}
					placeholder={t('reviews.moderate_reason_placeholder', 'Reason for this moderation action…')}
					class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				></textarea>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showModerateModal = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmModerate} disabled={acting}>
			{acting ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- ── Delete (hard delete) confirm modal ────────────────────────────────── -->
<Modal bind:open={showDeleteConfirm} title={t('reviews.hard_delete_title', 'Permanently Delete Review')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('reviews.hard_delete_confirm', 'This will permanently remove the review from the database. This action cannot be undone.')}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting…') : t('reviews.hard_delete', 'Hard Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet (slide-over body) ────────────────────────────────────── -->
{#snippet ReviewView({ row }: { row: Review })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('reviews.entity_type', 'Entity Type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.entity_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('reviews.entity_name', 'Entity')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.entity_name}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('reviews.rating', 'Rating')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.rating}/5</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('reviews.status', 'Status')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('reviews.created', 'Created')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{new Date(row.created_at).toLocaleString()}
			</p>
		</div>
	</div>
{/snippet}
