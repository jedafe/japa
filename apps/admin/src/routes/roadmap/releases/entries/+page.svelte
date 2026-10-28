<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /roadmap/releases/entries — commit-entry triage queue (§113, ADR-135).
	// Entries land via CI ingest (roadmap.entries.ingest) into the
	// unreleased queue; admins retype/retitle/relink to promote 'other'
	// commits onto public release pages, or delete noise.
	import { EntityPage, type EntityPageConfig } from '$lib/components/entity';
	import { roadmapEntriesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import type { FilterDef } from '$lib/components/ui/data-table';

	type ReleaseEntry = {
		id: string;
		release_id: string | null;
		item_id: string | null;
		commit_sha: string | null;
		commit_url: string | null;
		commit_type: string;
		scope: string | null;
		is_breaking: boolean;
		title: string;
		author_name: string | null;
		committed_at: string | null;
	};

	const TYPE_OPTIONS = [
		{ value: 'feature', label: t('roadmap.type_feature', 'Features') },
		{ value: 'fix', label: t('roadmap.type_fix', 'Fixes') },
		{ value: 'ui', label: t('roadmap.type_ui', 'UI') },
		{ value: 'performance', label: t('roadmap.type_performance', 'Performance') },
		{ value: 'security', label: t('roadmap.type_security', 'Security') },
		{ value: 'translation', label: t('roadmap.type_translation', 'Translations') },
		{ value: 'accessibility', label: t('roadmap.type_accessibility', 'Accessibility') },
		{ value: 'docs', label: t('roadmap.type_docs', 'Docs') },
		{ value: 'chore', label: t('roadmap.type_chore', 'Chores') },
		{ value: 'refactor', label: t('roadmap.type_refactor', 'Refactors') },
		{ value: 'test', label: t('roadmap.type_test', 'Tests') },
		{ value: 'ci', label: t('roadmap.type_ci', 'CI') },
		{ value: 'other', label: t('roadmap.type_other', 'Other') }
	];

	const QUEUE_OPTIONS = [
		{ value: 'true', label: t('roadmap.entries.unreleased', 'Unreleased') },
		{ value: 'false', label: t('roadmap.entries.all', 'All entries') }
	];

	const filterDefs: FilterDef[] = [
		{ key: 'unreleased', label: t('roadmap.entries.queue', 'Queue'), options: QUEUE_OPTIONS },
		{ key: 'type', label: t('roadmap.entries.type', 'Type'), options: TYPE_OPTIONS }
	];

	const config: EntityPageConfig = {
		title: t('roadmap.entries.title', 'Changelog Entries'),
		description: t('roadmap.entries.desc', 'Commit-sourced release entries — retype or retitle to publish'),
		columns: [
			{ key: 'title', label: t('common.title', 'Title') },
			{
				key: 'commit_type',
				label: t('roadmap.entries.type', 'Type'),
				render: (v: string) => v
			},
			{
				key: 'scope',
				label: t('roadmap.entries.scope', 'Scope'),
				render: (v: string | null) => v ?? `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'author_name',
				label: t('roadmap.entries.author', 'Author'),
				render: (v: string | null) => v ?? `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'committed_at',
				label: t('roadmap.entries.committed', 'Committed'),
				sortable: true,
				render: (v: string | null) =>
					v ? new Date(v).toLocaleDateString() : `<span style="color: var(--color-muted)">—</span>`
			}
		],
		filters: filterDefs,
		fields: [
			{ key: 'title', label: t('common.title', 'Title'), type: 'text', help: t('common.title_hint', 'The title shown in listings and detail views.'), required: true },
			{
				key: 'commit_type',
				label: t('roadmap.entries.type', 'Type'),
				type: 'select',
			help: t('roadmap.entries.commit_type_hint', 'Conventional-commit type — e.g. feat, fix.'),
				options: TYPE_OPTIONS
			},
			{ key: 'scope', label: t('roadmap.entries.scope', 'Scope'), type: 'text', help: t('roadmap.entries.scope_hint', 'What this permission or key can access.'),},
			{ key: 'item_id', label: t('roadmap.entries.item_id', 'Roadmap item ID'), type: 'text', help: t('roadmap.entries.item_id_hint', 'The item this is linked to.') }
		],
		hideCreate: true, // entries come from commit ingest only — no manual create
		editPermission: 'roadmap.edit',
		deletePermission: 'roadmap.edit',
		load: async ({ page, pageSize, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String((page - 1) * pageSize)
			});
			if (filter?.unreleased === 'true') params.set('unreleased', 'true');
			if (filter?.type) params.set('type', filter.type);
			const res = await roadmapEntriesAdminApi.list(params.toString());
			if (!res.ok) return { data: [], total: 0, totalPages: 0 };
			const d = res.data as { entries: ReleaseEntry[]; total: number; total_pages: number };
			return { data: d.entries ?? [], total: d.total, totalPages: d.total_pages };
		},
		update: async (id, body) => {
			const res = await roadmapEntriesAdminApi.update(id as string, body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await roadmapEntriesAdminApi.delete(id as string);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: () => false // entries are hard-deleted — no deleted_at signal
	};
</script>

<svelte:head>
	<title>{t('roadmap.entries.title', 'Changelog Entries')} — {siteName()}</title>
</svelte:head>

<EntityPage {config} />
