<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Briefcase, Lock, Loader2, Save, ArrowLeft } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import CustomFieldGroupCmp from '$lib/components/create/panel/CustomFieldGroup.svelte';
	import JobGeoFields from '$lib/components/jobs/JobGeoFields.svelte';
	import type { JobDetail } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('jobs.create_title', 'Post a job'));
	const description = $derived(t('jobs.create_description', 'Create a new job listing for your organization.'));

	let submitting = $state(false);
	let error = $state<string | null>(null);
	let titleVal = $state('');
	let bodyVal = $state('');
	let orgId = $state<string>(untrack(() => data.organizations[0]?.id ?? ''));
	const orgOptions = $derived([
		{ value: '', label: `— ${t('jobs.select_org', 'Select organization')} —` },
		...data.organizations.map((o) => ({ value: o.id, label: o.name }))
	]);
	let fieldValues = $state<Record<string, unknown>>({});

	// country/city are rendered by the geo-space cascade (JobGeoFields), not
	// the generic custom-field inputs — exclude them here.
	const nonGeoFields = $derived(
		data.fieldGroup?.fields.filter((f) => f.key !== 'country' && f.key !== 'city') ?? []
	);
	const hasGeoFields = $derived(
		data.fieldGroup?.fields.some((f) => f.key === 'country' || f.key === 'city') ?? false
	);

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
		const res = await api.post<JobDetail>(`/org/${encodeURIComponent(orgId)}/jobs`, {
			title: titleVal.trim(),
			body: bodyVal.trim(),
			field_values: toStringFieldValues(fieldValues)
		});

		if (res.error) {
			submitting = false;
			error = res.error.message ?? t('jobs.error.create_failed', 'Failed to create job listing.');
			return;
		}

		await goto(`/jobs/${res.data?.slug ?? ''}`);
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="w-full p-4">
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
				<SelectField
					id="job-org"
					value={orgId}
					options={orgOptions}
					onValueChange={(v) => (orgId = v)}
				/>
			</div>

			{#if data.fieldGroup && (nonGeoFields.length > 0 || hasGeoFields)}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{data.fieldGroup.name}</h2>
					{#if nonGeoFields.length > 0}
						<CustomFieldGroupCmp
							fields={nonGeoFields}
							values={fieldValues}
							onvalues={(v) => { fieldValues = v; }}
							onedit={markDirty}
						/>
					{/if}
					{#if hasGeoFields}
						<div class:mt-3={nonGeoFields.length > 0}>
							<JobGeoFields
								values={fieldValues}
								onvalues={(v) => { fieldValues = v; }}
								onedit={markDirty}
							/>
						</div>
					{/if}
				</div>
			{/if}

			<div class="flex items-center justify-between pt-2">
				<a
					href="/jobs"
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
						{t('jobs.create_cta', 'Post job')}
					{/if}
				</button>
			</div>
		</form>
	{/if}
</div>
