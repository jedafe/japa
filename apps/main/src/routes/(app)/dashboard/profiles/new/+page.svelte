<script lang="ts">
	// Ref: japaconnect-engineering/sections/33-professional-profiles.md (profile creation)
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Briefcase, Plus, Loader2 } from 'lucide-svelte';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let name = $state('');
	let slug = $state('');
	let typeId = $state('');
	let tagline = $state('');
	let description = $state('');
	let websiteUrl = $state('');
	let verificationType = $state('');
	let submitting = $state(false);
	let slugTouched = $state(false);

	const pageTitle = $derived(t('profiles.new.title'));
	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	function suggestSlug(value: string): string {
		return value.toLowerCase().trim().replace(/[^a-z0-9\s-]/g, '').replace(/\s+/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '');
	}

	function onNameInput() {
		if (!slugTouched) slug = suggestSlug(name);
	}

	function onSlugInput() {
		slugTouched = true;
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={t('profiles.new.subtitle')} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Briefcase size={20} /> {t('profiles.new.heading')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('profiles.new.subtitle')}</p>
	</div>

	<div class="p-4">
		{#if form?.message}
			<div
				class="mb-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]"
			>
				{form.message === 'upgrade_required' ? t('profiles.new.error_upgrade_required') : form.message}
			</div>
		{/if}

		<form
			method="POST"
			use:enhance={() => {
				submitting = true;
				return async ({ update }) => {
					await update();
					submitting = false;
				};
			}}
			class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<div>
				<label for="name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_name')}</label>
				<input id="name" name="name" type="text" required bind:value={name} oninput={onNameInput} placeholder={t('profiles.new.name_placeholder')} class={inputClass} />
			</div>

			<div>
				<label for="slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_slug')}</label>
				<input id="slug" name="slug" type="text" required bind:value={slug} oninput={onSlugInput} placeholder={t('profiles.new.slug_placeholder')} class={inputClass} />
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('profiles.new.slug_hint')}</p>
			</div>

			<div>
				<label for="type_id" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_type')}</label>
				<select id="type_id" name="type_id" bind:value={typeId} class={inputClass}>
					<option value="">—</option>
					{#each data.profileTypes as pt (pt.id)}
						<option value={pt.id}>{pt.name}</option>
					{/each}
				</select>
			</div>

			<div>
				<label for="tagline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_tagline')}</label>
				<input id="tagline" name="tagline" type="text" bind:value={tagline} placeholder={t('profiles.new.tagline_placeholder')} class={inputClass} />
			</div>

			<div>
				<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_description')}</label>
				<textarea id="description" name="description" rows="4" bind:value={description} placeholder={t('profiles.new.description_placeholder')} class={inputClass}></textarea>
			</div>

			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label for="website_url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_website')}</label>
					<input id="website_url" name="website_url" type="url" bind:value={websiteUrl} placeholder="https://example.com" class={inputClass} />
				</div>
				<div>
					<label for="verification_type" class="mb-1 block text-sm font-medium text-[var(--color-text)]">{t('profiles.new.field_verification')}</label>
					<select id="verification_type" name="verification_type" bind:value={verificationType} class={inputClass}>
						<option value="">—</option>
						<option value="embassy">Embassy</option>
						<option value="business">Business</option>
						<option value="notable">Notable</option>
					</select>
				</div>
			</div>

			<div class="flex justify-end">
				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Plus size={16} />
					{/if}
					{t('profiles.new.submit')}
				</button>
			</div>
		</form>
	</div>
</div>
