<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Building2, Plus, Loader2 } from 'lucide-svelte';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let name = $state('');
	let slug = $state('');
	let tagline = $state('');
	let description = $state('');
	let websiteUrl = $state('');
	let countryCode = $state('');
	let orgType = $state('business');
	let submitting = $state(false);
	let slugTouched = $state(false);

	const pageTitle = $derived(t('organizations.new.title', 'New organization — JapaConnect'));
	const pageDescription = $derived(
		t('organizations.new.description', 'Create a new organization on JapaConnect.')
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	// Auto-suggest slug from name unless the user has manually edited the slug.
	function suggestSlug(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onNameInput() {
		if (!slugTouched) {
			slug = suggestSlug(name);
		}
	}

	function onSlugInput() {
		slugTouched = true;
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Building2 size={20} /> {t('organizations.new.heading', 'New organization')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('organizations.new.subtitle', 'Create a new organization on JapaConnect.')}
		</p>
	</div>

	<div class="p-3">
		{#if form?.message}
			<div
				class="mb-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]"
			>
				{form.message}
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
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<div class="space-y-4">
				<div>
					<label for="name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_name', 'Name')}
					</label>
					<input
						id="name"
						name="name"
						type="text"
						required
						bind:value={name}
						oninput={onNameInput}
						placeholder={t('organizations.new.name_placeholder', 'Organization name')}
						class={inputClass}
					/>
				</div>

				<div>
					<label for="slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_slug', 'Slug')}
					</label>
					<input
						id="slug"
						name="slug"
						type="text"
						required
						bind:value={slug}
						oninput={onSlugInput}
						placeholder={t('organizations.new.slug_placeholder', 'organization-slug')}
						class={inputClass}
					/>
					<p class="mt-1 text-xs text-[var(--color-muted)]">
						{t('organizations.new.slug_hint', 'Used in the URL: /org/your-slug')}
					</p>
				</div>

				<div>
					<label for="tagline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_tagline', 'Tagline')}
					</label>
					<input
						id="tagline"
						name="tagline"
						type="text"
						bind:value={tagline}
						placeholder={t('organizations.new.tagline_placeholder', 'A short tagline')}
						class={inputClass}
					/>
				</div>

				<div>
					<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_description', 'Description')}
					</label>
					<textarea
						id="description"
						name="description"
						rows="4"
						bind:value={description}
						placeholder={t('organizations.new.description_placeholder', 'Describe the organization…')}
						class={inputClass}
					></textarea>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<div>
						<label for="website_url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('organizations.new.field_website', 'Website URL')}
						</label>
						<input
							id="website_url"
							name="website_url"
							type="url"
							bind:value={websiteUrl}
							placeholder="https://example.com"
							class={inputClass}
						/>
					</div>

					<div>
						<label for="country_code" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('organizations.new.field_country', 'Country code')}
						</label>
						<input
							id="country_code"
							name="country_code"
							type="text"
							maxlength="2"
							bind:value={countryCode}
							placeholder="US"
							class={inputClass}
						/>
					</div>
				</div>

				<div>
					<label for="org_type" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_type', 'Organization type')}
					</label>
					<select id="org_type" name="org_type" bind:value={orgType} class={inputClass}>
						<option value="business">{t('organizations.new.type_business', 'Business')}</option>
						<option value="ngo">{t('organizations.new.type_ngo', 'NGO')}</option>
						<option value="government">{t('organizations.new.type_government', 'Government')}</option>
						<option value="embassy">{t('organizations.new.type_embassy', 'Embassy')}</option>
						<option value="university">{t('organizations.new.type_university', 'University')}</option>
						<option value="media">{t('organizations.new.type_media', 'Media')}</option>
						<option value="community">{t('organizations.new.type_community', 'Community')}</option>
					</select>
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
						{t('organizations.new.submit', 'Create organization')}
					</button>
				</div>
			</div>
		</form>
	</div>
</div>
