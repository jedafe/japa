<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Select } from "$lib/components/ui/select";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw } from "lucide-svelte";
	import { couponsAdminApiExtended } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import {
		dateInputToStartOfDayISO,
		dateInputToEndOfDayISO,
		isoTimestampToDateInput
	} from "$lib/utils/date";

	type Coupon = {
		id: string;
		code: string;
		discount_type: string;
		discount_value: number;
		is_active: boolean;
		valid_from?: string;
		valid_until?: string;
		usage_limit?: number;
		used_count: number;
		created_at: string;
	};

	let coupons = $state<Coupon[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManageCoupons = $derived(hasPermission("coupons.manage"));
	
	const discountTypeOptions = $derived([
		{ value: "percent", label: t("coupons.percentage", "Percentage") },
		{ value: "fixed", label: t("coupons.fixed", "Fixed") }
	]);

	// Modal state
	let showModal = $state(false);
	let modalMode = $state<"create" | "edit">("create");
	let editingCoupon = $state<Coupon | null>(null);
	let saving = $state(false);

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Coupon | null>(null);

	// Form state
	let code = $state("");
	let discountType = $state("percent");
	let discountValue = $state("");
	let validFrom = $state("");
	let validUntil = $state("");
	let usageLimit = $state("");
	let isActive = $state(true);

	const columns = [
		{ key: "code", label: t("coupons.code", "Code"), sortable: true },
		{
			key: "discount_value",
			label: t("coupons.discount", "Discount"),
			sortable: true,
			render: (v: number, row: Coupon) => row.discount_type === "percentage" ? `${v}%` : `${v} credits`
		},
		{
			key: "used_count",
			label: t("coupons.used", "Used"),
			sortable: true,
			render: (v: number, row: Coupon) => `${v ?? 0}${row.usage_limit ? ` / ${row.usage_limit}` : ""}`
		},
		{
			key: "valid_until",
			label: t("coupons.expires", "Expires"),
			sortable: true,
			render: (v: string) => v ? new Date(v).toLocaleDateString() : t("coupons.never", "Never")
		},
		{ key: "is_active", label: t("common.status", "Status"), sortable: true },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-32",
			componentSnippet: CouponActionsCell
		}
	];

	async function loadCoupons() {
		loading = true;
		error = "";
		try {
			const result = await couponsAdminApiExtended.list();
			if (result.ok) {
				coupons = (result.data as Coupon[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load coupons");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load coupons");
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		modalMode = "create";
		editingCoupon = null;
		code = "";
		discountType = "percent";
		discountValue = "";
		validFrom = "";
		validUntil = "";
		usageLimit = "";
		isActive = true;
		showModal = true;
	}

	function openEdit(coupon: Coupon) {
		modalMode = "edit";
		editingCoupon = coupon;
		code = coupon.code;
		discountType = coupon.discount_type === "percentage" ? "percent" : coupon.discount_type || "percent";
		discountValue = String(coupon.discount_value);
		validFrom = isoTimestampToDateInput(coupon.valid_from);
		validUntil = isoTimestampToDateInput(coupon.valid_until);
		usageLimit = coupon.usage_limit ? String(coupon.usage_limit) : "";
		isActive = coupon.is_active;
		showModal = true;
	}

	async function saveCoupon() {
		if (!code.trim() || !discountValue) return;
		saving = true;
		error = "";
		try {
			const body: Record<string, unknown> = {
				title: code.trim(),
				code: code.trim().toUpperCase(),
				coupon_type: "external",
				discount_type: discountType,
				discount_value: Number(discountValue),
				status: isActive ? "active" : "inactive"
			};
			if (validFrom) body.valid_from = dateInputToStartOfDayISO(validFrom);
			if (validUntil) body.valid_until = dateInputToEndOfDayISO(validUntil);
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const result = editingCoupon
				? await couponsAdminApiExtended.update(editingCoupon.id, body)
				: await couponsAdminApiExtended.create(body);

			if (result.ok) {
				showModal = false;
				success = editingCoupon ? t("coupons.updated", "Coupon updated") : t("coupons.created", "Coupon created");
				loadCoupons();
			} else {
				error = result.error ?? t("common.error", "Failed to save coupon");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save coupon");
		} finally {
			saving = false;
		}
	}

	function openDelete(coupon: Coupon) {
		deleteTarget = coupon;
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const result = await couponsAdminApiExtended.delete(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("coupons.deleted", "Coupon deleted");
			loadCoupons();
		} else {
			error = result.error ?? t("common.error", "Failed to delete coupon");
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = "";
		try {
			for (const id of selectedIds) {
				await couponsAdminApiExtended.delete(id);
			}
			selectedIds = new Set();
			success = t("coupons.bulk_deleted", "Selected coupons deleted");
			loadCoupons();
		} catch {
			error = t("common.error", "Failed to delete selected coupons");
		}
		bulkDeleting = false;
	}

</script>

<svelte:head>
	<title>{t("coupons.title", "Coupons")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("coupons.title", "Coupons")}
	description={t("coupons.description", "Manage discount coupons")}
	actions={[
		...(canManageCoupons ? [{ label: t("coupons.create", "Create Coupon"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadCoupons }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<DataTable {columns} data={coupons} pageSize={10} selectable bind:selectedIds>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t("common.deleting", "Deleting...") : t("common.deleteSelected", "Delete Selected")} ({selectedIds.size})
					</Button>
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={modalMode === "create" ? t("coupons.create", "Create Coupon") : t("coupons.edit", "Edit Coupon")}>
	<div class="space-y-4">
		<div>
			<label for="coupon-code" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.code", "Code")}</label>
			<input id="coupon-code" type="text" bind:value={code} class="mt-1 w-full rounded-lg border p-2 text-sm uppercase" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="coupon-type" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.type", "Type")}</label>
				<Select id="coupon-type" options={discountTypeOptions} bind:value={discountType} />
			</div>
			<div>
				<label for="coupon-value" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.value", "Value")}</label>
				<input id="coupon-value" type="number" bind:value={discountValue} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="coupon-from" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.valid_from", "Valid From")}</label>
				<input id="coupon-from" type="date" bind:value={validFrom} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
			<div>
				<label for="coupon-until" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.valid_until", "Valid Until")}</label>
				<input id="coupon-until" type="date" bind:value={validUntil} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="coupon-limit" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.usage_limit", "Usage Limit")}</label>
				<input id="coupon-limit" type="number" bind:value={usageLimit} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="coupon-active" type="checkbox" bind:checked={isActive} class="h-4 w-4 rounded" style="accent-color: var(--color-primary)" />
				<label for="coupon-active" class="text-sm font-medium" style="color: var(--color-foreground)">{t("coupons.active", "Active")}</label>
			</div>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={saveCoupon} disabled={saving}>{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("coupons.delete_confirm", "Delete coupon")} <strong class="font-mono">{deleteTarget.code}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- CouponActionsCell component -->
{#snippet CouponActionsCell({ value, row }: { value: any; row: Coupon })}
	<div class="flex items-center gap-1">
		{#if canManageCoupons}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
