<script lang="ts">
	// /email-templates — migrated to the EntityPage scaffold.
	//
	// Email templates are predefined (no create/delete). The page is READ + EDIT
	// with two page-local sub-flows: Reset (reset to default) and Test Send.
	// `readonly: true` hides the Create button in the header; a custom `actions`
	// array (View + Edit) renders regardless of readonly. Reset/Test buttons live
	// inside the `formSnippet` and trigger page-local confirm/test Modals.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Tabs as BitsTabs } from 'bits-ui';
	import { emailTemplatesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, RotateCcw, Send } from 'lucide-svelte';

	type EmailTemplate = {
		id: string;
		name: string;
		slug: string;
		subject: string;
		html_body?: string;
		plain_text_body?: string;
		is_active: boolean;
		updated_at: string;
	};

	const canManageTemplates = $derived(hasPermission('email.templates.manage'));

	// ── EntityPage controller (for page-local reload after reset/test) ───────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── Preview helpers (mirror legacy sample-variable substitution) ────────
	const templateSampleValues: Record<string, string> = {
		SiteName: 'JapaConnect',
		SiteURL: 'https://japaconnect.com',
		UserName: 'Test User',
		ActionURL: 'https://japaconnect.com/reset-password?token=abc123',
		Expiry: '1 hour',
		SupportURL: 'https://japaconnect.com/support',
	};

	function renderPreview(html: string): string {
		if (!html) return '';
		let body = html;
		for (const [variable, value] of Object.entries(templateSampleValues)) {
			const pattern = new RegExp(`{{\\s*\\.${variable}\\s*}}`, 'g');
			body = body.replace(pattern, value);
		}
		return body.replace(/\{\{\s*(?:if|else|range|end)\b[^}]*\}\}/g, '');
	}

	// ── Reset sub-flow (page-local Modal) ────────────────────────────────────
	let showResetModal = $state(false);
	let resetTarget = $state<EmailTemplate | null>(null);
	let resetting = $state(false);

	function openReset(row: EmailTemplate) {
		resetTarget = row;
		showResetModal = true;
	}

	async function confirmReset() {
		if (!resetTarget) return;
		if (resetting) return;
		resetting = true;
		pageError = '';
		try {
			const result = await emailTemplatesAdminApi.reset(resetTarget.id);
			if (result.ok) {
				showResetModal = false;
				resetTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to reset template');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to reset template');
		} finally {
			resetting = false;
		}
	}

	// ── Test send sub-flow (page-local Modal) ────────────────────────────────
	let showTestModal = $state(false);
	let testTarget = $state<EmailTemplate | null>(null);
	let testEmail = $state('');
	let testing = $state(false);

	function openTest(row: EmailTemplate) {
		testTarget = row;
		testEmail = '';
		showTestModal = true;
	}

	async function sendTest() {
		if (!testTarget || !testEmail.trim()) return;
		if (testing) return;
		testing = true;
		pageError = '';
		try {
			const result = await emailTemplatesAdminApi.test(testTarget.id, { email: testEmail.trim() });
			if (result.ok) {
				showTestModal = false;
				testTarget = null;
			} else {
				pageError = result.error ?? t('common.error', 'Failed to send test');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to send test');
		} finally {
			testing = false;
		}
	}

	// ── Body tab state for the edit/preview tabs in the form ─────────────────
	let bodyTab = $state('edit');

	// `fields` seeds the edit-form value; the actual inputs are in `formSnippet`.
	const fields: FieldDef[] = [
		{ key: 'subject', label: t('email_templates.subject', 'Subject'), type: 'text' },
		{ key: 'html_body', label: t('email_templates.body', 'Body (HTML)'), type: 'textarea' },
		{ key: 'plain_text_body', label: t('email_templates.plain_text', 'Plain Text Body'), type: 'textarea' },
		{ key: 'is_active', label: t('common.status', 'Status'), type: 'checkbox' },
	];

	// Fetch full template detail (list returns summary only) then open edit/view.
	async function fetchDetail(row: EmailTemplate): Promise<EmailTemplate> {
		try {
			const res = await emailTemplatesAdminApi.get(row.id);
			if (res.ok && res.data) {
				const d = res.data as Record<string, any>;
				return {
					...row,
					subject: d.subject ?? row.subject,
					html_body: d.html_body ?? '',
					plain_text_body: d.plain_text_body ?? '',
					is_active: d.is_active ?? row.is_active,
				};
			}
		} catch {
			// fall through with original row
		}
		return row;
	}

	const config: EntityPageConfig = {
		title: t('email_templates.title', 'Email Templates'),
		description: t('email_templates.description', 'Manage system email templates'),
		// readonly hides the Create button (no create API); custom actions below
		// still render because `config.actions` replaces the defaults entirely.
		readonly: true,
		editPermission: 'email.templates.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('email_templates.name', 'Name') },
			{ key: 'slug', label: t('email_templates.slug', 'Slug') },
			{ key: 'subject', label: t('email_templates.subject', 'Subject') },
			{ key: 'is_active', label: t('common.status', 'Status'), componentSnippet: StatusCell },
			{
				key: 'updated_at',
				label: t('common.updated', 'Updated'),
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—'),
			},
		],
		filters: [
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'true', label: t('common.active', 'Active') },
					{ value: 'false', label: t('common.inactive', 'Inactive') },
				],
			},
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: async (row: EmailTemplate, ctx) => {
					const enriched = await fetchDetail(row);
					ctx?.openView(enriched);
				},
			},
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: async (row: EmailTemplate, ctx) => {
					if (!canManageTemplates) return;
					bodyTab = 'edit';
					const enriched = await fetchDetail(row);
					ctx?.openEdit(enriched);
				},
				show: () => canManageTemplates,
			},
		],
		fields,
		load: async ({ page, pageSize, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize),
			});
			if (filter.is_active) params.set('is_active', filter.is_active);
			const result = await emailTemplatesAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as
					| { items: EmailTemplate[]; total?: number; total_pages: number }
					| EmailTemplate[];
				if (Array.isArray(d)) {
					return { data: d, total: d.length, totalPages: 1 };
				}
				const items = d.items ?? [];
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1,
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load email templates'));
		},
		update: async (id, body) => {
			const res = await emailTemplatesAdminApi.update(id, {
				subject: String(body.subject ?? '').trim(),
				html_body: String(body.html_body ?? ''),
				plain_text_body: String(body.plain_text_body ?? ''),
				is_active: !!body.is_active,
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TemplateView,
		formSnippet: TemplateForm,
	};
</script>

<svelte:head>
	<title>{t('email_templates.title', 'Email Templates')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Reset confirm Modal (page-local) -->
<Modal bind:open={showResetModal} title={t('email_templates.reset', 'Reset Template')}>
	{#if resetTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('email_templates.reset_confirm', 'Reset template')} <strong>{resetTarget.name}</strong>
			{t('email_templates.reset_to_default', 'to its default content?')}
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">
			{t('common.cannotUndo', 'This action cannot be undone.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showResetModal = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmReset} disabled={resetting}>
			{resetting ? t('common.saving', 'Resetting...') : t('email_templates.reset', 'Reset')}
		</Button>
	</div>
</Modal>

<!-- Test send Modal (page-local) -->
<Modal bind:open={showTestModal} title={t('email_templates.test', 'Send Test Email')}>
	{#if testTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('email_templates.test_to', 'Send a test of')} <strong>{testTarget.name}</strong>:
		</p>
		<div class="mt-4">
			<label for="et-test-email" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('email_templates.test_email', 'Test Email Address')}
			</label>
			<input
				id="et-test-email"
				type="email"
				bind:value={testEmail}
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="admin@example.com"
			/>
		</div>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showTestModal = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={sendTest} disabled={testing || !testEmail.trim()}>
			{testing ? t('common.sending', 'Sending...') : t('email_templates.send_test', 'Send Test')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- is_active badge column -->
{#snippet StatusCell({ value }: { value: any; row: EmailTemplate })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value
			? 'bg-success/15 text-success'
			: 'bg-muted/15 text-muted'}"
	>
		{value ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body -->
{#snippet TemplateView({ row }: { row: EmailTemplate })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.name', 'Name')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.slug', 'Slug')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.subject', 'Subject')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.subject}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('common.status', 'Status')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}
				</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('common.updated', 'Updated')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}
				</p>
			</div>
		</div>
		{#if row.html_body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.body', 'Body (HTML)')}
				</p>
				<iframe
					srcdoc={renderPreview(row.html_body)}
					title="Email body preview"
					class="mt-1 w-full rounded-lg border"
					style="border-color: var(--color-border); min-height: 300px;"
				></iframe>
			</div>
		{/if}
		{#if row.plain_text_body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.plain_text', 'Plain Text Body')}
				</p>
				<pre class="mt-1 whitespace-pre-wrap rounded-lg border p-3 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{row.plain_text_body}</pre>
			</div>
		{/if}
	</div>
{/snippet}

<!-- Edit form body (rich form: subject + html body with edit/preview tabs + plain text + is_active + Reset/Test) -->
{#snippet TemplateForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		{#if editing}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('email_templates.name', 'Name')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{editing.name}</p>
				<p class="mt-0.5 text-xs font-mono" style="color: var(--color-muted)">{editing.slug}</p>
			</div>
		{/if}

		<div>
			<label for="et-subject" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('email_templates.subject', 'Subject')}
			</label>
			<input
				id="et-subject"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.subject}
			/>
		</div>

		<div>
			<label for="et-body" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('email_templates.body', 'Body (HTML)')}
			</label>
			<Tabs
				tabs={[
					{ value: 'edit', label: t('email_templates.tab_edit', 'Edit') },
					{ value: 'preview', label: t('email_templates.tab_preview', 'Preview') },
				]}
				bind:value={bodyTab}
				class="mt-1"
			>
				{#snippet children()}
					<BitsTabs.Content value="edit">
						<textarea
							id="et-body"
							bind:value={value.html_body}
							rows="12"
							class="w-full rounded-lg border p-2 text-sm font-mono"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						></textarea>
					</BitsTabs.Content>
					<BitsTabs.Content value="preview">
						<iframe
							srcdoc={renderPreview(value.html_body ?? '')}
							title="Email body preview"
							class="w-full rounded-lg border"
							style="border-color: var(--color-border); min-height: 300px;"
						></iframe>
					</BitsTabs.Content>
				{/snippet}
			</Tabs>
		</div>

		<div>
			<label for="et-plain" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('email_templates.plain_text', 'Plain Text Body')}
			</label>
			<textarea
				id="et-plain"
				bind:value={value.plain_text_body}
				rows="6"
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			></textarea>
		</div>

		<div class="flex items-center gap-2">
			<input
				id="et-active"
				type="checkbox"
				bind:checked={value.is_active}
				class="h-4 w-4 rounded"
				style="border-color: var(--color-border)"
			/>
			<label for="et-active" class="text-sm font-medium" style="color: var(--color-foreground)">
				{t('common.active', 'Active')}
			</label>
		</div>

		{#if editing && canManageTemplates}
			<div class="flex gap-2 border-t pt-4" style="border-color: var(--color-border)">
				<Button
					variant="outline"
					onclick={() => openReset(editing as EmailTemplate)}
				>
					<RotateCcw class="mr-1 h-4 w-4" />
					{t('email_templates.reset', 'Reset to Default')}
				</Button>
				<Button
					variant="outline"
					onclick={() => openTest(editing as EmailTemplate)}
				>
					<Send class="mr-1 h-4 w-4" />
					{t('email_templates.send_test', 'Send Test')}
				</Button>
			</div>
		{/if}
	</div>
{/snippet}
