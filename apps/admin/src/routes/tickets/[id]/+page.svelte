<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { ticketsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	// Ref: japaconnect-engineering/sections/40-support-tickets.md — admin ticket
	// detail with event timeline, internal notes, public comments, and resolve.
	import {
		ArrowLeft, RefreshCw, Pencil, Trash2, CheckCircle, Clock,
		AlertCircle, MessageSquare, StickyNote, Send, Ticket as TicketIcon
	} from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type TicketItem = {
		id: string;
		ticket_number?: number;
		subject: string;
		type_name: string;
		type_slug: string;
		type_id: string;
		status: string;
		priority: string;
		assigned_to?: string;
		assigned_group_id?: string;
		submitter_user_id?: string;
		submitter_email?: string;
		description: string;
		resolution_note?: string;
		created_at: string;
		updated_at: string;
	};

	type TicketEvent = {
		id: string;
		event_type: string;
		body?: string;
		actor?: string;
		created_at: string;
	};

	const ticketId = $derived(page.params.id ?? '');
	// Per-action RBAC gates matching the backend route middleware.
	// Ref: japaconnect-engineering/sections/38-ticket-system.md
	const canViewAllTickets = $derived(hasPermission('tickets.view_all'));
	const canManageTickets = $derived(hasPermission('tickets.manage'));
	const canDeleteTickets = $derived(hasPermission('tickets.admin'));
	const canCommentInternal = $derived(hasPermission('tickets.comment.internal'));

	let ticket = $state<TicketItem | null>(null);
	let events = $state<TicketEvent[]>([]);
	let loading = $state(true);
	let error = $state('');
	let success = $state('');

	let newComment = $state('');
	let newNote = $state('');
	let addingComment = $state(false);
	let addingNote = $state(false);

	// Edit modal
	let showEditModal = $state(false);
	let editForm = $state({ subject: '', status: '', priority: '', assigned_to: '' });
	let savingEdit = $state(false);

	// Resolve modal
	let showResolveModal = $state(false);
	let resolutionNote = $state('');
	let resolving = $state(false);

	// Delete modal
	let showDeleteConfirm = $state(false);
	let deleting = $state(false);

	const statusBadgeVariant = (status: string): 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'info' | 'outline' => {
		if (status === 'resolved') return 'success';
		if (status === 'open') return 'info';
		if (status === 'in_progress') return 'warning';
		if (status === 'closed') return 'secondary';
		return 'default';
	};

	const priorityBadgeVariant = (priority: string): 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'info' | 'outline' => {
		if (priority === 'urgent') return 'destructive';
		if (priority === 'high') return 'warning';
		if (priority === 'normal') return 'info';
		if (priority === 'low') return 'secondary';
		return 'default';
	};

	function flash(msg: string, isError = false) {
		success = isError ? '' : msg;
		error = isError ? msg : '';
		setTimeout(() => {
			success = '';
			error = '';
		}, 3000);
	}

	async function loadTicket() {
		loading = true;
		error = '';
		try {
			const [ticketRes, eventsRes] = await Promise.all([
				ticketsAdminApi.get(ticketId),
				ticketsAdminApi.getEvents(ticketId),
			]);
			if (ticketRes.ok) {
				const detail = ticketRes.data as { ticket: TicketItem; comments: any[]; attachments: any[]; events: any[] };
				ticket = detail?.ticket ?? (ticketRes.data as TicketItem);
			} else {
				error = t('tickets.detail.load_error', 'Failed to load ticket');
			}
			if (eventsRes.ok) {
				const ed = eventsRes.data as any;
				events = ed?.events ?? ed?.data ?? (Array.isArray(ed) ? ed : []) ?? [];
			}
		} catch {
			error = t('tickets.detail.load_error', 'Failed to load ticket');
		} finally {
			loading = false;
		}
	}

	async function reloadEvents() {
		const eventsRes = await ticketsAdminApi.getEvents(ticketId);
		if (eventsRes.ok) {
			const ed = eventsRes.data as any;
			events = ed?.events ?? ed?.data ?? (Array.isArray(ed) ? ed : []) ?? [];
		}
	}

	async function submitComment() {
		if (!newComment.trim()) return;
		addingComment = true;
		try {
			const result = await ticketsAdminApi.addComment(ticketId, { body: newComment.trim() });
			if (result.ok) {
				newComment = '';
				await reloadEvents();
				flash(t('tickets.detail.comment_added', 'Comment added'));
			} else {
				flash(t('tickets.detail.comment_error', 'Failed to add comment'), true);
			}
		} catch {
			flash(t('tickets.detail.comment_error', 'Failed to add comment'), true);
		} finally {
			addingComment = false;
		}
	}

	async function submitNote() {
		if (!newNote.trim()) return;
		addingNote = true;
		try {
			const result = await ticketsAdminApi.addNote(ticketId, { body: newNote.trim() });
			if (result.ok) {
				newNote = '';
				await reloadEvents();
				flash(t('tickets.detail.note_added', 'Note added'));
			} else {
				flash(t('tickets.detail.note_error', 'Failed to add note'), true);
			}
		} catch {
			flash(t('tickets.detail.note_error', 'Failed to add note'), true);
		} finally {
			addingNote = false;
		}
	}

	function openEdit() {
		if (!ticket) return;
		editForm = {
			subject: ticket.subject,
			status: ticket.status,
			priority: ticket.priority,
			assigned_to: ticket.assigned_to ?? ''
		};
		showEditModal = true;
	}

	async function submitEdit() {
		savingEdit = true;
		try {
			const result = await ticketsAdminApi.update(ticketId, {
				subject: editForm.subject,
				status: editForm.status,
				priority: editForm.priority,
				assigned_to: editForm.assigned_to
			});
			if (result.ok) {
				showEditModal = false;
				flash(t('tickets.detail.updated', 'Ticket updated'));
				await loadTicket();
			} else {
				flash(result.error ?? t('tickets.detail.update_error', 'Failed to update ticket'), true);
			}
		} catch {
			flash(t('tickets.detail.update_error', 'Failed to update ticket'), true);
		} finally {
			savingEdit = false;
		}
	}

	async function submitResolve() {
		resolving = true;
		try {
			const result = await ticketsAdminApi.resolve(ticketId, { resolution_note: resolutionNote.trim() });
			if (result.ok) {
				showResolveModal = false;
				resolutionNote = '';
				flash(t('tickets.detail.resolved', 'Ticket resolved'));
				await loadTicket();
			} else {
				flash(t('tickets.detail.resolve_error', 'Failed to resolve ticket'), true);
			}
		} catch {
			flash(t('tickets.detail.resolve_error', 'Failed to resolve ticket'), true);
		} finally {
			resolving = false;
		}
	}

	async function executeDelete() {
		deleting = true;
		try {
			const result = await ticketsAdminApi.delete(ticketId);
			if (result.ok) {
				showDeleteConfirm = false;
				goto('/tickets');
			} else {
				flash(t('tickets.detail.delete_error', 'Failed to delete ticket'), true);
			}
		} catch {
			flash(t('tickets.detail.delete_error', 'Failed to delete ticket'), true);
		} finally {
			deleting = false;
		}
	}

	function eventIcon(type: string) {
		if (type === 'comment') return MessageSquare;
		if (type === 'note') return StickyNote;
		if (type === 'status_change') return RefreshCw;
		if (type === 'resolve') return CheckCircle;
		return TicketIcon;
	}

	$effect(() => {
		if (ticketId) loadTicket();
	});
</script>

<PageHeader title={ticket ? `${ticket.subject} (${ticket.ticket_number ? `JC-${ticket.ticket_number}` : `#${ticket.id.slice(0, 8)}`})` : t('tickets.detail.title', 'Ticket Detail')} />

{#if error}
	<div class="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">{error}</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">{success}</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<Button variant="outline" size="sm" onclick={() => goto('/tickets')}>
		<ArrowLeft class="me-1.5 h-3.5 w-3.5 rtl:-scale-x-100" />
		{t('tickets.detail.back', 'Back to Tickets')}
	</Button>
	<Button variant="outline" size="sm" onclick={loadTicket} disabled={loading}>
		<RefreshCw class="me-1.5 h-3.5 w-3.5" />
		{t('common.refresh', 'Refresh')}
	</Button>
	{#if (canManageTickets || canDeleteTickets || canCommentInternal) && ticket}
		<div class="ms-auto flex gap-2">
{#if canManageTickets}
				<Button variant="outline" size="sm" onclick={openEdit}>
					<Pencil class="me-1.5 h-3.5 w-3.5" />
					{t('common.edit', 'Edit')}
				</Button>
			{/if}
			{#if ticket.status !== 'resolved' && ticket.status !== 'closed'}
				<Button variant="outline" size="sm" onclick={() => (showResolveModal = true)}>
					<CheckCircle class="me-1.5 h-3.5 w-3.5" />
					{t('tickets.detail.resolve', 'Resolve')}
				</Button>
			{/if}
{#if canDeleteTickets}
				<Button variant="destructive" size="sm" onclick={() => (showDeleteConfirm = true)}>
					<Trash2 class="me-1.5 h-3.5 w-3.5" />
					{t('common.delete', 'Delete')}
				</Button>
			{/if}
		</div>
	{/if}
</div>

{#if loading}
	<TableSkeleton rows={6} />
{:else if ticket}
	<div class="grid gap-4 lg:grid-cols-3">
		<!-- Ticket details + timeline (2 cols) -->
		<div class="lg:col-span-2 space-y-4">
			<Card>
				<CardHeader>
					<h2 class="text-lg font-semibold">{t('tickets.detail.details', 'Details')}</h2>
				</CardHeader>
				<CardContent>
					<dl class="grid grid-cols-2 gap-4 text-sm">
						<div>
							<dt class="text-muted-foreground">{t('tickets.subject', 'Subject')}</dt>
							<dd class="font-medium">{ticket.subject}</dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('tickets.type', 'Type')}</dt>
							<dd class="font-medium">{ticket.type_name}</dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('common.status', 'Status')}</dt>
							<dd><Badge variant={statusBadgeVariant(ticket.status)}>{ticket.status}</Badge></dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('tickets.priority', 'Priority')}</dt>
							<dd><Badge variant={priorityBadgeVariant(ticket.priority)}>{ticket.priority}</Badge></dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('tickets.assignee', 'Assignee')}</dt>
							<dd class="font-medium">{ticket.assigned_to || '—'}</dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('tickets.detail.submitter', 'Submitter')}</dt>
							<dd class="font-medium">{ticket.submitter_email || ticket.submitter_user_id || '—'}</dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('common.created', 'Created')}</dt>
							<dd class="font-medium">{new Date(ticket.created_at).toLocaleString()}</dd>
						</div>
						<div>
							<dt class="text-muted-foreground">{t('common.updated', 'Updated')}</dt>
							<dd class="font-medium">{new Date(ticket.updated_at).toLocaleString()}</dd>
						</div>
					</dl>
					{#if ticket.description}
						<div class="mt-4">
							<dt class="text-muted-foreground text-sm">{t('tickets.detail.description', 'Description')}</dt>
							<dd class="mt-1 rounded-lg border border-border p-3 text-sm">{ticket.description}</dd>
						</div>
					{/if}
					{#if ticket.resolution_note}
						<div class="mt-4">
							<dt class="text-muted-foreground text-sm">{t('tickets.detail.resolution', 'Resolution')}</dt>
							<dd class="mt-1 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3 text-sm">{ticket.resolution_note}</dd>
						</div>
					{/if}
				</CardContent>
			</Card>

			<!-- Event timeline -->
			<Card>
				<CardHeader>
					<h2 class="text-lg font-semibold">{t('tickets.detail.timeline', 'Event Timeline')}</h2>
				</CardHeader>
				<CardContent>
					{#if events.length === 0}
						<p class="text-muted-foreground py-6 text-center text-sm">{t('tickets.detail.no_events', 'No events yet.')}</p>
					{:else}
						<ol class="relative space-y-4 border-s border-border ps-6">
							{#each events as ev (ev)}
								{@const EvIcon = eventIcon(ev.event_type)}
								<li class="relative">
									<span class="absolute -start-[31px] flex h-6 w-6 items-center justify-center rounded-full border border-border bg-background">
										<EvIcon class="h-3 w-3 text-muted-foreground" />
									</span>
									<div class="flex items-center gap-2">
										<Badge variant="secondary">{ev.event_type}</Badge>
										{#if ev.actor}
											<span class="text-xs text-muted-foreground">{ev.actor}</span>
										{/if}
										<span class="text-xs text-muted-foreground">{new Date(ev.created_at).toLocaleString()}</span>
									</div>
									{#if ev.body}
										<p class="mt-1 text-sm">{ev.body}</p>
									{/if}
								</li>
							{/each}
						</ol>
					{/if}
				</CardContent>
			</Card>
		</div>

		<!-- Actions sidebar (1 col) -->
		<div class="space-y-4">
			<!-- Add comment -->
			<Card>
				<CardHeader>
					<h3 class="text-sm font-semibold">
						<MessageSquare class="me-1.5 inline h-4 w-4" />
						{t('tickets.detail.add_comment', 'Add Comment')}
					</h3>
				</CardHeader>
				<CardContent>
					<textarea
						bind:value={newComment}
						rows="3"
						class="w-full rounded-lg border border-border bg-background p-2 text-sm"
						placeholder={t('tickets.detail.comment_placeholder', 'Write a public comment...')}
					></textarea>
					<Button class="mt-2 w-full" size="sm" onclick={submitComment} disabled={addingComment || !newComment.trim()}>
						<Send class="me-1.5 h-3.5 w-3.5 rtl:-scale-x-100" />
						{addingComment ? t('common.saving', 'Sending...') : t('tickets.detail.send_comment', 'Send Comment')}
					</Button>
				</CardContent>
			</Card>

			<!-- Add internal note -->
			<Card>
				<CardHeader>
					<h3 class="text-sm font-semibold">
						<StickyNote class="me-1.5 inline h-4 w-4" />
						{t('tickets.detail.add_note', 'Add Internal Note')}
					</h3>
				</CardHeader>
				<CardContent>
					<textarea
						bind:value={newNote}
						rows="3"
						class="w-full rounded-lg border border-border bg-background p-2 text-sm"
						placeholder={t('tickets.detail.note_placeholder', 'Internal note (not visible to submitter)...')}
					></textarea>
					<Button class="mt-2 w-full" size="sm" variant="outline" onclick={submitNote} disabled={addingNote || !newNote.trim()}>
						<Send class="me-1.5 h-3.5 w-3.5 rtl:-scale-x-100" />
						{addingNote ? t('common.saving', 'Saving...') : t('tickets.detail.save_note', 'Save Note')}
					</Button>
				</CardContent>
			</Card>
		</div>
	</div>
{/if}

<!-- Edit modal -->
<Modal bind:open={showEditModal} title={t('tickets.detail.edit_title', 'Edit Ticket')}>
	<div class="space-y-4">
		<div>
			<label class="text-sm font-medium" for="edit-subject">{t('tickets.subject', 'Subject')}
			<HintIcon text={t('tickets.edit_subject_hint', 'The subject the user sees.')}  />
</label>
			<input id="edit-subject" bind:value={editForm.subject} class="mt-1 w-full rounded-lg border border-border bg-background p-2 text-sm" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label class="text-sm font-medium" for="edit-status">{t('common.status', 'Status')}
				<HintIcon text={t('tickets.edit_status_hint', 'The ticket state.')}  />
</label>
				<AutocompleteSelect
					id="edit-status"
					bind:value={editForm.status}
					options={[{ value: 'open', label: 'open' }, { value: 'in_progress', label: 'in_progress' }, { value: 'waiting_on_user', label: 'waiting_on_user' }, { value: 'resolved', label: 'resolved' }, { value: 'closed', label: 'closed' }, { value: 'spam', label: 'spam' }]}
					class="w-full"
				/>
			</div>
			<div>
				<label class="text-sm font-medium" for="edit-priority">{t('tickets.priority', 'Priority')}
				<HintIcon text={t('tickets.edit_priority_hint', 'Higher priority tickets surface first.')}  />
</label>
				<AutocompleteSelect
					id="edit-priority"
					bind:value={editForm.priority}
					options={[{ value: 'low', label: 'low' }, { value: 'normal', label: 'normal' }, { value: 'high', label: 'high' }, { value: 'urgent', label: 'urgent' }]}
					class="w-full"
				/>
			</div>
		</div>
		<div>
			<label class="text-sm font-medium" for="edit-assignee">{t('tickets.assignee', 'Assignee')}
			<HintIcon text={t('tickets.edit_assignee_hint', 'The admin handling this ticket.')}  />
</label>
			<input id="edit-assignee" bind:value={editForm.assigned_to} class="mt-1 w-full rounded-lg border border-border bg-background p-2 text-sm" placeholder={t('tickets.detail.assignee_placeholder', 'User ID or username')} />
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showEditModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={submitEdit} disabled={savingEdit}>
			{savingEdit ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- Resolve modal -->
<Modal bind:open={showResolveModal} title={t('tickets.detail.resolve_title', 'Resolve Ticket')}>
	<p class="text-sm text-muted-foreground">{t('tickets.detail.resolve_desc', 'Add a resolution note and mark this ticket as resolved.')}</p>
	<textarea
		bind:value={resolutionNote}
		rows="4"
		class="mt-4 w-full rounded-lg border border-border bg-background p-2 text-sm"
		placeholder={t('tickets.detail.resolution_placeholder', 'Describe how the ticket was resolved...')}
	></textarea>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showResolveModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={submitResolve} disabled={resolving}>
			<CheckCircle class="me-1.5 h-4 w-4" />
			{resolving ? t('common.saving', 'Saving...') : t('tickets.detail.resolve', 'Resolve')}
		</Button>
	</div>
</Modal>

<!-- Delete confirm -->
<Modal bind:open={showDeleteConfirm} title={t('tickets.detail.delete_title', 'Delete Ticket')}>
	<p class="text-sm text-muted-foreground">
		{t('tickets.detail.delete_confirm', 'Are you sure you want to delete this ticket? This action cannot be undone.')}
	</p>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={deleting}>
			<Trash2 class="me-1.5 h-4 w-4" />
			{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
