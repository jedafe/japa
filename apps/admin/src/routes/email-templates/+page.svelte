<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
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
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state("");

	// View/Edit modal
	let showViewModal = $state(false);
	let viewingTemplate = $state<EmailTemplate | null>(null);
	let editSubject = $state("");
	let editBody = $state("");
	let saving = $state(false);

	// Reset modal
	let showResetModal = $state(false);
	let resetTarget = $state<EmailTemplate | null>(null);

	// Test modal
	let showTestModal = $state(false);
	let testTarget = $state<EmailTemplate | null>(null);
	let testEmail = $state("");
	let testing = $state(false);

	const canManageTemplates = $derived(hasPermission("email.templates.manage"));

	const columns = [
		{ key: "name", label: t("email_templates.name", "Name"), sortable: true },
		{ key: "slug", label: t("email_templates.slug", "Slug"), sortable: true },
		{ key: "subject", label: t("email_templates.subject", "Subject"), sortable: true },
		{ key: "is_active", label: t("common.status", "Status"), sortable: true,
			render: (v: boolean) => v ? t("common.active", "Active") : t("common.inactive", "Inactive")
		},
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-48",
			component: TemplateActionsCell
		}
	];

	async function loadTemplates() {
		loading = true;
		error = "";
		try {
			const result = await emailTemplatesAdminApi.list();
			if (result.ok) {
				let items = (result.data as EmailTemplate[]) ?? [];
				if (searchQuery) {
					const q = searchQuery.toLowerCase();
					items = items.filter(t => t.name.toLowerCase().includes(q) || t.slug.toLowerCase().includes(q) || t.subject.toLowerCase().includes(q));
				}
				templates = items;
			} else {
				error = result.error ?? t("common.error", "Failed to load email templates");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load email templates");
		} finally {
			loading = false;
		}
	}

	async function openView(id: string) {
		const template = templates.find((tpl) => tpl.id === id);
		if (!template) return;
		viewingTemplate = template;
		loading = true;
		try {
			const result = await emailTemplatesAdminApi.get(id);
			if (result.ok && result.data) {
				const data = result.data as { subject?: string; html_body?: string; plain_text_body?: string };
				editSubject = data.subject ?? template.subject;
				editBody = data.html_body ?? data.plain_text_body ?? "";
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load template");
		} finally {
			loading = false;
			showViewModal = true;
		}
	}

	async function saveTemplate() {
		if (!viewingTemplate) return;
		saving = true;
		error = "";
		try {
			const result = await emailTemplatesAdminApi.update(viewingTemplate.id, {
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

	loadTemplates();
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
			<DataTable {columns} data={templates} pageSize={10} searchable onsearch={(q) => { searchQuery = q; loadTemplates(); }} />
		{/if}
	</CardContent>
</Card>

<!-- View/Edit Modal -->
<Modal bind:open={showViewModal} title={t("email_templates.view", "Email Template")}>
	{#if viewingTemplate}
		<div class="space-y-4">
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{viewingTemplate.name}</span>
				<p class="text-xs font-mono" style="color: var(--color-muted)">{viewingTemplate.slug}</p>
			</div>
			<div>
				<label for="et-subject" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.subject", "Subject")}</label>
				<input id="et-subject" type="text" bind:value={editSubject} class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" />
			</div>
			<div>
				<label for="et-body" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("email_templates.body", "Body (HTML)")}</label>
				<textarea id="et-body" bind:value={editBody} rows="12" class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"></textarea>
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
		<Button variant="ghost" size="sm" onclick={() => openView(row.id)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTemplates}
			<Button variant="ghost" size="sm" onclick={() => openView(row.id)} style="color: var(--color-primary)">
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
