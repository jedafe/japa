<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { t, locale } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Select } from '$lib/components/ui/select';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { ArrowUp, ArrowDown, Plus, Trash2, Pencil, Eye } from 'lucide-svelte';
	import { formsAdminApi, fieldGroupsAdminApiExtended } from '$lib/api';
	import { PUBLIC_JC_SITE_URL } from '$env/static/public';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { TableSkeleton } from '$lib/components/ui/skeleton';

	// --- Types ---

	type FormDef = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		is_multi_step: boolean;
		submit_label?: string;
		success_message?: string;
		status: 'active' | 'draft';
		is_system: boolean;
		form_type?: string;
		space_id?: string;
		field_group_id?: string;
		fields?: FieldDef[];
		submission_fee_credits?: number;
		created_at: string;
	};

	type FieldDef = {
		id: string;
		label: string;
		type: string;
		required: boolean;
		sort_order: number;
		placeholder?: string;
		default_value?: string;
		options: string[];
	};

	type FormStep = {
		id: string;
		title: string;
		position: number;
		field_group_id?: string;
		fields?: FieldDef[];
	};

	type Submission = {
		id: string;
		user_id?: string;
		data: Record<string, unknown>;
		status: 'new' | 'reviewed' | 'actioned' | 'spam';
		created_at: string;
	};

	type Trigger = {
		id: string;
		trigger_type: string;
		config: Record<string, unknown>;
		position: number;
	};

	// --- Route & Permissions ---

	const formId = $derived(page.params.formId ?? '');
	const canManage = $derived(hasPermission('forms.edit_any'));

	// --- Main Form Data ---

	let form = $state<FormDef | null>(null);
	let loading = $state(true);
	let activeTab = $state('metadata');
	let errorMessage = $state('');
	let saveMsg = $state('');

	// --- Metadata Tab ---

	let editName = $state('');
	let editDescription = $state('');
	let editSubmitLabel = $state('');
	let editSuccessMessage = $state('');
	let editIsActive = $state(true);
	let editSubmissionFeeCredits = $state(0);
	let saving = $state(false);

	// --- Fields Tab ---

	let fields = $state<FieldDef[]>([]);
	let loadingFields = $state(false);
	let fieldGroupId = $state('');
	let showAddFieldModal = $state(false);
	let newFieldLabel = $state('');
	let newFieldType = $state('text');
	let newFieldRequired = $state(false);
	let newFieldPlaceholder = $state('');
	let savingField = $state(false);
	// For multi-step forms: which step are we adding a field to?
	let selectedStepForField = $state<FormStep | null>(null);
	// For multi-step forms: which step's field are we editing/deleting/moving?
	let editingFieldStepGroupId = $state('');

	let showEditFieldModal = $state(false);
	let editingField = $state<FieldDef | null>(null);
	let editFieldLabel = $state('');
	let editFieldType = $state('text');
	let editFieldPlaceholder = $state('');
	let editFieldRequired = $state(false);

	// --- Submissions Tab ---

	let submissions = $state<Submission[]>([]);
	let loadingSubmissions = $state(false);
	let showSubmissionModal = $state(false);
	let selectedSubmission = $state<Submission | null>(null);

	const submissionActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Submission) => { selectedSubmission = row; showSubmissionModal = true; } }
	];

	// --- Triggers Tab ---

	let triggers = $state<Trigger[]>([]);
	let loadingTriggers = $state(false);

	// Trigger CRUD modal state
	let showAddTriggerModal = $state(false);
	let newTriggerType = $state('send_email');
	let savingTrigger = $state(false);
	let triggerError = $state('');

	// Trigger config fields per type
	let triggerConfigEmailTo = $state('');
	let triggerConfigEmailSubject = $state('');
	let triggerConfigEmailTemplate = $state('');
	let triggerConfigWebhookUrl = $state('');
	let triggerConfigWebhookSecret = $state('');
	let triggerConfigGroupId = $state('');
	let triggerConfigTicketPriority = $state('medium');
	let triggerConfigTicketCategory = $state('');
	let triggerConfigNotifyMessage = $state('');

	const TRIGGER_TYPES = [
		{ value: 'assign_user_group', label: t('forms.trigger.assign_user_group', 'Assign User Group') },
		{ value: 'send_email', label: t('forms.trigger.send_email', 'Send Email') },
		{ value: 'send_webhook', label: t('forms.trigger.send_webhook', 'Send Webhook') },
		{ value: 'create_ticket', label: t('forms.trigger.create_ticket', 'Create Ticket') },
		{ value: 'notify_admin', label: t('forms.trigger.notify_admin', 'Notify Admin') },
	];

	const TICKET_PRIORITIES = [
		{ value: 'low', label: t('forms.priority.low', 'Low') },
		{ value: 'medium', label: t('forms.priority.medium', 'Medium') },
		{ value: 'high', label: t('forms.priority.high', 'High') },
		{ value: 'urgent', label: t('forms.priority.urgent', 'Urgent') },
	];

	// --- Steps Tab (multi-step) ---

	let steps = $state<FormStep[]>([]);
	let loadingSteps = $state(false);
	let showAddStepModal = $state(false);
	let newStepTitle = $state('');
	let savingStep = $state(false);
	let stepError = $state('');

	let showEditStepModal = $state(false);
	let editingStep = $state<FormStep | null>(null);
	let editStepTitle = $state('');

	// --- Loaded Flags (prevent infinite retry loops) ---

	let fieldsLoaded = $state(false);
	let submissionsLoaded = $state(false);
	let triggersLoaded = $state(false);
	let stepsLoaded = $state(false);

	// --- Delete ---

	let showDeleteModal = $state(false);
	let deleting = $state(false);

	// --- Constants ---

	const fieldTypes = [
		{ value: 'text', label: t('forms.field_type.text', 'Text') },
		{ value: 'textarea', label: t('forms.field_type.textarea', 'Textarea') },
		{ value: 'rich_text', label: t('forms.field_type.rich_text', 'Rich Text') },
		{ value: 'number', label: t('forms.field_type.number', 'Number') },
		{ value: 'email', label: t('forms.field_type.email', 'Email') },
		{ value: 'url', label: t('forms.field_type.url', 'URL') },
		{ value: 'phone', label: t('forms.field_type.phone', 'Phone') },
		{ value: 'boolean', label: t('forms.field_type.boolean', 'Boolean') },
		{ value: 'select', label: t('forms.field_type.select', 'Select') },
		{ value: 'multiselect', label: t('forms.field_type.multiselect', 'Multi-Select') },
		{ value: 'date', label: t('forms.field_type.date', 'Date') },
		{ value: 'datetime', label: t('forms.field_type.datetime', 'Date & Time') },
		{ value: 'time', label: t('forms.field_type.time', 'Time') },
		{ value: 'image_single', label: t('forms.field_type.image_single', 'Image (Single)') },
		{ value: 'image_multi', label: t('forms.field_type.image_multi', 'Image (Multi)') },
		{ value: 'rating', label: t('forms.field_type.rating', 'Rating') },
		{ value: 'color', label: t('forms.field_type.color', 'Color') },
		{ value: 'currency', label: t('forms.field_type.currency', 'Currency') },
		{ value: 'percentage', label: t('forms.field_type.percentage', 'Percentage') },
		{ value: 'duration', label: t('forms.field_type.duration', 'Duration') },
	];

	const tabs = [
		{ value: 'metadata', label: t('forms.tabMetadata', 'Metadata'), icon: Pencil },
		{ value: 'fields', label: t('forms.tabFields', 'Fields'), icon: Plus },
		{ value: 'submissions', label: t('forms.tabSubmissions', 'Submissions'), icon: Eye },
		{ value: 'triggers', label: t('forms.tabTriggers', 'Triggers'), icon: ArrowDown },
	];

	// === Data Loading ===

	async function loadForm() {
		loading = true;
		try {
			const result = await formsAdminApi.get(formId);
			if (result.ok) {
				const raw = result.data;
				form = (raw as any)?.data ?? raw;
				if (form) {
					editName = form.name;
					editDescription = form.description ?? '';
					editSubmitLabel = form.submit_label ?? '';
					editSuccessMessage = form.success_message ?? '';
					editIsActive = String(form.status ?? 'draft').toLowerCase() !== 'draft';
					editSubmissionFeeCredits = form.submission_fee_credits ?? 0;
					// Extract field_group_id for field operations
					if (form.field_group_id) {
						fieldGroupId = form.field_group_id;
					}
					// Seed steps from the form response (GetForm returns steps[] for
					// multi-step forms) so the metadata tab renders without a second
					// round-trip.
					if (Array.isArray((form as any).steps)) {
						steps = (form as any).steps;
						stepsLoaded = true;
					}
				}
			}
		} catch {
			errorMessage = t('forms.loadError', 'Failed to load form');
		} finally {
			loading = false;
		}
	}

	async function loadFields() {
		loadingFields = true;
		try {
			// Fields are already in the form response from loadForm()
			if (form?.fields) {
				fields = form.fields;
			}
			// If fieldGroupId was already set from loadForm, we're good
			// If not, try to get it from the form response
			if (!fieldGroupId && form?.field_group_id) {
				fieldGroupId = form.field_group_id;
			}
		} catch {
			// ignore
		} finally {
			loadingFields = false;
		}
	}

	async function loadSubmissions() {
		loadingSubmissions = true;
		try {
			const result = await formsAdminApi.listSubmissions(formId);
			if (result.ok) {
				const raw = result.data;
				submissions = Array.isArray(raw) ? raw : (raw as any)?.data ?? [];
			}
		} catch {
			// ignore
		} finally {
			loadingSubmissions = false;
		}
	}

	async function loadTriggers() {
		loadingTriggers = true;
		try {
			const result = await formsAdminApi.listTriggers(formId);
			if (result.ok) {
				const raw = result.data;
				triggers = Array.isArray(raw) ? raw : (raw as any)?.data ?? [];
			} else {
				// Backend may not have trigger routes yet — show empty state
				triggers = [];
			}
		} catch {
			triggers = [];
		} finally {
			loadingTriggers = false;
		}
	}

	function resetTriggerConfig() {
		newTriggerType = 'send_email';
		triggerConfigEmailTo = '';
		triggerConfigEmailSubject = '';
		triggerConfigEmailTemplate = '';
		triggerConfigWebhookUrl = '';
		triggerConfigWebhookSecret = '';
		triggerConfigGroupId = '';
		triggerConfigTicketPriority = 'medium';
		triggerConfigTicketCategory = '';
		triggerConfigNotifyMessage = '';
		triggerError = '';
	}

	function buildTriggerConfig(): Record<string, unknown> {
		switch (newTriggerType) {
			case 'assign_user_group':
				return { group_id: triggerConfigGroupId };
			case 'send_email':
				return { to: triggerConfigEmailTo, subject: triggerConfigEmailSubject, template: triggerConfigEmailTemplate };
			case 'send_webhook':
				return { url: triggerConfigWebhookUrl, secret: triggerConfigWebhookSecret || undefined };
			case 'create_ticket':
				return { priority: triggerConfigTicketPriority, category: triggerConfigTicketCategory };
			case 'notify_admin':
				return { message: triggerConfigNotifyMessage };
			default:
				return {};
		}
	}

	function triggerConfigSummary(trigger: Trigger): string {
		const c = trigger.config ?? {};
		switch (trigger.trigger_type) {
			case 'assign_user_group':
				return `Group: ${c.group_id ?? '—'}`;
			case 'send_email':
				return `To: ${c.to ?? '—'} | Subject: ${c.subject ?? '—'}`;
			case 'send_webhook':
				return `URL: ${c.url ?? '—'}`;
			case 'create_ticket':
				return `Priority: ${c.priority ?? '—'} | Category: ${c.category ?? '—'}`;
			case 'notify_admin':
				return `Message: ${c.message ?? '—'}`;
			default:
				return JSON.stringify(c);
		}
	}

	async function addTrigger() {
		savingTrigger = true;
		triggerError = '';
		try {
			const config = buildTriggerConfig();
			const result = await formsAdminApi.createTrigger(formId, {
				trigger_type: newTriggerType,
				config,
				position: triggers.length,
			});
			if (result.ok) {
				showAddTriggerModal = false;
				resetTriggerConfig();
				loadTriggers();
			} else {
				triggerError = result.error;
			}
		} catch (e) {
			triggerError = e instanceof Error ? e.message : 'Failed to create trigger';
		} finally {
			savingTrigger = false;
		}
	}

	async function deleteTrigger(triggerId: string) {
		try {
			await formsAdminApi.deleteTrigger(formId, triggerId);
			loadTriggers();
		} catch {
			// ignore
		}
	}

	async function moveTrigger(triggerId: string, direction: -1 | 1) {
		const idx = triggers.findIndex((t) => t.id === triggerId);
		if (idx < 0) return;
		const newIdx = idx + direction;
		if (newIdx < 0 || newIdx >= triggers.length) return;
		const reordered = [...triggers];
		[reordered[idx], reordered[newIdx]] = [reordered[newIdx], reordered[idx]];
		triggers = reordered;
		try {
			await formsAdminApi.reorderTriggers(formId, {
				trigger_ids: triggers.map((t) => t.id),
			});
		} catch {
			// Reload to undo optimistic reorder on failure
			loadTriggers();
		}
	}

	async function loadSteps() {
		loadingSteps = true;
		try {
			if (form?.is_multi_step) {
				const result = await formsAdminApi.listSteps(formId);
				if (result.ok) {
					const raw = result.data;
					steps = Array.isArray(raw) ? raw : (raw as any)?.data ?? [];
				} else {
					steps = [];
				}
			} else {
				steps = [];
			}
		} catch {
			steps = [];
		} finally {
			loadingSteps = false;
		}
	}

	async function addStep() {
		if (!newStepTitle.trim()) return;
		savingStep = true;
		stepError = '';
		try {
			const result = await formsAdminApi.createStep(formId, {
				title: newStepTitle.trim(),
				position: steps.length,
			});
			if (result.ok) {
				showAddStepModal = false;
				newStepTitle = '';
				await loadSteps();
			} else {
				stepError = result.error;
			}
		} catch (e) {
			stepError = e instanceof Error ? e.message : 'Failed to create step';
		} finally {
			savingStep = false;
		}
	}

	function openEditStep(step: FormStep) {
		editingStep = step;
		editStepTitle = step.title;
		showEditStepModal = true;
	}

	async function updateStep() {
		if (!editingStep || !editStepTitle.trim()) return;
		savingStep = true;
		stepError = '';
		try {
			const result = await formsAdminApi.updateStep(formId, editingStep.id, {
				title: editStepTitle.trim(),
				position: editingStep.position,
			});
			if (result.ok) {
				showEditStepModal = false;
				editingStep = null;
				await loadSteps();
			} else {
				stepError = result.error;
			}
		} catch (e) {
			stepError = e instanceof Error ? e.message : 'Failed to update step';
		} finally {
			savingStep = false;
		}
	}

	async function deleteStep(stepId: string) {
		try {
			await formsAdminApi.deleteStep(formId, stepId);
			await loadSteps();
		} catch {
			// ignore
		}
	}

	async function moveStep(stepId: string, direction: -1 | 1) {
		const idx = steps.findIndex((s) => s.id === stepId);
		if (idx < 0) return;
		const newIdx = idx + direction;
		if (newIdx < 0 || newIdx >= steps.length) return;
		const reordered = [...steps];
		[reordered[idx], reordered[newIdx]] = [reordered[newIdx], reordered[idx]];
		// Update position fields in-place so downstream edits (which send
		// editingStep.position) use the new index, not the stale pre-swap value.
		reordered.forEach((s, i) => (s.position = i));
		steps = reordered;
		try {
			await formsAdminApi.reorderSteps(formId, {
				step_ids: steps.map((s) => s.id),
			});
			// Reload to confirm the reorder persisted server-side.
			await loadSteps();
		} catch {
			// Reload to undo optimistic reorder on failure
			await loadSteps();
		}
	}

	// Lazy-load a tab's data on activation — reactive so it also covers
	// ?tab= deep-links and back/forward (Tabs binds `activeTab` directly).
	$effect(() => {
		const tab = activeTab;
		if (tab === 'fields' && !fieldsLoaded && !loadingFields) { fieldsLoaded = true; void loadFields(); }
		if (tab === 'submissions' && !submissionsLoaded && !loadingSubmissions) { submissionsLoaded = true; void loadSubmissions(); }
		if (tab === 'triggers' && !triggersLoaded && !loadingTriggers) { triggersLoaded = true; void loadTriggers(); }
		if (tab === 'metadata' && form?.is_multi_step && !stepsLoaded && !loadingSteps) { stepsLoaded = true; void loadSteps(); }
	});

	// === Metadata Actions ===

	async function saveMetadata() {
		if (!editName.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await formsAdminApi.update(formId, {
				name: editName.trim(),
				description: editDescription.trim() || undefined,
				submit_label: editSubmitLabel.trim() || undefined,
				success_message: editSuccessMessage.trim() || undefined,
				status: editIsActive ? 'published' : 'draft',
				submission_fee_credits: editSubmissionFeeCredits,
			});
			if (result.ok) {
				saveMsg = t('common.saved', 'Saved');
				setTimeout(() => (saveMsg = ''), 3000);
				loadForm();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('forms.saveError', 'Failed to save');
		} finally {
			saving = false;
		}
	}

	// === Field Actions ===

	function toKey(label: string): string {
		return label
			.toLowerCase()
			.replace(/[^a-z0-9\s_]/g, '')
			.replace(/\s+/g, '_')
			.replace(/_+/g, '_')
			.replace(/^_|_$/g, '');
	}

	// Returns the field group ID to use for field CRUD:
	// - For multi-step forms: the selected step's field_group_id
	// - For non-multi-step forms: the form's field_group_id
	function activeFieldGroupId(): string {
		if (form?.is_multi_step && selectedStepForField?.field_group_id) {
			return selectedStepForField.field_group_id;
		}
		return fieldGroupId;
	}

	async function addField() {
		if (!newFieldLabel.trim()) return;
		savingField = true;
		try {
			let groupId = activeFieldGroupId();
			// For non-multi-step forms, ensure the form has a field group.
			if (!form?.is_multi_step && !groupId) {
				const ensureResult = await formsAdminApi.ensureFieldGroup(formId);
				if (ensureResult.ok) {
					const data = (ensureResult.data as any)?.data ?? ensureResult.data;
					fieldGroupId = data?.field_group_id;
					groupId = fieldGroupId;
				}
				if (!groupId) return;
			}
			if (!groupId) return;

			// For multi-step forms, position is relative to the step's fields.
			const positionBase = form?.is_multi_step && selectedStepForField?.fields
				? selectedStepForField.fields.length
				: fields.length;

			const key = toKey(newFieldLabel.trim());
			const result = await fieldGroupsAdminApiExtended.addField(groupId, {
				label: newFieldLabel.trim(),
				key,
				field_type: newFieldType,
				is_required: newFieldRequired,
				placeholder: newFieldPlaceholder.trim() || undefined,
				position: positionBase,
			});
			if (result.ok) {
				showAddFieldModal = false;
				newFieldLabel = '';
				newFieldType = 'text';
				newFieldRequired = false;
				newFieldPlaceholder = '';
				selectedStepForField = null;
				await loadForm();
				if (form?.fields) {
					fields = form.fields;
				}
			}
		} catch {
			// ignore
		} finally {
			savingField = false;
		}
	}

	function openEditField(field: FieldDef, stepGroupId?: string) {
		editingField = field;
		editFieldLabel = field.label;
		editFieldType = field.type;
		editFieldPlaceholder = field.placeholder ?? '';
		editFieldRequired = field.required ?? false;
		// For multi-step forms, remember which step's field group this field belongs to.
		editingFieldStepGroupId = stepGroupId ?? '';
		showEditFieldModal = true;
	}

	async function updateField() {
		const groupId = editingFieldStepGroupId || fieldGroupId;
		if (!groupId || !editingField) return;
		savingField = true;
		try {
			const result = await fieldGroupsAdminApiExtended.updateField(groupId, editingField.id, {
				label: editFieldLabel.trim(),
				field_type: editFieldType,
				is_required: editFieldRequired,
				placeholder: editFieldPlaceholder.trim() || undefined,
			});
			if (result.ok) {
				showEditFieldModal = false;
				editingField = null;
				editingFieldStepGroupId = '';
				await loadForm();
				if (form?.fields) {
					fields = form.fields;
				}
			}
		} catch {
			// ignore
		} finally {
			savingField = false;
		}
	}

	async function deleteField(fieldId: string, stepGroupId?: string) {
		const groupId = stepGroupId || fieldGroupId;
		if (!groupId) return;
		try {
			await fieldGroupsAdminApiExtended.deleteField(groupId, fieldId);
			await loadForm();
			if (form?.fields) {
				fields = form.fields;
			}
		} catch {
			// ignore
		}
	}

	async function moveField(fieldId: string, direction: -1 | 1, stepFields?: FieldDef[], stepGroupId?: string) {
		const groupId = stepGroupId || fieldGroupId;
		if (!groupId) return;
		// For multi-step forms, reorder within the step's field list.
		const fieldList = stepFields ?? fields;
		const idx = fieldList.findIndex((f) => f.id === fieldId);
		if (idx < 0) return;
		const newIdx = idx + direction;
		if (newIdx < 0 || newIdx >= fieldList.length) return;
		const newFieldArr = [...fieldList];
		[newFieldArr[idx], newFieldArr[newIdx]] = [newFieldArr[newIdx], newFieldArr[idx]];
		if (stepFields) {
			// Optimistically update the step's fields in the steps array
			steps = steps.map((s) =>
				s.id === (selectedStepForField?.id ?? '') ? { ...s, fields: newFieldArr } : s
			);
		} else {
			fields = newFieldArr;
		}
		try {
			await fieldGroupsAdminApiExtended.reorderFields(groupId, {
				field_ids: newFieldArr.map((f) => f.id),
			});
			// Reload form to confirm reorder persisted
			await loadForm();
			if (form?.fields) {
				fields = form.fields;
			}
		} catch {
			// Reload to undo optimistic reorder on failure
			await loadForm();
			if (form?.fields) {
				fields = form.fields;
			}
		}
	}

	// === Submission Actions ===

	async function updateSubmissionStatus(subId: string, status: string) {
		try {
			await formsAdminApi.updateSubmission(formId, subId, { status });
			loadSubmissions();
			showSubmissionModal = false;
		} catch {
			// ignore
		}
	}

	async function deleteSubmission(subId: string) {
		try {
			await formsAdminApi.deleteSubmission(formId, subId);
			loadSubmissions();
			showSubmissionModal = false;
		} catch {
			// ignore
		}
	}

	// === Delete Form ===

	async function deleteForm() {
		deleting = true;
		try {
			await formsAdminApi.delete(formId);
			goto('/forms');
		} catch {
			errorMessage = t('forms.deleteError', 'Failed to delete');
		} finally {
			deleting = false;
		}
	}

	// === Init & Tab Switching ===

	let initialized = $state(false);
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadForm();
		}
	});

	$effect(() => {
		const tab = activeTab;
		if (!formId) return;
		if (tab === 'fields' && !fieldsLoaded && !loadingFields) { fieldsLoaded = true; loadFields(); }
		else if (tab === 'submissions' && !submissionsLoaded && !loadingSubmissions) { submissionsLoaded = true; loadSubmissions(); }
		else if (tab === 'triggers' && !triggersLoaded && !loadingTriggers) { triggersLoaded = true; loadTriggers(); }
		else if (tab === 'metadata' && form?.is_multi_step && !stepsLoaded && !loadingSteps) { stepsLoaded = true; loadSteps(); }
	});
</script>

<svelte:head>
	<title>{form?.name ?? 'Form'} — {siteName()} Admin</title>
</svelte:head>

{#if saveMsg}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{saveMsg}
	</div>
{/if}

<PageHeader
	title={form?.name ?? t('forms.loading', 'Loading...')}
	description={form ? `${t('forms.slug', 'Slug')}: ${form.slug ?? form.id ?? ''}` : ''}
	actions={[
		{ label: t('common.back', 'Back'), icon: ArrowUp, onclick: () => goto('/forms'), variant: 'outline' as const },
		...(canManage ? [{ label: t('common.delete', 'Delete'), icon: Trash2, onclick: () => (showDeleteModal = true), variant: 'destructive' as const }] : []),
	]}
/>

<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

{#if loading}
	<TableSkeleton />
{:else if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">{errorMessage}</div>
{:else}
	<!-- Metadata Tab -->
	{#if activeTab === 'metadata'}
		<Card class="mt-4">
			<CardContent class="p-6">
				<div class="space-y-5">
					<div>
						<label for="form-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.name', 'Name')} *</label>
						<input
							id="form-name"
							type="text"
							class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							placeholder={t('forms.namePlaceholder', 'e.g., Contact Form')}
							bind:value={editName}
						/>
					</div>
					<div>
						<label for="form-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.descriptionLabel', 'Description')}</label>
						<textarea
							id="form-desc"
							rows="3"
							class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							placeholder={t('forms.descriptionPlaceholder', 'Brief description of this form')}
							bind:value={editDescription}
						></textarea>
					</div>
					<div class="grid gap-4 sm:grid-cols-2">
						<div>
							<label for="form-submit-label" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.submitLabel', 'Submit Button Label')}</label>
							<input
								id="form-submit-label"
								type="text"
								class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								placeholder={t('forms.submitLabelPlaceholder', 'e.g., Send')}
								bind:value={editSubmitLabel}
							/>
						</div>
						<div>
							<label for="form-success-msg" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.successMessage', 'Success Message')}</label>
							<input
								id="form-success-msg"
								type="text"
								class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								placeholder={t('forms.successMessagePlaceholder', 'Thank you for your submission!')}
								bind:value={editSuccessMessage}
							/>
						</div>
					</div>
					<div>
						<label for="form-fee-credits" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fee_credits', 'Submission Fee (credits)')}</label>
						<input
							id="form-fee-credits"
							type="number"
							min="0"
							step="1"
							class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={editSubmissionFeeCredits}
						/>
						<p class="mt-1 text-xs text-muted">{t('forms.fee_credits_hint', '0 = free submission')}</p>
					</div>
					<div class="flex items-center gap-2">
						<input id="form-active" type="checkbox" bind:checked={editIsActive} class="rounded" />
						<label for="form-active" class="text-sm text-foreground-alt">{t('forms.activeLabel', 'Active')}</label>
					</div>
					<div class="flex justify-end pt-2">
						<Button onclick={saveMetadata} disabled={saving || !editName.trim()}>
							{saving ? t('common.saving', 'Saving...') : t('common.saveChanges', 'Save Changes')}
						</Button>
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Steps (multi-step standard forms). Assessment forms derive their
			 steps from field dimensions — the backend rejects explicit steps
			 (assessment_derives_steps), so the editor is hidden for them. -->
		{#if form?.is_multi_step && form?.form_type !== 'assessment'}
			<Card class="mt-4">
				<CardContent class="p-6">
					<div class="mb-4 flex items-center justify-between">
						<h3 class="text-sm font-semibold text-foreground">{t('forms.steps', 'Form Steps')}</h3>
						{#if canManage}
							<Button onclick={() => { stepError = ''; newStepTitle = ''; showAddStepModal = true; }}>
								<Plus class="me-1 h-4 w-4" /> {t('forms.addStep', 'Add Step')}
							</Button>
						{/if}
					</div>
					{#if loadingSteps}
						<TableSkeleton />
					{:else if steps.length === 0}
						<p class="py-4 text-center text-sm text-muted">{t('forms.noSteps', 'No steps configured')}</p>
					{:else}
						<div class="space-y-2">
							{#each steps as step, idx (idx)}
								<div class="flex items-center justify-between rounded-md border border-border bg-surface px-4 py-3">
									<div class="flex items-center gap-3">
										<span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary/10 text-xs font-bold text-primary">{idx + 1}</span>
										<span class="text-sm font-medium">{step.title}</span>
									</div>
									{#if canManage}
										<div class="flex items-center gap-1">
											<Button variant="ghost" size="sm" onclick={() => openEditStep(step)}>
												<Pencil class="h-3.5 w-3.5" />
											</Button>
											<Button variant="ghost" size="sm" disabled={idx === 0} onclick={() => moveStep(step.id, -1)}>
												<ArrowUp class="h-3.5 w-3.5" />
											</Button>
											<Button variant="ghost" size="sm" disabled={idx === steps.length - 1} onclick={() => moveStep(step.id, 1)}>
												<ArrowDown class="h-3.5 w-3.5" />
											</Button>
											<Button variant="ghost" size="sm" style="color: var(--color-danger)" onclick={() => deleteStep(step.id)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>
		{/if}

		<!-- Add Step Modal -->
		<Modal bind:open={showAddStepModal} title={t('forms.addStep', 'Add Step')} size="sm">
			<div class="space-y-4">
				{#if stepError}
					<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">{stepError}</div>
				{/if}
				<div>
					<label for="new-step-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.stepTitle', 'Step Title')} *</label>
					<input id="new-step-title" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newStepTitle} placeholder={t('forms.stepTitlePlaceholder', 'e.g., Personal Information')} />
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Button variant="outline" onclick={() => (showAddStepModal = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={addStep} disabled={savingStep || !newStepTitle.trim()}>
					{savingStep ? t('common.creating', 'Adding...') : t('forms.addStep', 'Add Step')}
				</Button>
			</div>
		</Modal>

		<!-- Edit Step Modal -->
		<Modal bind:open={showEditStepModal} title={t('forms.editStep', 'Edit Step')} size="sm">
			<div class="space-y-4">
				{#if stepError}
					<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">{stepError}</div>
				{/if}
				<div>
					<label for="edit-step-title" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.stepTitle', 'Step Title')} *</label>
					<input id="edit-step-title" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editStepTitle} />
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Button variant="outline" onclick={() => (showEditStepModal = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={updateStep} disabled={savingStep || !editStepTitle.trim()}>
					{savingStep ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
				</Button>
			</div>
		</Modal>

		<!-- Embed & Share -->
		{#if form}
			<Card class="mt-4">
				<CardContent class="p-6">
					<h3 class="mb-4 text-sm font-semibold text-foreground">{t('forms.embedShare', 'Embed & Share')}</h3>
					{#if form.is_system}
						<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-info); background-color: color-mix(in srgb, var(--color-info) 8%, transparent); color: var(--color-info)">
							{t('forms.systemFormNote', 'This is a platform system form. Embedding and sharing is managed by the platform.')}
						</div>
					{/if}
					<div class="space-y-4">
						<div>
							<p class="mb-1 text-xs font-medium uppercase tracking-wider text-muted">{t('forms.publicUrl', 'Public URL')}</p>
							<div class="flex items-center gap-2">
								<code class="flex-1 rounded-md border border-border bg-surface px-3 py-2 text-sm font-mono text-foreground">{PUBLIC_JC_SITE_URL}/forms/{form!.slug}</code>
								<Button variant="outline" size="sm" onclick={() => navigator.clipboard.writeText(`${PUBLIC_JC_SITE_URL}/forms/${form!.slug}`)}>
									{t('common.copy', 'Copy')}
								</Button>
							</div>
						</div>
						<div>
							<p class="mb-1 text-xs font-medium uppercase tracking-wider text-muted">{t('forms.embedCode', 'Embed Code')}</p>
							<div class="flex items-center gap-2">
								<code class="flex-1 rounded-md border border-border bg-surface px-3 py-2 text-xs font-mono text-foreground break-all">&lt;iframe src="{PUBLIC_JC_SITE_URL}/forms/{form!.slug}" width="100%" height="600" frameborder="0"&gt;&lt;/iframe&gt;</code>
								<Button variant="outline" size="sm" onclick={() => navigator.clipboard.writeText(`<iframe src="${PUBLIC_JC_SITE_URL}/forms/${form!.slug}" width="100%" height="600" frameborder="0"></iframe>`)}>
									{t('common.copy', 'Copy')}
								</Button>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>
		{/if}

	<!-- Fields Tab -->
	{:else if activeTab === 'fields'}
		{#if loadingFields}
			<TableSkeleton />
		{:else if form?.is_multi_step && steps.length > 0}
			<!-- Multi-step: fields grouped per step. A multi-step form with no
				 steps yet can still hold form-level fields (e.g. the seeded
				 readiness assessment) — those fall through to the flat list. -->
			{#each steps as step, stepIdx (stepIdx)}
					<Card class="mt-4">
						<CardContent class="p-4">
							<div class="mb-4 flex items-center justify-between">
								<div class="flex items-center gap-2">
									<span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary/10 text-xs font-bold text-primary">{stepIdx + 1}</span>
									<h4 class="text-sm font-semibold text-foreground">{step.title}</h4>
								</div>
								{#if canManage}
									<Button size="sm" onclick={() => { selectedStepForField = step; newFieldLabel = ''; newFieldType = 'text'; newFieldRequired = false; newFieldPlaceholder = ''; showAddFieldModal = true; }}>
										<Plus class="me-1 h-4 w-4" /> {t('forms.addFieldToStep', 'Add Field to Step')}
									</Button>
								{/if}
							</div>
							{#if !step.fields || step.fields.length === 0}
								<p class="py-4 text-center text-sm text-muted">{t('forms.noFieldsInStep', 'No fields in this step yet')}</p>
							{:else}
								<div class="space-y-2">
									{#each step.fields as field, idx (idx)}
										<div class="flex items-center justify-between rounded-md border border-border bg-surface px-4 py-3">
											<div class="flex items-center gap-3">
												<span class="text-sm font-medium">{field.label}</span>
												<Badge variant="secondary">{field.type}</Badge>
												{#if field.required}
													<Badge variant="destructive">{t('forms.required', 'Required')}</Badge>
												{/if}
											</div>
											{#if canManage}
												<div class="flex items-center gap-1">
													<Button variant="ghost" size="sm" onclick={() => openEditField(field, step.field_group_id)}>
														<Pencil class="h-3.5 w-3.5" />
													</Button>
													<Button variant="ghost" size="sm" disabled={idx === 0} onclick={() => { selectedStepForField = step; moveField(field.id, -1, step.fields, step.field_group_id); }}>
														<ArrowUp class="h-3.5 w-3.5" />
													</Button>
													<Button variant="ghost" size="sm" disabled={idx === step.fields.length - 1} onclick={() => { selectedStepForField = step; moveField(field.id, 1, step.fields, step.field_group_id); }}>
														<ArrowDown class="h-3.5 w-3.5" />
													</Button>
													<Button variant="ghost" size="sm" style="color: var(--color-danger)" onclick={() => deleteField(field.id, step.field_group_id)}>
														<Trash2 class="h-3.5 w-3.5" />
													</Button>
												</div>
											{/if}
										</div>
									{/each}
								</div>
							{/if}
						</CardContent>
					</Card>
			{/each}
		{:else}
			<!-- Non-multi-step: flat field list -->
			<Card class="mt-4">
				<CardContent class="p-4">
					<div class="mb-4 flex justify-end">
						<Button onclick={() => { selectedStepForField = null; (showAddFieldModal = true); }}>
							<Plus class="me-1 h-4 w-4" /> {t('forms.addField', 'Add Field')}
						</Button>
					</div>
					{#if fields.length === 0}
						<p class="py-8 text-center text-sm text-muted">{t('forms.noFields', 'No fields yet')}</p>
					{:else}
						<div class="space-y-2">
							{#each fields as field, idx (idx)}
								<div class="flex items-center justify-between rounded-md border border-border bg-surface px-4 py-3">
									<div class="flex items-center gap-3">
										<span class="text-sm font-medium">{field.label}</span>
										<Badge variant="secondary">{field.type}</Badge>
										{#if field.required}
											<Badge variant="destructive">{t('forms.required', 'Required')}</Badge>
										{/if}
									</div>
									<div class="flex items-center gap-1">
										<Button variant="ghost" size="sm" onclick={() => openEditField(field)}>
											<Pencil class="h-3.5 w-3.5" />
										</Button>
										<Button variant="ghost" size="sm" disabled={idx === 0} onclick={() => moveField(field.id, -1)}>
											<ArrowUp class="h-3.5 w-3.5" />
										</Button>
										<Button variant="ghost" size="sm" disabled={idx === fields.length - 1} onclick={() => moveField(field.id, 1)}>
											<ArrowDown class="h-3.5 w-3.5" />
										</Button>
										<Button variant="ghost" size="sm" style="color: var(--color-danger)" onclick={() => deleteField(field.id)}>
											<Trash2 class="h-3.5 w-3.5" />
										</Button>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>
		{/if}

		<!-- Field Modals (shared by multi-step and non-multi-step Fields tabs) -->
		<!-- Add Field Modal -->
		<Modal bind:open={showAddFieldModal} title={t('forms.addField', 'Add Field')} size="sm">
			<div class="space-y-4">
				{#if form?.is_multi_step && selectedStepForField}
					<div class="rounded-md border border-border bg-surface px-3 py-2 text-sm text-muted">
						{t('forms.addingToStep', 'Adding to step')}: <strong>{selectedStepForField.title}</strong>
					</div>
				{/if}
				<div>
					<label for="new-field-label" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldLabel', 'Label')} *</label>
					<input id="new-field-label" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newFieldLabel} placeholder={t('forms.fieldLabelPlaceholder', 'e.g., Your Name')} />
				</div>
				<div>
					<label for="new-field-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldType', 'Type')}</label>
					<Select options={fieldTypes} bind:value={newFieldType} />
				</div>
				<div>
					<label for="new-field-placeholder" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldPlaceholder', 'Placeholder')}</label>
					<input id="new-field-placeholder" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={newFieldPlaceholder} />
				</div>
				<div class="flex items-center gap-2">
					<input id="new-field-required" type="checkbox" bind:checked={newFieldRequired} class="rounded" />
					<label for="new-field-required" class="text-sm text-foreground-alt">{t('forms.fieldRequired', 'Required')}</label>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Button variant="outline" onclick={() => (showAddFieldModal = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={addField} disabled={savingField || !newFieldLabel.trim()}>
					{savingField ? t('common.creating', 'Adding...') : t('forms.addField', 'Add Field')}
				</Button>
			</div>
		</Modal>

		<!-- Edit Field Modal -->
		<Modal bind:open={showEditFieldModal} title={t('forms.editField', 'Edit Field')} size="sm">
			<div class="space-y-4">
				<div>
					<label for="edit-field-label" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldLabel', 'Label')} *</label>
					<input id="edit-field-label" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editFieldLabel} />
				</div>
				<div>
					<label for="edit-field-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldType', 'Type')}</label>
					<Select options={fieldTypes} bind:value={editFieldType} />
				</div>
				<div>
					<label for="edit-field-placeholder" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.fieldPlaceholder', 'Placeholder')}</label>
					<input id="edit-field-placeholder" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={editFieldPlaceholder} />
				</div>
				<div class="flex items-center gap-2">
					<input id="edit-field-required" type="checkbox" bind:checked={editFieldRequired} class="rounded" />
					<label for="edit-field-required" class="text-sm text-foreground-alt">{t('forms.fieldRequired', 'Required')}</label>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Button variant="outline" onclick={() => (showEditFieldModal = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={updateField} disabled={savingField || !editFieldLabel.trim()}>
					{savingField ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
				</Button>
			</div>
		</Modal>
	{:else if activeTab === 'submissions'}
		{#if loadingSubmissions}
			<TableSkeleton />
		{:else}
			<Card class="mt-4">
				<CardContent class="p-0">
					<DataTable
						columns={[
							{ key: 'created_at', label: t('forms.submitted', 'Submitted'), sortable: true, render: (v: string) => new Date(v).toLocaleDateString(locale.current, { year: 'numeric', month: 'short', day: 'numeric' }) },
							{ key: 'status', label: t('forms.status', 'Status'), sortable: true, render: (v: string) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold">${v}</span>` },
						]}
						data={submissions}
						pageSize={20}
						emptyMessage={t('forms.noSubmissions', 'No submissions yet')}
						actions={submissionActions}
					/>
				</CardContent>
			</Card>

			<!-- Submission Detail Modal -->
			<Modal bind:open={showSubmissionModal} title={t('forms.submissionDetail', 'Submission Detail')} size="lg">
				{#if selectedSubmission}
					<div class="space-y-4">
						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('forms.submitted', 'Submitted')}</p>
								<p class="mt-1 text-sm">{new Date(selectedSubmission.created_at).toLocaleString()}</p>
							</div>
							<div>
								<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('forms.status', 'Status')}</p>
								<Badge variant={selectedSubmission.status === 'new' ? 'info' : selectedSubmission.status === 'reviewed' ? 'success' : selectedSubmission.status === 'spam' ? 'destructive' : 'warning'}>
									{selectedSubmission.status}
								</Badge>
							</div>
						</div>
						<div>
							<p class="mb-2 text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('forms.responseData', 'Response Data')}</p>
							<div class="rounded-md border border-border bg-surface p-4">
								{#each Object.entries(selectedSubmission.data) as [key, value] (key)}
									<div class="mb-2 last:mb-0">
										<p class="text-xs font-medium" style="color: var(--color-muted)">{key}</p>
										<p class="text-sm">{typeof value === 'object' ? JSON.stringify(value) : String(value ?? '—')}</p>
									</div>
								{/each}
							</div>
						</div>
					</div>
				{/if}
				{#snippet footer()}
					<div class="flex flex-wrap justify-end gap-3">
						<Button variant="outline" onclick={() => (showSubmissionModal = false)}>{t('common.close', 'Close')}</Button>
						{#if selectedSubmission && selectedSubmission.status !== 'reviewed'}
							<Button variant="outline" onclick={() => updateSubmissionStatus(selectedSubmission!.id, 'reviewed')}>{t('forms.markReviewed', 'Mark Reviewed')}</Button>
						{/if}
						{#if selectedSubmission && selectedSubmission.status !== 'actioned'}
							<Button variant="outline" onclick={() => updateSubmissionStatus(selectedSubmission!.id, 'actioned')}>{t('forms.markActioned', 'Mark Actioned')}</Button>
						{/if}
						{#if selectedSubmission && selectedSubmission.status !== 'spam'}
							<Button variant="destructive" onclick={() => updateSubmissionStatus(selectedSubmission!.id, 'spam')}>{t('forms.markSpam', 'Mark Spam')}</Button>
						{/if}
						{#if selectedSubmission}
							<Button variant="destructive" onclick={() => deleteSubmission(selectedSubmission!.id)}>{t('common.delete', 'Delete')}</Button>
						{/if}
					</div>
				{/snippet}
			</Modal>
		{/if}

	<!-- Triggers Tab -->
	{:else if activeTab === 'triggers'}
		{#if loadingTriggers}
			<TableSkeleton />
		{:else}
			<Card class="mt-4">
				<CardContent class="p-6">
					<div class="mb-4 flex items-center justify-between">
						<p class="text-sm text-muted">{t('forms.triggersDescription', 'Configure triggers that fire when this form is submitted.')}</p>
						{#if canManage}
							<Button onclick={() => { resetTriggerConfig(); showAddTriggerModal = true; }}>
								<Plus class="me-1 h-4 w-4" /> {t('forms.addTrigger', 'Add Trigger')}
							</Button>
						{/if}
					</div>
					{#if triggers.length === 0}
						<div class="rounded-md border border-dashed border-border p-8 text-center">
							<p class="text-sm text-muted">{t('forms.noTriggers', 'No triggers configured')}</p>
							<p class="mt-1 text-xs text-muted">{t('forms.noTriggersHint', 'Triggers can send emails, webhooks, or create records when a form is submitted.')}</p>
						</div>
					{:else}
						<div class="space-y-2">
							{#each triggers as trigger, idx (idx)}
								<div class="flex items-center justify-between rounded-md border border-border bg-surface px-4 py-3">
									<div class="flex items-center gap-3 min-w-0 flex-1">
										<Badge variant="secondary">{trigger.trigger_type}</Badge>
										<span class="text-sm text-muted truncate">{triggerConfigSummary(trigger)}</span>
									</div>
									<div class="flex items-center gap-1 shrink-0">
										{#if canManage}
											<Button variant="ghost" size="sm" disabled={idx === 0} onclick={() => moveTrigger(trigger.id, -1)}>
												<ArrowUp class="h-3.5 w-3.5" />
											</Button>
											<Button variant="ghost" size="sm" disabled={idx === triggers.length - 1} onclick={() => moveTrigger(trigger.id, 1)}>
												<ArrowDown class="h-3.5 w-3.5" />
											</Button>
											<Button variant="ghost" size="sm" style="color: var(--color-danger)" onclick={() => deleteTrigger(trigger.id)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>

			<!-- Add Trigger Modal -->
			<Modal bind:open={showAddTriggerModal} title={t('forms.addTrigger', 'Add Trigger')} size="sm">
				<div class="space-y-4">
					{#if triggerError}
						<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">{triggerError}</div>
					{/if}
					<div>
						<label for="trigger-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.triggerType', 'Trigger Type')} *</label>
						<Select options={TRIGGER_TYPES} bind:value={newTriggerType} />
					</div>

					{#if newTriggerType === 'assign_user_group'}
						<div>
							<label for="trigger-group-id" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_group_id', 'Group ID')} *</label>
							<input id="trigger-group-id" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigGroupId} placeholder={t('forms.ph_enter_user_group_id', 'Enter user group ID')} />
						</div>
					{:else if newTriggerType === 'send_email'}
						<div>
							<label for="trigger-email-to" class="mb-1 block text-sm font-medium text-foreground-alt">To (email) *</label>
							<input id="trigger-email-to" type="email" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigEmailTo} placeholder="admin@example.com" />
						</div>
						<div>
							<label for="trigger-email-subject" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_email_subject', 'Subject')} *</label>
							<input id="trigger-email-subject" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigEmailSubject} placeholder={t('forms.ph_new_form_submission', 'New form submission')} />
						</div>
						<div>
							<label for="trigger-email-template" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_email_template', 'Template')}</label>
							<input id="trigger-email-template" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigEmailTemplate} placeholder={t('forms.ph_optional_template_name', 'Optional template name')} />
						</div>
					{:else if newTriggerType === 'send_webhook'}
						<div>
							<label for="trigger-webhook-url" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_webhook_url', 'Webhook URL')} *</label>
							<input id="trigger-webhook-url" type="url" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigWebhookUrl} placeholder="https://example.com/webhook" />
						</div>
						<div>
							<label for="trigger-webhook-secret" class="mb-1 block text-sm font-medium text-foreground-alt">Secret (optional)</label>
							<input id="trigger-webhook-secret" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigWebhookSecret} placeholder={t('forms.ph_hmac_signing_secret', 'HMAC signing secret')} />
						</div>
					{:else if newTriggerType === 'create_ticket'}
						<div>
							<label for="trigger-ticket-priority" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_ticket_priority', 'Priority')} *</label>
							<Select options={TICKET_PRIORITIES} bind:value={triggerConfigTicketPriority} />
						</div>
						<div>
							<label for="trigger-ticket-category" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_ticket_category', 'Category')}</label>
							<input id="trigger-ticket-category" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigTicketCategory} placeholder={t('forms.ph_eg_feedback', 'e.g., Feedback')} />
						</div>
					{:else if newTriggerType === 'notify_admin'}
						<div>
							<label for="trigger-notify-message" class="mb-1 block text-sm font-medium text-foreground-alt">{t('forms.trigger_notify_message', 'Message')} *</label>
							<textarea id="trigger-notify-message" rows="3" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={triggerConfigNotifyMessage} placeholder={t('forms.ph_notification_message_text', 'Notification message text')}></textarea>
						</div>
					{/if}
				</div>
				<div class="mt-6 flex justify-end gap-3">
					<Button variant="outline" onclick={() => (showAddTriggerModal = false)}>{t('common.cancel', 'Cancel')}</Button>
					<Button onclick={addTrigger} disabled={savingTrigger}>
						{savingTrigger ? t('common.creating', 'Adding...') : t('common.create', 'Create Trigger')}
					</Button>
				</div>
			</Modal>
		{/if}
	{/if}
{/if}

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('forms.deleteForm', 'Delete Form')} size="sm">
	<p class="text-sm">{t('forms.deleteConfirm', 'Are you sure you want to delete this form? All submissions will be permanently lost.')}</p>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={deleteForm} disabled={deleting}>
			{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
