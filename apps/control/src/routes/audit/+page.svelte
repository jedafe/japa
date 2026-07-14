<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { Select } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import type { PageData } from './$types';

	type AuditEntry = {
		id: string;
		actor_id: string;
		action_type: string;
		target_type: string;
		target_id: string;
		created_at: string;
	};

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let entries = $state<AuditEntry[]>(data.initial?.entries ?? []);
	let page = $state(0);
	// svelte-ignore state_referenced_locally
	let totalPages = $state(data.initial?.total_pages ?? 1);
	let loading = $state(false);
	// svelte-ignore state_referenced_locally
	let apiAvailable = $state(data.initial !== null);

	let filterActor = $state('');
	let filterAction = $state('all');
	let filterTarget = $state('');
	let filterFrom = $state('');
	let filterTo = $state('');

	const actionTypes = [
		{ value: 'all', label: 'All Actions' },
		{ value: 'create', label: 'Create' },
		{ value: 'update', label: 'Update' },
		{ value: 'delete', label: 'Delete' },
		{ value: 'login', label: 'Login' },
		{ value: 'logout', label: 'Logout' },
	];

	const columns = [
		{ key: 'created_at', label: 'Timestamp' },
		{ key: 'actor_id', label: 'Actor' },
		{ key: 'action_type', label: 'Action' },
		{ key: 'target_type', label: 'Target Type' },
		{ key: 'target_id', label: 'Target ID' }
	];

	function buildQuery() {
		const params = new URLSearchParams({ page: String(page) });
		if (filterActor) params.set('actor', filterActor);
		if (filterAction !== 'all') params.set('action_type', filterAction);
		if (filterTarget) params.set('target_type', filterTarget);
		if (filterFrom) params.set('from', filterFrom);
		if (filterTo) params.set('to', filterTo);
		return params.toString();
	}

	async function load() {
		loading = true;
		try {
			const result = await api<{
				entries: AuditEntry[];
				page: number;
				total_pages: number;
			}>(`/control/audit-logs?${buildQuery()}`);
			if (result.ok) {
				entries = result.data.entries;
				totalPages = result.data.total_pages;
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		page = 0;
		load();
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}
</script>

<PageHeader title={t('audit.title')} description={t('audit.description')} />

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		Audit log API is not available.
	</div>
{/if}

<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<input
		type="text"
		placeholder="Actor ID"
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterActor}
	/>
	<Select.Root type="single" bind:value={filterAction} items={actionTypes}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 w-full items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label="Filter action type"
		>
			<Select.Value placeholder="All Actions" />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each actionTypes as item}
						<Select.Item
							class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
							value={item.value}
							label={item.label}
						>
							{#snippet children({ selected })}
								{item.label}
								{#if selected}<span class="ml-auto">✓</span>{/if}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>
	<input
		type="text"
		placeholder="Target type"
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterTarget}
	/>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterFrom}
	/>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterTo}
	/>
</div>
<div class="mb-4">
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-sm font-medium transition-colors"
		onclick={applyFilters}
	>
		Apply Filters
	</button>
</div>

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">Loading...</p>
{:else}
	<DataTable {columns} rows={entries} {page} {totalPages} onPageChange={handlePageChange} />
{/if}
