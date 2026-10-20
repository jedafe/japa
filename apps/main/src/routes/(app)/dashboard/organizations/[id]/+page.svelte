<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organizations.md (org admin dashboard)
	// Org admin view: edit org fields (PATCH /organizations/{id}), manage
	// members (list + remove + change role), invite members, revoke invitations,
	// and delete the org. Mutations go through the browser API client; the org
	// row is seeded from the server load.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { UrlTabs, type UrlTab, AutocompleteSelect } from '$lib/components/ui';
	import { Building2, Save, Trash2, Mail, Check, X, FileText, BarChart3, Send, UserPlus, UserCheck, Megaphone, Image } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll, goto } from '$app/navigation';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { OrganizationAdmin, OrgInvitation, OrgMedia, OrgMember, ProfilePost } from '$lib/types';

	let { data } = $props();
	const org = $derived(data.org);
	// The caller's role on this org. Mirrors the backend checks:
	// PATCH org + member add/remove/invite = owner|admin; member role
	// changes + DELETE org = owner only; self-removal = any non-owner.
	const memberRole = $derived(data.memberRole ?? '');
	const isOwner = $derived(memberRole === 'owner');
	const canManage = $derived(memberRole === 'owner' || memberRole === 'admin');
	// 'owner' is never assignable — the backend rejects it (single-owner
	// invariant), so it is filtered out of every role selector.
	const assignableRoles = $derived(
		(data.orgMeta?.member_roles ?? []).filter((r) => r.value !== 'owner')
	);
	const invitations = $derived(data.invitations as OrgInvitation[]);
	const content = $derived(data.content as ProfilePost[]);
	const media = $derived(data.media as OrgMedia[]);
	const analytics = $derived(data.analytics as {
		total_views?: number;
		total_posts?: number;
		total_followers?: number;
		total_members?: number;
		posts_last_30d?: number;
	} | null);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'settings'));
	// Keep the bound tab in sync with ?tab= on back/forward navigation —
	// UrlTabs only writes the URL on click, it doesn't re-read it.
	$effect(() => {
		const tabParam = page.url.searchParams.get('tab');
		untrack(() => { activeTab = tabParam ?? 'settings'; });
	});

	// Country autocomplete — filters the SSR-loaded country spaces
	// (data.countries from /geo/countries) client-side (Rule 16).
	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const list = data.countries ?? [];
		const filtered = needle
			? list.filter(
					(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
				)
			: list;
		return Promise.resolve(
			filtered.slice(0, 50).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function countryInitialFetcher(): Promise<AutocompleteItem[]> {
		const list = data.countries ?? [];
		return Promise.resolve(
			list.slice(0, 8).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function countryLabel(id: string): string {
		return (data.countries ?? []).find((c) => c.geo_code === id)?.name ?? id;
	}

	const tabs: UrlTab[] = [
		{ value: 'settings', label: t('org_admin.tab_settings', 'Settings') },
		{ value: 'content', label: t('org_admin.tab_content', 'Content') },
		{ value: 'media', label: t('org_admin.tab_media', 'Media') },
		{ value: 'analytics', label: t('org_admin.tab_analytics', 'Analytics') }
	];

	let name = $state(untrack(() => data.org?.name ?? ''));
	let tagline = $state(untrack(() => data.org?.tagline ?? ''));
	let description = $state(untrack(() => data.org?.description ?? ''));
	let websiteUrl = $state(untrack(() => data.org?.website_url ?? ''));
	let countryCode = $state(untrack(() => data.org?.country_code ?? ''));
	let orgType = $state(untrack(() => data.org?.org_type ?? 'business'));

	// Reseed the settings form only when a *different* org is loaded
	// (cross-org navigation) — keyed on org id so invalidateAll() after a
	// save or member mutation doesn't clobber edits in progress.
	$effect(() => {
		void data.org?.id;
		untrack(() => {
			name = data.org?.name ?? '';
			tagline = data.org?.tagline ?? '';
			description = data.org?.description ?? '';
			websiteUrl = data.org?.website_url ?? '';
			countryCode = data.org?.country_code ?? '';
			orgType = data.org?.org_type ?? 'business';
		});
	});

	let saving = $state(false);
	let savedKey = $state('');
	let errorKey = $state('');

	// Derived, not seeded: member mutations (add/remove/change-role) end with
	// invalidateAll() which re-runs the load — a one-time seed would keep
	// showing the pre-mutation list.
	const members = $derived<OrgMember[]>(data.members ?? []);

	// Invite-member form state
	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);
	let inviteErrorKey = $state('');

	// Add-member (by username) form state — POST /organizations/{id}/members.
	let addUsername = $state('');
	let addRole = $state('member');
	let adding = $state(false);
	let addErrorKey = $state('');

	// Delete-org state
	let deleting = $state(false);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	async function saveOrg(e: SubmitEvent) {
		e.preventDefault();
		if (!org) return;
		errorKey = '';
		saving = true;
		try {
			// Empty strings are sent as '' — the backend clears the column
			// to NULL (present-empty semantics), while absent keys keep the
			// stored value.
			const res = await api.patch(`/organizations/${org.id}`, {
				name,
				tagline,
				description,
				website_url: websiteUrl,
				country_code: countryCode,
				org_type: orgType
			});
			if (res.error) {
				errorKey = 'org_admin.errors.save_failed';
			} else {
				savedKey = 'org_admin.saved';
				setTimeout(() => (savedKey = ''), 2500);
				await invalidateAll();
			}
		} catch {
			errorKey = 'org_admin.errors.save_failed';
		} finally {
			saving = false;
		}
	}

	async function removeMember(memberUserId: string) {
		if (!org) return;
		if (!confirm(t('org_admin.remove_member_confirm'))) return;
		try {
			const res = await api.delete(`/organizations/${org.id}/members/${memberUserId}`);
			if (!res.error) {
				await invalidateAll();
			}
		} catch {
			// best-effort
		}
	}

	// Self-removal: any non-owner member may leave the org (backend allows
	// DELETE /members/{self} for non-owners). The owner sees Delete instead.
	let leaving = $state(false);
	async function leaveOrg() {
		if (!org || !page.data.user?.id) return;
		if (!confirm(t('org_admin.leave_confirm', 'Leave this organization?'))) return;
		leaving = true;
		try {
			const res = await api.delete(`/organizations/${org.id}/members/${page.data.user.id}`);
			if (!res.error) {
				await goto('/dashboard/organizations');
			}
		} catch {
			// best-effort
		} finally {
			leaving = false;
		}
	}

	async function changeRole(memberUserId: string, role: string) {
		if (!org) return;
		const target = members.find((m) => m.user.id === memberUserId);
		if (!target) return;
		if (target.role === 'owner') return; // owner role is immutable

		try {
			const res = await api.patch(`/organizations/${org.id}/members/${memberUserId}`, { role });
			if (!res.error) {
				await invalidateAll();
			}
		} catch {
			// best-effort
		}
	}

	async function inviteMember(e: SubmitEvent) {
		e.preventDefault();
		if (!org || !inviteEmail.trim()) return;
		inviteErrorKey = '';
		inviting = true;
		try {
			const res = await api.post(`/organizations/${org.id}/invitations`, {
				email: inviteEmail.trim(),
				role: inviteRole
			});
			if (res.error) {
				inviteErrorKey = 'org_admin.errors.invite_failed';
			} else {
				inviteEmail = '';
				inviteRole = 'member';
				await invalidateAll();
			}
		} catch {
			inviteErrorKey = 'org_admin.errors.invite_failed';
		} finally {
			inviting = false;
		}
	}

	// Section 07 — "add existing member by user ID". The UI resolves a
	// username to a user ID via GET /users/{username}, then POSTs to
	// POST /organizations/{id}/members. Duplicates return 409 already_exists.
	async function addMember(e: SubmitEvent) {
		e.preventDefault();
		if (!org || !addUsername.trim()) return;
		addErrorKey = '';
		adding = true;
		try {
			const username = addUsername.trim().replace(/^@/, '');
			const userRes = await api.get<{ id: string }>(`/users/${encodeURIComponent(username)}`);
			if (!userRes.data?.id) {
				addErrorKey = 'org_admin.errors.user_not_found';
				return;
			}
			const res = await api.post(`/organizations/${org.id}/members`, {
				user_id: userRes.data.id,
				role: addRole
			});
			if (res.error) {
				addErrorKey = res.error.code === 'already_exists'
					? 'org_admin.errors.already_member'
					: 'org_admin.errors.add_member_failed';
			} else {
				addUsername = '';
				addRole = 'member';
				await invalidateAll();
			}
		} catch {
			addErrorKey = 'org_admin.errors.add_member_failed';
		} finally {
			adding = false;
		}
	}

	async function revokeInvitation(invitationId: string) {
		if (!org) return;
		if (!confirm(t('org_admin.revoke_invite_confirm', 'Revoke this invitation?'))) return;
		try {
			const res = await api.delete(`/organizations/${org.id}/invitations/${invitationId}`);
			if (!res.error) {
				await invalidateAll();
			}
		} catch {
			// best-effort
		}
	}

	async function deleteOrg() {
		if (!org) return;
		if (!confirm(t('org_admin.delete_confirm', 'Delete this organization? This cannot be undone.'))) return;
		deleting = true;
		try {
			const res = await api.delete(`/organizations/${org.id}`);
			if (!res.error) {
				await goto('/dashboard/organizations');
			}
		} catch {
			// best-effort
		} finally {
			deleting = false;
		}
	}
</script>

<section class="space-y-6">
	<header>
		<a href="/dashboard/organizations" class="text-sm text-[var(--color-muted)] hover:underline">
			← {t('org_admin.back')}
		</a>
		<div class="mt-2 flex items-center gap-3">
			<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Building2 class="h-5 w-5" />
			</div>
			<div class="flex-1">
				<h1 class="text-lg font-medium text-[var(--color-text)]">{org?.name}</h1>
				<p class="text-xs text-[var(--color-muted)]">/{org?.slug} · {t(`org_admin.status.${org?.status ?? 'active'}`)}</p>
			</div>
			<!-- §61: Link to self-serve ads dashboard -->
			<a
				href="/dashboard/organizations/{org?.id}/ads"
				class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
			>
				<Megaphone class="h-4 w-4" />
				{t('org_ads.title', 'Ad Campaigns')}
			</a>
		</div>
	</header>

	<!-- Tabbed shell (Phase 29.01) -->
	<UrlTabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'content'}
		<!-- Content tab (Phase 29.04) -->
		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="text-sm font-medium text-[var(--color-text)]">
				{t('org_admin.content_section', 'Published content')} ({content.length})
			</h2>
			{#if content.length === 0}
				<EmptyState icon={FileText} title={t('org_admin.no_content', 'No content yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)]">
					{#each content as post (post.id)}
						<a href="/content/{post.slug}" class="block py-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]">{post.content_type}</span>
								<span>· {post.read_time_minutes} {t('feed.min_read')}</span>
							</div>
							<h3 class="mt-1 text-sm font-medium text-[var(--color-text)]">{post.title}</h3>
							<div class="mt-1 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{post.view_count} {t('profile.views', 'views')}</span>
								<span>{post.comment_count} {t('profile.comments', 'comments')}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{:else if activeTab === 'media'}
	<!-- Media tab -->
	<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
		<h2 class="text-sm font-medium text-[var(--color-text)]">
			{t('org_admin.media_section', 'Media')} ({media.length})
		</h2>
		{#if media.length === 0}
			<EmptyState icon={Image} title={t('org_admin.no_media', 'No media yet')} />
		{:else}
			<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
				{#each media as item (item.id)}
					{#if item.mime_type?.startsWith('image/')}
						<a
							href={item.url}
							target="_blank"
							rel="noopener"
							class="group block overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
							title={item.alt_text ?? item.filename}
						>
							<img src={item.url} alt={item.alt_text ?? item.filename} loading="lazy" class="aspect-square w-full object-cover transition-transform group-hover:scale-[1.03]" />
						</a>
					{:else}
						<a
							href={item.url}
							target="_blank"
							rel="noopener"
							class="flex aspect-square flex-col items-center justify-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-center transition-colors hover:bg-[var(--color-surface-subtle)]"
							title={item.filename}
						>
							<Image class="h-5 w-5 text-[var(--color-muted)]" />
							<span class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{item.filename}</span>
						</a>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
{:else if activeTab === 'analytics'}
		<!-- Analytics tab (Phase 29.06) -->
		<div class="flex items-center justify-end gap-2">
			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<span>{t('org_admin.analytics_period', 'Period')}:</span>
				<AutocompleteSelect
					class="w-44"
					value={data.period}
					options={[
						{ value: '7d', label: t('org_admin.analytics_7d', 'Last 7 days') },
						{ value: '30d', label: t('org_admin.analytics_30d', 'Last 30 days') },
						{ value: '90d', label: t('org_admin.analytics_90d', 'Last 90 days') }
					]}
					onValueChange={(v) => {
						const url = new URL(page.url);
						url.searchParams.set('tab', 'analytics');
						url.searchParams.set('period', v);
						goto(`${url.pathname}${url.search}`);
					}}
				/>
			</label>
		</div>
		{#if analytics}
			<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
						<BarChart3 size={14} /> {t('org_admin.analytics_views', 'Total views')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_views ?? 0}</p>
				</div>
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
						<FileText size={14} /> {t('org_admin.analytics_posts', 'Total posts')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_posts ?? 0}</p>
				</div>
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
						<Building2 size={14} /> {t('org_admin.analytics_members', 'Members')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_members ?? 0}</p>
				</div>
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
						<Building2 size={14} /> {t('org_admin.analytics_followers', 'Followers')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_followers ?? 0}</p>
				</div>
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
						<FileText size={14} /> {t('org_admin.analytics_posts_period', 'Posts in period')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.posts_last_30d ?? 0}</p>
				</div>
			</div>
		{:else}
			<EmptyState icon={BarChart3} title={t('org_admin.analytics_unavailable', 'Analytics unavailable')} />
		{/if}
	{:else}
	<!-- Edit org fields — owner|admin only (PATCH /organizations/{id}
	     enforces the same role gate server-side). -->
	{#if canManage}
	<form onsubmit={saveOrg} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
		<h2 class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.edit_section')}</h2>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
		{/if}
		{#if savedKey}
			<p class="rounded-[var(--radius)] bg-[var(--color-success)]/10 px-3 py-2 text-sm text-[var(--color-success)]" role="status">{t(savedKey)}</p>
		{/if}

		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_name')}</span>
				<input class={inputClass} value={name} oninput={(e) => (name = e.currentTarget.value)} />
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_tagline')}</span>
				<input class={inputClass} value={tagline} oninput={(e) => (tagline = e.currentTarget.value)} />
			</label>
		</div>

		<label class="flex flex-col gap-1.5">
			<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_description')}</span>
			<textarea class={inputClass} rows={4} value={description} oninput={(e) => (description = e.currentTarget.value)}></textarea>
		</label>

		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_website')}</span>
				<input class={inputClass} value={websiteUrl} oninput={(e) => (websiteUrl = e.currentTarget.value)} placeholder="https://" />
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_country')}</span>
				<Autocomplete
					placeholder={t('organizations.new.country_placeholder', 'Search country…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={countryFetcher}
					initialFetcher={countryInitialFetcher}
					selectedId={countryCode || null}
					selectedLabel={countryCode ? countryLabel(countryCode) : null}
					onSelect={(item) => { countryCode = item.id; }}
					onClear={() => { countryCode = ''; }}
				/>
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_type')}</span>
				<AutocompleteSelect
					bind:value={orgType}
					options={data.orgMeta?.org_types ?? []}
					disabled={!data.orgMeta?.org_types?.length}
				/>
			</label>
		</div>

		<div class="flex items-center justify-between">
			{#if isOwner}
				<button
					type="button"
					onclick={deleteOrg}
					disabled={deleting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-danger)]/30 px-4 py-2 text-sm font-medium text-[var(--color-danger)] transition hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
				>
					<Trash2 class="h-4 w-4" />
					{deleting ? t('common.loading') : t('org_admin.delete', 'Delete organization')}
				</button>
			{:else}
				<button
					type="button"
					onclick={leaveOrg}
					disabled={leaving}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{leaving ? t('common.loading') : t('org_admin.leave', 'Leave organization')}
				</button>
			{/if}
			<button
				type="submit"
				disabled={saving}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				<Save class="h-4 w-4" />
				{saving ? t('common.loading') : t('org_admin.save')}
			</button>
		</div>
	</form>
	{/if}

	{#if canManage}
	<!-- Invite member -->
	<form onsubmit={inviteMember} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
		<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<UserPlus class="h-4 w-4" />
			{t('org_admin.invite_section', 'Invite member')}
		</h2>
		{#if inviteErrorKey}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(inviteErrorKey)}</p>
		{/if}
		<div class="flex flex-col gap-3 sm:flex-row sm:items-end">
			<label class="flex flex-1 flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.invite_email', 'Email')}</span>
				<input class={inputClass} type="email" bind:value={inviteEmail} placeholder="member@example.com" required />
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.invite_role', 'Role')}</span>
				<AutocompleteSelect
					bind:value={inviteRole}
					options={data.orgMeta?.invitation_roles ?? []}
					disabled={!data.orgMeta?.invitation_roles?.length}
				/>
			</label>
			<button
				type="submit"
				disabled={inviting || !inviteEmail.trim()}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				<Send class="h-4 w-4" />
				{inviting ? t('common.loading') : t('org_admin.invite_send', 'Send invite')}
			</button>
		</div>
	</form>

	<!-- Add member by username — POST /organizations/{id}/members (Section 07
	     "add existing member by user ID"; the username resolves to a user ID
	     via GET /users/{username}). -->
	<form onsubmit={addMember} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
		<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<UserCheck class="h-4 w-4" />
			{t('org_admin.add_member_section', 'Add member')}
		</h2>
		<p class="text-xs text-[var(--color-muted)]">
			{t('org_admin.add_member_hint', 'Add an existing {site} user directly by username.')}
		</p>
		{#if addErrorKey}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(addErrorKey)}</p>
		{/if}
		<div class="flex flex-col gap-3 sm:flex-row sm:items-end">
			<label class="flex flex-1 flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.add_member_username', 'Username')}</span>
				<input class={inputClass} type="text" bind:value={addUsername} placeholder="@username" required />
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.invite_role', 'Role')}</span>
				<AutocompleteSelect
					bind:value={addRole}
					options={(data.orgMeta?.member_roles ?? []).filter((r) => r.value !== 'owner')}
					disabled={!data.orgMeta?.member_roles?.length}
				/>
			</label>
			<button
				type="submit"
				disabled={adding || !addUsername.trim()}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				<UserCheck class="h-4 w-4" />
				{adding ? t('common.loading') : t('org_admin.add_member_submit', 'Add member')}
			</button>
		</div>
	</form>
	{/if}

	<!-- Members management — list is visible to every member; role changes
	     are owner-only, removals are owner|admin (same gates as the API). -->
	<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
		<h2 class="text-sm font-medium text-[var(--color-text)]">
			{t('org_admin.members_section')} ({members.length})
		</h2>

		{#if members.length === 0}
			<EmptyState icon={Building2} title={t('org_admin.no_members')} />
		{:else}
			<ul class="space-y-2">
				{#each members as m (m.user.id)}
					<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
						<div class="flex min-w-0 items-center gap-3">
							{#if m.user.avatar_url}
								<img src={m.user.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div class="flex h-8 w-8 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-xs font-medium text-[var(--color-primary)]">
									{(m.user.display_name ?? m.user.username).slice(0, 1).toUpperCase()}
								</div>
							{/if}
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{m.user.display_name ?? m.user.username}</p>
								<p class="truncate text-xs text-[var(--color-muted)]">@{m.user.username}</p>
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-2">
							{#if isOwner && m.role !== 'owner'}
								<AutocompleteSelect
									class="w-40"
									value={m.role}
									options={assignableRoles}
									onValueChange={(v) => changeRole(m.user.id, v)}
									ariaLabel={t('org_admin.role_label')}
									disabled={assignableRoles.length === 0}
									inputClass="text-xs"
								/>
							{:else}
								<span class="rounded-full bg-[var(--color-surface)] px-2 py-0.5 text-xs capitalize text-[var(--color-text-subtle)]">{m.role}</span>
							{/if}
							{#if canManage && m.role !== 'owner'}
								<button
									type="button"
									onclick={() => removeMember(m.user.id)}
									class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]"
									aria-label={t('org_admin.remove_member')}
								>
									<Trash2 class="h-4 w-4" />
								</button>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<!-- Invitations — owner|admin only (the API gates them identically). -->
	{#if canManage && invitations.length > 0}
		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Mail class="h-4 w-4" />
				{t('org_admin.invitations_section')} ({invitations.length})
			</h2>
			<ul class="space-y-2">
				{#each invitations as inv (inv.id)}
					<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{inv.email}</p>
							<p class="text-xs text-[var(--color-muted)]">
								{inv.role ?? 'member'} · {inv.status ?? 'pending'}
								{#if inv.created_at}· {new Date(inv.created_at).toLocaleDateString()}{/if}
							</p>
						</div>
						{#if inv.status === 'pending'}
							<div class="flex shrink-0 items-center gap-2">
								<span class="rounded-full bg-[var(--color-warning)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-warning)]">
									{t('org_admin.invite_pending')}
								</span>
								<button
									type="button"
									onclick={() => revokeInvitation(inv.id)}
									class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]"
									aria-label={t('org_admin.revoke_invite', 'Revoke invitation')}
								>
									<X class="h-4 w-4" />
								</button>
							</div>
						{:else}
							<Check class="h-4 w-4 shrink-0 text-[var(--color-success)]" />
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<!-- Leave organization — non-owner members (member|editor) get
	     self-removal here; owner/admin use the gated controls above. -->
	{#if memberRole && !isOwner}
		<div class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<p class="text-sm text-[var(--color-muted)]">
				{t('org_admin.leave_hint', 'You are a {role} of this organization.').replace('{role}', memberRole)}
			</p>
			<button
				type="button"
				onclick={leaveOrg}
				disabled={leaving}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-danger)]/30 px-4 py-2 text-sm font-medium text-[var(--color-danger)] transition hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
			>
				{leaving ? t('common.loading') : t('org_admin.leave', 'Leave organization')}
			</button>
		</div>
	{/if}
	{/if}
</section>
