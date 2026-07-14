<script lang="ts">
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
  import { FileText, Plus, Download, RefreshCw } from "lucide-svelte";
  import { api, contentAdminApi } from "$lib/api";

  type ContentItem = {
    id: string;
    title: string;
    content_type: string;
    status: string;
    author_id: string;
    author_name: string;
    space_name?: string;
    created_at: string;
    removed_at?: string;
    thumbnail?: string;
    url?: string;
    views?: number;
  };

  type ContentStats = {
    total: number;
    published: number;
    drafts: number;
    archived: number;
  };

  let { data } = $props();

  let items = $state<ContentItem[]>([]);
  let loading = $state(true);
  let errorMessage = $state("");
  let activeTab = $state("all");
  let filters = $state<Record<string, string>>({});

  // Remove modal state
  let showRemoveModal = $state(false);
  let removeTarget = $state<ContentItem | null>(null);
  let removeReason = $state("");
  let removing = $state(false);

  // Restore modal state
  let showRestoreModal = $state(false);
  let restoreTarget = $state<ContentItem | null>(null);
  let restoring = $state(false);

  const tabs = [
    { value: "all", label: "All Content" },
    { value: "published", label: "Published" },
    { value: "draft", label: "Drafts" },
    { value: "archived", label: "Archived" },
    { value: "removed", label: "Removed" }
  ];

  const columns = [
    {
      key: "title",
      label: "Title",
      sortable: true,
      render: (_: any, row: any) => `
        <div class="flex items-center gap-3">
          ${row.thumbnail ? `<img src="${row.thumbnail}" alt="" class="h-10 w-10 rounded object-cover" />` : '<div class="h-10 w-10 rounded bg-gray-100 flex items-center justify-center"><span class="text-gray-400">📄</span></div>'}
          <div>
            <p class="font-medium text-gray-900">${row.title || "Untitled"}</p>
            <p class="text-sm text-gray-500">${row.author_name || "Unknown"}</p>
          </div>
        </div>
      `
    },
    {
      key: "content_type",
      label: "Type",
      sortable: true,
      render: (value: string) =>
        `<span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-800">${value || "post"}</span>`
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (value: string) => {
        const variants: Record<string, string> = {
          published: "bg-success-100 text-success-800",
          draft: "bg-warning-100 text-warning-800",
          removed: "bg-danger-100 text-danger-800",
          archived: "bg-gray-100 text-gray-800"
        };
        return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[value] || "bg-gray-100 text-gray-800"}">${value || "draft"}</span>`;
      }
    },
    {
      key: "space_name",
      label: "Space",
      sortable: true,
      render: (value: string) => value || "—"
    },
    {
      key: "views",
      label: "Views",
      sortable: true,
      render: (value: number) => (value ?? 0).toLocaleString()
    },
    {
      key: "created_at",
      label: "Created",
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
      class: "w-32",
      render: (_: any, row: any) => {
        if (row.status === "removed" || row.removed_at) {
          return `
            <div class="flex items-center gap-1">
              <button onclick="window.__contentRestore('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-success-700 hover:bg-success-50">Restore</button>
            </div>
          `;
        }
        return `
          <div class="flex items-center gap-1">
            <button onclick="window.location.href='/content/${row.id}'" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
            <button onclick="window.open('${row.url || "#"}', '_blank')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">View</button>
            <button onclick="window.__contentRemove('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Remove</button>
          </div>
        `;
      }
    }
  ];

  let filteredItems = $derived.by(() => {
    let result = [...items];

    if (activeTab === "removed") {
      result = result.filter((i) => i.status === "removed" || i.removed_at);
    } else if (activeTab !== "all") {
      result = result.filter(
        (i) => i.status === activeTab && i.status !== "removed" && !i.removed_at
      );
    } else {
      result = result.filter((i) => i.status !== "removed" && !i.removed_at);
    }

    if (filters.status) {
      result = result.filter((i) => i.status === filters.status);
    }
    if (filters.content_type) {
      result = result.filter((i) => i.content_type === filters.content_type);
    }
    if (filters.search) {
      const q = filters.search.toLowerCase();
      result = result.filter(
        (i) =>
          i.title?.toLowerCase().includes(q) ||
          i.author_name?.toLowerCase().includes(q)
      );
    }

    return result;
  });

  async function load() {
    loading = true;
    errorMessage = "";
    try {
      const result = await api<ContentItem[]>("/admin/content");
      if (result.ok) {
        items = (result.data as ContentItem[]) ?? [];
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to load content";
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

  function openRemove(item: ContentItem) {
    removeTarget = item;
    removeReason = "";
    showRemoveModal = true;
  }

  async function confirmRemove() {
    if (!removeTarget) return;
    removing = true;
    errorMessage = "";
    try {
      const result = await contentAdminApi.remove(removeTarget.id);
      if (result.ok) {
        showRemoveModal = false;
        load();
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to remove content";
    } finally {
      removing = false;
    }
  }

  function openRestore(item: ContentItem) {
    restoreTarget = item;
    showRestoreModal = true;
  }

  async function confirmRestore() {
    if (!restoreTarget) return;
    restoring = true;
    errorMessage = "";
    try {
      const result = await contentAdminApi.restore(restoreTarget.id);
      if (result.ok) {
        showRestoreModal = false;
        load();
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to restore content";
    } finally {
      restoring = false;
    }
  }

  // Expose action functions for DataTable inline handlers
  if (typeof window !== "undefined") {
    (window as any).__contentRemove = (id: string) => {
      const item = items.find((i) => i.id === id);
      if (item) openRemove(item);
    };
    (window as any).__contentRestore = (id: string) => {
      const item = items.find((i) => i.id === id);
      if (item) openRestore(item);
    };
  }

  $effect(() => {
    load();
  });
</script>

<svelte:head>
  <title>Content — JapaConnect Admin</title>
</svelte:head>

<PageHeader
  title="Content"
  description="Manage all platform content"
  actions={[
    { label: "New Post", icon: Plus, href: "/content/new" },
    { label: "Export", icon: Download, variant: "outline" }
  ]}
/>

<div class="space-y-6">
  <Tabs {tabs} bind:value={activeTab} />

  <FilterBar
    bind:value={filters}
    filters={[
      {
        key: "content_type",
        label: "Type",
        options: [
          { value: "post", label: "Post" },
          { value: "comment", label: "Comment" },
          { value: "blog", label: "Blog" },
          { value: "forum", label: "Forum" },
          { value: "page", label: "Page" }
        ]
      },
      {
        key: "status",
        label: "Status",
        options: [
          { value: "published", label: "Published" },
          { value: "draft", label: "Draft" },
          { value: "archived", label: "Archived" }
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

  <Card>
    <CardContent class="p-0">
      {#if loading}
        <div class="flex items-center justify-center py-12">
          <RefreshCw class="h-5 w-5 animate-spin text-gray-400" />
          <span class="ml-2 text-sm text-gray-500">Loading content...</span>
        </div>
      {:else}
        <DataTable
          {columns}
          data={filteredItems}
          pageSize={20}
          selectable={true}
          emptyMessage="No content found"
        />
      {/if}
    </CardContent>
  </Card>
</div>

<!-- Remove Modal -->
<Modal bind:open={showRemoveModal} title="Remove Content">
  <div class="space-y-4">
    <p class="text-sm text-gray-600">
      Are you sure you want to remove this content? This action can be undone by restoring the content.
    </p>
    {#if removeTarget}
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm">
        <span class="font-medium">{removeTarget.title || "Untitled"}</span>
        &middot; {removeTarget.content_type}
      </div>
    {/if}
    <div>
      <label for="remove-reason" class="mb-1 block text-sm font-medium text-gray-700">
        Reason (optional)
      </label>
      <textarea
        id="remove-reason"
        bind:value={removeReason}
        rows="3"
        class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
        placeholder="Explain why this content is being removed..."
      ></textarea>
    </div>
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showRemoveModal = false)}>Cancel</Button>
      <Button variant="destructive" onclick={confirmRemove} disabled={removing}>
        {#if removing}
          <RefreshCw class="h-4 w-4 mr-2 animate-spin" />
          Removing...
        {:else}
          Remove
        {/if}
      </Button>
    </div>
  </div>
</Modal>

<!-- Restore Modal -->
<Modal bind:open={showRestoreModal} title="Restore Content">
  <div class="space-y-4">
    <p class="text-sm text-gray-600">
      Are you sure you want to restore this content? It will become visible again on the platform.
    </p>
    {#if restoreTarget}
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm">
        <span class="font-medium">{restoreTarget.title || "Untitled"}</span>
        &middot; {restoreTarget.content_type}
      </div>
    {/if}
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showRestoreModal = false)}>Cancel</Button>
      <Button onclick={confirmRestore} disabled={restoring}>
        {#if restoring}
          <RefreshCw class="h-4 w-4 mr-2 animate-spin" />
          Restoring...
        {:else}
          Restore
        {/if}
      </Button>
    </div>
  </div>
</Modal>
