<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Briefcase, Lock, Loader2, Save, ArrowLeft } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { EmptyState, AutocompleteSelect } from '$lib/components/ui';
	import CustomFieldGroupCmp from '$lib/components/create/panel/CustomFieldGroup.svelte';
	import JobGeoFields from '$lib/components/jobs/JobGeoFields.svelte';
	import JobLogoUpload from '$lib/components/jobs/JobLogoUpload.svelte';
	import type { JobDetail } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const job = $derived(data.job);
	const title = $derived(t('jobs.edit_title', 'Edit job'));
	const description = $derived(t('jobs.edit_description', 'Update this job listing for your organization.'));

	let submitting = $state(false);
	let error = $state<string | null>(null);
	let titleVal = $state(untrack(() => job.title));
	let bodyVal = $state(untrack(() => job.body));
	let orgId = $state<string>(untrack(() => job.organization_id ?? data.organizations[0]?.id ?? ''));
	let fieldValues = $state<Record<string, unknown>>(untrack(() => {
		// field_values arrive as strings; coerce boolean-typed fields so the
		// checkbox binding (values[key] === true) reflects stored 'true'.
		const vals: Record<string, unknown> = { ...job.field_values };
		for (const f of data.fieldGroup?.fields ?? []) {
			if (f.field_type === 'boolean' && typeof vals[f.key] === 'string') {
				vals[f.key] = vals[f.key] === 'true';
			}
		}
		// skills_required stores comma-joined text; the repeater needs string[].
		if (typeof vals.skills_required === 'string') {
			vals.skills_required = (vals.skills_required as string)
				.split(',')
				.map((s) => s.trim())
				.filter(Boolean);
		}
		return vals;
	}));

	// country/city are rendered by the geo-space cascade (JobGeoFields) and
	// company_logo by the dedicated media upload (JobLogoUpload) — exclude
	// them from the generic custom-field inputs. company_name/company_website
	// sit beside the logo on one identity row, and all boolean toggles share
	// a single row so the form doesn't scatter.
	const isManagedKey = (k: string) =>
		k === 'country' || k === 'city' || k === 'company_logo' || k === 'company_name' || k === 'company_website';

	const companyFields = $derived(
		data.fieldGroup?.fields.filter((f) => f.key === 'company_name' || f.key === 'company_website') ?? []
	);
	const boolFields = $derived(
		data.fieldGroup?.fields.filter((f) => f.field_type === 'boolean' && f.key !== 'is_featured') ?? []
	);
	const mainFields = $derived(
		data.fieldGroup?.fields
			.filter((f) => !isManagedKey(f.key) && f.field_type !== 'boolean')
			// Salary currency is a dropdown fed by the managed currency list
			// (Control → Currencies) — the stored value stays the code (e.g. USD).
			// Skills required is a repeater; string[] serializes to the same
			// comma-joined value_text the textarea produced (toStringFieldValues).
			.map((f) =>
				f.key === 'salary_currency'
					? { ...f, field_type: 'select', options: { source: 'wallet_currencies' } }
					: f.key === 'skills_required'
						? { ...f, field_type: 'repeater', placeholder: t('jobs.skill_placeholder', 'e.g. TypeScript') }
						: f
			) ?? []
	);
	const hasGeoFields = $derived(
		data.fieldGroup?.fields.some((f) => f.key === 'country' || f.key === 'city') ?? false
	);
	const hasLogoField = $derived(
		data.fieldGroup?.fields.some((f) => f.key === 'company_logo') ?? false
	);
	const hasCompanyRow = $derived(hasLogoField || companyFields.length > 0);

	function markDirty() {
		// no-op kept for CustomFieldGroupCmp callback interface
	}

	function toStringFieldValues(values: Record<string, unknown>): Record<string, string> {
		const out: Record<string, string> = {};
		for (const [k, v] of Object.entries(values)) {
			if (v == null) continue;
			if (typeof v === 'boolean') out[k] = v ? 'true' : 'false';
			else if (typeof v === 'number') out[k] = String(v);
			else if (Array.isArray(v)) out[k] = v.join(',');
			else out[k] = String(v);
		}
		return out;
	}

	function validate(): string | null {
		if (!titleVal.trim()) return t('jobs.error.title_required', 'Job title is required.');
		if (!orgId) return t('jobs.error.org_required', 'Please select an organization.');
		if (data.fieldGroup) {
			for (const f of data.fieldGroup.fields) {
				if (f.is_required && (fieldValues[f.key] == null || fieldValues[f.key] === '')) {
					return t('jobs.error.field_required', '{field} is required.').replace('{field}', f.label);
				}
			}
		}
		return null;
	}

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		error = validate();
		if (error || submitting) return;

		submitting = true;
		const res = await api.patch<JobDetail>(
			`/org/${encodeURIComponent(orgId)}/jobs/${job.id}`,
			{
				title: titleVal.trim(),
				body: bodyVal.trim(),
				field_values: toStringFieldValues(fieldValues)
			}
		);

		if (res.error) {
			submitting = false;
			error = res.error.message ?? t('jobs.error.edit_failed', 'Failed to update job listing.');
			return;
		}

		await goto(`/jobs/${res.data?.slug ?? job.slug}`);
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-4">
	<div class="mb-4 flex items-center gap-2 border-b border-[var(--color-border)] pb-3">
		<Briefcase size={20} />
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{title}</h1>
	</div>

	{#if data.disabled}
		<EmptyState
			icon={Lock}
			title={t('tools.disabled_title', 'This tool is currently unavailable')}
			body={t('tools.disabled_body', 'Check back soon or contact support.')}
		/>
	{:else if data.organizations.length === 0}
		<EmptyState
			icon={Briefcase}
			title={t('jobs.no_org_title', 'No eligible organization')}
			body={t('jobs.no_org_body', 'You need to be an owner, admin, or editor of an organization to post a job.')}
		/>
	{:else}
		<form onsubmit={(e) => onsubmit(e)} class="space-y-5">
			{#if error}
				<div class="rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{error}
				</div>
			{/if}

			<div class="space-y-2">
				<label for="job-title" class="text-sm font-medium text-[var(--color-text)]">
					{t('jobs.job_title', 'Job title')} <span class="text-[var(--destructive)]">*</span>
				</label>
				<input
					id="job-title"
					type="text"
					bind:value={titleVal}
					placeholder={t('jobs.title_placeholder', 'e.g. Senior Software Engineer')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
			</div>

			<div class="space-y-2">
				<label for="job-body" class="text-sm font-medium text-[var(--color-text)]">
					{t('jobs.body', 'Description')}
				</label>
				<textarea
					id="job-body"
					rows="5"
					bind:value={bodyVal}
					placeholder={t('jobs.body_placeholder', 'Describe the role, responsibilities, and requirements…')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				></textarea>
			</div>

			<div class="space-y-2">
				<label for="job-org" class="text-sm font-medium text-[var(--color-text)]">
					{t('jobs.organization', 'Organization')} <span class="text-[var(--destructive)]">*</span>
				</label>
				<AutocompleteSelect
					id="job-org"
					bind:value={orgId}
					disabled={true}
					options={(data.organizations ?? []).map((org) => ({ value: org.id, label: org.name }))}
				/>
				<p class="text-xs text-[var(--color-muted)]">{t('jobs.org_readonly', 'The organization for an existing job cannot be changed.')}</p>
			</div>

			{#if data.fieldGroup && (mainFields.length > 0 || hasGeoFields || hasCompanyRow || boolFields.length > 0)}
				<div class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="text-sm font-semibold text-[var(--color-text)]">{data.fieldGroup.name}</h2>
					{#if hasCompanyRow}
						<div class="flex flex-wrap items-start gap-3">
							{#if hasLogoField}
								<JobLogoUpload
									values={fieldValues}
									onvalues={(v) => { fieldValues = v; }}
									onedit={markDirty}
								/>
							{/if}
							{#if companyFields.length > 0}
								<div class="min-w-0 flex-1">
									<CustomFieldGroupCmp
										fields={companyFields}
										values={fieldValues}
										onvalues={(v) => { fieldValues = v; }}
										onedit={markDirty}
									/>
								</div>
							{/if}
						</div>
					{/if}
					{#if mainFields.length > 0}
						<CustomFieldGroupCmp
							fields={mainFields}
							values={fieldValues}
							onvalues={(v) => { fieldValues = v; }}
							onedit={markDirty}
						/>
					{/if}
					{#if hasGeoFields}
						<JobGeoFields
							values={fieldValues}
							onvalues={(v) => { fieldValues = v; }}
							onedit={markDirty}
						/>
					{/if}
					{#if boolFields.length > 0}
						<CustomFieldGroupCmp
							fields={boolFields}
							values={fieldValues}
							onvalues={(v) => { fieldValues = v; }}
							onedit={markDirty}
						/>
					{/if}
				</div>
			{/if}

			<div class="flex items-center justify-between pt-2">
				<a
					href="/jobs/{job.slug}"
					class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					<ArrowLeft size={16} /> {t('common.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
						{t('common.saving', 'Saving…')}
					{:else}
						<Save size={16} />
						{t('common.save', 'Save')}
					{/if}
				</button>
			</div>
		</form>
	{/if}
</div>
