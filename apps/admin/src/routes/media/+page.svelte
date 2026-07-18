<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import PageHeader from "$lib/components/layout/page-header.svelte";
  import { Card, CardContent } from "$lib/components/ui/card";
  import { Tabs } from "$lib/components/ui/tabs";
  import { DataTable } from "$lib/components/ui/data-table";
  import { FilterBar } from "$lib/components/ui/filter-bar";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Modal } from "$lib/components/ui/modal";
  import {
    Upload,
    Image,
    FileText,
    Film,
    Music,
    Grid,
    List,
    RefreshCw,
    CheckCircle,
    XCircle,
    Trash2,
    Shield,
    Eye,
    Pencil
  } from "lucide-svelte";
  import { api, mediaAdminApi } from "$lib/api";
  import { hasPermission } from "$lib/stores/permissions.svelte";

  type MediaItem = {
    id: string;
    filename: string;
    mime_type: string;
    file_size: number;
    alt_text?: string;
    status: string;
    created_at: string;
    url?: string;
  };

  let { data } = $props();

  let items = $state<MediaItem[]>([]);
  let loading = $state(true);
  let errorMessage = $state("");
  let activeTab = $state("all");
  let filters = $state<Record<string, string>>({});
  let viewMode = $state<"grid" | "list">("grid");
  let selectedIds = $state<Set<string>>(new Set());

  // View modal
  let showViewModal = $state(false);
  let viewTarget = $state<MediaItem | null>(null);

  // Approve/Reject confirmation
  let showStateModal = $state(false);
  let stateAction = $state<"approve" | "reject">("approve");
  let stateTarget = $state<MediaItem | null>(null);

  // Delete confirmation
  let showDeleteModal = $state(false);
  let deleteTargetId = $state("");
  let deleteTargetName = $state("");

  const canManageMedia = $derived(hasPermission("media.manage"));

  function openView(item: MediaItem) {
    viewTarget = item;
    showViewModal = true;
  }

  function confirmState(item: MediaItem, action: "approve" | "reject") {
    stateTarget = item;
    stateAction = action;
    showStateModal = true;
  }

  async function executeState() {
    if (!stateTarget) return;
    const result = stateAction === "approve"
      ? await mediaAdminApi.approve(stateTarget.id)
      : await mediaAdminApi.reject(stateTarget.id);
    if (result.ok) {
      showStateModal = false;
      stateTarget = null;
      load();
    } else {
      errorMessage = result.error;
    }
  }

  function confirmDelete(item: MediaItem) {
    deleteTargetId = item.id;
    deleteTargetName = item.filename;
    showDeleteModal = true;
  }

  const tabs = [
    { value: "all", label: "All Media" },
    { value: "quarantined", label: "Quarantined", icon: Shield }
  ];

  const columns = [
    {
      key: "filename",
      label: "File",
      sortable: true,
      render: (_: any, row: any) => `
        <div class="flex items-center gap-3">
          ${getIconHtml(row.mime_type)}
          <div>
            <p class="font-medium text-foreground">${row.filename}</p>
            <p class="text-xs text-muted">${row.alt_text || ""}</p>
          </div>
        </div>
      `
    },
    {
      key: "mime_type",
      label: "Type",
      sortable: true,
      render: (value: string) =>
        `<span class="inline-flex items-center rounded-full bg-surface px-2.5 py-0.5 text-xs font-medium text-foreground">${value || "unknown"}</span>`
    },
    {
      key: "file_size",
      label: "Size",
      sortable: true,
      render: (value: number) => formatFileSize(value)
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (value: string) => {
        const variants: Record<string, string> = {
          approved: "bg-success/15 text-success",
          pending: "bg-warning/15 text-warning",
          quarantined: "bg-danger/15 text-danger",
          rejected: "bg-surface text-foreground"
        };
        return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[value] || "bg-surface text-foreground"}">${value || "pending"}</span>`;
      }
    },
    {
      key: "created_at",
      label: "Uploaded",
      sortable: true,
      render: (value: string) =>
        new Date(value).toLocaleDateString("en-US", {
          year: "numeric",
          month: "short",
          day: "numeric"
        })
    },
    {
      key: "actions",
      label: "",
      class: "w-40",
      component: MediaActionsCell
    }
  ];

  let filteredItems = $derived.by(() => {
    let result = [...items];

    if (activeTab === "quarantined") {
      result = result.filter((i) => i.status === "quarantined" || i.status === "pending");
    }

    if (filters.status) {
      result = result.filter((i) => i.status === filters.status);
    }
    if (filters.mime_type) {
      result = result.filter((i) => i.mime_type?.startsWith(filters.mime_type));
    }
    if (filters.search) {
      const q = filters.search.toLowerCase();
      result = result.filter(
        (i) =>
          i.filename?.toLowerCase().includes(q) ||
          i.alt_text?.toLowerCase().includes(q)
      );
    }

    return result;
  });

  function formatFileSize(bytes: number): string {
    if (!bytes || bytes === 0) return "0 Bytes";
    const k = 1024;
    const sizes = ["Bytes", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  }

  function getIcon(mimeType: string) {
    if (mimeType?.startsWith("image/")) return Image;
    if (mimeType?.startsWith("video/")) return Film;
    if (mimeType?.startsWith("audio/")) return Music;
    return FileText;
  }

  function getIconHtml(mimeType: string): string {
    if (mimeType?.startsWith("image/")) {
      return '<div class="flex h-10 w-10 items-center justify-center rounded bg-primary-100"><svg class="h-5 w-5 text-primary-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg></div>';
    }
    if (mimeType?.startsWith("video/")) {
      return '<div class="flex h-10 w-10 items-center justify-center rounded bg-warning/15"><svg class="h-5 w-5 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg></div>';
    }
    if (mimeType?.startsWith("audio/")) {
      return '<div class="flex h-10 w-10 items-center justify-center rounded bg-info-100"><svg class="h-5 w-5 text-info-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg></div>';
    }
    return '<div class="flex h-10 w-10 items-center justify-center rounded bg-surface"><svg class="h-5 w-5 text-muted" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg></div>';
  }

  async function load() {
    loading = true;
    errorMessage = "";
    try {
      const result =
        activeTab === "quarantined"
          ? await mediaAdminApi.listQuarantined()
          : await mediaAdminApi.list();
      if (result.ok) {
        items = (result.data as MediaItem[]) ?? [];
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to load media";
    } finally {
      loading = false;
    }
  }

  function handleSearch(searchFilters: Record<string, string>) {
    filters = searchFilters;
  }

  function handleClear() {
    filters = {};
  }

  async function executeDelete() {
    errorMessage = "";
    try {
      const result = await mediaAdminApi.delete(deleteTargetId);
      if (result.ok) {
        items = items.filter((i) => i.id !== deleteTargetId);
        showDeleteModal = false;
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to delete media";
    }
  }

  let bulkDeleting = $state(false);
  async function bulkDelete() {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    bulkDeleting = true;
    errorMessage = "";
    try {
      await Promise.all(ids.map((id) => mediaAdminApi.delete(id)));
      selectedIds = new Set();
      await load();
    } catch {
      errorMessage = "Failed to delete selected media";
    } finally {
      bulkDeleting = false;
    }
  }

  $effect(() => {
    load();
  });
</script>

<svelte:head>
  <title>Media Library — JapaConnect Admin</title>
</svelte:head>

<PageHeader
  title="Media Library"
  description="Manage uploaded files and media"
  actions={[{ label: "Upload", icon: Upload, href: "/media/upload" }]}
/>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <Tabs {tabs} bind:value={activeTab} />

    <div class="flex items-center gap-2">
      <Button
        variant={viewMode === "grid" ? "default" : "ghost"}
        size="icon"
        onclick={() => (viewMode = "grid")}
      >
        <Grid class="h-4 w-4" />
      </Button>
      <Button
        variant={viewMode === "list" ? "default" : "ghost"}
        size="icon"
        onclick={() => (viewMode = "list")}
      >
        <List class="h-4 w-4" />
      </Button>
      <Button variant="ghost" size="icon" onclick={load}>
        <RefreshCw class="h-4 w-4" />
      </Button>
    </div>
  </div>

  <FilterBar
    bind:value={filters}
    filters={[
      {
        key: "status",
        label: "Status",
        options: [
          { value: "approved", label: "Approved" },
          { value: "pending", label: "Pending" },
          { value: "quarantined", label: "Quarantined" },
          { value: "rejected", label: "Rejected" }
        ]
      },
      {
        key: "mime_type",
        label: "File Type",
        options: [
          { value: "image/", label: "Images" },
          { value: "video/", label: "Videos" },
          { value: "audio/", label: "Audio" },
          { value: "application/", label: "Documents" }
        ]
      }
    ]}
    onsearch={handleSearch}
    onclear={handleClear}
  />

  {#if errorMessage}
    <div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger">
      {errorMessage}
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <RefreshCw class="h-5 w-5 animate-spin text-muted" />
      <span class="ml-2 text-sm text-muted">{t('media.loading_media')}</span>
    </div>
  {:else if viewMode === "grid"}
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {#each filteredItems as item (item.id)}
        <Card class="cursor-pointer overflow-hidden transition-shadow hover:shadow-md">
          <div class="flex aspect-square items-center justify-center bg-surface">
            {#if item.mime_type?.startsWith("image/")}
              <img
                src={item.url || ""}
                alt={item.alt_text || item.filename}
                class="h-full w-full object-cover"
              />
            {:else}
              {@const Icon = getIcon(item.mime_type)}
              <Icon class="h-8 w-8 text-muted" />
            {/if}
          </div>
          <CardContent class="p-3">
            <p class="truncate text-sm font-medium text-foreground">{item.filename}</p>
            <p class="text-xs text-muted">{formatFileSize(item.file_size)}</p>
          </CardContent>
        </Card>
      {:else}
        <div class="col-span-full py-12 text-center text-muted">
          No media files found
        </div>
      {/each}
    </div>
  {:else}
    <Card>
      <CardContent class="p-0">
        <DataTable
          {columns}
          data={filteredItems}
          pageSize={20}
          selectable={true}
          bind:selectedIds
          emptyMessage="No media files found"
        >
          {#snippet bulkActions()}
            <Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
              <Trash2 class="mr-1 h-3.5 w-3.5" />
              {bulkDeleting ? "Deleting..." : `Delete Selected (${selectedIds.size})`}
            </Button>
          {/snippet}
        </DataTable>
      </CardContent>
    </Card>
  {/if}
</div>

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Media Details" size="lg">
  {#if viewTarget}
    <div class="space-y-4">
      <div class="flex aspect-video items-center justify-center rounded-lg" style="background: var(--color-surface)">
        {#if viewTarget.mime_type?.startsWith("image/")}
          <img src={viewTarget.url || ""} alt={viewTarget.alt_text || viewTarget.filename} class="max-h-full rounded-lg object-contain" />
        {:else}
          {@const Icon = getIcon(viewTarget.mime_type)}
          <Icon class="h-12 w-12" style="color: var(--color-muted)" />
        {/if}
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Filename</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.filename}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Type</p>
          <p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.mime_type}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Size</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFileSize(viewTarget.file_size)}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Status</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
        </div>
        <div class="col-span-2">
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Alt Text</p>
          <p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.alt_text || "—"}</p>
        </div>
      </div>
    </div>
  {/if}
  <div class="mt-6 flex justify-end">
    <Button variant="outline" onclick={() => (showViewModal = false)}>{t('media.cancel')}</Button>
  </div>
</Modal>

<!-- State Action (Approve/Reject) Confirm -->
<Modal bind:open={showStateModal} title={stateAction === "approve" ? "Approve Media" : "Reject Media"}>
  {#if stateTarget}
    <p class="text-sm" style="color: var(--color-foreground)">
      {stateAction === "approve" ? "Approve" : "Reject"} <strong>{stateTarget.filename}</strong>?
    </p>
  {/if}
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showStateModal = false)}>{t('media.cancel')}</Button>
    <Button variant={stateAction === "approve" ? "default" : "destructive"} onclick={executeState}>
      {stateAction === "approve" ? t('media.approve') : t('media.reject')}
    </Button>
  </div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Media">
  <div class="space-y-4">
    <p class="text-sm text-foreground-alt">
      Are you sure you want to delete "{deleteTargetName}"? This action cannot be undone.
    </p>
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('media.cancel')}</Button>
      <Button variant="destructive" onclick={executeDelete}>{t('media.delete')}</Button>
    </div>
  </div>
</Modal>

<!-- MediaActionsCell -->
{#snippet MediaActionsCell(row: MediaItem)}
  <div class="flex items-center gap-1">
    <Button variant="ghost" size="sm" onclick={() => openView(row)}>
      <Eye class="h-3.5 w-3.5" />
    </Button>
    {#if canManageMedia}
      {#if row.status === "quarantined" || row.status === "pending"}
        <Button variant="ghost" size="sm" onclick={() => confirmState(row, "approve")} style="color: var(--color-success)">
          <CheckCircle class="h-3.5 w-3.5" />
        </Button>
        <Button variant="ghost" size="sm" onclick={() => confirmState(row, "reject")} style="color: var(--color-warning)">
          <XCircle class="h-3.5 w-3.5" />
        </Button>
      {/if}
      <Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
        <Trash2 class="h-3.5 w-3.5" />
      </Button>
    {/if}
  </div>
{/snippet}
