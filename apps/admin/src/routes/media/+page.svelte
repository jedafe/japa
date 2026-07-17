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
    Shield
  } from "lucide-svelte";
  import { api, mediaAdminApi } from "$lib/api";

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

  // Delete confirmation
  let showDeleteModal = $state(false);
  let deleteTargetId = $state("");
  let deleteTargetName = $state("");

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
            <p class="font-medium text-gray-900">${row.filename}</p>
            <p class="text-xs text-gray-500">${row.alt_text || ""}</p>
          </div>
        </div>
      `
    },
    {
      key: "mime_type",
      label: "Type",
      sortable: true,
      render: (value: string) =>
        `<span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-800">${value || "unknown"}</span>`
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
          approved: "bg-success-100 text-success-800",
          pending: "bg-warning-100 text-warning-800",
          quarantined: "bg-danger-100 text-danger-800",
          rejected: "bg-gray-100 text-gray-800"
        };
        return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[value] || "bg-gray-100 text-gray-800"}">${value || "pending"}</span>`;
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
      render: (_: any, row: any) => {
        let actions = "";
        if (row.status === "quarantined" || row.status === "pending") {
          actions += `
            <button onclick="window.__mediaApprove('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1 text-xs font-medium text-success-700 hover:bg-success-50">
              <svg class="h-3 w-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
              Approve
            </button>
            <button onclick="window.__mediaReject('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1 text-xs font-medium text-warning-700 hover:bg-warning-50">
              <svg class="h-3 w-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
              Reject
            </button>
          `;
        }
        actions += `
          <button onclick="window.__mediaDelete('${row.id}', '${row.filename}')" class="inline-flex items-center justify-center rounded-md px-2 py-1 text-xs font-medium text-danger-700 hover:bg-danger-50">
            <svg class="h-3 w-3 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
            Delete
          </button>
        `;
        return `<div class="flex items-center gap-1">${actions}</div>`;
      }
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
      return '<div class="flex h-10 w-10 items-center justify-center rounded bg-warning-100"><svg class="h-5 w-5 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg></div>';
    }
    if (mimeType?.startsWith("audio/")) {
      return '<div class="flex h-10 w-10 items-center justify-center rounded bg-info-100"><svg class="h-5 w-5 text-info-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg></div>';
    }
    return '<div class="flex h-10 w-10 items-center justify-center rounded bg-gray-100"><svg class="h-5 w-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg></div>';
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

  async function approve(id: string) {
    errorMessage = "";
    try {
      const result = await mediaAdminApi.approve(id);
      if (result.ok) {
        load();
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to approve media";
    }
  }

  async function reject(id: string) {
    errorMessage = "";
    try {
      const result = await mediaAdminApi.reject(id);
      if (result.ok) {
        load();
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to reject media";
    }
  }

  function confirmDelete(id: string, filename: string) {
    deleteTargetId = id;
    deleteTargetName = filename;
    showDeleteModal = true;
  }

  async function deleteMedia() {
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

  // Expose action functions for DataTable inline handlers
  if (typeof window !== "undefined") {
    (window as any).__mediaApprove = (id: string) => approve(id);
    (window as any).__mediaReject = (id: string) => reject(id);
    (window as any).__mediaDelete = (id: string, name: string) =>
      confirmDelete(id, name);
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
    <div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-800">
      {errorMessage}
    </div>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <RefreshCw class="h-5 w-5 animate-spin text-gray-400" />
      <span class="ml-2 text-sm text-gray-500">{t('media.loading_media')}</span>
    </div>
  {:else if viewMode === "grid"}
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {#each filteredItems as item (item.id)}
        <Card class="cursor-pointer overflow-hidden transition-shadow hover:shadow-md">
          <div class="flex aspect-square items-center justify-center bg-gray-100">
            {#if item.mime_type?.startsWith("image/")}
              <img
                src={item.url || ""}
                alt={item.alt_text || item.filename}
                class="h-full w-full object-cover"
              />
            {:else}
              {@const Icon = getIcon(item.mime_type)}
              <Icon class="h-8 w-8 text-gray-400" />
            {/if}
          </div>
          <CardContent class="p-3">
            <p class="truncate text-sm font-medium text-gray-900">{item.filename}</p>
            <p class="text-xs text-gray-500">{formatFileSize(item.file_size)}</p>
          </CardContent>
        </Card>
      {:else}
        <div class="col-span-full py-12 text-center text-gray-500">
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
          emptyMessage="No media files found"
        />
      </CardContent>
    </Card>
  {/if}
</div>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Media">
  <div class="space-y-4">
    <p class="text-sm text-gray-600">
      Are you sure you want to delete "{deleteTargetName}"? This action cannot be undone.
    </p>
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('media.cancel')}</Button>
      <Button variant="destructive" onclick={deleteMedia}>{t('media.delete')}</Button>
    </div>
  </div>
</Modal>
