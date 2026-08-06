<script lang="ts">
	// /profiles — migrated to the EntityPage scaffold; read-only list with View + Verify/Unverify state actions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type Profile = {
		id: string;
		user_id: string;
		display_name: string;
		professional_type: string;
		is_verified: boolean;
		created_at: string;
	};

	const canModerate = $derived(hasPermission('admin.moderation'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// Verify/unverify are state actions with no confirm modal (mirrors legacy behavior); call API then reload via ctx.
	async function verifyProfile(row: Profile, ctx?: any) {
		pageError = '';
		try {
			const res = await api(`/admin/profiles/${row.id}/verify`, { method: 'POST', body: {} });
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'Failed to verify profile');
				return;
			}
			ctx?.reload?.();
		} catch {
			pageError = t('common.error', 'Failed to verify profile');
		}
	}

	async function unverifyProfile(row: Profile, ctx?: any) {
		pageError = '';
		try {
			const res = await api(`/admin/profiles/${row.id}/verify`, { method: 'DELETE' });
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'Failed to unverify profile');
				return;
			}
			ctx?.reload?.();
		} catch {
			pageError = t('common.error', 'Failed to unverify profile');
		}
	}

	// Custom actions replace defaults, so View is included explicitly; Verify/Unverify use show() predicates.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Profile, ctx?: any) => ctx?.openView?.(row) }
		];
		if (canModerate) {
			acts.push({
				label: t('profiles.verify', 'Verify'),
				icon: CheckCircle,
				onclick: verifyProfile,
				show: (row: Profile) => !row.is_verified
			});
			acts.push({
				label: t('profiles.unverify', 'Unverify'),
				icon: XCircle,
				variant: 'destructive',
				onclick: unverifyProfile,
				show: (row: Profile) => row.is_verified
			});
		}
		return acts;
	});

	// fields required by EntityPageConfig; readonly hides the form so these only seed view fallback.
	const fields: FieldDef[] = [
		{ key: 'display_name', label: t('profiles.name', 'Name'), type: 'text' },
		{ key: 'professional_type', label: t('profiles.type', 'Type'), type: 'text' },
		{ key: 'is_verified', label: t('profiles.verification', 'Verification'), type: 'checkbox' }
	];

	const filters: FilterDef[] = [
		{
			key: 'is_verified',
			label: t('profiles.verification', 'Verification'),
			options: [
				{ value: 'true', label: t('profiles.verified', 'Verified') },
				{ value: 'false', label: t('profiles.unverified', 'Unverified') }
			]
		}
	];

	const config = $derived<EntityPageConfig>({
		title: t('profiles.title', 'Profiles'),
		description: t('profiles.description', 'Manage user profiles'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'display_name',
				label: t('profiles.name', 'Name'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v}</span>`
			},
			{
				key: 'professional_type',
				label: t('profiles.type', 'Type'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'is_verified',
				label: t('profiles.verification', 'Verification'),
				sortable: true,
				render: (v: boolean) => {
					const cls = v ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning';
					const text = v ? t('profiles.verified', 'Verified') : t('profiles.unverified', 'Unverified');
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${text}</span>`;
				}
			}
		],
		filters,
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<Profile[]>('/profiles');
			if (!result.ok) {
				throw new Error(result.error ?? t('common.error', 'Failed to load profiles'));
			}
			let items = result.data ?? [];
			// Client-side search + filter (mirrors legacy behavior; /profiles returns unpaginated).
			if (q) {
				const lower = q.toLowerCase();
				items = items.filter(
					(p) =>
						p.display_name.toLowerCase().includes(lower) ||
						p.professional_type.toLowerCase().includes(lower)
				);
			}
			if (filter.is_verified) {
				const val = filter.is_verified === 'true';
				items = items.filter((p) => p.is_verified === val);
			}
			const total = items.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			return { data: items.slice(start, start + pageSize), total, totalPages };
		},
		viewSnippet: ProfileView
	});
</script>

<svelte:head>
	<title>{t('profiles.title', 'Profiles')} — JapaConnect Admin</title>
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

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet ProfileView({ row }: { row: Profile })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.display_name}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.type', 'Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.professional_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.verification', 'Verification')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_verified ? t('profiles.verified', 'Verified') : t('profiles.unverified', 'Unverified')}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
