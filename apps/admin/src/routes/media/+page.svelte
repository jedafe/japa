<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import PageHeader from "$lib/components/layout/page-header.svelte";
  import { Card, CardContent } from "$lib/components/ui/card";
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
    Pencil,
    Search
  } from "lucide-svelte";
  import { api, apiFetch, mediaAdminApi, stockAdminApi, type StockProviderAdmin } from "$lib/api";
  import { hasPermission } from "$lib/stores/permissions.svelte";
  import Pagination from "$lib/components/ui/Pagination.svelte";

  type MediaItem = {
    id: string;
    filename: string;
    mime_type: string;
    file_size: number;
    alt_text?: string;
    status: string;
    created_at: string;
    url?: string;
    user_group_id?: string;
  };

  let { data } = $props();

  let items = $state<MediaItem[]>([]);
  let loading = $state(true);
  let errorMessage = $state("");
  let activeTab = $state("all");
  let filters = $state<Record<string, string>>({});
  let viewMode = $state<"grid" | "list">("grid");
  let selectedIds = $state<Set<string>>(new Set());

  // --- Stock providers management ---
  let stockProviders = $state<StockProviderAdmin[]>([]);
  let stockProvidersLoading = $state(false);
  let stockProvidersError = $state("");
  let stockProvidersLoaded = $state(false);
  let updatingProvider = $state<string | null>(null);
  let providerSearch = $state("");

  let filteredStockProviders = $derived.by(() => {
    if (!providerSearch.trim()) return stockProviders;
    const q = providerSearch.toLowerCase();
    return stockProviders.filter(
      (p) =>
        p.provider_name?.toLowerCase().includes(q) ||
        p.display_name?.toLowerCase().includes(q)
    );
  });

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

  // Pagination
  let currentPage = $state(0);
  let pageSize = $state(20);
  let totalItems = $derived(filteredItems.length);
  let totalPages = $derived(Math.max(1, Math.ceil(totalItems / pageSize)));
  let paginatedItems = $derived.by(() => {
    const start = currentPage * pageSize;
    return filteredItems.slice(start, start + pageSize);
  });

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

  // Upload dialog
  let showUploadModal = $state(false);
  let uploadFiles = $state<File[]>([]);
  let uploadUserGroupId = $state("");
  let uploadAltText = $state("");
  let uploadSubmitting = $state(false);
  let uploadProgress = $state<{ current: number; total: number; filename: string } | null>(null);
  let userGroups = $state<Array<{ id: string; name: string; slug: string }>>([]);

  // Backend gates approve/reject/delete on media.quarantine.manage and media.delete_any
  const canManageMedia = $derived(hasPermission("media.quarantine.manage") || hasPermission("media.delete_any"));

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
    const status = stateAction === "approve" ? "approved" : "rejected";
    const result = await mediaAdminApi.updateStatus(stateTarget.id, status);
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

  // --- Stock providers management functions ---
  async function loadStockProviders() {
    stockProvidersLoading = true;
    stockProvidersError = "";
    try {
      const result = await stockAdminApi.listProviders();
      if (result.ok) {
        stockProviders = result.data.providers ?? [];
        stockProvidersLoaded = true;
      } else {
        stockProvidersError = result.error ?? "Failed to load providers";
      }
    } catch (err) {
      stockProvidersError = err instanceof Error ? err.message : "Failed to load providers";
    }
    stockProvidersLoading = false;
  }

  async function toggleStockProvider(name: string, enabled: boolean) {
    updatingProvider = name;
    const result = await stockAdminApi.patchProvider(name, { enabled });
    if (result.ok) {
      await loadStockProviders();
    } else {
      stockProvidersError = result.error ?? "Failed to update provider";
    }
    updatingProvider = null;
  }

  async function updateProviderKey(name: string, apiKey: string) {
    if (!apiKey.trim()) return;
    updatingProvider = name;
    const result = await stockAdminApi.patchProvider(name, { api_key: apiKey.trim() });
    if (result.ok) {
      await loadStockProviders();
    } else {
      stockProvidersError = result.error ?? "Failed to update API key";
    }
    updatingProvider = null;
  }

  // Load stock providers when switching to the tab (once).
  $effect(() => {
    if (activeTab === "stock-providers" && !stockProvidersLoaded) {
      loadStockProviders();
    }
  });

  const tabs = $derived([
    { value: "all", label: t('app.media.tab_all', 'All Media') },
    { value: "quarantined", label: t('app.media.tab_quarantined', 'Quarantined'), icon: Shield },
    { value: "stock-providers", label: t('app.media.tab_stock_providers', 'Stock Providers') }
  ]);

  const columns = [
    {
      key: "filename",
      label: t('app.media.col_file', 'File'),
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
      label: t('app.media.col_type', 'Type'),
      sortable: true,
      render: (value: string) =>
        `<span class="inline-flex items-center rounded-full bg-surface px-2.5 py-0.5 text-xs font-medium text-foreground">${value || "unknown"}</span>`
    },
    {
      key: "file_size",
      label: t('app.media.col_size', 'Size'),
      sortable: true,
      render: (value: number) => formatFileSize(value)
    },
    {
      key: "status",
      label: t('app.media.col_status', 'Status'),
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
      label: t('app.media.col_uploaded', 'Uploaded'),
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
      componentSnippet: MediaActionsCell
    }
  ];

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
      errorMessage = t('app.media.err_load', 'Failed to load media');
    } finally {
      loading = false;
    }
  }

  function handleSearch(searchFilters: Record<string, string>) {
    filters = searchFilters;
    currentPage = 0;
  }

  function handleClear() {
    filters = {};
    currentPage = 0;
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
      errorMessage = t('app.media.err_delete', 'Failed to delete media');
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
      errorMessage = t('app.media.err_bulk_delete', 'Failed to delete selected media');
    } finally {
      bulkDeleting = false;
    }
  }

  async function loadUserGroups() {
    try {
      const result = await api<{ groups: Array<{ id: string; name: string; slug: string }>; total: number }>('/admin/user-groups');
      if (result.ok) {
        userGroups = result.data?.groups ?? [];
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = t('app.media.err_load_user_groups', 'Failed to load user groups');
    }
  }

  function openUploadModal() {
    uploadFiles = [];
    uploadUserGroupId = "";
    uploadAltText = "";
    uploadProgress = null;
    showUploadModal = true;
    loadUserGroups();
  }

  async function handleUpload() {
    if (!uploadFiles.length) return;
    uploadSubmitting = true;
    errorMessage = "";
    const total = uploadFiles.length;
    let failed = 0;
    try {
      for (let i = 0; i < total; i++) {
        const file = uploadFiles[i];
        uploadProgress = { current: i + 1, total, filename: file.name };

        const formData = new FormData();
        formData.append("file", file);

        const result = await apiFetch<{ media_id: string; url: string }>(
          "/admin/media/upload",
          { method: "POST", body: formData }
        );

        if (result.error) {
          failed++;
          continue;
        }

        const mediaId = result.data?.media_id;
        if (uploadUserGroupId && mediaId) {
          await api(`/admin/media/${mediaId}`, {
            method: "PATCH",
            body: { user_group_id: uploadUserGroupId },
          });
        }
        if (uploadAltText && mediaId) {
          await api(`/admin/media/${mediaId}`, {
            method: "PATCH",
            body: { alt_text: uploadAltText },
          });
        }
      }

      if (failed === total) {
        errorMessage = t("app.media.err_upload", "Failed to upload media");
      }
      showUploadModal = false;
      await load();
    } catch {
      errorMessage = t("app.media.err_upload", "Failed to upload media");
    } finally {
      uploadSubmitting = false;
      uploadProgress = null;
    }
  }

  $effect(() => {
    load();
  });
</script>

<svelte:head>
  <title>{t('app.media.page_title', 'Media Library')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
  title={t('app.media.page_title', 'Media Library')}
  description={t('app.media.page_desc', 'Manage uploaded files and media')}
/>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div class="inline-flex h-11 items-center justify-center rounded-lg p-1" style="background-color: var(--color-surface); color: var(--color-muted)">
      {#each tabs as tab}
        <button
          type="button"
          class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
          style={activeTab === tab.value
            ? 'background-color: var(--color-surface); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
            : 'color: var(--color-muted)'}
          onclick={() => (activeTab = tab.value)}
        >
          {#if tab.icon}
            {@const Icon = tab.icon}
            <Icon class="mr-2 h-4 w-4" />
          {/if}
          {tab.label}
        </button>
      {/each}
    </div>

    <div class="flex items-center gap-2">
      {#if activeTab !== "stock-providers"}
      <Button
        size="icon"
        variant={viewMode === 'grid' ? 'default' : 'ghost'}
        class={viewMode === 'grid' ? '' : 'text-foreground-alt'}
        onclick={() => (viewMode = "grid")}
        aria-label="Grid view"
      >
        <Grid class="h-4 w-4" />
      </Button>
      <Button
        size="icon"
        variant={viewMode === 'list' ? 'default' : 'ghost'}
        class={viewMode === 'list' ? '' : 'text-foreground-alt'}
        onclick={() => (viewMode = "list")}
        aria-label="List view"
      >
        <List class="h-4 w-4" />
      </Button>
      {/if}
      <Button
        size="icon"
        variant="ghost"
        class="text-foreground-alt"
        onclick={load}
        aria-label="Refresh"
      >
        <RefreshCw class="h-4 w-4" />
      </Button>
      {#if canManageMedia}
        <Button variant="default" size="sm" onclick={openUploadModal}>
          <Upload class="h-4 w-4 mr-1" />
          {t('app.media.upload', 'Upload')}
        </Button>
      {/if}
    </div>
  </div>

  {#if activeTab !== "stock-providers"}
  <FilterBar
    bind:value={filters}
    filters={[
      {
        key: "status",
        label: t('app.media.filter_status', 'Status'),
        options: [
          { value: "approved", label: t('app.media.status_approved', 'Approved') },
          { value: "pending", label: t('app.media.status_pending', 'Pending') },
          { value: "quarantined", label: t('app.media.status_quarantined', 'Quarantined') },
          { value: "rejected", label: t('app.media.status_rejected', 'Rejected') }
        ]
      },
      {
        key: "mime_type",
        label: t('app.media.filter_file_type', 'File Type'),
        options: [
          { value: "image/", label: t('app.media.type_images', 'Images') },
          { value: "video/", label: t('app.media.type_videos', 'Videos') },
          { value: "audio/", label: t('app.media.type_audio', 'Audio') },
          { value: "application/", label: t('app.media.type_documents', 'Documents') }
        ]
      }
    ]}
    onsearch={handleSearch}
    onclear={handleClear}
  />
  {/if}

  {#if errorMessage}
    <div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger">
      {errorMessage}
    </div>
  {/if}

  {#if activeTab === "stock-providers"}
    <!-- Stock Providers management tab -->
    <Card>
      <CardContent class="p-0">
        {#if stockProvidersLoading}
          <div class="flex items-center justify-center py-12">
            <RefreshCw class="h-5 w-5 animate-spin text-muted" />
            <span class="ml-2 text-sm text-muted">Loading...</span>
          </div>
        {:else if stockProvidersError}
          <div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger">
            {stockProvidersError}
          </div>
        {:else}
          <!-- Provider search bar -->
          <div class="border-b p-3" style="border-color: var(--color-border)">
            <div class="relative max-w-sm">
              <Search class="absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
              <input
                type="text"
                placeholder="Search providers..."
                class="w-full rounded-md border py-1.5 pl-8 pr-3 text-sm"
                style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
                bind:value={providerSearch}
              />
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead class="border-b bg-surface text-left text-xs uppercase text-muted">
                <tr>
                  <th class="px-4 py-3 font-medium">Provider</th>
                  <th class="px-4 py-3 font-medium">Enabled</th>
                  <th class="px-4 py-3 font-medium">API Key</th>
                  <th class="px-4 py-3 font-medium">RBAC Permission</th>
                </tr>
              </thead>
              <tbody class="divide-y">
                {#each filteredStockProviders as p (p.provider_name)}
                  <tr class="hover:bg-surface/50">
                    <td class="px-4 py-3 font-medium text-foreground">{p.display_name}</td>
                    <td class="px-4 py-3">
                      <label class="inline-flex cursor-pointer items-center gap-2">
                        <input
                          type="checkbox"
                          checked={p.enabled}
                          disabled={updatingProvider === p.provider_name}
                          onchange={(e) => toggleStockProvider(p.provider_name, e.currentTarget.checked)}
                        />
                        <span class="text-xs text-muted">
                          {updatingProvider === p.provider_name ? 'Saving...' : (p.enabled ? 'On' : 'Off')}
                        </span>
                      </label>
                    </td>
                    <td class="px-4 py-3">
                      {#if p.provider_name === 'openverse'}
                        <span class="text-xs text-muted">No key required</span>
                      {:else}
                        <input
                          type="password"
                          class="w-full max-w-xs rounded border border-border bg-surface px-2 py-1 text-xs"
                          placeholder={p.has_api_key ? '•••••••• (enter new key to replace)' : 'Paste API key...'}
                          onkeydown={(e) => {
                            if (e.key === 'Enter' && e.currentTarget.value.trim()) {
                              updateProviderKey(p.provider_name, e.currentTarget.value);
                              e.currentTarget.value = '';
                            }
                          }}
                        />
                      {/if}
                    </td>
                    <td class="px-4 py-3">
                      {#if p.provider_name === 'openverse'}
                        <Badge>media.stock.search</Badge>
                      {:else}
                        <Badge>media.stock.providers</Badge>
                      {/if}
                    </td>
                  </tr>
                {/each}
                {#if filteredStockProviders.length === 0}
                  <tr>
                    <td colspan="4" class="px-4 py-8 text-center text-muted">
                      No providers match "{providerSearch}"
                    </td>
                  </tr>
                {/if}
              </tbody>
            </table>
          </div>
          <p class="border-t px-4 py-2 text-xs text-muted">
            Press <kbd class="rounded border px-1">Enter</kbd> in the API Key field to save. Keys are encrypted at rest.
          </p>
        {/if}
      </CardContent>
    </Card>
  {:else}
  {#if loading}
    <div class="flex items-center justify-center py-12">
      <RefreshCw class="h-5 w-5 animate-spin text-muted" />
      <span class="ml-2 text-sm text-muted">{t('media.loading_media')}</span>
    </div>
  {:else if viewMode === "grid"}
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {#each paginatedItems as item (item.id)}
        <Card class="group relative cursor-pointer overflow-hidden transition-shadow hover:shadow-md">
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
            <!-- Hover action overlay -->
            <div class="absolute inset-0 flex items-center justify-center gap-2 bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
              <Button variant="secondary" size="icon" class="h-8 w-8" onclick={() => openView(item)}>
                <Eye class="h-4 w-4" />
              </Button>
              {#if canManageMedia}
                {#if item.status === "quarantined" || item.status === "pending"}
                  <Button variant="secondary" size="icon" class="h-8 w-8" onclick={() => confirmState(item, "approve")} style="color: var(--color-success)">
                    <CheckCircle class="h-4 w-4" />
                  </Button>
                  <Button variant="secondary" size="icon" class="h-8 w-8" onclick={() => confirmState(item, "reject")} style="color: var(--color-warning)">
                    <XCircle class="h-4 w-4" />
                  </Button>
                {/if}
                <Button variant="secondary" size="icon" class="h-8 w-8" onclick={() => confirmDelete(item)} style="color: var(--color-danger)">
                  <Trash2 class="h-4 w-4" />
                </Button>
              {/if}
            </div>
          </div>
          <CardContent class="p-3">
            <p class="truncate text-sm font-medium text-foreground">{item.filename}</p>
            <div class="mt-1 flex items-center justify-between">
              <p class="text-xs text-muted">{formatFileSize(item.file_size)}</p>
              <span class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium {item.status === 'approved' ? 'bg-success/15 text-success' : item.status === 'pending' ? 'bg-warning/15 text-warning' : item.status === 'quarantined' ? 'bg-danger/15 text-danger' : 'bg-surface text-foreground-alt'}">
                {item.status || 'pending'}
              </span>
            </div>
          </CardContent>
        </Card>
      {:else}
        <div class="col-span-full py-12 text-center text-muted">
          {t('app.media.empty_grid', 'No media files found')}
        </div>
      {/each}
    </div>
  {:else}
    <Card>
      <CardContent class="p-0">
        <DataTable
          {columns}
          data={paginatedItems}
          pageSize={pageSize}
          selectable={true}
          bind:selectedIds
          emptyMessage={t('app.media.empty_table', 'No media files found')}
        >
          {#snippet bulkActions()}
            <Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
              <Trash2 class="mr-1 h-3.5 w-3.5" />
            {bulkDeleting ? t('app.media.deleting', 'Deleting...') : `${t('app.media.delete_selected', 'Delete Selected')} (${selectedIds.size})`}
            </Button>
          {/snippet}
        </DataTable>
      </CardContent>
    </Card>
  {/if}
  {/if}

  {#if totalItems > pageSize && activeTab !== "stock-providers"}
    <Pagination
      bind:page={currentPage}
      {totalPages}
      {totalItems}
      bind:pageSize
    />
  {/if}
</div>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.media.modal_details_title', 'Media Details')} size="lg">
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
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_filename', 'Filename')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.filename}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_type', 'Type')}</p>
          <p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.mime_type}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_size', 'Size')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFileSize(viewTarget.file_size)}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_status', 'Status')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
        </div>
        <div class="col-span-2">
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_alt_text', 'Alt Text')}</p>
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
<Modal bind:open={showStateModal} title={stateAction === "approve" ? t('app.media.modal_approve_title', 'Approve Media') : t('app.media.modal_reject_title', 'Reject Media')}>
  {#if stateTarget}
    <p class="text-sm" style="color: var(--color-foreground)">
      {stateAction === "approve" ? t('app.media.action_approve', 'Approve') : t('app.media.action_reject', 'Reject')} <strong>{stateTarget.filename}</strong>?
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
<Modal bind:open={showDeleteModal} title={t('app.media.modal_delete_title', 'Delete Media')}>
  <div class="space-y-4">
    <p class="text-sm text-foreground-alt">
      {t('app.media.delete_confirm', 'Are you sure you want to delete')} "{deleteTargetName}"? {t('app.media.delete_cannot_undo', 'This action cannot be undone.')}
    </p>
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('media.cancel')}</Button>
      <Button variant="destructive" onclick={executeDelete}>{t('media.delete')}</Button>
    </div>
  </div>
</Modal>

<!-- Upload Modal -->
<Modal bind:open={showUploadModal} title={t('app.media.modal_upload_title', 'Upload Media')}>
  <div class="space-y-4">
    <div>
      <label for="media-upload-files" class="block text-sm font-medium text-foreground mb-1">{t('app.media.upload_files', 'Files')}</label>
      <input
        id="media-upload-files"
        type="file"
        multiple
        accept="image/*,video/*,audio/*,application/pdf,.doc,.docx"
        onchange={(e) => {
          const files = e.currentTarget.files;
          if (files) uploadFiles = [...uploadFiles, ...Array.from(files)];
        }}
        class="block w-full text-sm text-foreground file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-sm file:font-semibold file:bg-primary file:text-white hover:file:bg-primary/80"
      />
    </div>
    {#if uploadFiles.length > 0}
      <div class="space-y-1 max-h-40 overflow-y-auto">
        {#each uploadFiles as file, i}
          <div class="flex items-center justify-between rounded-lg bg-surface px-3 py-1.5 text-sm">
            <span class="truncate text-foreground">{file.name}</span>
            <span class="ml-2 shrink-0 text-xs text-muted">{formatFileSize(file.size)}</span>
            <button
              type="button"
              class="ml-2 shrink-0 text-muted hover:text-danger"
              onclick={() => { uploadFiles = uploadFiles.filter((_, j) => j !== i); }}
            >
              ✕
            </button>
          </div>
        {/each}
      </div>
    {/if}
    <div>
      <label for="media-upload-group" class="block text-sm font-medium text-foreground mb-1">{t('app.media.upload_user_group', 'User Group (optional)')}</label>
      <select
        id="media-upload-group"
        bind:value={uploadUserGroupId}
        class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
      >
        <option value="">{t('app.media.upload_universal', 'Universal (all users)')}</option>
        {#each userGroups as group}
          <option value={group.id}>{group.name}</option>
        {/each}
      </select>
    </div>
    <div>
      <label for="media-upload-alt" class="block text-sm font-medium text-foreground mb-1">{t('app.media.upload_alt_text', 'Alt Text (optional)')}</label>
      <input
        id="media-upload-alt"
        type="text"
        bind:value={uploadAltText}
        placeholder={t('app.media.upload_alt_placeholder', 'Describe the media...')}
        class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
      />
    </div>
    {#if uploadProgress}
      <div class="text-sm text-muted">
        {t('app.media.upload_progress', 'Uploading {current} of {total}...').replace('{current}', String(uploadProgress.current)).replace('{total}', String(uploadProgress.total))}
        <span class="text-foreground-alt">— {uploadProgress.filename}</span>
      </div>
    {/if}
  </div>
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showUploadModal = false)}>{t('media.cancel')}</Button>
    <Button variant="default" onclick={handleUpload} disabled={!uploadFiles.length || uploadSubmitting}>
      {uploadSubmitting
        ? t('app.media.uploading', 'Uploading...')
        : t('app.media.upload_n_files', 'Upload {n} files').replace('{n}', String(uploadFiles.length))}
    </Button>
  </div>
</Modal>

<!-- MediaActionsCell -->
{#snippet MediaActionsCell({ value, row }: { value: any; row: MediaItem })}
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
