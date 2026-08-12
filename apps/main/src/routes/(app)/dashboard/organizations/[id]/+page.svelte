<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organizations.md (org admin dashboard)
	// Org admin view: edit org fields (PATCH /organizations/{id}), manage
	// members (list + remove + change role), invite members, revoke invitations,
	// and delete the org. Mutations go through the browser API client; the org
	// row is seeded from the server load.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { UrlTabs, type UrlTab } from '$lib/components/ui';
	import { Building2, Save, Trash2, Mail, Check, X, FileText, BarChart3, Send, UserPlus, Megaphone } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll, goto } from '$app/navigation';
	import type { OrganizationAdmin, OrgMember, ProfilePost } from '$lib/types';

	let { data } = $props();
	const org = $derived(data.org);
	const invitations = $derived(data.invitations as { id: string; email?: string; role?: string; status?: string; created_at?: string }[]);
	const content = $derived(data.content as ProfilePost[]);
	const analytics = $derived(data.analytics as {
		total_views?: number;
		total_posts?: number;
		total_followers?: number;
		total_members?: number;
		posts_last_30d?: number;
	} | null);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'settings'));

	const tabs: UrlTab[] = [
		{ value: 'settings', label: t('org_admin.tab_settings', 'Settings') },
		{ value: 'content', label: t('org_admin.tab_content', 'Content') },
		{ value: 'analytics', label: t('org_admin.tab_analytics', 'Analytics') }
	];

	let name = $state(untrack(() => data.org?.name ?? ''));
	let tagline = $state(untrack(() => data.org?.tagline ?? ''));
	let description = $state(untrack(() => data.org?.description ?? ''));
	let websiteUrl = $state(untrack(() => data.org?.website_url ?? ''));
	let countryCode = $state(untrack(() => data.org?.country_code ?? ''));
	let orgType = $state(untrack(() => data.org?.org_type ?? 'business'));

	let saving = $state(false);
	let savedKey = $state('');
	let errorKey = $state('');

	let members = $state<OrgMember[]>(untrack(() => data.members ?? []));

	// Invite-member form state
	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);
	let inviteErrorKey = $state('');

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
			const res = await api.patch(`/organizations/${org.id}`, {
				name,
				tagline: tagline || null,
				description: description || null,
				website_url: websiteUrl || null,
				country_code: countryCode || null,
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

	async function changeRole(memberUserId: string, role: string) {
		if (!org) return;
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
	{:else if activeTab === 'analytics'}
		<!-- Analytics tab (Phase 29.06) -->
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
						<FileText size={14} /> {t('org_admin.analytics_posts_30d', 'Posts (30d)')}
					</div>
					<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.posts_last_30d ?? 0}</p>
				</div>
			</div>
		{:else}
			<EmptyState icon={BarChart3} title={t('org_admin.analytics_unavailable', 'Analytics unavailable')} />
		{/if}
	{:else}
	<!-- Edit org fields -->
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
				<input class={inputClass} value={countryCode} oninput={(e) => (countryCode = e.currentTarget.value)} placeholder="NG" />
			</label>
			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.field_type')}</span>
				<select class={inputClass} bind:value={orgType}>
					{#if data.orgMeta?.org_types}
						{#each data.orgMeta.org_types as ot (ot.value)}
							<option value={ot.value}>{ot.label}</option>
						{/each}
					{:else}
						<option value="business">{t('organizations.new.type_business', 'Business')}</option>
						<option value="ngo">{t('organizations.new.type_ngo', 'NGO')}</option>
						<option value="government">{t('organizations.new.type_government', 'Government')}</option>
						<option value="embassy">{t('organizations.new.type_embassy', 'Embassy')}</option>
						<option value="university">{t('organizations.new.type_university', 'University')}</option>
						<option value="media">{t('organizations.new.type_media', 'Media')}</option>
						<option value="community">{t('organizations.new.type_community', 'Community')}</option>
					{/if}
				</select>
			</label>
		</div>

		<div class="flex items-center justify-between">
			<button
				type="button"
				onclick={deleteOrg}
				disabled={deleting}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-danger)]/30 px-4 py-2 text-sm font-medium text-[var(--color-danger)] transition hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
			>
				<Trash2 class="h-4 w-4" />
				{deleting ? t('common.loading') : t('org_admin.delete', 'Delete organization')}
			</button>
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
				<select class={inputClass} bind:value={inviteRole}>
					{#if data.orgMeta?.invitation_roles}
						{#each data.orgMeta.invitation_roles as ir (ir.value)}
							<option value={ir.value}>{ir.label}</option>
						{/each}
					{:else}
						<option value="admin">{t('org_admin.role.admin')}</option>
						<option value="editor">{t('org_admin.role.editor', 'Editor')}</option>
						<option value="member">{t('org_admin.role.member')}</option>
					{/if}
				</select>
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

	<!-- Members management -->
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
									{m.user.display_name.slice(0, 1).toUpperCase()}
								</div>
							{/if}
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{m.user.display_name}</p>
								<p class="truncate text-xs text-[var(--color-muted)]">@{m.user.username}</p>
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-2">
							<select
								value={m.role}
								onchange={(e) => changeRole(m.user.id, e.currentTarget.value)}
								class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)]"
								aria-label={t('org_admin.role_label')}
							>
								{#if data.orgMeta?.member_roles}
									{#each data.orgMeta.member_roles as mr (mr.value)}
										<option value={mr.value}>{mr.label}</option>
									{/each}
								{:else}
									<option value="owner">{t('org_admin.role.owner')}</option>
									<option value="admin">{t('org_admin.role.admin')}</option>
									<option value="editor">{t('org_admin.role.editor', 'Editor')}</option>
									<option value="member">{t('org_admin.role.member')}</option>
								{/if}
							</select>
							<button
								type="button"
								onclick={() => removeMember(m.user.id)}
								class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]"
								aria-label={t('org_admin.remove_member')}
							>
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<!-- Invitations -->
	{#if invitations.length > 0}
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
	{/if}
</section>
