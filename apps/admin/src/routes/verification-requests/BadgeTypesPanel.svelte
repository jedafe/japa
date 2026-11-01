<script lang="ts">
	// Managed verification badge-type taxonomy (migration 001013) — per-scope
	// CRUD over /admin/verification-types. Badge artwork uploads via a
	// dedicated "Set badge" row action + shared media picker. Seeded rows are
	// is_protected: deactivate, never delete.
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController } from "$lib/components/entity";
	import { verificationTypesAdminApi, type VerificationTypeItem } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Eye, Pencil, Trash2, Image as ImageIcon } from "lucide-svelte";
	import type { ActionItem, ActionContext } from "$lib/components/ui/data-table";

	const canManage = $derived(hasPermission("verification.types.manage"));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let badgeTarget = $state<VerificationTypeItem | null>(null);
	let badgePickerOpen = $state(false);

	const scopeOptions = [
		{ value: "organization", label: t("vtypes.scope_organization", "Organization") },
		{ value: "user", label: t("vtypes.scope_user", "User") },
		{ value: "service_listing", label: t("vtypes.scope_service_listing", "Service listing") }
	];

	const scopeLabel = (v: string) => scopeOptions.find((o) => o.value === v)?.label ?? v;

	const statusBadge = (v: boolean) =>
		v
			? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-success/15 text-success">${t("vtypes.active", "Active")}</span>`
			: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-muted/30 text-muted">${t("vtypes.inactive", "Inactive")}</span>`;

	async function setBadge(media: { id: string; url: string }) {
		if (!badgeTarget) return;
		pageError = "";
		const res = await verificationTypesAdminApi.update(badgeTarget.id, { badge_media_id: media.id });
		if (!res.ok) { pageError = res.error; return; }
		badgeTarget = null;
		entityController?.reload();
	}

	async function clearBadge() {
		if (!badgeTarget) return;
		pageError = "";
		const res = await verificationTypesAdminApi.update(badgeTarget.id, { badge_media_id: null });
		if (!res.ok) { pageError = res.error; return; }
		badgeTarget = null;
		entityController?.reload();
	}

	const config = $derived<EntityPageConfig>({
		title: t("vtypes.title", "Badge types"),
		description: t("vtypes.desc", "Manage verification badge types and custom badge artwork per entity scope"),
		createLabel: t("vtypes.create", "New badge type"),
		createPermission: "verification.types.manage",
		editPermission: "verification.types.manage",
		deletePermission: "verification.types.manage",
		pageSize: 25,
		columns: [
			{
				key: "badge_url",
				label: t("vtypes.col.badge", "Badge"),
				render: (v: string | null, row: VerificationTypeItem) =>
					v
						? `<img src="${v}" alt="${row.label}" class="h-8 w-8 rounded-md object-cover" style="border: 1px solid var(--color-border)" />`
						: `<span style="color: var(--color-muted)">—</span>`,
				hideable: false
			},
			{ key: "value", label: t("vtypes.col.value", "Value"), sortable: true },
			{ key: "label", label: t("vtypes.col.label", "Label"), sortable: true },
			{ key: "scope", label: t("vtypes.col.scope", "Scope"), render: scopeLabel },
			{ key: "position", label: t("vtypes.col.position", "Position"), sortable: true, width: 90 },
			{ key: "is_active", label: t("vtypes.col.status", "Status"), render: statusBadge }
		],
		filters: [
			{ key: "scope", label: t("vtypes.col.scope", "Scope"), options: scopeOptions },
			{
				key: "is_active",
				label: t("vtypes.col.status", "Status"),
				options: [
					{ value: "true", label: t("vtypes.active", "Active") },
					{ value: "false", label: t("vtypes.inactive", "Inactive") }
				]
			}
		],
		actions: [
			{
				label: t("common.view", "View"),
				icon: Eye,
				onclick: (row: VerificationTypeItem, ctx?: ActionContext) => ctx?.openView(row)
			},
			{
				label: t("common.edit", "Edit"),
				icon: Pencil,
				onclick: (row: VerificationTypeItem, ctx?: ActionContext) => ctx?.openEdit(row),
				show: () => canManage
			},
			{
				label: t("vtypes.set_badge", "Set badge"),
				icon: ImageIcon,
				onclick: (row: VerificationTypeItem) => { badgeTarget = row; badgePickerOpen = true; },
				show: () => canManage
			},
			{
				label: t("vtypes.badge_remove", "Remove badge"),
				icon: Trash2,
				onclick: (row: VerificationTypeItem) => { badgeTarget = row; void clearBadge(); },
				show: (row: VerificationTypeItem) => canManage && !!row.badge_media_id
			},
			{
				label: t("common.delete", "Delete"),
				icon: Trash2,
				variant: "destructive",
				onclick: (row: VerificationTypeItem, ctx?: ActionContext) => ctx?.openDelete(row),
				show: (row: VerificationTypeItem) => canManage && !row.is_protected
			}
		],
		fields: [
			{ key: "scope", label: t("vtypes.col.scope", "Scope"), type: "select", options: scopeOptions, required: true, disabledWhen: (e: VerificationTypeItem | null) => e != null, lockedHelp: t("vtypes.scope_locked", "Scope cannot change after creation") },
			{ key: "value", label: t("vtypes.col.value", "Value"), type: "text", required: true, placeholder: "snake_case_key", disabledWhen: (e: VerificationTypeItem | null) => e != null, lockedHelp: t("vtypes.value_locked", "Stored on entities — cannot change") },
			{ key: "label", label: t("vtypes.col.label", "Label"), type: "text", required: true },
			{ key: "description", label: t("vtypes.col.description", "Description"), type: "textarea" },
			{ key: "position", label: t("vtypes.col.position", "Position"), type: "number" },
			{ key: "is_active", label: t("vtypes.col.status", "Active"), type: "checkbox" }
		],
		load: async ({ filter }) => {
			const params = new URLSearchParams();
			if (filter.scope) params.set("scope", filter.scope);
			const res = await verificationTypesAdminApi.list(params.toString());
			if (!res.ok) throw new Error(res.error ?? t("common.error", "Failed to load"));
			let data = res.data ?? [];
			if (filter.is_active !== undefined && filter.is_active !== "") {
				data = data.filter((r) => String(r.is_active) === String(filter.is_active));
			}
			return { data, total: data.length, totalPages: 1 };
		},
		create: async (body) => {
			const res = await verificationTypesAdminApi.create(body as Parameters<typeof verificationTypesAdminApi.create>[0]);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await verificationTypesAdminApi.update(id, body as Parameters<typeof verificationTypesAdminApi.update>[1]);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await verificationTypesAdminApi.remove(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TypeView
	});
</script>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)" role="alert">{pageError}</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#snippet TypeView({ row }: { row: VerificationTypeItem })}
	<dl class="space-y-2 text-sm">
		{#if row.badge_url}
			<div><img src={row.badge_url} alt={row.label} class="h-16 w-16 rounded-lg object-cover" style="border: 1px solid var(--color-border)" /></div>
		{/if}
		<div><dt class="font-medium">{t("vtypes.col.label", "Label")}</dt><dd>{row.label}</dd></div>
		<div><dt class="font-medium">{t("vtypes.col.value", "Value")}</dt><dd><code>{row.value}</code></dd></div>
		<div><dt class="font-medium">{t("vtypes.col.scope", "Scope")}</dt><dd>{scopeLabel(row.scope)}</dd></div>
		{#if row.description}<div><dt class="font-medium">{t("vtypes.col.description", "Description")}</dt><dd>{row.description}</dd></div>{/if}
		<div><dt class="font-medium">{t("vtypes.col.position", "Position")}</dt><dd>{row.position}</dd></div>
		<div><dt class="font-medium">{t("vtypes.col.status", "Status")}</dt><dd>{@html statusBadge(row.is_active)}</dd></div>
		{#if row.is_protected}<div><dt class="font-medium">{t("vtypes.col.protected", "Protected")}</dt><dd class="text-xs" style="color: var(--color-muted)">{t("vtypes.protected_note", "Seeded type — cannot be deleted")}</dd></div>{/if}
	</dl>
{/snippet}

<MediaPickerModal
	bind:open={badgePickerOpen}
	onselect={(media: { id: string; url: string }) => setBadge(media)}
/>
