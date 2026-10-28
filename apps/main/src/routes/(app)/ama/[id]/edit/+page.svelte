<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ArrowLeft, Send, Loader2, Globe } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import { decodeTipTapDoc, tiptapToText, encodeTiptapDoc } from '$lib/tiptap/serialize';

	let { data }: { data: PageData } = $props();

	const session = $derived(data.session);

	// Decode the base64 TipTap body to plain text for the edit textarea.
	const decodedBodyText = $derived(tiptapToText(decodeTipTapDoc(session.body)));
	// Helper to parse ama_config metadata so we can display read-only host details.
	const amaConfig = $derived(
		session.ama_config && typeof session.ama_config === 'object' ? (session.ama_config as Record<string, unknown>) : {}
	);

	let title = $state(untrack(() => session.title));
	let body = $state(untrack(() => decodedBodyText));
	let isAnonymous = $state(untrack(() => session.is_anonymous ?? false));

	let loading = $state(false);
	let errorKey = $state('');
	let errorCode = $state('');

	const canSubmit = $derived(title.trim().length > 0 && !loading);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none';

	function buildTipTapDoc(text: string): ReturnType<typeof encodeTiptapDoc> {
		const cleaned = text.replace(/\r\n/g, '\n');
		const paragraphs = cleaned.split('\n').map((line) => ({
			type: 'paragraph',
			content: line.length ? [{ type: 'text', text: line }] : []
		}));
		return encodeTiptapDoc({ type: 'doc', content: paragraphs });
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!canSubmit) return;
		loading = true;
		try {
			const res = await api.patch<{ id: string }>(`/forum/ama/${session.id}`, {
				title: title.trim(),
				body: buildTipTapDoc(body.trim()),
				is_anonymous: isAnonymous
			});
			if (res.data?.id) {
				loading = false;
				await goto(`/ama/${res.data.id}`);
			} else if (res.error) {
				errorCode = res.error.code ?? '';
				errorKey = errorCode === 'ama_active_exists' ? 'ama.errors.active_exists' : errorCode === 'ama_cooldown' ? 'ama.errors.cooldown' : 'ama.errors.edit_failed';
				loading = false;
			} else {
				errorKey = 'ama.errors.edit_failed';
				loading = false;
			}
		} catch {
			errorKey = 'ama.errors.edit_failed';
			loading = false;
		}
	}

	const hostDestName = $derived(
		typeof amaConfig.host_dest_country_id === 'string' && amaConfig.host_dest_country_id
			? (data.countries ?? []).find((c) => c.id === amaConfig.host_dest_country_id)?.name ?? amaConfig.host_dest_country_id
			: t('common.unknown', 'Unknown')
	);
</script>

<svelte:head>
	<title>{t('ama.edit_title', 'Edit AMA — {site}')}</title>
	<meta name="description" content={t('ama.edit_description', 'Update your AMA session title and introduction.')} />
	<meta name="robots" content="noindex" />
	<meta property="og:title" content={t('ama.edit_title', 'Edit AMA')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-auto flex max-w-2xl items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/ama/{session.id}"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('ama.back', 'Back to AMA')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('ama.edit_heading', 'Edit AMA')}
		</h1>
	</div>

	<form onsubmit={submit} class="mx-auto max-w-2xl space-y-5 p-4" novalidate>
		<!-- Title -->
		<div class="flex flex-col gap-1.5">
			<label for="title" class="text-sm font-medium text-[var(--color-text)]">
				{t('ama.field_title', 'AMA title')} <span class="text-red-500">*</span>
			
			<HintIcon text={t('ama.title_hint', 'Format: Ask a [profession] who [did the thing].')}  />
</label>
			<input
				id="title"
				type="text"
				value={title}
				oninput={(e) => (title = e.currentTarget.value)}
				placeholder={t('ama.placeholder_title', 'e.g. Ask a Nurse who relocated to the UK in 2021')}
				class={inputClass}
				required
			/>
		</div>

		<!-- Body / intro -->
		<AiBodyField
			id="body"
			label={t('ama.field_intro', 'Introduction')}
			contentType="ama"
			{title}
			body={body}
			onBody={(v) => (body = v)}
			onTitle={(v) => (title = v)}
			user={data.user}
			features={data.features ?? {}}
			placeholder={t('ama.placeholder_intro', 'Introduce yourself and the kind of questions you can answer…')}
			rows={5}
		/>
			<HintIcon text={t('ama.intro_hint', 'Your background and the topics you can answer — builds trust.')} />

		<!-- Anonymous -->
		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input
				type="checkbox"
				checked={isAnonymous}
				onchange={(e) => (isAnonymous = e.currentTarget.checked)}
				class="h-4 w-4 accent-[var(--color-primary)]"
			/>
			{t('ama.field_anonymous', 'Post anonymously')}
		</label>

		<!-- Read-only host metadata (preserved from the original AMA config) -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
			<h2 class="mb-2 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
				<Globe size={16} /> {t('ama.host_info', 'Host details')}
			</h2>
			<dl class="grid grid-cols-2 gap-2 text-sm">
				<div>
					<dt class="text-[var(--color-muted)]">{t('ama.field_profession', 'Profession')}</dt>
					<dd class="text-[var(--color-text)]">{(amaConfig.host_profession as string) ?? t('common.not_set', 'Not set')}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted)]">{t('ama.field_years_abroad', 'Years abroad')}</dt>
					<dd class="text-[var(--color-text)]">{(amaConfig.host_years_abroad as number)?.toString() ?? t('common.not_set', 'Not set')}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted)]">{t('ama.field_destination', 'Destination country')}</dt>
					<dd class="text-[var(--color-text)]">{hostDestName}</dd>
				</div>
				<div>
					<dt class="text-[var(--color-muted)]">{t('ama.field_duration', 'Duration (hours)')}</dt>
					<dd class="text-[var(--color-text)]">{(amaConfig.open_duration_hours as number)?.toString() ?? t('common.not_set', 'Not set')}</dd>
				</div>
			</dl>
		</div>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey, 'Failed to update your AMA. Please try again.')}
			</p>
		{/if}

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
			<a
				href="/ama/{session.id}"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if loading}
					<Loader2 size={16} class="animate-spin" />
				{:else}
					<Send class="rtl:-scale-x-100" size={16} />
				{/if}
				{loading ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
			</button>
		</div>
	</form>
</div>
