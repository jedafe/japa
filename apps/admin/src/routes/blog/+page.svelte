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
  import { FileText, Plus, Download, RefreshCw, ExternalLink } from "lucide-svelte";
  import { api, blogAdminApi } from "$lib/api";

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

  let { data } = $props();

  let posts = $state<BlogPost[]>([]);
  let loading = $state(true);
  let errorMessage = $state("");
  let activeTab = $state("all");
  let filters = $state<Record<string, string>>({});

  // Reject modal state
  let showRejectModal = $state(false);
  let rejectTarget = $state<BlogPost | null>(null);
  let rejectReason = $state("");
  let rejecting = $state(false);

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
      sortable: true,
      render: (_: any, row: any) => `
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded bg-primary-100">
            <span class="text-primary-600">📝</span>
          </div>
          <div>
            <p class="font-medium text-gray-900">${row.title || "Untitled"}</p>
            <p class="text-sm text-gray-500">${row.excerpt ? row.excerpt.slice(0, 60) + "..." : ""}</p>
          </div>
        </div>
      `
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
      sortable: false,
      render: (value: string[]) => {
        if (!value || value.length === 0) return '<span class="text-gray-400">—</span>';
        return value
          .slice(0, 2)
          .map(
            (c) =>
              `<span class="inline-flex items-center rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-800 mr-1">${c}</span>`
          )
          .join("") + (value.length > 2 ? `<span class="text-xs text-gray-500">+${value.length - 2}</span>` : "");
      }
    },
    {
      key: "tags",
      label: "Tags",
      sortable: false,
      render: (value: string[]) => {
        if (!value || value.length === 0) return '<span class="text-gray-400">—</span>';
        return value
          .slice(0, 3)
          .map(
            (t) =>
              `<span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700 mr-1">${t}</span>`
          )
          .join("");
      }
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (value: string) => {
        const variants: Record<string, string> = {
          published: "bg-success-100 text-success-800",
          draft: "bg-warning-100 text-warning-800",
          review: "bg-info-100 text-info-800",
          archived: "bg-gray-100 text-gray-800"
        };
        return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[value] || "bg-gray-100 text-gray-800"}">${value || "draft"}</span>`;
      }
    },
    {
      key: "published_at",
      label: "Published",
      sortable: true,
      render: (value: string) =>
        value
          ? new Date(value).toLocaleDateString("en-US", {
              year: "numeric",
              month: "short",
              day: "numeric"
            })
          : '<span class="text-gray-400">—</span>'
    },
    {
      key: "views",
      label: "Views",
      sortable: true,
      render: (value: number) => (value ?? 0).toLocaleString()
    },
    {
      key: "actions",
      label: "",
      class: "w-32",
      render: (_: any, row: any) => `
        <div class="flex items-center gap-1">
          <button onclick="window.location.href='/blog/${row.slug || row.id}'" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
          <button onclick="window.open('${row.url || "#"}', '_blank')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">
            <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>
          </button>
        </div>
      `
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

  async function load() {
    loading = true;
    errorMessage = "";
    try {
      const result = await api<BlogPost[]>("/admin/blog");
      if (result.ok) {
        posts = (result.data as BlogPost[]) ?? [];
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to load blog posts";
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

  function openReject(post: BlogPost) {
    rejectTarget = post;
    rejectReason = "";
    showRejectModal = true;
  }

  async function confirmReject() {
    if (!rejectTarget) return;
    rejecting = true;
    errorMessage = "";
    try {
      const result = await blogAdminApi.rejectPost(rejectTarget.id, {
        reason: rejectReason
      });
      if (result.ok) {
        showRejectModal = false;
        load();
      } else {
        errorMessage = result.error;
      }
    } catch {
      errorMessage = "Failed to reject post";
    } finally {
      rejecting = false;
    }
  }

  $effect(() => {
    load();
  });
</script>

<svelte:head>
  <title>Blog — JapaConnect Admin</title>
</svelte:head>

<PageHeader
  title="Blog Posts"
  description="Manage blog content and review submissions"
  actions={[
    { label: "New Post", icon: Plus, href: "/blog/new" },
    { label: "Review Queue", icon: FileText, href: "/blog/review-queue" },
    { label: "Export", icon: Download, variant: "outline" }
  ]}
/>

<div class="space-y-6">
  <Tabs {tabs} bind:value={activeTab} />

  <FilterBar
    bind:value={filters}
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
          <span class="ml-2 text-sm text-gray-500">Loading posts...</span>
        </div>
      {:else}
        <DataTable
          {columns}
          data={filteredPosts}
          pageSize={20}
          selectable={true}
          emptyMessage="No blog posts found"
        />
      {/if}
    </CardContent>
  </Card>
</div>

<!-- Reject Modal -->
<Modal bind:open={showRejectModal} title="Reject Post">
  <div class="space-y-4">
    <p class="text-sm text-gray-600">
      Are you sure you want to reject "{rejectTarget?.title}"? The author will be notified.
    </p>
    <div>
      <label for="reject-reason" class="mb-1 block text-sm font-medium text-gray-700">
        Reason (optional)
      </label>
      <textarea
        id="reject-reason"
        bind:value={rejectReason}
        rows="3"
        class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
        placeholder="Explain why this post was rejected..."
      ></textarea>
    </div>
    <div class="flex justify-end gap-3">
      <Button variant="outline" onclick={() => (showRejectModal = false)}>Cancel</Button>
      <Button variant="destructive" onclick={confirmReject} disabled={rejecting}>
        {#if rejecting}
          <RefreshCw class="h-4 w-4 mr-2 animate-spin" />
          Rejecting...
        {:else}
          Reject
        {/if}
      </Button>
    </div>
  </div>
</Modal>
