<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
  import { goto } from "$app/navigation";
  import PageHeader from "$lib/components/layout/page-header.svelte";
  import { Card, CardContent } from "$lib/components/ui/card";
  import { Tabs } from "$lib/components/ui/tabs";
  import { DataTable } from "$lib/components/ui/data-table";
  import { FilterBar } from "$lib/components/ui/filter-bar";
  import { Button } from "$lib/components/ui/button";
  import { Modal } from "$lib/components/ui/modal";
  import { TableSkeleton } from "$lib/components/ui/skeleton";
  import { FileText, Plus, Download, RefreshCw, Eye, Pencil, Trash2, ExternalLink } from "lucide-svelte";
  import { api, blogAdminApi } from "$lib/api";
  import { hasPermission } from "$lib/stores/permissions.svelte";

  type BlogPost = {
    id: string;
    title: string;
    slug: string;
    author_id: string;
    author_name: string;
    status: string;
    categories: string[];
    tags: string[];
    published_at?: string;
    created_at: string;
    excerpt?: string;
    views?: number;
    url?: string;
  };

  let posts = $state<BlogPost[]>([]);
  let loading = $state(true);
  let error = $state("");
  let activeTab = $state("all");
  let filters = $state<Record<string, string>>({});
  let selectedIds = $state<Set<string>>(new Set());

  // Modals
  let showViewModal = $state(false);
  let viewTarget = $state<BlogPost | null>(null);
  let showRejectModal = $state(false);
  let rejectTarget = $state<BlogPost | null>(null);
  let rejectReason = $state("");
  let acting = $state(false);
  let showDeleteConfirm = $state(false);
  let deleteTarget = $state<BlogPost | null>(null);

  // Backend gates blog mutations on content.create
  const canManageBlog = $derived(hasPermission("content.create"));

  const tabs = [
    { value: "all", label: "All Posts" },
    { value: "published", label: "Published" },
    { value: "draft", label: "Drafts" },
    { value: "review", label: "In Review" }
  ];

  const columns = [
    {
      key: "title",
      label: "Title",
      sortable: true
    },
    {
      key: "author_name",
      label: "Author",
      sortable: true,
      render: (value: string) => value || "Unknown"
    },
    {
      key: "categories",
      label: "Categories",
      render: (value: string[]) => {
        if (!value || value.length === 0) return "—";
        return value.slice(0, 2).join(", ") + (value.length > 2 ? ` +${value.length - 2}` : "");
      }
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (value: string) => {
        const colors: Record<string, string> = {
          published: "bg-success/15 text-success dark:text-success",
          draft: "bg-warning/15 text-warning dark:text-warning",
          review: "bg-info/15 text-info dark:text-info",
          archived: "bg-muted/15 text-muted dark:text-muted"
        };
        return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[value] || colors.draft}">${value || "draft"}</span>`;
      }
    },
    {
      key: "published_at",
      label: "Published",
      sortable: true,
      render: (value: string) => value ? new Date(value).toLocaleDateString() : "—"
    },
    {
      key: "views",
      label: "Views",
      sortable: true,
      render: (value: number) => (value ?? 0).toLocaleString()
    },
    {
      key: "actions",
      label: "Actions",
      class: "w-40",
      component: BlogActionsCell
    }
  ];

  let filteredPosts = $derived.by(() => {
    let result = [...posts];
    if (activeTab !== "all") {
      result = result.filter((p) => p.status === activeTab);
    }
    if (filters.status) {
      result = result.filter((p) => p.status === filters.status);
    }
    if (filters.category) {
      result = result.filter((p) => p.categories?.includes(filters.category));
    }
    if (filters.search) {
      const q = filters.search.toLowerCase();
      result = result.filter(
        (p) =>
          p.title?.toLowerCase().includes(q) ||
          p.author_name?.toLowerCase().includes(q) ||
          p.tags?.some((t) => t.toLowerCase().includes(q))
      );
    }
    return result;
  });

  async function loadPosts() {
    loading = true;
    error = "";
    try {
      const result = await api<BlogPost[]>("/admin/blog");
      if (result.ok) {
        posts = (result.data as BlogPost[]) ?? [];
      } else {
        error = result.error;
      }
    } catch {
      error = "Failed to load blog posts";
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

  function openView(post: BlogPost) {
    viewTarget = post;
    showViewModal = true;
  }

  function openReject(post: BlogPost) {
    rejectTarget = post;
    rejectReason = "";
    showRejectModal = true;
  }

  async function confirmReject() {
    if (!rejectTarget) return;
    acting = true;
    error = "";
    try {
      const result = await blogAdminApi.rejectPost(rejectTarget.id, { reason: rejectReason });
      if (result.ok) {
        showRejectModal = false;
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = "Failed to reject post";
    } finally {
      acting = false;
    }
  }

  function confirmDelete(post: BlogPost) {
    deleteTarget = post;
    showDeleteConfirm = true;
  }

  async function executeDelete() {
    if (!deleteTarget) return;
    acting = true;
    try {
      const result = await api(`/admin/blog/${deleteTarget.id}`, { method: "DELETE" });
      if (result.ok) {
        showDeleteConfirm = false;
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = "Failed to delete post";
    } finally {
      acting = false;
    }
  }

  function openExternal(post: BlogPost) {
    if (post.url) {
      window.open(post.url, "_blank");
    }
  }

  let bulkDeleting = $state(false);
  async function bulkDelete() {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    bulkDeleting = true;
    error = "";
    try {
      await Promise.all(ids.map((id) => api(`/admin/blog/${id}`, { method: "DELETE" })));
      selectedIds = new Set();
      loadPosts();
    } catch {
      error = "Failed to delete selected posts";
    } finally {
      bulkDeleting = false;
    }
  }

  $effect(() => {
    loadPosts();
  });
</script>

<svelte:head>
  <title>Blog — JapaConnect Admin</title>
</svelte:head>

<PageHeader
  title="Blog Posts"
  description="Manage blog content and review submissions"
  actions={[
    { label: "New Post", icon: Plus, onclick: () => goto("/blog/new") },
    { label: "Review Queue", icon: FileText, onclick: () => goto("/blog/review-queue") },
    { label: "Refresh", icon: RefreshCw, variant: "ghost", onclick: loadPosts }
  ]}
/>

<div class="space-y-6">
  <Tabs {tabs} bind:value={activeTab} />

  <FilterBar
    filters={[
      {
        key: "status",
        label: "Status",
        options: [
          { value: "published", label: "Published" },
          { value: "draft", label: "Draft" },
          { value: "review", label: "In Review" },
          { value: "archived", label: "Archived" }
        ]
      },
      {
        key: "category",
        label: "Category",
        options: [
          { value: "technology", label: "Technology" },
          { value: "community", label: "Community" },
          { value: "events", label: "Events" },
          { value: "news", label: "News" }
        ]
      }
    ]}
    onsearch={handleSearch}
    onclear={handleClear}
  />

  {#if error}
    <div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
      {error}
    </div>
  {/if}

  <Card>
    <CardContent class="p-0">
      {#if loading}
        <TableSkeleton rows={8} columns={columns.length} />
      {:else}
        <div aria-live="polite">
          <DataTable
            {columns}
            data={filteredPosts}
            selectable
            bind:selectedIds
            emptyMessage="No blog posts found"
          >
            {#snippet bulkActions()}
              <Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
                <Trash2 class="mr-1 h-3.5 w-3.5" />
                {bulkDeleting ? "Deleting..." : `Delete Selected (${selectedIds.size})`}
              </Button>
            {/snippet}
          </DataTable>
        </div>
      {/if}
    </CardContent>
  </Card>
</div>

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Post Details" size="default">
  {#if viewTarget}
    <div class="space-y-4">
      <div>
        <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.title')}</p>
        <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
      </div>
      {#if viewTarget.excerpt}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.excerpt')}</p>
          <p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.excerpt}</p>
        </div>
      {/if}
      <div class="grid grid-cols-2 gap-4">
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.author')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.author_name}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.status')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.views')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.views ?? 0).toLocaleString()}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.published')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.published_at ? new Date(viewTarget.published_at).toLocaleString() : "—"}</p>
        </div>
      </div>
      {#if viewTarget.categories?.length}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.categories')}</p>
          <div class="mt-1 flex flex-wrap gap-1">
            {#each viewTarget.categories as cat}
              <span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)">{cat}</span>
            {/each}
          </div>
        </div>
      {/if}
      {#if viewTarget.tags?.length}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('blog.tags')}</p>
          <div class="mt-1 flex flex-wrap gap-1">
            {#each viewTarget.tags as tag}
              <span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background: var(--color-surface); color: var(--color-muted)">{tag}</span>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}
  <div class="mt-6 flex justify-end gap-2">
    <Button variant="outline" onclick={() => goto(`/blog/${viewTarget?.slug || viewTarget?.id}`)}>{t('blog.edit')}</Button>
    {#if viewTarget?.url}
      <Button variant="outline" onclick={() => openExternal(viewTarget!)}>
        <ExternalLink class="mr-1 h-3.5 w-3.5" /> View Live
      </Button>
    {/if}
    <Button variant="outline" onclick={() => (showViewModal = false)}>{t('blog.close')}</Button>
  </div>
</Modal>

<!-- Reject Modal -->
<Modal bind:open={showRejectModal} title="Reject Post">
  <div class="space-y-4">
    <p class="text-sm" style="color: var(--color-foreground)">
      Are you sure you want to reject "{rejectTarget?.title}"? The author will be notified.
    </p>
    <div>
      <label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('blog.reason_optional')}</label>
      <textarea
        id="reject-reason"
        bind:value={rejectReason}
        rows="3"
        class="w-full rounded-lg border p-3 text-sm"
        style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
        placeholder="Explain why this post was rejected..."
      ></textarea>
    </div>
  </div>
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('blog.cancel')}</Button>
    <Button variant="destructive" onclick={confirmReject} disabled={acting}>
      {acting ? "Rejecting..." : "Reject"}
    </Button>
  </div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Delete Post">
  <div class="space-y-4">
    <p class="text-sm" style="color: var(--color-foreground)">
      Are you sure you want to delete "{deleteTarget?.title}"? This action cannot be undone.
    </p>
  </div>
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('blog.cancel')}</Button>
    <Button variant="destructive" onclick={executeDelete} disabled={acting}>
      {acting ? "Deleting..." : "Delete"}
    </Button>
  </div>
</Modal>

<!-- BlogActionsCell component -->
{#snippet BlogActionsCell(post: BlogPost)}
  <div class="flex items-center gap-1">
    <Button variant="ghost" size="sm" onclick={() => openView(post)}>
      <Eye class="h-3.5 w-3.5" />
    </Button>
    <Button variant="ghost" size="sm" onclick={() => goto(`/blog/${post.slug || post.id}`)}>
      <Pencil class="h-3.5 w-3.5" />
    </Button>
    {#if post.url}
      <Button variant="ghost" size="sm" onclick={() => openExternal(post)}>
        <ExternalLink class="h-3.5 w-3.5" />
      </Button>
    {/if}
    {#if post.status === "review"}
      <Button variant="ghost" size="sm" onclick={() => openReject(post)} style="color: var(--color-danger)">
        <Trash2 class="h-3.5 w-3.5" />
      </Button>
    {:else if canManageBlog}
      <Button variant="ghost" size="sm" onclick={() => confirmDelete(post)} style="color: var(--color-danger)">
        <Trash2 class="h-3.5 w-3.5" />
      </Button>
    {/if}
  </div>
{/snippet}
