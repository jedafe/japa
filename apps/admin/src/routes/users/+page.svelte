<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import PageHeader from "$lib/components/layout/page-header.svelte";
  import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
  import { Tabs } from "$lib/components/ui/tabs";
  import { DataTable } from "$lib/components/ui/data-table";
  import { FilterBar } from "$lib/components/ui/filter-bar";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Modal } from "$lib/components/ui/modal";
  import { Select } from "$lib/components/ui/select";
  import { Users, UserPlus, Download, RefreshCw } from "lucide-svelte";
  import { api, impersonationApi, userManagementApi } from "$lib/api";

  type User = {
    id: string;
    username: string;
    email: string;
    groups: string[];
    status: string;
    created_at: string;
    avatar_url?: string;
  };

  type UserStats = {
    total: number;
    active: number;
    pending: number;
    suspended: number;
  };

  let { data } = $props();

  let users = $state<User[]>([]);
  let currentPage = $state(0);
  let totalPages = $state(1);
  let loading = $state(true);
  let searchQuery = $state("");
  let filterValues = $state<Record<string, string>>({});
  let filterGroup = $derived(filterValues.group || "");
  let groupOptions = $state<string[]>(["user", "moderator", "admin", "super_admin"]);
  let stats = $state<UserStats>({ total: 0, active: 0, pending: 0, suspended: 0 });

  // Impersonation state
  let impersonating = $state(false);
  let impersonatingUsername = $state("");
  let impersonateTarget = $state<User | null>(null);
  let showImpersonateConfirm = $state(false);
  let impersonateError = $state("");

  // Tab navigation - derived from current URL
  let activeTab = $derived.by(() => {
    const path = page.url.pathname;
    if (path === "/users") return "users";
    if (path.startsWith("/users/roles")) return "roles";
    if (path.startsWith("/users/groups")) return "groups";
    return "users";
  });

  const tabs = [
    { value: "users", label: "Users", icon: Users },
    { value: "roles", label: "Roles", icon: Users },
    { value: "groups", label: "Groups", icon: Users }
  ];

  const columns = [
    {
      key: "username",
      label: "Username",
      sortable: true,
      render: (_: any, row: any) => `
        <div class="flex items-center gap-3">
          <img src="${row.avatar_url || '/default-avatar.png'}" alt="" class="h-8 w-8 rounded-full" />
          <div>
            <p class="font-medium text-gray-900">${row.username || 'Unknown'}</p>
            <p class="text-sm text-gray-500">${row.email || ''}</p>
          </div>
        </div>
      `
    },
    {
      key: "groups",
      label: "Groups",
      sortable: true,
      render: (value: string[]) => {
        return value.map(g => `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-gray-100 text-gray-800">${g}</span>`).join(" ");
      }
    },
    {
      key: "status",
      label: "Status",
      sortable: true,
      render: (value: string) => {
        const variants: Record<string, string> = { active: "success", pending: "warning", suspended: "danger", banned: "destructive" };
        const classes: Record<string, string> = {
          active: "bg-success-100 text-success-800",
          pending: "bg-warning-100 text-warning-800",
          suspended: "bg-danger-100 text-danger-800",
          banned: "bg-danger-100 text-danger-800"
        };
        return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${classes[value] || 'bg-gray-100 text-gray-800'}">${value || 'active'}</span>`;
      }
    },
    {
      key: "created_at",
      label: "Joined",
      sortable: true,
      render: (value: string) => new Date(value).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
    },
    {
      key: "actions",
      label: "",
      class: "w-24",
      render: (_: any, row: any) => `
        <div class="flex items-center gap-1">
          <button onclick="window.location.href='/users/${row.id}'" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">View</button>
          <button onclick="window.location.href='/users/${row.id}?edit=true'" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
        </div>
      `
    }
  ];

  let filterDefs = $derived([
    { key: "group", label: "Group", options: groupOptions.map(g => ({ value: g, label: g })) }
  ]);

  async function loadUsers() {
    loading = true;
    const params = new URLSearchParams({ page: String(currentPage) });
    if (searchQuery) params.set("search", searchQuery);
    if (filterGroup) params.set("group", filterGroup);
    try {
      const result = await api<{ users: User[]; page: number; total_pages: number; stats?: UserStats }>(`/users?${params}`);
      if (result.ok) {
        users = result.data.users;
        totalPages = result.data.total_pages;
        stats = result.data.stats || { total: 0, active: 0, pending: 0, suspended: 0 };
      }
    } catch {
      // API not available
    } finally {
      loading = false;
    }
  }

  function handleFilter(values: Record<string, string>) {
    searchQuery = values.search ?? "";
    filterValues = values;
    currentPage = 0;
    loadUsers();
  }

  function handlePageChange(p: number) {
    currentPage = p;
    loadUsers();
  }

  async function checkImpersonationStatus() {
    const result = await impersonationApi.status();
    if (result.ok) {
      const data = result.data as { impersonating: boolean; username?: string };
      impersonating = data.impersonating;
      impersonatingUsername = data.username ?? "";
    }
  }

  function startImpersonate(user: User) {
    impersonateTarget = user;
    impersonateError = "";
    showImpersonateConfirm = true;
  }

  async function confirmImpersonate() {
    if (!impersonateTarget) return;
    impersonateError = "";
    const result = await impersonationApi.start(impersonateTarget.id);
    if (result.ok) {
      const data = result.data as { token: string };
      showImpersonateConfirm = false;
      // Redirect to main app
      window.location.href = "/";
    } else {
      impersonateError = result.error;
    }
  }

  async function endImpersonation() {
    const result = await impersonationApi.end();
    if (result.ok) {
      window.location.reload();
    }
  }

  function handleExport() {
    // TODO: Implement user export
    console.log("Export users");
  }

  // Load data on mount
  let initialized = false;
  $effect(() => {
    if (!initialized) {
      initialized = true;
      loadUsers();
      checkImpersonationStatus();
    }
  });
</script>

<svelte:head>
  <title>Users — JapaConnect Admin</title>
</svelte:head>

<!-- Impersonation Banner -->
{#if impersonating}
  <div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent)">
    <span style="color: var(--color-warning)">You are impersonating <strong>{impersonatingUsername}</strong></span>
    <button
      class="rounded-md border px-3 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
      style="border-color: var(--color-warning); color: var(--color-warning)"
      onclick={endImpersonation}
    >
      End Impersonation
    </button>
  </div>
{/if}

<PageHeader
  title="Users"
  description="Manage user accounts, roles, and permissions"
  actions={[
    { label: "Add User", icon: UserPlus, href: "/users/new" },
    { label: "Export", icon: Download, onclick: handleExport, variant: "outline" }
  ]}
/>

<div class="space-y-6">
  <Tabs {tabs} bind:value={activeTab} />
  
  <FilterBar
    bind:value={filterValues}
    filters={filterDefs}
    onsearch={handleFilter}
    onclear={() => {
      searchQuery = "";
      filterValues = {};
      currentPage = 0;
      loadUsers();
    }}
  />
  
  {#if impersonateError}
    <div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
      {impersonateError}
    </div>
  {/if}

  <Card>
    <CardContent class="p-0">
      {#if loading}
        <p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
      {:else}
        <DataTable
          {columns}
          data={users}
          pageSize={20}
          selectable={true}
          emptyMessage="No users found"
        />
      {/if}
    </CardContent>
  </Card>
</div>

<!-- Impersonation Confirmation Modal -->
<Modal bind:open={showImpersonateConfirm} title="Confirm Impersonation" size="sm">
  {#if impersonateTarget}
    <div class="flex items-center gap-4 rounded-lg bg-gray-50 p-4">
      <img src={impersonateTarget.avatar_url || '/default-avatar.png'} alt="" class="h-12 w-12 rounded-full" />
      <div>
        <p class="font-medium text-gray-900">{impersonateTarget.username}</p>
        <p class="text-sm text-gray-500">{impersonateTarget.email}</p>
      </div>
    </div>
  {/if}
  
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => showImpersonateConfirm = false}>Cancel</Button>
    <Button variant="destructive" onclick={confirmImpersonate} disabled={loading}>
      {#if loading}
        <RefreshCw class="h-4 w-4 mr-2 animate-spin" />
        Impersonating...
      {:else}
        Impersonate
      {/if}
    </Button>
  </div>
</Modal>
