<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-spaces.md (creation modal)
	// Modal dialog for creating a new space (POST /spaces). Fetches space types
	// and permission presets on open, then submits the form. On success,
	// navigates to the new space's page.
	import { Dialog } from 'bits-ui';
	import { X, Rocket } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';

	interface SpaceType {
		id: string;
		name: string;
	}

	interface Props {
		open?: boolean;
	}
	let { open = $bindable(false) }: Props = $props();

	let name = $state('');
	let slug = $state('');
	let description = $state('');
	let level = $state('custom');
	let spaceTypeId = $state('');
	let presetName = $state('default');
	let spaceTypes = $state<SpaceType[]>([]);
	let submitting = $state(false);
	let error = $state('');

	// Fetch space types when the modal opens.
	$effect(() => {
		if (open) {
			fetchSpaceTypes();
		}
	});

	async function fetchSpaceTypes() {
		try {
			const res = await api.get<{ data: SpaceType[] }>('/space-types');
			if (res.data?.data) {
				spaceTypes = res.data.data;
				if (spaceTypes.length > 0 && !spaceTypeId) {
					spaceTypeId = spaceTypes[0].id;
				}
			}
		} catch {
			spaceTypes = [];
		}
	}

	// Auto-generate slug from name.
	$effect(() => {
		if (name && !slug) {
			slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
		}
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!name || !slug || !spaceTypeId) {
			error = t('spaces.create_error_fields');
			return;
		}
		submitting = true;
		try {
			const res = await api.post<{ data: { slug: string } }>('/spaces', {
				name,
				slug,
				space_type_id: spaceTypeId,
				level,
				description: description || undefined,
				preset_name: presetName || undefined
			});
			if (res.data?.data?.slug) {
				open = false;
				await goto(`/spaces/${res.data.data.slug}`);
			} else {
				error = t('spaces.create_error');
			}
		} catch {
			error = t('spaces.create_error');
		} finally {
			submitting = false;
		}
	}

	function reset() {
		name = '';
		slug = '';
		description = '';
		level = 'custom';
		spaceTypeId = '';
		presetName = 'default';
		error = '';
	}
</script>

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) reset(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-[480px] -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
			<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]">
				<X size={20} />
			</Dialog.Close>

			<div class="mb-4 flex items-center gap-2">
				<Rocket size={20} class="text-[var(--color-primary)]" />
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('spaces.create_title')}</h2>
			</div>

			{#if spaceTypes.length === 0}
				<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
					{t('spaces.no_types')}
				</p>
			{:else}
				<form onsubmit={submit} class="space-y-3">
					<div>
						<label for="space-name" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.name_label')}</label>
						<input
							id="space-name"
							value={name}
							oninput={(e) => (name = e.currentTarget.value)}
							placeholder={t('spaces.name_placeholder')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						/>
					</div>

					<div>
						<label for="space-slug" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.slug_label')}</label>
						<input
							id="space-slug"
							value={slug}
							oninput={(e) => (slug = e.currentTarget.value)}
							placeholder="my-space"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						/>
					</div>

					<div class="grid grid-cols-2 gap-3">
						<div>
							<label for="space-type" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.type_label')}</label>
							<select
								id="space-type"
								value={spaceTypeId}
								onchange={(e) => (spaceTypeId = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							>
								{#each spaceTypes as st (st.id)}
									<option value={st.id}>{st.name}</option>
								{/each}
							</select>
						</div>
						<div>
							<label for="space-level" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.level_label')}</label>
							<select
								id="space-level"
								value={level}
								onchange={(e) => (level = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							>
								<option value="custom">{t('spaces.level_custom')}</option>
								<option value="country">{t('spaces.level_country')}</option>
								<option value="state">{t('spaces.level_state')}</option>
								<option value="city">{t('spaces.level_city')}</option>
							</select>
						</div>
					</div>

					<div>
						<label for="space-desc" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.description_label')}</label>
						<textarea
							id="space-desc"
							value={description}
							oninput={(e) => (description = e.currentTarget.value)}
							rows={2}
							placeholder={t('spaces.description_placeholder')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						></textarea>
					</div>

					{#if error}
						<p class="text-sm text-red-500" role="alert">{error}</p>
					{/if}

					<div class="flex justify-end gap-2 pt-1">
						<button type="button" onclick={() => (open = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
							{t('spaces.create_cancel')}
						</button>
						<button type="submit" disabled={submitting} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50">
							{submitting ? '...' : t('spaces.create_submit')}
						</button>
					</div>
				</form>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
