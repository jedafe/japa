<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import PageHeader from "$lib/components/layout/page-header.svelte";
  import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
  import { Tabs } from "$lib/components/ui/tabs";
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import { Modal } from "$lib/components/ui/modal";
  import { UserPlus, Users, Shield, AlertTriangle, Trash2, Ban, Play } from "lucide-svelte";
  import { userManagementApi, impersonationApi } from "$lib/api";
  import { hasPermission } from "$lib/stores/permissions.svelte";

  type UserDetail = {
    id: string;
    username: string;
    email: string;
    groups: string[];
    status: string;
    created_at: string;
    avatar_url?: string;
    last_login_at?: string;
  };

  type Activity = {
    type: string;
    content: string;
    created_at: string;
  };

  type AiContextEntry = {
    id: string;
    entry_type: string;
    content: string;
    created_at: string;
  };

  type Session = {
    id: string;
    ip_address: string;
    device: string;
    started_at: string;
  };

  let { data } = $props();

  let user = $state<UserDetail | null>(null);
  let activities = $state<Activity[]>([]);
  let aiEntries = $state<AiContextEntry[]>([]);
  let sessions = $state<Session[]>([]);
  let loading = $state(true);
  let activeTab = $state("overview");

  // Modal states
  let showBanConfirm = $state(false);
  let showUnbanConfirm = $state(false);
  let showSuspendConfirm = $state(false);
  let showDeleteConfirm = $state(false);
  let showWarningModal = $state(false);
  let showGroupModal = $state(false);
  let warningReason = $state("");
  let selectedGroups = $state<string[]>([]);
  let groupOptions = $state<string[]>(["user", "moderator", "admin", "super_admin"]);
  let groupIdsByName = $state<Record<string, string>>({});
  let actionError = $state("");
  let actionSuccess = $state("");

  // Impersonation
  let impersonating = $state(false);
  let impersonatingUsername = $state("");

  const userId = $derived(page.params.userId ?? "");
  
  // Permission checks
  const canManageUsers = $derived(hasPermission('users.manage'));
  const canImpersonate = $derived(hasPermission('admin.impersonate'));
  const canDeleteAiContext = $derived(hasPermission('ai.context.admin_delete_entry'));

  const tabs = [
    { value: "overview", label: "Overview", icon: Users },
    { value: "activity", label: "Activity", icon: Users },
    { value: "ai", label: "AI Context", icon: Users },
    { value: "sessions", label: "Sessions", icon: Users }
  ];

  async function loadUser() {
    if (!userId) return;
    loading = true;
    actionError = "";
    try {
      const result = await userManagementApi.getUser(userId);
      if (result.ok) {
        user = result.data;
        selectedGroups = [...result.data.groups];
      }
    } catch {
      actionError = "Failed to load user";
    } finally {
      loading = false;
    }
  }

  async function loadActivity() {
    try {
      const result = await userManagementApi.getUserActivity(userId, "limit=20");
      if (result.ok) activities = result.data.activities;
    } catch {
      /* ignore */
    }
  }

  async function loadAiContext() {
    try {
      const result = await userManagementApi.getAiContext(userId);
      if (result.ok) aiEntries = result.data;
    } catch {
      /* ignore */
    }
  }

  async function loadSessions() {
    try {
      const result = await userManagementApi.getUserSessions(userId);
      if (result.ok) sessions = result.data;
    } catch {
      /* ignore */
    }
  }

  function switchTab(tab: string) {
    activeTab = tab;
    if (tab === "activity") loadActivity();
    else if (tab === "ai") loadAiContext();
    else if (tab === "sessions") loadSessions();
  }

  function flash(msg: string, isError = false) {
    actionSuccess = isError ? "" : msg;
    actionError = isError ? msg : "";
    setTimeout(() => {
      actionSuccess = "";
      actionError = "";
    }, 3000);
  }

  async function handleBan() {
    showBanConfirm = false;
    const result = await userManagementApi.banUser(userId);
    if (result.ok) {
      flash("User banned successfully");
      loadUser();
    } else flash("Failed to ban user", true);
  }

  async function handleUnban() {
    showUnbanConfirm = false;
    const result = await userManagementApi.unbanUser(userId);
    if (result.ok) {
      flash("User unbanned successfully");
      loadUser();
    } else flash("Failed to unban user", true);
  }

  async function handleSuspend() {
    showSuspendConfirm = false;
    const result = await userManagementApi.suspendUser(userId, { status: "suspended" });
    if (result.ok) {
      flash("User suspended successfully");
      loadUser();
    } else flash("Failed to suspend user", true);
  }

  async function handleDelete() {
    showDeleteConfirm = false;
    const result = await userManagementApi.deleteUser(userId);
    if (result.ok) {
      flash("User deleted successfully");
      goto("/users");
    } else flash("Failed to delete user", true);
  }

  async function handleWarning() {
    showWarningModal = false;
    if (!warningReason.trim()) return;
    const result = await userManagementApi.issueWarning(userId, { reason: warningReason });
    if (result.ok) {
      flash("Warning sent successfully");
      warningReason = "";
    } else flash("Failed to send warning", true);
  }

  async function handleGroupChange() {
    showGroupModal = false;
    const current = new Set(user?.groups ?? []);
    const selected = new Set(selectedGroups);
    const added = selectedGroups.filter((g) => !current.has(g));
    const removed = [...current].filter((g) => !selected.has(g));

    let allOk = true;
    for (const name of added) {
      const groupId = groupIdsByName[name];
      if (!groupId) {
        allOk = false;
        continue;
      }
      const result = await userManagementApi.addGroupMember(groupId, userId);
      if (!result.ok) allOk = false;
    }
    for (const name of removed) {
      const groupId = groupIdsByName[name];
      if (!groupId) {
        allOk = false;
        continue;
      }
      const result = await userManagementApi.removeGroupMember(groupId, userId);
      if (!result.ok) allOk = false;
    }

    if (allOk) {
      flash("Groups updated successfully");
      loadUser();
    } else flash("Failed to update groups", true);
  }

  async function deleteAiEntry(entryId: string) {
    const result = await userManagementApi.deleteAiContextEntry(userId, entryId);
    if (result.ok) {
      flash("AI entry deleted successfully");
      aiEntries = aiEntries.filter((e) => e.id !== entryId);
    } else flash("Failed to delete AI entry", true);
  }

  async function startImpersonate() {
    if (!user) return;
    const result = await impersonationApi.start(user.id);
    if (result.ok) {
      window.location.href = "/";
    } else flash("Failed to start impersonation", true);
  }

  // Load data on mount
  let initialized = false;
  $effect(() => {
    if (!initialized) {
      initialized = true;
      loadUser();
      const loadGroups = async () => {
        try {
          const groupsResult = await userManagementApi.listGroups();
          if (groupsResult.ok) {
            groupOptions = groupsResult.data.groups.map((g) => g.name);
            groupIdsByName = Object.fromEntries(groupsResult.data.groups.map((g) => [g.name, g.id]));
          }
        } catch {
          // Keep default group options
        }
      };
      loadGroups();
    }
  });
</script>

<svelte:head>
  <title>{user?.username ?? "User"} — JapaConnect Admin</title>
</svelte:head>

<!-- Impersonation Banner -->
{#if impersonating}
  <div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent)">
    <span style="color: var(--color-warning)">Impersonating <strong>{impersonatingUsername}</strong></span>
    <a href="/users" class="rounded-md border px-3 py-1 text-xs font-medium" style="border-color: var(--color-warning); color: var(--color-warning)">Back to Users</a>
  </div>
{/if}

<a href="/users" class="mb-4 inline-flex items-center gap-1 text-sm font-medium transition-colors hover:underline" style="color: var(--color-primary)">
  ← Back to Users
</a>

{#if actionSuccess}
  <div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
    {actionSuccess}
  </div>
{/if}

{#if actionError}
  <div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
    {actionError}
  </div>
{/if}

{#if loading}
  <p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if user}
  <!-- User Header -->
  <div class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
    <div class="flex items-center gap-4">
      <div class="flex h-16 w-16 items-center justify-center rounded-2xl text-2xl font-bold text-white" style="background: var(--gradient-primary)">
        {user.username.charAt(0).toUpperCase()}
      </div>
      <div>
        <h1 class="text-2xl font-bold tracking-tight">{user.username}</h1>
        <p class="text-sm" style="color: var(--color-muted)">{user.email}</p>
        <div class="mt-1 flex items-center gap-2">
          <Badge variant={user.status === "active" ? "success" : user.status === "banned" ? "destructive" : "warning"}>
            {user.status}
          </Badge>
          {#each user.groups as g}
            <Badge variant="secondary">{g}</Badge>
          {/each}
        </div>
      </div>
    </div>

    <!-- Actions -->
    <div class="flex flex-wrap gap-2">
      {#if canManageUsers}
        <Button variant="outline" size="sm" onclick={() => { showWarningModal = true; warningReason = ""; }}>
          <AlertTriangle class="h-4 w-4 mr-2" />
          Warn
        </Button>
        {#if user.status === "active"}
          <Button variant="destructive" size="sm" onclick={() => (showBanConfirm = true)}>
            <Ban class="h-4 w-4 mr-2" />
            Ban
          </Button>
          <Button variant="outline" size="sm" onclick={() => (showSuspendConfirm = true)}>
            <AlertTriangle class="h-4 w-4 mr-2" />
            Suspend
          </Button>
        {:else if user.status === "banned"}
          <Button variant="outline" size="sm" onclick={() => (showUnbanConfirm = true)}>
            <Play class="h-4 w-4 mr-2" />
            Unban
          </Button>
        {/if}
        <Button variant="outline" size="sm" onclick={() => { selectedGroups = [...user!.groups]; showGroupModal = true; }}>
          <Users class="h-4 w-4 mr-2" />
          Change Group
        </Button>
        <Button variant="destructive" size="sm" onclick={() => (showDeleteConfirm = true)}>
          <Trash2 class="h-4 w-4 mr-2" />
          Delete
        </Button>
      {/if}
      {#if canImpersonate}
        <Button variant="outline" size="sm" onclick={startImpersonate}>
          <UserPlus class="h-4 w-4 mr-2" />
          Impersonate
        </Button>
      {/if}
    </div>
  </div>

  <!-- Tabs -->
  <Tabs {tabs} bind:value={activeTab} />

  <!-- Tab Content -->
  <Card class="mt-4">
    <CardContent class="p-5">
      {#if activeTab === "overview"}
        <div class="space-y-4">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Username</p>
              <p class="mt-1 text-sm font-medium">{user.username}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Email</p>
              <p class="mt-1 text-sm font-medium">{user.email}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Status</p>
              <p class="mt-1 text-sm font-medium">{user.status}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Created</p>
              <p class="mt-1 text-sm font-medium">{user.created_at}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Last Login</p>
              <p class="mt-1 text-sm font-medium">{user.last_login_at ?? "—"}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Groups</p>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each user.groups as g}
                  <Badge variant="secondary">{g}</Badge>
                {/each}
              </div>
            </div>
          </div>
        </div>

      {:else if activeTab === "activity"}
        {#if activities.length > 0}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Type</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Content</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Date</th>
                </tr>
              </thead>
              <tbody>
                {#each activities as act}
                  <tr class="border-t" style="border-color: var(--color-border)">
                    <td class="px-3 py-2">
                      <Badge variant="secondary">{act.type}</Badge>
                    </td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{act.content}</td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{act.created_at}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="py-4 text-center text-sm" style="color: var(--color-muted)">No activity yet</p>
        {/if}

      {:else if activeTab === "ai"}
        {#if aiEntries.length > 0}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Type</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Content</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Created</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)"></th>
                </tr>
              </thead>
              <tbody>
                {#each aiEntries as entry}
                  <tr class="border-t" style="border-color: var(--color-border)">
                    <td class="px-3 py-2">
                      <Badge variant="secondary">{entry.entry_type}</Badge>
                    </td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{entry.content}</td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{entry.created_at}</td>
                    <td class="px-3 py-2">
                      {#if canDeleteAiContext}
                        <Button variant="ghost" size="sm" onclick={() => deleteAiEntry(entry.id)}>
                          Delete
                        </Button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="py-4 text-center text-sm" style="color: var(--color-muted)">No AI context entries</p>
        {/if}

      {:else if activeTab === "sessions"}
        {#if sessions.length > 0}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">IP Address</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Device</th>
                  <th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">Started</th>
                </tr>
              </thead>
              <tbody>
                {#each sessions as sess}
                  <tr class="border-t" style="border-color: var(--color-border)">
                    <td class="px-3 py-2 font-mono text-xs">{sess.ip_address}</td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{sess.device}</td>
                    <td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{sess.started_at}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="py-4 text-center text-sm" style="color: var(--color-muted)">No sessions yet</p>
        {/if}
      {/if}
    </CardContent>
  </Card>
{/if}

<!-- Ban Confirm Modal -->
<Modal bind:open={showBanConfirm} title="Ban User" size="sm">
  <p class="text-sm" style="color: var(--color-muted)">Are you sure you want to ban this user? They will not be able to access the platform.</p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showBanConfirm = false)}>Cancel</Button>
    <Button variant="destructive" onclick={handleBan}>Ban User</Button>
  </div>
</Modal>

<!-- Unban Confirm Modal -->
<Modal bind:open={showUnbanConfirm} title="Unban User" size="sm">
  <p class="text-sm" style="color: var(--color-muted)">Are you sure you want to unban this user? They will regain access to the platform.</p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showUnbanConfirm = false)}>Cancel</Button>
    <Button variant="default" onclick={handleUnban}>Unban User</Button>
  </div>
</Modal>

<!-- Suspend Confirm Modal -->
<Modal bind:open={showSuspendConfirm} title="Suspend User" size="sm">
  <p class="text-sm" style="color: var(--color-muted)">Are you sure you want to suspend this user? Their account will be temporarily disabled.</p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showSuspendConfirm = false)}>Cancel</Button>
    <Button variant="destructive" onclick={handleSuspend}>Suspend User</Button>
  </div>
</Modal>

<!-- Delete Confirm Modal -->
<Modal bind:open={showDeleteConfirm} title="Delete User" size="sm">
  <p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this user? This action cannot be undone.</p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
    <Button variant="destructive" onclick={handleDelete}>Delete User</Button>
  </div>
</Modal>

<!-- Warning Modal -->
<Modal bind:open={showWarningModal} title="Send Warning" size="sm">
  <div>
    <label for="warning-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Reason for warning</label>
    <textarea
      id="warning-reason"
      rows="3"
      class="w-full rounded-md border px-3 py-2 text-sm"
      style="border-color: var(--color-border); background-color: var(--color-surface)"
      placeholder="Enter the reason for this warning..."
      bind:value={warningReason}
    ></textarea>
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showWarningModal = false)}>Cancel</Button>
    <Button variant="default" onclick={handleWarning} disabled={!warningReason.trim()}>Send Warning</Button>
  </div>
</Modal>

<!-- Change Group Modal -->
<Modal bind:open={showGroupModal} title="Change Group" size="sm">
  <div class="space-y-2">
    {#each groupOptions as group}
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" value={group} bind:group={selectedGroups} class="rounded" />
        {group}
      </label>
    {/each}
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showGroupModal = false)}>Cancel</Button>
    <Button variant="default" onclick={handleGroupChange}>Save Changes</Button>
  </div>
</Modal>
