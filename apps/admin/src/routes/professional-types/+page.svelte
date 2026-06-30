<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { professionalAdminApi, type ProfileType, type ProfileTypeDetail, type ProfileMember, type ProfileInvitation } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Briefcase, ChevronDown, ChevronRight, Users, Mail, Settings2 } from 'lucide-svelte';

	let types = $state<ProfileType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showModal = $state(false);
	let saving = $state(false);

	let form = $state({
		name: '',
		slug: '',
		description: ''
	});

	// Expanded detail state
	let expandedId = $state<string | null>(null);
	let detail = $state<ProfileTypeDetail | null>(null);
	let detailLoading = $state(false);

	// Members
	let members = $state<ProfileMember[]>([]);
	let membersLoading = $state(false);
	let addMemberForm = $state({ user_id: '', role: 'member' });
	let addMemberSaving = $state(false);
	let editRoleUserId = $state<string | null>(null);
	let editRoleValue = $state('');

	// Invitations
	let invitations = $state<ProfileInvitation[]>([]);
	let invitationsLoading = $state(false);
	let invitationForm = $state({ email: '', role: 'member' });
	let invitationSaving = $state(false);

	const memberRoles = ['owner', 'admin', 'member', 'viewer'];

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const result = await professionalAdminApi.listTypes();
		if (result.ok) {
			types = result.data ?? [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	async function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			detail = null;
			members = [];
			invitations = [];
			return;
		}
		expandedId = id;
		detailLoading = true;
		detail = null;
		members = [];
		invitations = [];

		const result = await professionalAdminApi.getType(id);
		if (result.ok) {
			detail = result.data;
		}
		detailLoading = false;

		await loadMembers(id);
		await loadInvitations(id);
	}

	async function loadMembers(profileId: string) {
		membersLoading = true;
		const result = await professionalAdminApi.listMembers(profileId);
		if (result.ok) {
			members = result.data ?? [];
		}
		membersLoading = false;
	}

	async function loadInvitations(profileId: string) {
		invitationsLoading = true;
		const result = await professionalAdminApi.listInvitations(profileId);
		if (result.ok) {
			invitations = result.data ?? [];
		}
		invitationsLoading = false;
	}

	function openCreate() {
		form = { name: '', slug: '', description: '' };
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const result = await professionalAdminApi.createType({
			name: form.name.trim(),
			slug,
			description: form.description.trim()
		});

		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	async function addMember() {
		if (!expandedId || !addMemberForm.user_id.trim()) return;
		addMemberSaving = true;
		const result = await professionalAdminApi.addMember(expandedId, {
			user_id: addMemberForm.user_id.trim(),
			role: addMemberForm.role
		});
		if (result.ok) {
			addMemberForm = { user_id: '', role: 'member' };
			await loadMembers(expandedId);
		} else {
			errorMessage = result.error;
		}
		addMemberSaving = false;
	}

	async function removeMember(userId: string) {
		if (!expandedId) return;
		const result = await professionalAdminApi.removeMember(expandedId, userId);
		if (result.ok) {
			await loadMembers(expandedId);
		} else {
			errorMessage = result.error;
		}
	}

	async function startEditRole(userId: string, currentRole: string) {
		editRoleUserId = userId;
		editRoleValue = currentRole;
	}

	async function saveRole() {
		if (!expandedId || !editRoleUserId) return;
		const result = await professionalAdminApi.updateMember(expandedId, editRoleUserId, { role: editRoleValue });
		if (result.ok) {
			editRoleUserId = null;
			await loadMembers(expandedId);
		} else {
			errorMessage = result.error;
		}
	}

	async function sendInvitation() {
		if (!expandedId || !invitationForm.email.trim()) return;
		invitationSaving = true;
		const result = await professionalAdminApi.sendInvitation(expandedId, {
			email: invitationForm.email.trim(),
			role: invitationForm.role
		});
		if (result.ok) {
			invitationForm = { email: '', role: 'member' };
			await loadInvitations(expandedId);
		} else {
			errorMessage = result.error;
		}
		invitationSaving = false;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('professional_types.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('professional_types.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('professional_types.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('professional_types.add')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('professional_types.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('professional_types.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('professional_types.description')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('professional_types.profiles')}</th>
				</tr>
			</thead>
			<tbody>
				{#each types as item}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<button
								class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10"
								onclick={() => toggleExpand(item.id)}
								aria-label="Expand details"
							>
								{#if expandedId === item.id}
									<ChevronDown class="h-4 w-4" style="color: var(--color-muted)" />
								{:else}
									<ChevronRight class="h-4 w-4" style="color: var(--color-muted)" />
								{/if}
							</button>
						</td>
						<td class="px-4 py-3 font-medium">{item.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{item.slug}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.description || '—'}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">—</td>
					</tr>
					{#if expandedId === item.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="5" class="px-4 py-4" style="background-color: var(--color-surface)">
								{#if detailLoading}
									<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
								{:else if detail}
									<div class="space-y-6">
										<!-- Custom Fields -->
										<div>
											<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
												<Settings2 class="h-4 w-4" style="color: var(--color-primary)" />
												{t('professional_types.fields')}
											</h3>
											{#if detail.fields.length > 0}
												<div class="space-y-1">
													{#each detail.fields as field}
														<div class="flex items-center gap-2 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
															<span class="font-medium">{field.name}</span>
															<span class="rounded-full bg-slate-500/15 px-2 py-0.5 text-xs" style="color: var(--color-muted)">{field.field_type}</span>
															{#if field.required}
																<span class="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs text-amber-700 dark:text-amber-400">required</span>
															{/if}
														</div>
													{/each}
												</div>
											{:else}
												<p class="text-xs" style="color: var(--color-muted)">{t('professional_types.no_fields')}</p>
											{/if}
										</div>

										<!-- Members -->
										<div>
											<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
												<Users class="h-4 w-4" style="color: var(--color-primary)" />
												{t('professional_types.members')}
											</h3>
											{#if membersLoading}
												<p class="py-2 text-xs" style="color: var(--color-muted)">{t('common.loading')}</p>
											{:else if members.length > 0}
												<div class="overflow-x-auto rounded-md border" style="border-color: var(--color-border)">
													<table class="w-full text-xs">
														<thead>
															<tr style="border-color: var(--color-border)">
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.username')}</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.role')}</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.joined')}</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)"></th>
															</tr>
														</thead>
														<tbody>
															{#each members as member}
																<tr class="border-t" style="border-color: var(--color-border)">
																	<td class="px-3 py-2">
																		<span class="font-medium">{member.display_name || member.username}</span>
																		{#if member.display_name && member.username}
																			<span style="color: var(--color-muted)"> @{member.username}</span>
																		{/if}
																	</td>
																	<td class="px-3 py-2">
																		{#if editRoleUserId === member.user_id}
																			<div class="flex items-center gap-1">
																				<select class="rounded border px-1 py-0.5 text-xs" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={editRoleValue}>
																					{#each memberRoles as r}
																						<option value={r}>{r}</option>
																					{/each}
																				</select>
																				<button class="rounded px-1.5 py-0.5 text-xs text-white" style="background-color: var(--color-primary)" onclick={saveRole}>{t('common.save')}</button>
																				<button class="rounded px-1.5 py-0.5 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => editRoleUserId = null}>{t('common.cancel')}</button>
																			</div>
																		{:else}
																			<span class="rounded-full bg-purple-500/15 px-2 py-0.5 text-xs text-purple-700 dark:text-purple-400">{member.role}</span>
																			<button class="ml-2 rounded px-1.5 py-0.5 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-info)" onclick={() => startEditRole(member.user_id, member.role)}>{t('common.edit')}</button>
																		{/if}
																	</td>
																	<td class="px-3 py-2" style="color: var(--color-muted)">{new Date(member.joined_at).toLocaleDateString()}</td>
																	<td class="px-3 py-2">
																		<button class="rounded px-1.5 py-0.5 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-danger)" onclick={() => removeMember(member.user_id)}>{t('common.delete')}</button>
																	</td>
																</tr>
															{/each}
														</tbody>
													</table>
												</div>
											{:else}
												<p class="text-xs" style="color: var(--color-muted)">{t('professional_types.no_members')}</p>
											{/if}

											<!-- Add Member Form -->
											<div class="mt-3 flex items-end gap-2">
												<div class="flex-1">
													<label for="pt-add-member-user" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.user_id')}</label>
													<input
														id="pt-add-member-user"
														type="text"
														class="w-full rounded-md border px-3 py-1.5 text-xs"
														style="border-color: var(--color-border); background-color: var(--color-background)"
														placeholder="User ID"
														bind:value={addMemberForm.user_id}
													/>
												</div>
												<div>
													<label for="pt-add-member-role" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.role')}</label>
													<select id="pt-add-member-role" class="rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={addMemberForm.role}>
														{#each memberRoles as r}
															<option value={r}>{r}</option>
														{/each}
													</select>
												</div>
												<button
													class="rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors"
													style="background-color: var(--color-primary)"
													onclick={addMember}
													disabled={addMemberSaving}
												>
													{addMemberSaving ? t('common.saving') : t('professional_types.add_member')}
												</button>
											</div>
										</div>

										<!-- Invitations -->
										<div>
											<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
												<Mail class="h-4 w-4" style="color: var(--color-primary)" />
												{t('professional_types.invitations')}
											</h3>
											{#if invitationsLoading}
												<p class="py-2 text-xs" style="color: var(--color-muted)">{t('common.loading')}</p>
											{:else if invitations.length > 0}
												<div class="overflow-x-auto rounded-md border" style="border-color: var(--color-border)">
													<table class="w-full text-xs">
														<thead>
															<tr style="border-color: var(--color-border)">
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">Email</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.role')}</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.status')}</th>
																<th class="px-3 py-2 text-left font-medium" style="color: var(--color-muted)">{t('professional_types.sent')}</th>
															</tr>
														</thead>
														<tbody>
															{#each invitations as inv}
																<tr class="border-t" style="border-color: var(--color-border)">
																	<td class="px-3 py-2 font-mono">{inv.email}</td>
																	<td class="px-3 py-2">
																		<span class="rounded-full bg-purple-500/15 px-2 py-0.5 text-xs text-purple-700 dark:text-purple-400">{inv.role}</span>
																	</td>
																	<td class="px-3 py-2">
																		<span class="rounded-full px-2 py-0.5 text-xs {inv.status === 'pending' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : inv.status === 'accepted' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">{inv.status}</span>
																	</td>
																	<td class="px-3 py-2" style="color: var(--color-muted)">{new Date(inv.created_at).toLocaleDateString()}</td>
																</tr>
															{/each}
														</tbody>
													</table>
												</div>
											{:else}
												<p class="text-xs" style="color: var(--color-muted)">{t('professional_types.no_invitations')}</p>
											{/if}

											<!-- Send Invitation Form -->
											<div class="mt-3 flex items-end gap-2">
												<div class="flex-1">
													<label for="pt-invite-email" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Email</label>
													<input
														id="pt-invite-email"
														type="email"
														class="w-full rounded-md border px-3 py-1.5 text-xs"
														style="border-color: var(--color-border); background-color: var(--color-background)"
														placeholder="user@example.com"
														bind:value={invitationForm.email}
													/>
												</div>
												<div>
													<label for="pt-invite-role" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.role')}</label>
													<select id="pt-invite-role" class="rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={invitationForm.role}>
														{#each memberRoles as r}
															<option value={r}>{r}</option>
														{/each}
													</select>
												</div>
												<button
													class="rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors"
													style="background-color: var(--color-primary)"
													onclick={sendInvitation}
													disabled={invitationSaving}
												>
													{invitationSaving ? t('common.saving') : t('professional_types.send_invitation')}
												</button>
											</div>
										</div>
									</div>
								{:else}
									<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.error')}</p>
								{/if}
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">{t('professional_types.empty')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create Modal -->
<Modal bind:open={showModal} title={t('professional_types.add')} onConfirm={submit} confirmLabel={saving ? t('common.saving') : t('common.create')}>
	<div class="space-y-3">
		<div>
			<label for="pt-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.name')}</label>
			<input id="pt-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.name} oninput={() => { form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="pt-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.slug')}</label>
			<input id="pt-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
		</div>
		<div>
			<label for="pt-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.description')}</label>
			<textarea id="pt-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
	</div>
</Modal>
