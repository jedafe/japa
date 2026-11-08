<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	import { t } from '#lib/stores/i18n.svelte.js';
  import { page } from "$app/state";
  import { goto, invalidateAll } from "$app/navigation";
  import PageHeader from "#lib/components/layout/page-header.svelte";
  import { Card, CardContent, CardHeader } from "#lib/components/ui/card/index.js";
  import { Tabs } from "#lib/components/ui/tabs/index.js";
  import { Badge } from "#lib/components/ui/badge/index.js";
  import { Button } from "#lib/components/ui/button/index.js";
  import { Modal } from "#lib/components/ui/modal/index.js";
  import { DataTable } from "#lib/components/ui/data-table/index.js";
  import type { ActionItem } from "#lib/components/ui/data-table/index.js";
  import Pagination from "#lib/components/ui/Pagination.svelte";
  import Autocomplete from "#lib/components/ui/Autocomplete.svelte";
  import type { AutocompleteItem } from "#lib/components/ui/Autocomplete.svelte";
  import { UserPlus, Users, Shield, AlertTriangle, Trash2, Ban, Play, RotateCcw, Coins, BadgeCheck } from "lucide-svelte";
   import { userManagementApi, impersonationApi, usersWarningsAdminApi, verificationAdminApi, type UserSession, type WalletLedgerEntry, type ModerationConfigOption, type VerificationTypeItem } from "#lib/api.js";
  import { hasPermission } from "#lib/stores/permissions.svelte.js";

  type UserDetail = {
    id: string;
    username: string;
    email: string;
    groups: string[];
    // Machine-readable memberships — `groups` display names collide on
    // duplicates, so the group editor diffs on ids via group_refs.
    group_refs?: Array<{ id: string; name: string; slug: string }>;
    status: string;
    verified?: boolean;
    profile_type: 'regular' | 'pro' | 'business';
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

   type Warning = {
     id: string;
     reason: string;
     violation_type?: string;
     created_at: string;
     expires_at?: string;
   };

   type Restriction = {
     id: string;
     operation: string;
     reason: string | null;
     expires_at: string | null;
     created_at: string;
   };

   let user = $state<UserDetail | null>(null);
   let activities = $state<Activity[]>([]);
   let aiEntries = $state<AiContextEntry[]>([]);
   let sessions = $state<UserSession[]>([]);
   let warnings = $state<Warning[]>([]);
   let restrictions = $state<Restriction[]>([]);
   let ledger = $state<WalletLedgerEntry[]>([]);
   let ledgerSearch = $state("");
   let ledgerPage = $state(0);
   let ledgerPageSize = $state(10);
   let showReverseModal = $state(false);
   let reverseTarget = $state<WalletLedgerEntry | null>(null);
   let reverseReason = $state("");
   let reverseBusy = $state(false);
   let loading = $state(true);
   let activeTab = $state("overview");

  // Client-side search and pagination for tabs
  let activitySearch = $state("");
  let sessionSearch = $state("");
  let aiSearch = $state("");
  let warningSearch = $state("");

  let activityPage = $state(0);
  let activityPageSize = $state(10);
  let sessionPage = $state(0);
  let sessionPageSize = $state(10);
  let aiPage = $state(0);
  let aiPageSize = $state(10);
  let warningPage = $state(0);
  let warningPageSize = $state(10);

  const filteredActivities = $derived(
    activitySearch
      ? activities.filter(a =>
          a.type.toLowerCase().includes(activitySearch.toLowerCase()) ||
          a.content.toLowerCase().includes(activitySearch.toLowerCase())
        )
      : activities
  );
  const activityTotalPages = $derived(Math.max(1, Math.ceil(filteredActivities.length / activityPageSize)));
  const pagedActivities = $derived(filteredActivities.slice(activityPage * activityPageSize, (activityPage + 1) * activityPageSize));

  const filteredSessions = $derived(
    sessionSearch
      ? sessions.filter(s =>
          s.id.toLowerCase().includes(sessionSearch.toLowerCase())
        )
      : sessions
  );
  const sessionTotalPages = $derived(Math.max(1, Math.ceil(filteredSessions.length / sessionPageSize)));
  const pagedSessions = $derived(filteredSessions.slice(sessionPage * sessionPageSize, (sessionPage + 1) * sessionPageSize));

  const filteredAiEntries = $derived(
    aiSearch
      ? aiEntries.filter(e =>
          e.entry_type.toLowerCase().includes(aiSearch.toLowerCase()) ||
          e.content.toLowerCase().includes(aiSearch.toLowerCase())
        )
      : aiEntries
  );
  const aiTotalPages = $derived(Math.max(1, Math.ceil(filteredAiEntries.length / aiPageSize)));
  const pagedAiEntries = $derived(filteredAiEntries.slice(aiPage * aiPageSize, (aiPage + 1) * aiPageSize));

  const filteredWarnings = $derived(
    warningSearch
      ? warnings.filter(w =>
          w.reason.toLowerCase().includes(warningSearch.toLowerCase())
        )
      : warnings
  );
  const warningTotalPages = $derived(Math.max(1, Math.ceil(filteredWarnings.length / warningPageSize)));
  const pagedWarnings = $derived(filteredWarnings.slice(warningPage * warningPageSize, (warningPage + 1) * warningPageSize));

  let showBanConfirm = $state(false);
  let showUnbanConfirm = $state(false);
  let showVerifyModal = $state(false);
  let verifyType = $state("");
  let verificationTypes = $state<VerificationTypeItem[]>([]);
  let banReason = $state("");
  let banDays = $state(0);
  let showDeleteConfirm = $state(false);
  let showWarningModal = $state(false);
  let showGroupModal = $state(false);
  let showProfileTypeModal = $state(false);
  let showRestrictModal = $state(false);
  let showSuspendModal = $state(false);
  let warningReason = $state("");
  let warningViolationType = $state("");
  let selectedGroups = $state<string[]>([]);
  let selectedProfileType = $state("regular");
  // Restrict modal state
  let restrictOperations = $state<string[]>([]);
  let restrictDuration = $state("7d");
  let restrictReason = $state("");
  let suspendReason = $state("");
  // Group options carry id+slug — membership diffs are keyed on ids, not
  // display names (duplicate group names would silently map to one id).
  let groupOptions = $state<Array<{ id: string; name: string; slug: string }>>([]);
  let actionError = $state("");
  let actionSuccess = $state("");

  let impersonating = $state(false);
  let impersonatingTargetId = $state<string | null>(null);
  let impersonatingTargetName = $state<string | null>(null);

  const userId = $derived(page.params.userId ?? "");
  
  // Per-node gates — the backend enforces different nodes per action:
  // admin.users.list (profile read), admin.users.manage (sessions/activity/
  // profile-type), admin.moderation (ban/warn/suspend/restrict/status/
  // warnings), admin.rbac.manage (group membership), user.delete (soft
  // delete), admin.impersonate (§46), ai.context.admin_view /
  // ai.context.admin_delete_entry (AI Context tab), wallet.admin.adjust
  // (Points tab).
  const canManageUsers = $derived(hasPermission('admin.users.manage'));
  const canModerate = $derived(hasPermission('admin.moderation'));
  const canManageRbac = $derived(hasPermission('admin.rbac.manage'));
  const canDeleteUsers = $derived(hasPermission('user.delete'));
  const canImpersonate = $derived(hasPermission('admin.impersonate'));
  const canViewAiContext = $derived(hasPermission('ai.context.admin_view'));
  const canDeleteAiContext = $derived(hasPermission('ai.context.admin_delete_entry'));
  const canViewWallet = $derived(hasPermission('wallet.admin.adjust'));

   const tabs = $derived([
     { value: "overview", label: "Overview", icon: Users, show: true },
     { value: "activity", label: "Activity", icon: Users, show: canManageUsers },
     { value: "ai", label: "AI Context", icon: Users, show: canViewAiContext },
     { value: "sessions", label: "Sessions", icon: Users, show: canManageUsers },
     { value: "warnings", label: "Warnings", icon: AlertTriangle, show: canModerate },
     { value: "wallet", label: "Points", icon: Coins, show: canViewWallet }
   ].filter((tab) => tab.show));

  const activityColumns = [
    { key: 'type', label: t('users.userId.type'), sortable: true, render: (v: string) => `<span class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${v}</span>` },
    { key: 'content', label: t('users.userId.content'), sortable: false, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
    { key: 'created_at', label: t('users.userId.date'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` }
  ];

  const aiColumns = [
    { key: 'entry_type', label: t('users.userId.type'), sortable: false, render: (v: string) => `<span class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${v}</span>` },
    { key: 'content', label: t('users.userId.content'), sortable: false, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
    { key: 'created_at', label: t('users.userId.created'), sortable: false, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` }
  ];

  const aiActions: ActionItem[] = [
    {
      label: 'Delete',
      icon: Trash2,
      variant: 'destructive',
      onclick: (row: AiContextEntry) => deleteAiEntry(row.id),
      show: () => canDeleteAiContext
    }
  ];

  const sessionColumns = [
    { key: 'id', label: t('users.userId.session_id', 'Session ID'), sortable: false, class: 'font-mono text-xs' },
    { key: 'created_at', label: t('users.userId.created'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` },
    { key: 'expires_at', label: t('users.userId.expires', 'Expires'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` }
  ];

  const warningColumns = [
    { key: 'violation_type', label: t('users.userId.violation_type', 'Violation'), sortable: false, render: (v: string) => `<span class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-foreground)">${v || '—'}</span>` },
    { key: 'reason', label: t('users.userId.reason'), sortable: false },
    { key: 'created_at', label: t('users.userId.issued'), sortable: false, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` }
  ];

  const filteredLedger = $derived(
    ledgerSearch
      ? ledger.filter(e =>
          (e.source ?? '').toLowerCase().includes(ledgerSearch.toLowerCase()) ||
          (e.reason ?? '').toLowerCase().includes(ledgerSearch.toLowerCase()) ||
          e.operation.toLowerCase().includes(ledgerSearch.toLowerCase())
        )
      : ledger
  );
  const ledgerTotalPages = $derived(Math.max(1, Math.ceil(filteredLedger.length / ledgerPageSize)));
  const pagedLedger = $derived(filteredLedger.slice(ledgerPage * ledgerPageSize, (ledgerPage + 1) * ledgerPageSize));

  const warningActions: ActionItem[] = [
    {
      label: t('common.delete', 'Delete'),
      icon: Trash2,
      variant: 'destructive',
      onclick: (row: Warning) => deleteWarning(row.id),
      show: () => canModerate
    }
  ];

  const ledgerColumns = [
    { key: 'created_at', label: t('users.userId.issued', 'Date'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>` },
    { key: 'ledger_type', label: t('users.userId.type'), sortable: false, render: (v: string) => `<span class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${v}</span>` },
    { key: 'source', label: t('users.userId.source', 'Source'), sortable: false, class: 'text-xs', render: (v: string | null) => `<span style="color: var(--color-muted)">${v ?? '—'}</span>` },
    { key: 'amount', label: t('users.userId.amount', 'Amount'), sortable: true, render: (v: number) => `<span class="font-mono ${v < 0 ? 'text-destructive' : ''}">${v > 0 ? '+' : ''}${v}</span>` },
    { key: 'resulting_balance', label: t('users.userId.balance', 'Balance'), sortable: false, class: 'font-mono text-xs' },
    { key: 'reason', label: t('users.userId.reason'), sortable: false, class: 'text-xs', render: (v: string | null) => `<span style="color: var(--color-muted)">${v ?? '—'}</span>` }
  ];

  const canAdjustWallet = $derived(hasPermission('wallet.admin.adjust'));

  const ledgerActions: ActionItem[] = [
    {
      label: t('users.userId.reverse_entry', 'Reverse'),
      icon: RotateCcw,
      onclick: (row: WalletLedgerEntry) => {
        reverseTarget = row;
        reverseReason = '';
        showReverseModal = true;
      },
      show: (row) => canAdjustWallet && row.amount !== 0
    }
  ];

  async function loadUser() {
    if (!userId) return;
    loading = true;
    actionError = "";
    try {
      const result = await userManagementApi.getUser(userId);
      if (result.ok) {
        const d = result.data;
        d.groups = d.groups ?? [];
        d.group_refs = d.group_refs ?? [];
        user = d;
        selectedGroups = d.group_refs.map((g: any) => g.id);
        loadRestrictions();
      } else {
        user = null;
        actionError = result.error ?? t('users.detail.load_error', 'Failed to load user details');
      }
    } catch {
      user = null;
      actionError = t('users.detail.load_error', 'Failed to load user details');
    } finally {
      loading = false;
    }
  }

  async function loadActivity() {
    try {
      const result = await userManagementApi.getUserActivity(userId, "limit=20");
      if (result.ok) activities = result.data.activities;
    } catch {
    }
  }

  async function loadAiContext() {
    try {
      const result = await userManagementApi.getAiContext(userId);
      if (result.ok) {
        const d = result.data as any;
        aiEntries = Array.isArray(d) ? d : (d?.data ?? []);
      } else {
        aiEntries = [];
      }
    } catch {
      aiEntries = [];
    }
  }

  async function loadSessions() {
    try {
      const result = await userManagementApi.getUserSessions(userId);
      if (result.ok) sessions = result.data;
    } catch {
    }
  }

  async function loadWarnings() {
    try {
      const result = await usersWarningsAdminApi.list(userId);
      if (result.ok) warnings = (result.data as Warning[]) ?? [];
    } catch {
    }
  }

  async function loadLedger() {
    try {
      const result = await userManagementApi.getUserLedger(userId, "page_size=100");
      if (result.ok) {
        const d = result.data as any;
        ledger = Array.isArray(d) ? d : (d?.data ?? []);
      }
    } catch {
    }
  }

  async function reverseEntry() {
    if (!reverseTarget || reverseBusy) return;
    reverseBusy = true;
    const result = await userManagementApi.reverseLedgerEntry(reverseTarget.id, { reason: reverseReason || undefined });
    reverseBusy = false;
    showReverseModal = false;
    if (result.ok) {
      flash(t('users.userId.reverse_success', 'Ledger entry reversed'));
      loadLedger();
    } else {
      flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
    }
  }

  async function loadRestrictions() {
    // GET /admin/users/{id}/restrictions requires admin.moderation — skip
    // entirely for viewers without the node instead of firing a 403.
    if (!canModerate) { restrictions = []; return; }
    try {
      const result = await userManagementApi.listRestrictions(userId);
      if (result.ok) {
        const d = result.data as any;
        restrictions = Array.isArray(d) ? d : (d?.data ?? []);
      }
    } catch {
    }
  }

  // Load a tab's data when it becomes active — covers tab clicks, ?tab=
  // deep-links, and back/forward navigation. Kept fresh on each activation
  // so moderation state is never stale. (The shared Tabs component binds
  // `activeTab` directly, so this must be an effect, not a click handler.)
  function loadTabData(tab: string) {
    if (tab === "activity") void loadActivity();
    else if (tab === "ai") void loadAiContext();
    else if (tab === "sessions") void loadSessions();
    else if (tab === "warnings") void loadWarnings();
    else if (tab === "wallet") void loadLedger();
  }
  $effect(() => {
    if (!userId) return;
    loadTabData(activeTab);
  });

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
    const result = await userManagementApi.banUser(userId, {
      reason: banReason.trim() || undefined,
      days: banDays > 0 ? banDays : undefined,
    });
    if (result.ok) {
      flash(t('users.detail.ban_success', 'User banned'));
      banReason = "";
      banDays = 0;
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  // §07 Verification Requests — profile badge grant/revoke on the detail
  // surface for parity with the /users row menu (admin.moderation-gated).
  async function handleVerify() {
    showVerifyModal = false;
    const result = await verificationAdminApi.verifyUser(userId, { verification_type: verifyType || undefined });
    if (result.ok) {
      flash(t('users.detail.verify_success', 'User verified'));
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleUnverify() {
    const result = await verificationAdminApi.unverifyUser(userId);
    if (result.ok) {
      flash(t('users.detail.unverify_success', 'Verification removed'));
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleUnban() {
    showUnbanConfirm = false;
    const result = await userManagementApi.unbanUser(userId);
    if (result.ok) {
      flash(t('users.detail.unban_success', 'Ban lifted'));
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleDelete() {
    showDeleteConfirm = false;
    const result = await userManagementApi.deleteUser(userId);
    if (result.ok) {
      flash(t('users.detail.delete_success', 'User deleted'));
      goto("/users");
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleUnsuspend() {
    // POST /suspend bans every operation permanently AND sets status —
    // unsuspending must lift all operation bans (which also restores
    // status='active'), not just patch the status field.
    const result = await userManagementApi.liftAllRestrictions(userId);
    if (result.ok) {
      flash(t('users.detail.unsuspend_success', 'User unsuspended'));
      loadUser();
      loadRestrictions();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleRestore() {
    const result = await userManagementApi.suspendUser(userId, { status: "active" });
    if (result.ok) {
      flash(t('users.restore', 'User restored'));
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleWarnTyped() {
    showWarningModal = false;
    if (!warningReason.trim()) return;
    const result = await userManagementApi.warnUserTyped(userId, {
      reason: warningReason,
      violation_type: warningViolationType,
    });
    if (result.ok) {
      const d = result.data as { warning_count: number; auto_restricted: boolean };
      let msg = t('users.detail.warning_sent', 'Warning issued');
      if (d.auto_restricted) {
        msg += ` — Auto-restricted after ${d.warning_count} ${warningViolationType} warnings`;
      }
      flash(msg);
      warningReason = "";
      loadWarnings();
      loadRestrictions();
      if (d.auto_restricted) loadUser(); // status may have flipped to 'restricted'
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleRestrict() {
    showRestrictModal = false;
    if (restrictOperations.length === 0) return;
    const result = await userManagementApi.restrictUser(userId, {
      operations: restrictOperations,
      reason: restrictReason,
      duration: restrictDuration,
    });
    if (result.ok) {
      flash(t('users.detail.restricted', 'User restricted'));
      restrictOperations = [];
      restrictReason = "";
      loadRestrictions();
      loadUser(); // backend also sets status='restricted'
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function handleSuspendUser() {
    showSuspendModal = false;
    const result = await userManagementApi.suspendUserFull(userId, { reason: suspendReason });
    if (result.ok) {
      flash(t('users.detail.suspended', 'User suspended'));
      suspendReason = "";
      loadUser();
      loadRestrictions();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function deleteWarning(warningId: string) {
    const result = await usersWarningsAdminApi.delete(userId, warningId);
    if (result.ok) {
      flash(t('users.detail.warning_deleted', 'Warning deleted'));
      loadWarnings();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function liftRestriction(operation: string) {
    const result = await userManagementApi.liftRestriction(userId, operation);
    if (result.ok) {
      flash(t('users.detail.restriction_lifted', 'Restriction lifted'));
      loadRestrictions();
      loadUser(); // status flips back to 'active' when no bans remain
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function liftAllRestrictions() {
    const result = await userManagementApi.liftAllRestrictions(userId);
    if (result.ok) {
      flash(t('users.detail.all_restrictions_lifted', 'All restrictions lifted'));
      loadRestrictions();
      loadUser(); // backend restores status='active'
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  let groupSaving = $state(false);

  async function handleGroupChange() {
    if (groupSaving) return; // in-flight guard — duplicate submit fires dup API calls
    groupSaving = true;
    showGroupModal = false;
    // Membership diff is keyed on group ids via group_refs — name-keyed
    // lookups silently collapse duplicate display names.
    const current = new Set<string>((user?.group_refs ?? []).map((g: any) => g.id));
    const selected = new Set(selectedGroups);
    const added = selectedGroups.filter((id) => !current.has(id));
    const removed = [...current].filter((id) => !selected.has(id));

    let allOk = true;
    let lastError: string | undefined;
    for (const groupId of added) {
      const result = await userManagementApi.addGroupMember(groupId, userId);
      if (!result.ok) { allOk = false; lastError = result.error; }
    }
    for (const groupId of removed) {
      const result = await userManagementApi.removeGroupMember(groupId, userId);
      if (!result.ok) { allOk = false; lastError = result.error; }
    }

    if (allOk) {
      flash(t('users.detail.group_changed', 'Group updated'));
    } else {
      flash(lastError ?? t('users.detail.action_error', 'Action failed'), true);
    }
    // Always re-fetch — a partial failure may still have applied some
    // membership changes, so the UI must reflect the real server state.
    groupSaving = false;
    loadUser();
  }

  // Moderation config options — fetched from the backend only (Rule 16).
  // No hardcoded fallbacks; loadModerationConfig() populates these from
  // /admin/users/config on mount.
  let profileTypeOptions = $state<string[]>([]);
  let violationTypes = $state<ModerationConfigOption[]>([]);
  let restrictionOps = $state<ModerationConfigOption[]>([]);
  let restrictionDurations = $state<ModerationConfigOption[]>([]);

  // Fetch moderation config from the backend; on failure the option lists
  // stay empty rather than silently using stale hardcoded values.
  async function loadModerationConfig() {
    try {
      const result = await userManagementApi.getModerationConfig();
      if (result.ok) {
        const cfg = result.data;
        if (cfg.profile_types?.length) profileTypeOptions = cfg.profile_types.map((o) => o.value);
        if (cfg.violation_types?.length) violationTypes = cfg.violation_types;
        if (cfg.restriction_operations?.length) restrictionOps = cfg.restriction_operations;
        if (cfg.restriction_durations?.length) restrictionDurations = cfg.restriction_durations;
      }
    } catch {
      // Options stay empty — the modals disable submit until config loads.
    }
  }

  async function handleProfileTypeChange() {
    showProfileTypeModal = false;
    const result = await userManagementApi.updateProfileType(userId, { profile_type: selectedProfileType });
    if (result.ok) {
      flash(t('users.detail.profile_type_changed', 'Tier updated'));
      loadUser();
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  async function deleteAiEntry(entryId: string) {
    const result = await userManagementApi.deleteAiContextEntry(userId, entryId);
    if (result.ok) {
      flash(t('users.detail.ai_entry_deleted', 'Entry deleted'));
      aiEntries = aiEntries.filter((e) => e.id !== entryId);
    } else flash(t('users.detail.ai_entry_delete_error', 'Failed to delete entry'), true);
  }

  async function checkImpersonation() {
    try {
      const result = await impersonationApi.status();
      if (result.ok) {
        impersonating = result.data.active;
        impersonatingTargetId = result.data.target_user_id;
        impersonatingTargetName = result.data.target?.username ?? null;
      }
    } catch {
      // banner simply stays hidden
    }
  }

  async function startImpersonate() {
    if (!user) return;
    const result = await impersonationApi.start(user.id);
    if (result.ok) {
      await invalidateAll();
      goto("/", { invalidateAll: true });
    } else flash(result.error ?? t('users.detail.action_error', 'Action failed'), true);
  }

  let groupsLoaded = false;
  $effect(() => {
    if (!userId) return;
    loadUser();
    if (!groupsLoaded) {
      groupsLoaded = true;
      const loadGroups = async () => {
        // GET /admin/user-groups requires admin.rbac.manage — skip the call
        // entirely for viewers without the node instead of firing a 403.
        if (!canManageRbac) return;
        try {
          const groupsResult = await userManagementApi.listGroups();
          if (groupsResult.ok) {
            groupOptions = groupsResult.data.groups.map((g) => ({ id: g.id, name: g.name, slug: g.slug }));
            // Fallback for responses without group_refs: map names→ids.
            if (selectedGroups.length === 0 && (user?.groups?.length ?? 0) > 0) {
              selectedGroups = user!.groups
                .map((n: string) => groupOptions.find((o) => o.name === n)?.id)
                .filter(Boolean) as string[];
            }
          }
        } catch {
        }
      };
      loadGroups();
    }
  });

  // Load moderation config (profile types, violation types, restriction
  // operations + durations) from the backend on mount, and pick up an
  // active impersonation session for the banner.
  $effect(() => {
    loadModerationConfig();
    checkImpersonation();
    // §07 badge-type vocabulary for the verify picker (Rule 16).
    if (canModerate) {
      verificationAdminApi.types("user").then((r) => { if (r.ok) verificationTypes = r.data.types ?? []; }).catch(() => {});
    }
  });
</script>

<svelte:head>
  <title>{user?.username ?? "User"} — {siteName()} Admin</title>
</svelte:head>

{#if impersonating}
  <div class="mb-4 flex items-center justify-between gap-2 flex-wrap rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent)">
    <span style="color: var(--color-warning)">{t('users.userId.impersonating')} <strong>{impersonatingTargetName ?? impersonatingTargetId ?? ''}</strong></span>
    <a href="/users" class="rounded-md border px-3 py-1 text-xs font-medium" style="border-color: var(--color-warning); color: var(--color-warning)">{t('users.userId.back_to_users')}</a>
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
  <p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('users.userId.loading')}</p>
{:else if user}
  <!-- User Card -->
  <Card class="mb-6">
    <CardContent class="p-6">
      <div class="flex flex-col gap-6 sm:flex-row sm:items-start">
        <!-- Avatar -->
        <div class="flex-shrink-0">
          {#if user.avatar_url}
            <img src={user.avatar_url} alt={user.username} class="h-24 w-24 rounded-2xl object-cover ring-2 ring-border" />
          {:else}
            <div class="flex h-24 w-24 items-center justify-center rounded-2xl text-3xl font-bold text-white ring-2 ring-border" style="background: var(--gradient-primary)">
              {user.username.charAt(0).toUpperCase()}
            </div>
          {/if}
        </div>

        <!-- User Info -->
        <div class="flex-1 min-w-0">
          <div class="flex items-start justify-between flex-wrap gap-4">
            <div class="min-w-0">
              <h1 class="text-2xl font-bold tracking-tight truncate">{user.username}</h1>
              <p class="mt-0.5 text-sm truncate" style="color: var(--color-muted)">{user.email}</p>
            </div>
            <div class="flex items-center gap-2 flex-shrink-0">
              <Badge variant={user.status === "active" ? "success" : user.status === "banned" ? "destructive" : user.status === "deleted" ? "destructive" : "warning"}>
                {user.status}
              </Badge>
              {#each user.groups as g, gi (`${g}::${gi}`)}
                <Badge variant="secondary">{g}</Badge>
              {/each}
            </div>
          </div>

          <!-- Quick Stats -->
          <div class="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-4">
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.created', 'Created')}</p>
              <p class="mt-0.5 text-sm font-medium">{new Date(user.created_at).toLocaleDateString()}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.last_login', 'Last Login')}</p>
              <p class="mt-0.5 text-sm font-medium">{user.last_login_at ? new Date(user.last_login_at).toLocaleDateString() : "—"}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.status', 'Status')}</p>
              <p class="mt-0.5 text-sm font-medium capitalize">{user.status}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.groups', 'Groups')}</p>
              <div class="mt-0.5 flex flex-wrap gap-1">
                {#each user.groups as g, gi (`${g}::${gi}`)}
                  <Badge variant="secondary" class="text-xs">{g}</Badge>
                {:else}
                  <span class="text-sm" style="color: var(--color-muted)">—</span>
                {/each}
              </div>
            </div>
          </div>

          <!-- Action Buttons -->
          <div class="mt-4 flex flex-wrap gap-2">
            {#if canModerate}
              <Button variant="outline" size="sm" onclick={() => { warningReason = ""; warningViolationType = violationTypes[0]?.value ?? ''; showWarningModal = true; }}>
                <AlertTriangle class="h-4 w-4 me-2" />
                {t('users.detail.warning', 'Warn')}
              </Button>
              {#if user.status === "banned"}
                <Button variant="outline" size="sm" onclick={() => (showUnbanConfirm = true)}>
                  <RotateCcw class="h-4 w-4 me-2" />
                  {t('users.detail.unban', 'Unban')}
                </Button>
              {:else if user.status !== "deleted"}
                <Button variant="destructive" size="sm" onclick={() => { banReason = ""; banDays = 0; showBanConfirm = true; }}>
                  <Ban class="h-4 w-4 me-2" />
                  {t('users.detail.ban', 'Ban')}
                </Button>
              {/if}
              {#if user.status !== "deleted"}
                {#if user.verified}
                  <Button variant="outline" size="sm" onclick={handleUnverify}>
                    <BadgeCheck class="h-4 w-4 me-2" />
                    {t('users.unverify', 'Unverify')}
                  </Button>
                {:else}
                  <Button variant="outline" size="sm" onclick={() => { verifyType = ""; showVerifyModal = true; }}>
                    <BadgeCheck class="h-4 w-4 me-2" />
                    {t('users.verify', 'Verify')}
                  </Button>
                {/if}
              {/if}
              {#if user.status === "active" || user.status === "restricted"}
                <Button variant="destructive" size="sm" onclick={() => { restrictOperations = []; restrictDuration = "7d"; restrictReason = ""; showRestrictModal = true; }}>
                  <Ban class="h-4 w-4 me-2" />
                  {t('users.detail.restrict', 'Restrict')}
                </Button>
                <Button variant="outline" size="sm" onclick={() => { suspendReason = ""; showSuspendModal = true; }}>
                  <Shield class="h-4 w-4 me-2" />
                  {t('users.detail.suspend', 'Suspend')}
                </Button>
                {#if restrictions.length > 0}
                  <Button variant="outline" size="sm" onclick={liftAllRestrictions}>
                    <RotateCcw class="h-4 w-4 me-2" />
                    {t('users.detail.lift_all', 'Lift All Restrictions')}
                  </Button>
                {/if}
              {:else if user.status === "suspended"}
                <Button variant="outline" size="sm" onclick={handleUnsuspend}>
                  <RotateCcw class="h-4 w-4 me-2" />
                  {t('users.detail.unsuspend', 'Unsuspend')}
                </Button>
              {:else if user.status === "deleted"}
                <Button variant="default" size="sm" onclick={handleRestore}>
                  <RotateCcw class="h-4 w-4 me-2" />
                  {t('users.restore', 'Restore')}
                </Button>
              {/if}
            {/if}
            {#if canManageRbac}
              <Button variant="outline" size="sm" onclick={() => { selectedGroups = (user?.group_refs ?? []).map((g) => g.id); showGroupModal = true; }}>
                <Users class="h-4 w-4 me-2" />
                {t('users.detail.change_group', 'Change Group')}
              </Button>
            {/if}
            {#if canManageUsers}
              <Button variant="outline" size="sm" onclick={() => { selectedProfileType = user!.profile_type || 'regular'; showProfileTypeModal = true; }}>
                <Users class="h-4 w-4 me-2" />
                {t('users.detail.change_profile_type', 'Change Profile Type')}
              </Button>
            {/if}
            {#if canDeleteUsers && user.status !== "deleted"}
              <Button variant="destructive" size="sm" onclick={() => (showDeleteConfirm = true)}>
                <Trash2 class="h-4 w-4 me-2" />
                {t('users.detail.delete', 'Delete')}
              </Button>
            {/if}
            {#if canImpersonate && user.status !== "deleted"}
              <Button variant="outline" size="sm" onclick={startImpersonate}>
                <UserPlus class="h-4 w-4 me-2" />
                {t('users.detail.impersonate', 'Impersonate')}
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </CardContent>
  </Card>

  <Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

  <Card class="mt-4">
    <CardContent class="p-5">
      {#if activeTab === "overview"}
        <div class="space-y-4">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.username')}</p>
              <p class="mt-1 text-sm font-medium">{user.username}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.email')}</p>
              <p class="mt-1 text-sm font-medium">{user.email}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.created')}</p>
              <p class="mt-1 text-sm font-medium">{new Date(user.created_at).toLocaleString()}</p>
            </div>
            <div>
              <p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.userId.last_login')}</p>
              <p class="mt-1 text-sm font-medium">{user.last_login_at ? new Date(user.last_login_at).toLocaleString() : "—"}</p>
            </div>
          </div>

          {#if restrictions.length > 0}
            <div class="mt-4">
              <h3 class="text-sm font-semibold mb-2" style="color: var(--color-foreground)">{t('users.detail.active_restrictions', 'Active Restrictions')}</h3>
              <div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
                <table class="w-full text-sm">
                  <thead>
                    <tr style="border-color: var(--color-border)">
                      <th class="h-10 px-4 text-start font-medium" style="color: var(--color-muted)">{t('users.detail.col_operation', 'Operation')}</th>
                      <th class="h-10 px-4 text-start font-medium" style="color: var(--color-muted)">{t('common.reason', 'Reason')}</th>
                      <th class="h-10 px-4 text-start font-medium" style="color: var(--color-muted)">{t('users.detail.col_expires', 'Expires')}</th>
                      <th class="h-10 px-4 text-end font-medium" style="color: var(--color-muted)">{t('common.action', 'Action')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each restrictions as r (r.id)}
                      <tr style="border-color: var(--color-border)">
                        <td class="px-4 py-2">
                          <span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-warning/15 text-warning dark:text-warning">{r.operation}</span>
                        </td>
                        <td class="px-4 py-2 text-xs" style="color: var(--color-muted)">{r.reason ?? '—'}</td>
                        <td class="px-4 py-2 text-xs" style="color: var(--color-muted)">{r.expires_at ? new Date(r.expires_at).toLocaleString() : t('common.permanent', 'Permanent')}</td>
                        <td class="px-4 py-2 text-end">
                          {#if canModerate}
                            <Button variant="ghost" size="sm" onclick={() => liftRestriction(r.operation)}>
                              <RotateCcw class="h-3 w-3 me-1" /> Lift
                            </Button>
                          {/if}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        </div>

      {:else if activeTab === "activity"}
        <!-- urlSync={false}: tab-level search/sort is internal state — it must
             not write ?q=/?sort= into the user-detail URL. -->
        <DataTable columns={activityColumns} data={pagedActivities} searchable={true} urlSync={false} searchPlaceholder={t('common.search', 'Search')} onsearch={(q) => { activitySearch = q; activityPage = 0; }} emptyMessage={t('users.userId.no_activity_yet')} />
        <Pagination bind:page={activityPage} totalPages={activityTotalPages} totalItems={filteredActivities.length} bind:pageSize={activityPageSize} />

      {:else if activeTab === "ai"}
        <DataTable columns={aiColumns} data={pagedAiEntries} searchable={true} urlSync={false} searchPlaceholder={t('common.search', 'Search')} onsearch={(q) => { aiSearch = q; aiPage = 0; }} actions={aiActions} emptyMessage={t('users.userId.no_ai_context_entries')} />
        <Pagination bind:page={aiPage} totalPages={aiTotalPages} totalItems={filteredAiEntries.length} bind:pageSize={aiPageSize} />

      {:else if activeTab === "sessions"}
        <DataTable columns={sessionColumns} data={pagedSessions} searchable={true} urlSync={false} searchPlaceholder={t('common.search', 'Search')} onsearch={(q) => { sessionSearch = q; sessionPage = 0; }} emptyMessage={t('users.userId.no_sessions_yet')} />
        <Pagination bind:page={sessionPage} totalPages={sessionTotalPages} totalItems={filteredSessions.length} bind:pageSize={sessionPageSize} />

      {:else if activeTab === "warnings"}
        <DataTable columns={warningColumns} data={pagedWarnings} searchable={true} urlSync={false} searchPlaceholder={t('common.search', 'Search')} onsearch={(q) => { warningSearch = q; warningPage = 0; }} actions={warningActions} emptyMessage={t('users.userId.no_warnings_yet')} />
        <Pagination bind:page={warningPage} totalPages={warningTotalPages} totalItems={filteredWarnings.length} bind:pageSize={warningPageSize} />

      {:else if activeTab === "wallet"}
        <DataTable columns={ledgerColumns} data={pagedLedger} searchable={true} urlSync={false} searchPlaceholder={t('common.search', 'Search')} onsearch={(q) => { ledgerSearch = q; ledgerPage = 0; }} actions={ledgerActions} emptyMessage={t('users.userId.no_ledger_entries', 'No ledger entries yet')} />
        <Pagination bind:page={ledgerPage} totalPages={ledgerTotalPages} totalItems={filteredLedger.length} bind:pageSize={ledgerPageSize} />
      {/if}
    </CardContent>
  </Card>
{:else}
  <!-- Failed to load (404/403/network) — never render a silent blank page. -->
  <Card class="mb-6">
    <CardContent class="p-8 text-center">
      <p class="text-sm font-medium" style="color: var(--color-foreground)">
        {t('users.detail.not_found', 'User not found')}
      </p>
      {#if actionError}
        <p class="mt-1 text-xs" style="color: var(--color-muted)">{actionError}</p>
      {/if}
      <Button variant="outline" size="sm" class="mt-4" onclick={() => goto('/users')}>
        {t('users.userId.back_to_users', 'Back to Users')}
      </Button>
    </CardContent>
  </Card>
{/if}

<!-- Restrict Modal -->
<Modal bind:open={showRestrictModal} title={t('users.detail.restrict_user', 'Restrict User')} size="default">
  <div class="space-y-4">
    {#if restrictions.length > 0}
      <div class="rounded-md border p-3" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent)">
        <p class="text-xs font-medium mb-2" style="color: var(--color-warning)">{t('users.detail.current_restrictions', 'Currently restricted operations:')}</p>
        <div class="flex flex-wrap gap-2">
          {#each restrictions as r (r.id)}
            <span class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium bg-warning/15 text-warning dark:text-warning">
              {r.operation} — {r.expires_at ? new Date(r.expires_at).toLocaleDateString() : t('common.permanent', 'Permanent')}
              <button type="button" class="ms-1 hover:opacity-70" onclick={() => liftRestriction(r.operation)}>&times;</button>
            </span>
          {/each}
        </div>
      </div>
    {/if}
    <div>
      <p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.restrict_operations', 'Operations to restrict')}</p>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        {#each restrictionOps as op (op)}
          {@const alreadyRestricted = restrictions.some(r => r.operation === op.value)}
          <label class="flex items-center gap-2 text-sm" class:opacity-50={alreadyRestricted}>
            <input type="checkbox" value={op.value} bind:group={restrictOperations} class="rounded" disabled={alreadyRestricted} />
            {op.label}
            {#if alreadyRestricted}
              <span class="text-xs" style="color: var(--color-warning)">{t('users.detail.already_restricted', '(already restricted)')}</span>
            {/if}
          </label>
        {/each}
      </div>
    </div>
    <div>
      <label for="restrict-duration" class="mb-1 block text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.duration', 'Duration')}</label>
      <Autocomplete
        id="restrict-duration"
        placeholder={t('users.detail.select_duration', 'Select duration')}
        emptyMessage={t('common.no_results', 'No results found')}
        fetcher={async (q: string) => {
          const needle = q.trim().toLowerCase();
          return restrictionDurations
            .filter((d) => !needle || d.label.toLowerCase().includes(needle) || d.value.toLowerCase().includes(needle))
            .map((d) => ({ id: d.value, label: d.label }));
        }}
        initialFetcher={async () => restrictionDurations.map((d) => ({ id: d.value, label: d.label }))}
        selectedId={restrictDuration || null}
        selectedLabel={restrictDuration ? (restrictionDurations.find((d) => d.value === restrictDuration)?.label ?? restrictDuration) : null}
        onSelect={(item: AutocompleteItem) => { restrictDuration = item.id; }}
        onClear={() => { restrictDuration = ''; }}
      />
    </div>
    <div>
      <label for="restrict-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.reason', 'Reason')}</label>
      <textarea id="restrict-reason" rows="2" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder={t('users.ph_reason_for_restriction', 'Reason for restriction...')} bind:value={restrictReason}></textarea>
    </div>
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showRestrictModal = false)}>{t('common.cancel', 'Cancel')}</Button>
    <Button variant="destructive" onclick={handleRestrict} disabled={restrictOperations.length === 0}>{t('users.detail.restrict_user', 'Restrict User')}</Button>
  </div>
</Modal>

<!-- Suspend Modal -->
<Modal bind:open={showSuspendModal} title={t('users.detail.suspend_user', 'Suspend User')} size="sm">
  <p class="text-sm" style="color: var(--color-foreground)">
    {t('users.detail.suspend_body', 'Suspend {username}? This permanently restricts all operations.').replace('{username}', user?.username ?? '')}
  </p>
  <p class="mt-2 text-sm" style="color: var(--color-danger)">
    {t('users.detail.suspend_effect', 'The user will be unable to perform any action on the platform until unsuspended.')}
  </p>
  <div class="mt-3">
    <label for="suspend-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.reason', 'Reason')}</label>
    <textarea id="suspend-reason" rows="2" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder={t('users.ph_reason_for_suspension', 'Reason for suspension...')} bind:value={suspendReason}></textarea>
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showSuspendModal = false)}>{t('common.cancel', 'Cancel')}</Button>
    <Button variant="destructive" onclick={handleSuspendUser}>{t('users.detail.suspend_user', 'Suspend User')}</Button>
  </div>
</Modal>

<!-- Ban Confirmation -->
<Modal bind:open={showBanConfirm} title={t('users.detail.ban', 'Ban User')} size="sm">
  <p class="text-sm" style="color: var(--color-foreground)">
    {t('users.detail.ban_confirm', 'Are you sure you want to ban this user?')} <strong>{user?.username}</strong>
  </p>
  <p class="mt-2 text-sm" style="color: var(--color-danger)">
    {t('users.detail.ban_notice', 'The user will lose access to the platform until the ban is lifted.')}
  </p>
  <div class="mt-3">
    <label for="ban-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.ban_reason', 'Reason (optional)')}</label>
    <textarea id="ban-reason" rows="2" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder={t('users.ban_reason_ph', 'Reason for ban...')} bind:value={banReason}></textarea>
  </div>
  <div class="mt-3">
    <label for="ban-days" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.ban_days', 'Duration in days (0 = permanent)')}</label>
    <input id="ban-days" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={banDays} />
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showBanConfirm = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="destructive" onclick={handleBan}>{t('users.detail.ban', 'Ban')}</Button>
  </div>
</Modal>

<!-- Verify (§07 — grant badge with optional type) -->
<Modal bind:open={showVerifyModal} title={t('users.verify_user', 'Verify User')} size="sm">
  <p class="text-sm" style="color: var(--color-foreground)">
    {t('users.verify_confirm', 'Grant a verification badge to')} <strong>{user?.username}</strong>?
  </p>
  <label class="mt-3 block text-sm" style="color: var(--color-foreground)">
    {t('users.verify_type', 'Badge type')}
    <select
      class="mt-1 w-full rounded-md border px-3 py-2 text-sm"
      style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
      bind:value={verifyType}
    >
      <option value="">{t('users.verify_type_none', '— none —')}</option>
      {#each verificationTypes as ty (ty.value)}
        <option value={ty.value}>{ty.label}</option>
      {/each}
    </select>
  </label>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showVerifyModal = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" onclick={handleVerify}>{t('users.verify', 'Verify')}</Button>
  </div>
</Modal>

<!-- Unban Confirmation -->
<Modal bind:open={showUnbanConfirm} title={t('users.detail.unban', 'Unban User')} size="sm">
  <p class="text-sm" style="color: var(--color-foreground)">
    {t('users.detail.unban_confirm', 'Are you sure you want to lift the ban for this user?')} <strong>{user?.username}</strong>
  </p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showUnbanConfirm = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" onclick={handleUnban}>{t('users.detail.unban', 'Unban')}</Button>
  </div>
</Modal>

<!-- Reverse Ledger Entry -->
<Modal bind:open={showReverseModal} title={t('users.userId.reverse_entry', 'Reverse Ledger Entry')} size="sm">
  <p class="text-sm" style="color: var(--color-foreground)">
    {t('users.userId.reverse_confirm', 'Post a compensating entry that undoes this ledger row?')}
  </p>
  {#if reverseTarget}
    <p class="mt-2 font-mono text-xs" style="color: var(--color-muted)">
      {reverseTarget.ledger_type} · {reverseTarget.source ?? 'entry'} · {reverseTarget.amount > 0 ? '+' : ''}{reverseTarget.amount}
    </p>
  {/if}
  <div class="mt-4">
    <label for="reverse-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('common.reason', 'Reason (optional)')}</label>
    <textarea id="reverse-reason" rows="2" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder={t('users.userId.reverse_reason_ph', 'e.g. False-positive spam flag')} bind:value={reverseReason}></textarea>
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showReverseModal = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" disabled={reverseBusy} onclick={reverseEntry}>{t('users.userId.reverse_entry', 'Reverse')}</Button>
  </div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('users.detail.delete', 'Delete User')} size="sm">
  <p class="text-sm" style="color: var(--color-muted)">{t('users.userId.are_you_sure_you_want_to_delete_this_use')}</p>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showDeleteConfirm = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="destructive" onclick={handleDelete}>{t('users.userId.delete_user')}</Button>
  </div>
</Modal>

<!-- Warn Modal (with violation type) -->
<Modal bind:open={showWarningModal} title={t('users.detail.warning', 'Issue Warning')} size="sm">
  <div class="space-y-3">
    <div>
      <label for="warning-violation" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.violation_type', 'Violation Type')}</label>
      <Autocomplete
        id="warning-violation"
        placeholder={t('users.detail.select_violation', 'Select violation type')}
        emptyMessage={t('common.no_results', 'No results found')}
        fetcher={async (q: string) => {
          const needle = q.trim().toLowerCase();
          return violationTypes
            .filter((vt) => !needle || vt.label.toLowerCase().includes(needle) || vt.value.toLowerCase().includes(needle))
            .map((vt) => ({ id: vt.value, label: vt.label }));
        }}
        initialFetcher={async () => violationTypes.map((vt) => ({ id: vt.value, label: vt.label }))}
        selectedId={warningViolationType || null}
        selectedLabel={warningViolationType ? (violationTypes.find((vt) => vt.value === warningViolationType)?.label ?? warningViolationType) : null}
        onSelect={(item: AutocompleteItem) => { warningViolationType = item.id; }}
        onClear={() => { warningViolationType = ''; }}
      />
    </div>
    <div>
      <label for="warning-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.userId.reason_for_warning')}</label>
      <textarea id="warning-reason" rows="3" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder={t('users.ph_enter_the_reason_for_this', 'Enter the reason for this warning...')} bind:value={warningReason}></textarea>
    </div>
    <p class="text-xs" style="color: var(--color-muted)">After 3 warnings of the same type, the user will be auto-restricted from related operations for 7 days.</p>
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showWarningModal = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" onclick={handleWarnTyped} disabled={!warningReason.trim() || !warningViolationType}>{t('users.userId.send_warning')}</Button>
  </div>
</Modal>

<Modal bind:open={showGroupModal} title={t('users.detail.change_group', 'Change Group')} size="sm">
  <div class="space-y-2">
    {#each groupOptions as group (group.id)}
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" value={group.id} bind:group={selectedGroups} class="rounded" />
        {group.name}
        <span class="text-xs" style="color: var(--color-muted)">({group.slug})</span>
      </label>
    {/each}
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showGroupModal = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" onclick={handleGroupChange}>{t('users.userId.save_changes')}</Button>
  </div>
</Modal>

<Modal bind:open={showProfileTypeModal} title={t('users.detail.change_profile_type', 'Change Profile Type')} size="sm">
  <div>
    <label for="profile-type-select" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.profile_type', 'Profile Type')}</label>
    <Autocomplete
      id="profile-type-select"
      placeholder={t('users.detail.select_profile_type', 'Select profile type')}
      emptyMessage={t('common.no_results', 'No results found')}
      fetcher={async (q: string) => {
        const needle = q.trim().toLowerCase();
        return profileTypeOptions
          .map((pt) => ({ id: pt, label: t('users.profile_type_' + pt, pt) }))
          .filter((o) => !needle || o.label.toLowerCase().includes(needle) || o.id.toLowerCase().includes(needle));
      }}
      initialFetcher={async () => profileTypeOptions.map((pt) => ({ id: pt, label: t('users.profile_type_' + pt, pt) }))}
      selectedId={selectedProfileType || null}
      selectedLabel={selectedProfileType ? t('users.profile_type_' + selectedProfileType, selectedProfileType) : null}
      onSelect={(item: AutocompleteItem) => { selectedProfileType = item.id; }}
      onClear={() => { selectedProfileType = ''; }}
    />
  </div>
  <div class="mt-6 flex justify-end gap-3">
    <Button variant="outline" onclick={() => (showProfileTypeModal = false)}>{t('users.userId.cancel')}</Button>
    <Button variant="default" onclick={handleProfileTypeChange} disabled={!selectedProfileType}>{t('users.userId.save_changes')}</Button>
  </div>
</Modal>
