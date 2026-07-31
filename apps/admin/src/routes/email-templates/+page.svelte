<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Tabs } from "$lib/components/ui/tabs";
	import { Tabs as BitsTabs } from "bits-ui";
	import { RefreshCw, Eye, RotateCcw, Send, Pencil } from "lucide-svelte";
	import { emailTemplatesAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type EmailTemplate = {
		id: string;
		name: string;
		slug: string;
		subject: string;
		is_active: boolean;
		updated_at: string;
	};

	let templates = $state<EmailTemplate[]>([]);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");

	// View/Edit modal
	let showViewModal = $state(false);
	let viewingTemplate = $state<EmailTemplate | null>(null);
	let editName = $state("");
	let editSubject = $state("");
	let editBody = $state("");
	let bodyTab = $state("edit");
	let saving = $state(false);
	let formError = $state("");
	let modalLoading = $state(false);

	const bodyTabs = [
		{ value: "edit", label: t("email_templates.tab_edit", "Edit") },
		{ value: "preview", label: t("email_templates.tab_preview", "Preview") }
	];

	// Reset modal
	let showResetModal = $state(false);
	let resetTarget = $state<EmailTemplate | null>(null);

	// Test modal
	let showTestModal = $state(false);
	let testTarget = $state<EmailTemplate | null>(null);
	let testEmail = $state("");
	let testing = $state(false);

	const canManageTemplates = $derived(hasPermission("email.templates.manage"));

	const templateSampleValues: Record<string, string> = {
		SiteName: "JapaConnect",
		SiteURL: "https://japaconnect.com",
		UserName: "Test User",
		ActionURL: "https://japaconnect.com/reset-password?token=abc123",
		Expiry: "1 hour",
		SupportURL: "https://japaconnect.com/support",
	};

	const previewBody = $derived.by(() => {
		if (!editBody) return "";
		let body = editBody;
		for (const [variable, value] of Object.entries(templateSampleValues)) {
			const pattern = new RegExp(`{{\\s*\\.${variable}\\s*}}`, "g");
			body = body.replace(pattern, value);
		}
		return body.replace(/\{\{\s*(?:if|else|range|end)\b[^}]*\}\}/g, "");
	});

	const columns = [
		{ key: "name", label: t("email_templates.name", "Name"), sortable: true },
		{ key: "slug", label: t("email_templates.slug", "Slug"), sortable: true },
		{ key: "subject", label: t("email_templates.subject", "Subject"), sortable: true },
		{ key: "is_active", label: t("common.status", "Status"), sortable: true,
			render: (v: boolean) => v ? t("common.active", "Active") : t("common.inactive", "Inactive")
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48",
			componentSnippet: TemplateActionsCell
		}
	];

	async function loadTemplates() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			const result = await emailTemplatesAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as unknown as { items: EmailTemplate[]; total?: number; page: number; total_pages: number };
				templates = d.items ?? [];
				totalItems = d.total ?? (d.items?.length ?? 0) + (d.total_pages > 1 ? (d.total_pages - 1) * pageSize : 0);
				totalPages = d.total_pages ?? 1;
			} else {
				error = result.error ?? t("common.error", "Failed to load email templates");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load email templates");
		} finally {
			loading = false;
		}
	}

	async function openView(id: string, defaultTab: string = "edit") {
		let template = templates.find((tpl) => tpl.id === id);
		if (!template) return;
		bodyTab = defaultTab;
		viewingTemplate = template;
		formError = "";
		editName = template.name;
		modalLoading = true;
		try {
			const result = await emailTemplatesAdminApi.get(id);
			if (result.ok && result.data) {
				const data = result.data as { subject?: string; html_body?: string; plain_text_body?: string };
				editSubject = data.subject ?? template.subject;
				editBody = data.html_body || data.plain_text_body || "";
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load template");
		} finally {
			modalLoading = false;
			showViewModal = true;
		}
	}

	async function saveTemplate() {
		if (!viewingTemplate) return;
		if (!editName.trim()) {
			formError = t("email_templates.name_required", "Name cannot be empty");
			return;
		}
		if (!editSubject.trim()) {
			formError = t("email_templates.subject_required", "Subject cannot be empty");
			return;
		}
		if (!editBody.trim()) {
			formError = t("email_templates.body_required", "Body cannot be empty");
			return;
		}
		formError = "";
		saving = true;
		error = "";
		try {
			const result = await emailTemplatesAdminApi.update(viewingTemplate.id, {
				name: editName.trim(),
				subject: editSubject,
				html_body: editBody
			});
			if (result.ok) {
				showViewModal = false;
				success = t("email_templates.updated", "Template updated");
				loadTemplates();
			} else {
				error = result.error ?? t("common.error", "Failed to save template");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save template");
		} finally {
			saving = false;
		}
	}

	async function confirmReset() {
		if (!resetTarget) return;
		const result = await emailTemplatesAdminApi.reset(resetTarget.id);
		if (result.ok) {
			showResetModal = false;
			resetTarget = null;
			success = t("email_templates.reset_done", "Template reset to default");
			loadTemplates();
		} else {
			error = result.error ?? t("common.error", "Failed to reset template");
		}
	}

	async function sendTest() {
		if (!testTarget || !testEmail.trim()) return;
		testing = true;
		try {
			const result = await emailTemplatesAdminApi.test(testTarget.id, { email: testEmail.trim() });
			if (result.ok) {
				showTestModal = false;
				testTarget = null;
				success = t("email_templates.test_sent", "Test email sent");
			} else {
				error = result.error ?? t("common.error", "Failed to send test");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to send test");
		} finally {
			testing = false;
		}
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		loadTemplates();
	});
</script>

<svelte:head>
	<title>{t("email_templates.title", "Email Templates")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("email_templates.title", "Email Templates")}
	description={t("email_templates.description", "Manage system email templates")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadTemplates }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else if templates.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("email_templates.empty", "No email templates found")}
			</div>
		{:else}
			<DataTable {columns} data={templates} showPagination={false} />
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

<!-- View/Edit Modal -->
<Modal bind:open={showViewModal} title={t("email_templates.view", "Email Template")}>
	{#if viewingTemplate}
		<div class="space-y-4">
			{#if formError}
				<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
					{formError}
				</div>
			{/if}
			<div>
				<label for="et-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.name", "Name")}</label>
				<input id="et-name" type="text" bind:value={editName} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
			<div>
				<p class="text-xs font-mono" style="color: var(--color-muted)">{viewingTemplate.slug}</p>
			</div>
			<div>
				<label for="et-subject" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.subject", "Subject")}</label>
				<input id="et-subject" type="text" bind:value={editSubject} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
			<div>
				<label for="et-body" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.body", "Body (HTML)")}</label>
				<Tabs tabs={bodyTabs} bind:value={bodyTab} class="mt-1">
					{#snippet children()}
						<BitsTabs.Content value="edit">
							<textarea id="et-body" bind:value={editBody} rows="12" class="w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"></textarea>
						</BitsTabs.Content>
						<BitsTabs.Content value="preview">
							<iframe srcdoc={previewBody} title="Email body preview" class="w-full rounded-lg border" style="border-color: var(--color-border); min-height: 300px;"></iframe>
						</BitsTabs.Content>
					{/snippet}
				</Tabs>
			</div>
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => showViewModal = false}>{t("common.cancel", "Cancel")}</Button>
			<Button onclick={saveTemplate} disabled={saving}>{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
		</div>
	{/if}
</Modal>

<!-- Reset Confirm -->
<Modal bind:open={showResetModal} title={t("email_templates.reset", "Reset Template")}>
	{#if resetTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("email_templates.reset_confirm", "Reset template")} <strong>{resetTarget.name}</strong> {t("email_templates.reset_to_default", "to its default content?")}
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showResetModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmReset}>{t("email_templates.reset", "Reset")}</Button>
	</div>
</Modal>

<!-- Test Modal -->
<Modal bind:open={showTestModal} title={t("email_templates.test", "Send Test Email")}>
	{#if testTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t("email_templates.test_to", "Send a test of")} <strong>{testTarget.name}</strong>:</p>
		<div class="mt-4">
			<label for="et-test-email" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.test_email", "Test Email Address")}</label>
			<input id="et-test-email" type="email" bind:value={testEmail} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="admin@example.com" />
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showTestModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={sendTest} disabled={testing || !testEmail.trim()}>{testing ? t("common.sending", "Sending...") : t("email_templates.send_test", "Send Test")}</Button>
	</div>
</Modal>

{#snippet TemplateActionsCell({ value, row }: { value: any; row: EmailTemplate })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row.id, "preview")}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTemplates}
			<Button variant="ghost" size="sm" onclick={() => openView(row.id, "edit")} style="color: var(--color-primary)">
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => { resetTarget = row; showResetModal = true; }} style="color: var(--color-foreground)">
				<RotateCcw class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => { testTarget = row; testEmail = ""; showTestModal = true; }}>
				<Send class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
