<script lang="ts">
	// /bots — migrated to the EntityPage scaffold; declarative CRUD entity with
	// is_system show-predicate on delete and bot_type/policy filters.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { botsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2, Eye, Pencil } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Bot = {
		id: string;
		name: string;
		user_agent_pattern: string;
		bot_type: string;
		policy: string;
		is_system: boolean;
		crawl_delay: number;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	const canManageBots = $derived(hasPermission('bots.admin'));

	const botTypeLabels: Record<string, string> = {
		search_engine: t('bots.type_search_engine', 'Search Engine'),
		ai_llm: t('bots.type_ai_llm', 'AI / LLM'),
		social: t('bots.type_social', 'Social'),
		monitoring: t('bots.type_monitoring', 'Monitoring'),
		scraper: t('bots.type_scraper', 'Scraper'),
		unknown: t('bots.type_unknown', 'Unknown')
	};

	const policyClasses: Record<string, string> = {
		allow: 'bg-success/15 text-success border-success',
		block: 'bg-danger/15 text-danger border-danger',
		limit: 'bg-warning/15 text-warning border-warning'
	};

	const botTypeClasses: Record<string, string> = {
		search_engine: 'bg-primary-100 text-primary-800 border-primary-200',
		ai_llm: 'bg-info-100 text-info-800 border-info-200',
		social: 'bg-success/15 text-success border-success',
		monitoring: 'bg-warning/15 text-warning border-warning',
		scraper: 'bg-danger/15 text-danger border-danger',
		unknown: 'bg-surface text-foreground border'
	};

	const botTypeOptions = [
		{ value: 'search_engine', label: t('bots.type_search_engine', 'Search Engine') },
		{ value: 'ai_llm', label: t('bots.type_ai_llm', 'AI / LLM') },
		{ value: 'social', label: t('bots.type_social', 'Social') },
		{ value: 'monitoring', label: t('bots.type_monitoring', 'Monitoring') },
		{ value: 'scraper', label: t('bots.type_scraper', 'Scraper') },
		{ value: 'unknown', label: t('bots.type_unknown', 'Unknown') }
	];

	const policyOptions = [
		{ value: 'allow', label: t('bots.policy_allow', 'Allow') },
		{ value: 'block', label: t('bots.policy_block', 'Block') },
		{ value: 'limit', label: t('bots.policy_limit', 'Limit') }
	];

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => botsAdminApiExtended.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete bots');
		} finally {
			bulkDeleting = false;
		}
	}

	const fields: FieldDef[] = [
		{ key: 'name', label: t('bots.name', 'Name'), type: 'text', required: true, placeholder: 'e.g., Googlebot' },
		{ key: 'user_agent_pattern', label: t('app.bots.user_agent_pattern', 'User-Agent Pattern'), type: 'text', required: true, placeholder: 'e.g., Googlebot' },
		{ key: 'bot_type', label: t('bots.type', 'Type'), type: 'select', options: botTypeOptions },
		{ key: 'policy', label: t('app.bots.policy', 'Policy'), type: 'select', options: policyOptions },
		{ key: 'crawl_delay', label: t('app.bots.crawl_delay', 'Crawl Delay (seconds)'), type: 'number', min: 0 }
	];

	// Custom actions: View + Edit + Delete (delete hidden for system bots).
	const actions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManageBots
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: (row: Bot) => canManageBots && !row.is_system
		}
	];

	const config: EntityPageConfig = {
		title: t('app.bots.title', 'Bot & Crawler Management'),
		description: t('app.bots.description', 'Manage known crawlers, bots, and their access policies'),
		createLabel: t('app.bots.add_bot', 'Add Bot'),
		createPermission: 'bots.admin',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('app.bots.column_bot', 'Bot'),
				sortable: true,
				render: (_: any, row: Bot) => `
					<div class="flex items-center gap-3">
						<div>
							<p class="font-medium text-foreground">${row.name}${row.is_system ? ' <span class="text-xs text-muted">(system)</span>' : ''}</p>
							<p class="text-sm text-muted">${row.user_agent_pattern}</p>
						</div>
					</div>
				`
			},
			{
				key: 'bot_type',
				label: t('app.bots.column_type', 'Type'),
				sortable: true,
				render: (v: string) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${botTypeClasses[v] || botTypeClasses.unknown}">${botTypeLabels[v] || v}</span>`
			},
			{
				key: 'policy',
				label: t('app.bots.column_policy', 'Policy'),
				sortable: true,
				render: (v: string) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${policyClasses[v] || 'bg-surface text-foreground border'}">${v}</span>`
			},
			{
				key: 'crawl_delay',
				label: t('app.bots.column_crawl_delay', 'Crawl Delay'),
				sortable: true,
				render: (v: number) => `<span class="text-sm text-muted">${v > 0 ? v + 's' : '—'}</span>`
			}
		],
		filters: [
			{ key: 'bot_type', label: t('bots.type', 'Type'), options: botTypeOptions },
			{ key: 'policy', label: t('app.bots.policy', 'Policy'), options: policyOptions }
		],
		bulkActions: [
			{ label: t('app.bots.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
		],
		actions,
		fields,
		load: async ({ q, filter }) => {
			const result = await botsAdminApiExtended.list('page_size=100');
			if (result.ok) {
				let data = (Array.isArray(result.data) ? result.data : []) as Bot[];
				if (filter.bot_type) data = data.filter((b) => b.bot_type === filter.bot_type);
				if (filter.policy) data = data.filter((b) => b.policy === filter.policy);
				if (q) {
					const lq = q.toLowerCase();
					data = data.filter((b) => b.name.toLowerCase().includes(lq) || b.user_agent_pattern.toLowerCase().includes(lq) || b.bot_type.toLowerCase().includes(lq));
				}
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load bots'));
		},
		create: async (body) => {
			const res = await botsAdminApiExtended.create({
				name: String(body.name ?? '').trim(),
				user_agent_pattern: String(body.user_agent_pattern ?? '').trim(),
				bot_type: body.bot_type ?? 'search_engine',
				policy: body.policy ?? 'allow',
				crawl_delay: Number(body.crawl_delay ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await botsAdminApiExtended.update(id, {
				name: String(body.name ?? '').trim(),
				user_agent_pattern: String(body.user_agent_pattern ?? '').trim(),
				bot_type: body.bot_type ?? 'search_engine',
				policy: body.policy ?? 'allow',
				crawl_delay: Number(body.crawl_delay ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await botsAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: BotView
	};
</script>

<svelte:head>
	<title>{t('app.bots.page_title', 'Bots')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#snippet BotView({ row }: { row: Bot })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('bots.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}{row.is_system ? ' (system)' : ''}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.bots.user_agent_pattern', 'User-Agent Pattern')}</p>
			<p class="mt-1 font-mono text-sm font-medium" style="color: var(--color-foreground)">{row.user_agent_pattern}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('bots.type', 'Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{botTypeLabels[row.bot_type] || row.bot_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.bots.policy', 'Policy')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.policy}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.bots.crawl_delay', 'Crawl Delay')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.crawl_delay > 0 ? row.crawl_delay + 's' : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.bots.is_system', 'System')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_system ? 'Yes' : 'No'}</p>
			</div>
		</div>
	</div>
{/snippet}
