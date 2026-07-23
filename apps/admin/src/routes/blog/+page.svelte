<script lang="ts">
	import { onMount } from 'svelte';
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
  import { FileText, Plus, Download, RefreshCw, Eye, Pencil, Trash2, ExternalLink, CheckCircle, XCircle } from "lucide-svelte";
  import { api, blogAdminApi } from "$lib/api";
  import { hasPermission } from "$lib/stores/permissions.svelte";

  type RawBlogPost = {
    id: string;
    title: string;
    slug: string;
    author_id: string;
    author_name?: string;
    status: string;
    content_type?: string;
    body?: string;
    body_html?: string;
    field_values?: string | null;
    published_at?: string | null;
    publish_at?: string | null;
    created_at: string;
    updated_at?: string;
    view_count?: number;
    views?: number;
    excerpt?: string;
    categories?: string[];
    tags?: string[];
    url?: string;
    canonical_url?: string | null;
  };

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

  function mapBlogPost(raw: RawBlogPost): BlogPost {
    let categories: string[] = raw.categories ?? [];
    let tags: string[] = raw.tags ?? [];

    // Extract from field_values JSON if available
    if (raw.field_values) {
      try {
        const fv = JSON.parse(raw.field_values);
        if (Array.isArray(fv.categories)) categories = fv.categories;
        if (Array.isArray(fv.tags)) tags = fv.tags;
      } catch { /* ignore parse errors */ }
    }

    // Derive excerpt from body if not provided
    let excerpt = raw.excerpt;
    if (!excerpt && raw.body) {
      try {
        const parsed = JSON.parse(raw.body);
        // Tiptap JSON: extract text from first few paragraphs
        const texts: string[] = [];
        const extract = (node: any) => {
          if (texts.join(' ').length > 160) return;
          if (node.type === 'text' && node.text) {
            const clean = node.text.trim();
            if (clean && clean !== '/') texts.push(clean);
          }
          if (node.content) node.content.forEach(extract);
        };
        if (parsed.content) parsed.content.slice(0, 5).forEach(extract);
        const joined = texts.join(' ').trim();
        excerpt = joined.substring(0, 160);
        if (joined.length > 160) excerpt += '…';
      } catch {
        const clean = raw.body.trim();
        if (clean && clean !== '/') excerpt = clean.substring(0, 160);
      }
    }

    return {
      id: raw.id,
      title: raw.title || raw.slug,
      slug: raw.slug,
      author_id: raw.author_id,
      author_name: raw.author_name || '—',
      status: raw.status || 'draft',
      categories,
      tags,
      published_at: raw.published_at ?? raw.publish_at ?? (raw.status === 'published' ? raw.created_at : undefined),
      created_at: raw.created_at,
      excerpt: excerpt || undefined,
      views: raw.views ?? raw.view_count ?? 0,
      url: raw.url || (raw.slug ? `/blog/${raw.slug}` : undefined),
    };
  }

  let posts = $state<BlogPost[]>([]);
  let postsLoaded = $state(false);
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

  const tabs = $derived([
    { value: "all", label: `${t('app.blog.tab_all', 'All Posts')} (${posts.length})` },
    { value: "published", label: `${t('app.blog.tab_published', 'Published')} (${posts.filter(p => p.status === 'published').length})` },
    { value: "draft", label: `${t('app.blog.tab_drafts', 'Drafts')} (${posts.filter(p => p.status === 'draft').length})` },
    { value: "in_review", label: `${t('app.blog.tab_in_review', 'In Review')} (${posts.filter(p => p.status === 'in_review').length})` },
    { value: "removed", label: `${t('app.blog.tab_removed', 'Removed')} (${posts.filter(p => p.status === 'removed').length})` }
  ]);

  const columns = $derived([
    {
      key: "title",
      label: t('app.blog.col_title', 'Title'),
      sortable: true
    },
    {
      key: "author_name",
      label: t('app.blog.col_author', 'Author'),
      sortable: true,
      render: (value: string) => value || t('app.blog.unknown', 'Unknown')
    },
    {
      key: "categories",
      label: t('app.blog.col_categories', 'Categories'),
      render: (value: string[]) => {
        if (!value || value.length === 0) return "—";
        return value.slice(0, 2).join(", ") + (value.length > 2 ? ` +${value.length - 2}` : "");
      }
    },
    {
      key: "status",
      label: t('app.blog.col_status', 'Status'),
      sortable: true,
      render: (value: string) => {
        const colors: Record<string, string> = {
          published: "var(--color-success)",
          draft: "var(--color-warning)",
          in_review: "var(--color-info)",
          review: "var(--color-info)",
          removed: "var(--color-danger)",
          archived: "var(--color-muted)"
        };
        const labels: Record<string, string> = {
          published: "published",
          draft: "draft",
          in_review: "in review",
          review: "in review",
          removed: "removed",
          archived: "archived"
        };
        const color = colors[value] || colors.draft;
        const label = labels[value] || value || "draft";
        return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, ${color} 15%, transparent); color: ${color}">${label}</span>`;
      }
    },
    {
      key: "published_at",
      label: t('app.blog.col_published', 'Published'),
      sortable: true,
      render: (value: string) => value ? new Date(value).toLocaleDateString() : "—"
    },
    {
      key: "views",
      label: t('app.blog.col_views', 'Views'),
      sortable: true,
      render: (value: number) => (value ?? 0).toLocaleString()
    },
    {
      key: "actions",
      label: t('app.blog.col_actions', 'Actions'),
      class: "w-40",
      componentSnippet: BlogActionsCell
    }
  ]);

  const categoryOptions = $derived.by(() => {
    const cats = new Set<string>();
    for (const p of posts) {
      if (p.categories) {
        for (const c of p.categories) cats.add(c);
      }
    }
    return Array.from(cats).sort().map((c) => ({ value: c, label: c }));
  });

  let filteredPosts = $derived.by(() => {
    let result = [...posts];
    if (activeTab !== "all") {
      result = result.filter((p) => p.status === activeTab);
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
    postsLoaded = false;
    error = "";
    try {
      const result = await api<RawBlogPost[]>("/admin/blog");
      postsLoaded = true;
      if (result.ok) {
        const raw = (result.data as RawBlogPost[]) ?? [];
        posts = raw.map(mapBlogPost);
      } else {
        error = result.error;
      }
    } catch {
      postsLoaded = true;
      error = t('app.blog.error_load', 'Failed to load blog posts');
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
      error = t('app.blog.error_reject', 'Failed to reject post');
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
      let result;
      if (deleteTarget.status === 'removed') {
        // Hard-delete: permanently remove from database
        result = await blogAdminApi.deletePost(deleteTarget.id);
      } else {
        // Soft-delete: move to "Removed" tab
        result = await blogAdminApi.updatePost(deleteTarget.id, { status: 'removed' });
      }
      if (result.ok) {
        showDeleteConfirm = false;
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = t('app.blog.error_delete', 'Failed to delete post');
    } finally {
      acting = false;
    }
  }

  async function handlePublish(post: BlogPost) {
    acting = true;
    error = "";
    try {
      const result = await blogAdminApi.publishPost(post.id);
      if (result.ok) {
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = t('app.blog.error_publish', 'Failed to publish post');
    } finally {
      acting = false;
    }
  }

  async function handleUnpublish(post: BlogPost) {
    acting = true;
    error = "";
    try {
      const result = await blogAdminApi.unpublishPost(post.id);
      if (result.ok) {
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = t('app.blog.error_unpublish', 'Failed to unpublish post');
    } finally {
      acting = false;
    }
  }

  async function handleRestore(post: BlogPost) {
    acting = true;
    error = "";
    try {
      const result = await blogAdminApi.restorePost(post.id);
      if (result.ok) {
        loadPosts();
      } else {
        error = result.error;
      }
    } catch {
      error = t('app.blog.error_restore', 'Failed to restore post');
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
      error = t('app.blog.error_bulk_delete', 'Failed to delete selected posts');
    } finally {
      bulkDeleting = false;
    }
  }

  onMount(() => {
    loadPosts();
  });
</script>

<svelte:head>
  <title>{t('app.blog.page_title', 'Blog — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
  title={t('app.blog.title', 'Blog Posts')}
  description={t('app.blog.description', 'Manage blog content and review submissions')}
  actions={[
    { label: t('app.blog.new_post', 'New Post'), icon: Plus, onclick: () => goto("/blog/new") },
    { label: t('app.blog.review_queue', 'Review Queue'), icon: FileText, onclick: () => goto("/blog/review-queue") },
    { label: t('app.blog.refresh', 'Refresh'), icon: RefreshCw, variant: "ghost", onclick: loadPosts }
  ]}
/>

<div class="space-y-6">
  <Tabs {tabs} bind:value={activeTab} />

  <FilterBar
    filters={[
      {
        key: "category",
        label: t('app.blog.filter_category', 'Category'),
        options: categoryOptions
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
      {#if !postsLoaded}
        <TableSkeleton rows={8} columns={columns.length} />
      {:else}
        <div aria-live="polite">
          <DataTable
            {columns}
            data={filteredPosts}
            selectable
            bind:selectedIds
            pageSize={25}
            emptyMessage={t('app.blog.empty_message', 'No blog posts found')}
          >
            {#snippet bulkActions()}
              <Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
                <Trash2 class="mr-1 h-3.5 w-3.5" />
                {bulkDeleting ? t('app.blog.deleting', 'Deleting...') : `${t('app.blog.delete_selected', 'Delete Selected')} (${selectedIds.size})`}
              </Button>
            {/snippet}
          </DataTable>
        </div>
      {/if}
    </CardContent>
  </Card>
</div>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.blog.modal_post_details', 'Post Details')} size="default">
  {#if viewTarget}
    <div class="space-y-4">
      <div>
        <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_title', 'Title')}</p>
        <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
      </div>
      {#if viewTarget.excerpt}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_excerpt', 'Excerpt')}</p>
          <p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.excerpt}</p>
        </div>
      {/if}
      <div class="grid grid-cols-2 gap-4">
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_author', 'Author')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.author_name}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_status', 'Status')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_views', 'Views')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.views ?? 0).toLocaleString()}</p>
        </div>
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_published', 'Published')}</p>
          <p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.published_at ? new Date(viewTarget.published_at).toLocaleString() : "—"}</p>
        </div>
      </div>
      {#if viewTarget.categories?.length}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_categories', 'Categories')}</p>
          <div class="mt-1 flex flex-wrap gap-1">
            {#each viewTarget.categories as cat}
              <span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)">{cat}</span>
            {/each}
          </div>
        </div>
      {/if}
      {#if viewTarget.tags?.length}
        <div>
          <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_tags', 'Tags')}</p>
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
    <Button variant="outline" onclick={() => goto(`/blog/${viewTarget?.slug || viewTarget?.id}`)}>{t('app.blog.detail_edit', 'Edit')}</Button>
    {#if viewTarget?.url}
      <Button variant="outline" onclick={() => openExternal(viewTarget!)}>
        <ExternalLink class="mr-1 h-3.5 w-3.5" /> {t('app.blog.view_live', 'View Live')}
      </Button>
    {/if}
  </div>
</Modal>

<!-- Reject Modal -->
<Modal bind:open={showRejectModal} title={t('app.blog.modal_reject_post', 'Reject Post')}>
  <div class="space-y-4">
    <p class="text-sm" style="color: var(--color-foreground)">
      {t('app.blog.reject_confirm', 'Are you sure you want to reject "{title}"? The author will be notified.').replace('{title}', rejectTarget?.title ?? '')}
    </p>
    <div>
      <label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.blog.reason_optional', 'Reason (optional)')}</label>
      <textarea
        id="reject-reason"
        bind:value={rejectReason}
        rows="3"
        class="w-full rounded-lg border p-3 text-sm"
        style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
        placeholder={t('app.blog.reject_placeholder', 'Explain why this post was rejected...')}
      ></textarea>
    </div>
  </div>
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
    <Button variant="destructive" onclick={confirmReject} disabled={acting}>
      {acting ? t('app.blog.rejecting', 'Rejecting...') : t('app.blog.reject', 'Reject')}
    </Button>
  </div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={deleteTarget?.status === 'removed' ? t('app.blog.modal_permanent_delete', 'Permanently Delete Post') : t('app.blog.modal_delete_post', 'Delete Post')}>
  <div class="space-y-4">
    <p class="text-sm" style="color: var(--color-foreground)">
      {#if deleteTarget?.status === 'removed'}
        {t('app.blog.permanent_delete_confirm', 'Are you sure you want to permanently delete "{title}"? This cannot be undone.').replace('{title}', deleteTarget?.title ?? '')}
      {:else}
        {t('app.blog.delete_confirm', 'Are you sure you want to delete "{title}"? It will be moved to the Removed tab.').replace('{title}', deleteTarget?.title ?? '')}
      {/if}
    </p>
  </div>
  <div class="mt-4 flex justify-end gap-2">
    <Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
    <Button variant="destructive" onclick={executeDelete} disabled={acting}>
      {acting ? t('app.blog.deleting', 'Deleting...') : (deleteTarget?.status === 'removed' ? t('app.blog.permanent_delete', 'Permanently Delete') : t('app.blog.delete', 'Delete'))}
    </Button>
  </div>
</Modal>

<!-- BlogActionsCell component -->
{#snippet BlogActionsCell({ value, row }: { value: any; row: BlogPost })}
  <div class="flex items-center gap-1">
    <Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t('app.blog.action_view', 'View post')}>
      <Eye class="h-3.5 w-3.5" />
    </Button>
    <Button variant="ghost" size="sm" onclick={() => goto(`/blog/${row.slug || row.id}`)} aria-label={t('app.blog.action_edit', 'Edit post')}>
      <Pencil class="h-3.5 w-3.5" />
    </Button>
    {#if row.status === 'draft' || row.status === 'in_review'}
      <Button variant="ghost" size="sm" onclick={() => handlePublish(row)} disabled={acting} style="color: var(--color-success)" aria-label={t('app.blog.action_publish', 'Publish')}>
        <CheckCircle class="h-3.5 w-3.5" />
      </Button>
    {:else if row.status === 'published'}
      <Button variant="ghost" size="sm" onclick={() => handleUnpublish(row)} disabled={acting} style="color: var(--color-warning)" aria-label={t('app.blog.action_unpublish', 'Unpublish')}>
        <XCircle class="h-3.5 w-3.5" />
      </Button>
    {:else if row.status === 'removed'}
      <Button variant="ghost" size="sm" onclick={() => handleRestore(row)} disabled={acting} style="color: var(--color-info)" aria-label={t('app.blog.action_restore', 'Restore')}>
        <RefreshCw class="h-3.5 w-3.5" />
      </Button>
    {/if}
    {#if row.url && row.status !== 'removed'}
      <Button variant="ghost" size="sm" onclick={() => openExternal(row)} aria-label={t('app.blog.action_view_live', 'View live')}>
        <ExternalLink class="h-3.5 w-3.5" />
      </Button>
    {/if}
    {#if row.status === 'removed'}
      <Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)" aria-label={t('app.blog.action_delete', 'Delete post')}>
        <Trash2 class="h-3.5 w-3.5" />
      </Button>
    {:else if canManageBlog}
      <Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)" aria-label={t('app.blog.action_delete', 'Delete post')}>
        <Trash2 class="h-3.5 w-3.5" />
      </Button>
    {/if}
  </div>
{/snippet}
