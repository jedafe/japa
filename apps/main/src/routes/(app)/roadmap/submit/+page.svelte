<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// Submit-a-request form (auth required — guests are redirected to login by
	// the server load). Posts to /roadmap/submissions which creates a
	// status=pending item for moderator triage. Categories come from
	// /roadmap/board via the load function (Rule 16 — no hardcoded options).
	// The body is encoded as base64 TipTap JSON via encodeReplyBody, matching
	// the §49 comments convention so the detail page renders it rich.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { roadmapErrorKey } from '$lib/enumOptions';
	import { encodeReplyBody } from '$lib/tiptap/serialize';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { CheckCircle, Lightbulb, Send } from 'lucide-svelte';

	let { data } = $props();
	const categories = $derived(data.categories ?? []);

	let title = $state('');
	let categoryId = $state('');
	let summary = $state('');
	let body = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let errorKey = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function validate(): boolean {
		if (!title.trim()) {
			errorKey = 'roadmap.errors.title_required';
			return false;
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!validate()) return;
		loading = true;
		try {
			const res = await api.post<{ id: string }>('/roadmap/submissions', {
				title: title.trim(),
				summary: summary.trim(),
				// Non-empty bodies go out as base64 TipTap JSON (§49 convention);
				// empty stays empty — never send an encoded empty document.
				body: body.trim() ? encodeReplyBody(body) : '',
				category_id: categoryId || ''
			});
			if (res.data) {
				submitted = true;
				title = '';
				categoryId = '';
				summary = '';
				body = '';
			} else {
				// Map backend codes (missing_title, feature_disabled,
				// rate_limit_exceeded, …) to a translated message; unknown
				// codes get the generic failure text.
				errorKey = roadmapErrorKey(res.error?.code);
			}
		} catch {
			errorKey = 'roadmap.errors.submit_failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('roadmap.submit.title', 'Submit a request')} — JapaConnect</title>
	<meta
		name="description"
		content={t('roadmap.submit.subtitle', 'Tell us what you would like to see on JapaConnect.')}
	/>
	<!-- Auth-required form — excluded from /sitemap-roadmap.xml, so mark it
	     noindex to match. -->
	<meta name="robots" content="noindex" />
</svelte:head>

<section class="mx-auto max-w-2xl space-y-6 px-4 py-8">
	<header class="flex items-center gap-3">
		<div
			class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
		>
			<Lightbulb class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">
				{t('roadmap.submit.title', 'Submit a request')}
			</h1>
			<p class="text-sm text-[var(--color-muted)]">
				{t('roadmap.submit.subtitle', 'Tell us what you would like to see on JapaConnect.')}
			</p>
		</div>
	</header>

	{#if submitted}
		<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
			<CheckCircle class="mx-auto h-8 w-8 text-green-600" />
			<p class="mt-3 text-lg font-medium text-green-600">
				{t('roadmap.submit.success', 'Request submitted')}
			</p>
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
				{t('roadmap.submit.success_body', 'Thanks — your request is in the review queue and will appear on the board once approved.')}
			</p>
			<div class="mt-4 flex justify-center gap-2">
				<a
					href="/roadmap"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
				>
					{t('roadmap.back_to_board', 'Back to roadmap')}
				</a>
				<button
					type="button"
					onclick={() => (submitted = false)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('roadmap.submit.submit_another', 'Submit another')}
				</button>
			</div>
		</div>
	{:else}
		<form
			onsubmit={submit}
			class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6"
			novalidate
		>
			<div class="flex flex-col gap-1.5">
				<label for="roadmap-title" class="text-sm font-medium text-[var(--color-text)]">
					{t('roadmap.submit.field_title', 'Title')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<input
					id="roadmap-title"
					type="text"
					bind:value={title}
					placeholder={t('roadmap.submit.field_title_placeholder', 'A short name for your request')}
					class={inputClass}
					required
				/>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="roadmap-category" class="text-sm font-medium text-[var(--color-text)]">
					{t('roadmap.submit.field_category', 'Category')}
				</label>
				<AutocompleteSelect
					id="roadmap-category"
					bind:value={categoryId}
					options={[
						{ value: '', label: t('roadmap.submit.field_category_placeholder', 'Select a category...') },
						...categories.map((c) => ({ value: c.id, label: c.name }))
					]}
				/>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="roadmap-summary" class="text-sm font-medium text-[var(--color-text)]">
					{t('roadmap.submit.field_summary', 'Summary')}
				</label>
				<textarea
					id="roadmap-summary"
					rows={2}
					bind:value={summary}
					placeholder={t('roadmap.submit.field_summary_placeholder', 'One or two sentences shown on the board card')}
					class={inputClass}
				></textarea>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="roadmap-body" class="text-sm font-medium text-[var(--color-text)]">
					{t('roadmap.submit.field_body', 'Details')}
				</label>
				<textarea
					id="roadmap-body"
					rows={6}
					bind:value={body}
					placeholder={t('roadmap.submit.field_body_placeholder', 'Describe the feature, who it helps, and why it matters')}
					class={inputClass}
				></textarea>
			</div>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey)}
				</p>
			{/if}

			<div class="flex justify-end pt-2">
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					<Send class="h-4 w-4" />
					{loading ? t('common.loading') : t('roadmap.submit.submit', 'Submit request')}
				</button>
			</div>
		</form>
	{/if}
</section>
