<script lang="ts">
	// Ref: japaconnect-engineering/sections/33-professional-profiles.md (profile admin dashboard)
	// Profile admin view: edit profile fields (PATCH /profiles/{id}), manage
	// members (list + remove + change role), invite members, revoke invitations,
	// manage relocations, documents, and verifiers, and delete the profile.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { UrlTabs, type UrlTab } from '$lib/components/ui';
	import { Briefcase, Save, Trash2, Mail, Check, X, FileText, MapPin, ShieldCheck, Send, UserPlus, Plus } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll, goto } from '$app/navigation';
	import type {
		ProfessionalProfile,
		ProfessionalProfileMember,
		ProfessionalProfileInvitation,
		ProfessionalRelocation,
		ProfessionalDocument,
		ProfessionalVerifier
	} from '$lib/types';

	let { data } = $props();
	const profile = $derived(data.profile);
	const invitations = $derived(data.invitations as ProfessionalProfileInvitation[]);
	const relocations = $derived(data.relocations as ProfessionalRelocation[]);
	const documents = $derived(data.documents as ProfessionalDocument[]);
	const verifiers = $derived(data.verifiers as ProfessionalVerifier[]);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'settings'));

	const tabs: UrlTab[] = [
		{ value: 'settings', label: t('profiles.detail.tab_settings') },
		{ value: 'members', label: t('profiles.detail.tab_members') },
		{ value: 'relocations', label: t('profiles.detail.tab_relocations') },
		{ value: 'documents', label: t('profiles.detail.tab_documents') },
		{ value: 'verifiers', label: t('profiles.detail.tab_verifiers') }
	];

	let name = $state(untrack(() => data.profile?.name ?? ''));
	let tagline = $state(untrack(() => data.profile?.tagline ?? ''));
	let description = $state(untrack(() => data.profile?.description ?? ''));
	let websiteUrl = $state(untrack(() => data.profile?.website_url ?? ''));

	let saving = $state(false);
	let savedKey = $state('');
	let errorKey = $state('');

	let members = $state<ProfessionalProfileMember[]>(untrack(() => data.members ?? []));

	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);
	let inviteErrorKey = $state('');

	let deleting = $state(false);

	// Relocation form state
	let relocCountry = $state('');
	let relocCity = $state('');
	let relocStatus = $state('planned');
	let relocYear = $state('');
	let addingReloc = $state(false);

	// Document form state
	let docTitle = $state('');
	let docType = $state('');
	let docUrl = $state('');
	let addingDoc = $state(false);

	// Verifier form state
	let verifierUserId = $state('');
	let verifierRole = $state('');
	let addingVerifier = $state(false);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	async function saveProfile(e: SubmitEvent) {
		e.preventDefault();
		if (!profile) return;
		errorKey = '';
		saving = true;
		try {
			const res = await api.patch(`/profiles/${profile.id}`, {
				name,
				tagline: tagline || null,
				description: description || null,
				website_url: websiteUrl || null
			});
			if (res.error) {
				errorKey = 'profiles.detail.error_save_failed';
			} else {
				savedKey = 'profiles.detail.saved';
				setTimeout(() => (savedKey = ''), 2500);
				await invalidateAll();
			}
		} catch {
			errorKey = 'profiles.detail.error_save_failed';
		} finally {
			saving = false;
		}
	}

	async function removeMember(memberUserId: string) {
		if (!profile) return;
		if (!confirm(t('profiles.detail.remove_member_confirm'))) return;
		try {
			const res = await api.delete(`/profiles/${profile.id}/members/${memberUserId}`);
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}

	async function changeRole(memberUserId: string, role: string) {
		if (!profile) return;
		try {
			const res = await api.patch(`/profiles/${profile.id}/members/${memberUserId}`, { role });
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}

	async function inviteMember(e: SubmitEvent) {
		e.preventDefault();
		if (!profile || !inviteEmail.trim()) return;
		inviteErrorKey = '';
		inviting = true;
		try {
			const res = await api.post(`/profiles/${profile.id}/invitations`, {
				email: inviteEmail.trim(),
				role: inviteRole
			});
			if (res.error) {
				inviteErrorKey = 'profiles.detail.invite_failed';
			} else {
				inviteEmail = '';
				inviteRole = 'member';
				await invalidateAll();
			}
		} catch {
			inviteErrorKey = 'profiles.detail.invite_failed';
		} finally {
			inviting = false;
		}
	}

	async function revokeInvitation(invitationId: string) {
		if (!profile) return;
		if (!confirm(t('profiles.detail.revoke_invite_confirm'))) return;
		try {
			const res = await api.delete(`/profiles/${profile.id}/invitations/${invitationId}`);
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}

	async function deleteProfile() {
		if (!profile) return;
		if (!confirm(t('profiles.detail.delete_confirm'))) return;
		deleting = true;
		try {
			const res = await api.delete(`/profiles/${profile.id}`);
			if (!res.error) await goto('/dashboard/profiles');
		} catch { /* best-effort */ } finally {
			deleting = false;
		}
	}

	async function addRelocation(e: SubmitEvent) {
		e.preventDefault();
		if (!profile || !relocCountry.trim()) return;
		addingReloc = true;
		try {
			const payload: Record<string, unknown> = { country: relocCountry.trim() };
			if (relocCity.trim()) payload.city = relocCity.trim();
			if (relocStatus) payload.status = relocStatus;
			if (relocYear) payload.year = parseInt(relocYear, 10);
			const res = await api.post(`/profiles/${profile.id}/relocations`, payload);
			if (!res.error) {
				relocCountry = '';
				relocCity = '';
				relocStatus = 'planned';
				relocYear = '';
				await invalidateAll();
			}
		} catch { /* best-effort */ } finally {
			addingReloc = false;
		}
	}

	async function deleteRelocation(relocId: string) {
		if (!profile) return;
		try {
			const res = await api.delete(`/profiles/relocations/${relocId}`);
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}

	async function addDocument(e: SubmitEvent) {
		e.preventDefault();
		if (!profile || !docTitle.trim()) return;
		addingDoc = true;
		try {
			const payload: Record<string, unknown> = { title: docTitle.trim(), document_type: docType || 'other' };
			if (docUrl.trim()) payload.file_url = docUrl.trim();
			const res = await api.post(`/profiles/${profile.id}/documents`, payload);
			if (!res.error) {
				docTitle = '';
				docType = '';
				docUrl = '';
				await invalidateAll();
			}
		} catch { /* best-effort */ } finally {
			addingDoc = false;
		}
	}

	async function deleteDocument(docId: string) {
		if (!profile) return;
		try {
			const res = await api.delete(`/profiles/documents/${docId}`);
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}

	async function addVerifier(e: SubmitEvent) {
		e.preventDefault();
		if (!profile || !verifierUserId.trim()) return;
		addingVerifier = true;
		try {
			const payload: Record<string, unknown> = { user_id: verifierUserId.trim() };
			if (verifierRole) payload.role = verifierRole;
			const res = await api.post(`/profiles/${profile.id}/verifiers`, payload);
			if (!res.error) {
				verifierUserId = '';
				verifierRole = '';
				await invalidateAll();
			}
		} catch { /* best-effort */ } finally {
			addingVerifier = false;
		}
	}

	async function deleteVerifier(verifierId: string) {
		if (!profile) return;
		try {
			const res = await api.delete(`/profiles/verifiers/${verifierId}`);
			if (!res.error) await invalidateAll();
		} catch { /* best-effort */ }
	}
</script>

<section class="space-y-6">
	<header>
		<a href="/dashboard/profiles" class="text-sm text-[var(--color-muted)] hover:underline">
			{t('profiles.detail.back')}
		</a>
		<div class="mt-2 flex items-center gap-3">
			<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Briefcase class="h-5 w-5" />
			</div>
			<div class="flex-1">
				<h1 class="text-lg font-medium text-[var(--color-text)]">{profile?.name}</h1>
				<p class="text-xs text-[var(--color-muted)]">/{profile?.slug}</p>
			</div>
		</div>
	</header>

	<UrlTabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'members'}
		<!-- Invite member -->
		<form onsubmit={inviteMember} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<UserPlus class="h-4 w-4" />
				{t('profiles.detail.invite_section')}
			</h2>
			{#if inviteErrorKey}
				<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(inviteErrorKey)}</p>
			{/if}
			<div class="flex flex-col gap-3 sm:flex-row sm:items-end">
				<label class="flex flex-1 flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.invite_email')}</span>
					<input class={inputClass} type="email" bind:value={inviteEmail} placeholder="member@example.com" required />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.invite_role')}</span>
					<select class={inputClass} bind:value={inviteRole}>
						<option value="owner">Owner</option>
						<option value="admin">Admin</option>
						<option value="editor">Editor</option>
						<option value="member">Member</option>
					</select>
				</label>
				<button
					type="submit"
					disabled={inviting || !inviteEmail.trim()}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					<Send class="h-4 w-4" />
					{t('profiles.detail.invite_send')}
				</button>
			</div>
		</form>

		<!-- Members management -->
		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.members_section')} ({members.length})</h2>
			{#if members.length === 0}
				<EmptyState icon={Briefcase} title={t('profiles.detail.no_members')} />
			{:else}
				<ul class="space-y-2">
					{#each members as m (m.user_id)}
						<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{m.username}</p>
								<p class="truncate text-xs text-[var(--color-muted)]">{m.email}</p>
							</div>
							<div class="flex shrink-0 items-center gap-2">
								<select
									value={m.role}
									onchange={(e) => changeRole(m.user_id, e.currentTarget.value)}
									class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)]"
								>
									<option value="owner">Owner</option>
									<option value="admin">Admin</option>
									<option value="editor">Editor</option>
									<option value="member">Member</option>
								</select>
								<button
									type="button"
									onclick={() => removeMember(m.user_id)}
									class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]"
									aria-label={t('profiles.detail.remove_member')}
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
					{t('profiles.detail.invitations_section')} ({invitations.length})
				</h2>
				<ul class="space-y-2">
					{#each invitations as inv (inv.id)}
						<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{inv.email}</p>
								<p class="text-xs text-[var(--color-muted)]">
									{inv.role} · {inv.accepted_at ? 'accepted' : t('profiles.detail.invite_pending')}
								</p>
							</div>
							{#if !inv.accepted_at}
								<button
									type="button"
									onclick={() => revokeInvitation(inv.id)}
									class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]"
									aria-label={t('profiles.detail.revoke_invite')}
								>
									<X class="h-4 w-4" />
								</button>
							{:else}
								<Check class="h-4 w-4 shrink-0 text-[var(--color-success)]" />
							{/if}
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	{:else if activeTab === 'relocations'}
		<form onsubmit={addRelocation} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<MapPin class="h-4 w-4" />
				{t('profiles.detail.relocation_add')}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-4">
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.relocation_country')}</span>
					<input class={inputClass} bind:value={relocCountry} placeholder="Japan" required />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.relocation_city')}</span>
					<input class={inputClass} bind:value={relocCity} placeholder="Tokyo" />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.relocation_status')}</span>
					<select class={inputClass} bind:value={relocStatus}>
						<option value="planned">Planned</option>
						<option value="in_progress">In progress</option>
						<option value="completed">Completed</option>
					</select>
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.relocation_year')}</span>
					<input class={inputClass} type="number" bind:value={relocYear} placeholder="2025" />
				</label>
			</div>
			<div class="flex justify-end">
				<button type="submit" disabled={addingReloc} class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50">
					<Plus class="h-4 w-4" />
					{t('profiles.detail.relocation_add')}
				</button>
			</div>
		</form>

		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.relocations_section')} ({relocations.length})</h2>
			{#if relocations.length === 0}
				<EmptyState icon={MapPin} title={t('profiles.detail.no_relocations')} />
			{:else}
				<ul class="space-y-2">
					{#each relocations as r (r.id)}
						<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							<div class="min-w-0">
								<p class="text-sm font-medium text-[var(--color-text)]">{r.country}{#if r.city}, {r.city}{/if}</p>
								<p class="text-xs text-[var(--color-muted)]">{r.status ?? '—'}{#if r.year} · {r.year}{/if}</p>
							</div>
							<button type="button" onclick={() => deleteRelocation(r.id)} class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]">
								<Trash2 class="h-4 w-4" />
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{:else if activeTab === 'documents'}
		<form onsubmit={addDocument} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<FileText class="h-4 w-4" />
				{t('profiles.detail.document_add')}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.document_title')}</span>
					<input class={inputClass} bind:value={docTitle} required />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.document_type')}</span>
					<select class={inputClass} bind:value={docType}>
						<option value="license">License</option>
						<option value="certificate">Certificate</option>
						<option value="registration">Registration</option>
						<option value="other">Other</option>
					</select>
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">File URL</span>
					<input class={inputClass} type="url" bind:value={docUrl} placeholder="https://..." />
				</label>
			</div>
			<div class="flex justify-end">
				<button type="submit" disabled={addingDoc} class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50">
					{t('profiles.detail.document_add')}
				</button>
			</div>
		</form>

		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.documents_section')} ({documents.length})</h2>
			{#if documents.length === 0}
				<EmptyState icon={FileText} title={t('profiles.detail.no_documents')} />
			{:else}
				<ul class="space-y-2">
					{#each documents as d (d.id)}
						<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{d.title}</p>
								<p class="text-xs text-[var(--color-muted)]">{d.document_type}{#if d.verified} · ✓{/if}</p>
							</div>
							<button type="button" onclick={() => deleteDocument(d.id)} class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]">
								<Trash2 class="h-4 w-4" />
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{:else if activeTab === 'verifiers'}
		<form onsubmit={addVerifier} class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<ShieldCheck class="h-4 w-4" />
				{t('profiles.detail.verifier_add')}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">User ID</span>
					<input class={inputClass} bind:value={verifierUserId} placeholder="UUID" required />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">Role</span>
					<input class={inputClass} bind:value={verifierRole} placeholder="verifier" />
				</label>
			</div>
			<div class="flex justify-end">
				<button type="submit" disabled={addingVerifier} class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50">
					{t('profiles.detail.verifier_add')}
				</button>
			</div>
		</form>

		<div class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.verifiers_section')} ({verifiers.length})</h2>
			{#if verifiers.length === 0}
				<EmptyState icon={ShieldCheck} title={t('profiles.detail.no_verifiers')} />
			{:else}
				<ul class="space-y-2">
					{#each verifiers as v (v.id)}
						<li class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{v.user_name}</p>
								<p class="text-xs text-[var(--color-muted)]">{v.role ?? 'verifier'} · {v.status ?? 'pending'}</p>
							</div>
							<button type="button" onclick={() => deleteVerifier(v.id)} class="rounded p-1 text-[var(--color-muted)] hover:bg-[var(--color-danger)]/10 hover:text-[var(--color-danger)]">
								<Trash2 class="h-4 w-4" />
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{:else}
		<!-- Settings tab -->
		<form onsubmit={saveProfile} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.edit_section')}</h2>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
			{/if}
			{#if savedKey}
				<p class="rounded-[var(--radius)] bg-[var(--color-success)]/10 px-3 py-2 text-sm text-[var(--color-success)]" role="status">{t(savedKey)}</p>
			{/if}

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.field_name')}</span>
				<input class={inputClass} value={name} oninput={(e) => (name = e.currentTarget.value)} />
			</label>

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.field_tagline')}</span>
				<input class={inputClass} value={tagline} oninput={(e) => (tagline = e.currentTarget.value)} />
			</label>

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.field_description')}</span>
				<textarea class={inputClass} rows={4} value={description} oninput={(e) => (description = e.currentTarget.value)}></textarea>
			</label>

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('profiles.detail.field_website')}</span>
				<input class={inputClass} value={websiteUrl} oninput={(e) => (websiteUrl = e.currentTarget.value)} placeholder="https://" />
			</label>

			<div class="flex items-center justify-between">
				<button
					type="button"
					onclick={deleteProfile}
					disabled={deleting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-danger)]/30 px-4 py-2 text-sm font-medium text-[var(--color-danger)] transition hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
				>
					<Trash2 class="h-4 w-4" />
					{t('profiles.detail.delete')}
				</button>
				<button
					type="submit"
					disabled={saving}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					<Save class="h-4 w-4" />
					{t('profiles.detail.save')}
				</button>
			</div>
		</form>
	{/if}
</section>
