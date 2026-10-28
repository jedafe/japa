<script lang="ts">
	// Manage slide-over for an extensions-catalog entry — edits the flag's
	// config blob (name, description, phase, rollout %, per-surface
	// visibility/audience/geo rules, show_on_roadmap) via PATCH
	// /control/extensions/{key}. The surfaces map is a whole-map replace on
	// the backend, so the draft always carries all eight surfaces.
	// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §6.2.
	import { Dialog } from 'bits-ui';
	import { X, Loader2, ChevronDown, ChevronUp, Route, Globe, Users } from 'lucide-svelte';
	import Switch from '$lib/components/ui/Switch.svelte';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import {
		SURFACES,
		PHASES,
		SURFACE_STATES,
		type Extension,
		type SurfaceName,
		type SurfaceState
	} from './types';

	interface Props {
		open?: boolean;
		ext: Extension | null;
		/** Delegates the enabled switch — the page owns the destructive
		 * disable confirmation flow. */
		onToggle?: (ext: Extension, checked: boolean) => void;
		/** Optimistically merge these fields into the card before the PATCH
		 * resolves (matches the page's setEnabled optimistic pattern). */
		onOptimistic?: (key: string, patch: Partial<Extension>) => void;
		/** Revert the card to the pre-save snapshot when the PATCH fails. */
		onRevert?: (key: string, prev: Partial<Extension>) => void;
		/** Fired after a successful save; warning is the API's optional
		 * "roadmap_disabled" flag (spec §6.3). */
		onSaved?: (key: string, warning?: string) => void;
		/** Fired after a successful DELETE — the card reverts to registry
		 * default (or disappears for Custom keys). */
		onDeleted?: (key: string) => void;
	}

	let {
		open = $bindable(false),
		ext,
		onToggle,
		onOptimistic,
		onRevert,
		onSaved,
		onDeleted
	}: Props = $props();

	interface DraftSurface {
		state: SurfaceState;
		audience: string[];
		geo_allow: string[];
		geo_deny: string[];
	}

	interface Draft {
		name: string;
		description: string;
		phase: string;
		rollout: number;
		showOnRoadmap: boolean;
		internal: boolean;
		surfaces: Record<SurfaceName, DraftSurface>;
	}

	let draft = $state<Draft | null>(null);
	let seededKey = $state('');
	let saving = $state(false);
	let deleting = $state(false);
	let saveError = $state('');
	let expanded = $state<Set<string>>(new Set());
	let dirtySurfaces = $state<Set<string>>(new Set());
	let storedSurfaces = $state<Set<string>>(new Set());

	// Option lists — fetched once (Rule 16: audience/geo vocabularies come
	// from the API, never hardcoded). Groups require admin.rbac.manage on
	// /api/v1; the control panel is super_admin-only so it resolves for the
	// operators who can open this page.
	interface Option {
		value: string;
		label: string;
		sublabel?: string;
	}

	let groupOptions = $state<Option[]>([]);
	let permOptions = $state<Option[]>([]);
	let countryOptions = $state<Option[]>([]);
	let groupsLoaded = $state(false);
	let permsLoaded = $state(false);
	let countriesLoaded = $state(false);
	let optionsError = $state('');

	let audienceOptions = $derived<Option[]>([...groupOptions, ...permOptions]);

	async function loadOptions() {
		optionsError = '';
		const tasks: Promise<void>[] = [];
		if (!groupsLoaded) {
			tasks.push(
				api<{ groups: { id: string; name: string; slug: string }[]; total: number }>(
					'/api/v1/admin/user-groups'
				).then((groups) => {
					if (groups.ok) {
						groupOptions = (groups.data?.groups ?? []).map((g) => ({
							value: `group:${g.slug}`,
							label: g.name,
							sublabel: `group:${g.slug}`
						}));
						groupsLoaded = true;
					}
				})
			);
		}
		if (!permsLoaded) {
			tasks.push(
				api<{ key: string; description: string }[]>('/control/permission-nodes').then((perms) => {
					if (perms.ok) {
						permOptions = (perms.data ?? []).map((p) => ({
							value: `perm:${p.key}`,
							label: p.key,
							sublabel: p.description
						}));
						permsLoaded = true;
					}
				})
			);
		}
		if (!countriesLoaded) {
			tasks.push(
				api<{ geo_code?: string; name: string }[]>('/api/v1/geo/countries').then((countries) => {
					if (countries.ok) {
						countryOptions = (countries.data ?? [])
							.filter((c) => !!c.geo_code)
							.map((c) => ({ value: c.geo_code!, label: c.name, sublabel: c.geo_code }));
						countriesLoaded = true;
					}
				})
			);
		}
		await Promise.all(tasks);
		// Surface a retry hint when any vocabulary failed to load — an empty
		// audience/country list otherwise looks like "no data".
		if (!groupsLoaded || !permsLoaded || !countriesLoaded) {
			optionsError = t('extensions.options_error', 'Some option lists failed to load.');
		}
	}

	function seed(e: Extension) {
		const surfaces = {} as Record<SurfaceName, DraftSurface>;
		for (const s of SURFACES) {
			const r = e.surfaces?.[s];
			surfaces[s] = {
				state: (r?.state as SurfaceState) ?? 'visible',
				audience: [...(r?.audience ?? [])],
				geo_allow: [...(r?.geo_allow ?? [])],
				geo_deny: [...(r?.geo_deny ?? [])]
			};
		}
		draft = {
			name: e.name,
			description: e.description,
			phase: e.phase ?? (e.enabled ? 'shipped' : 'planned'),
			rollout: e.rollout_percentage ?? 100,
			showOnRoadmap: !!e.show_on_roadmap,
			internal: !!e.internal,
			surfaces
		};
		saveError = '';
		expanded = new Set();
		dirtySurfaces = new Set();
		storedSurfaces = new Set(Object.keys(e.stored_surfaces ?? {}));
	}

	// Seed the draft when the sheet opens for an extension (or when a
	// different extension is picked while open). Poll-driven updates to the
	// same key must NOT reseed — they'd clobber in-progress edits.
	$effect(() => {
		if (open && ext && seededKey !== ext.key) {
			seededKey = ext.key;
			seed(ext);
		}
	});

	$effect(() => {
		if (open) loadOptions();
	});

	function handleOpenChange(o: boolean) {
		if (!o && (saving || deleting)) return;
		open = o;
		if (!o) {
			seededKey = '';
			saveError = '';
		}
	}

	// closeSheet goes through handleOpenChange so seededKey resets and the
	// next open reseeds the draft — setting `open = false` directly would
	// leave a stale draft behind for the same key.
	function closeSheet() {
		handleOpenChange(false);
	}

	let phaseOptions = $derived.by(() => {
		translations();
		return PHASES.map((p) => ({ value: p, label: phaseLabel(p) }));
	});

	function phaseLabel(p: string): string {
		return t(`extensions.phase_${p}`, p);
	}

	function toggleExpanded(surface: string) {
		const next = new Set(expanded);
		if (next.has(surface)) next.delete(surface);
		else next.add(surface);
		expanded = next;
	}

	function ruleFor(surface: string): DraftSurface | null {
		return draft?.surfaces[surface as SurfaceName] ?? null;
	}

	function setState(surface: string, state: SurfaceState) {
		const r = ruleFor(surface);
		if (r) {
			r.state = state;
			dirtySurfaces = new Set([...dirtySurfaces, surface]);
		}
	}

	function addAudience(surface: string, item: AutocompleteItem) {
		const r = ruleFor(surface);
		if (!r || r.audience.includes(item.id)) return;
		r.audience = [...r.audience, item.id];
		dirtySurfaces = new Set([...dirtySurfaces, surface]);
	}

	function removeAudience(surface: string, entry: string) {
		const r = ruleFor(surface);
		if (!r) return;
		r.audience = r.audience.filter((a) => a !== entry);
		dirtySurfaces = new Set([...dirtySurfaces, surface]);
	}

	function addGeo(surface: string, field: 'geo_allow' | 'geo_deny', item: AutocompleteItem) {
		const r = ruleFor(surface);
		if (!r) return;
		const code = item.id.trim().toUpperCase();
		if (!/^[A-Z]{2}$/.test(code) || r[field].includes(code)) return;
		r[field] = [...r[field], code];
		dirtySurfaces = new Set([...dirtySurfaces, surface]);
	}

	function removeGeo(surface: string, field: 'geo_allow' | 'geo_deny', code: string) {
		const r = ruleFor(surface);
		if (!r) return;
		r[field] = r[field].filter((c) => c !== code);
		dirtySurfaces = new Set([...dirtySurfaces, surface]);
	}

	function optionLabel(options: Option[], value: string): string {
		return options.find((o) => o.value === value)?.label ?? value;
	}

	// --- Autocomplete fetchers (chips multi-select, same pattern as
	// api-keys scopes): local filters over the fetched option lists,
	// excluding already-selected entries. ---
	function fetchAudience(q: string, selected: string[]): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return Promise.resolve(
			audienceOptions
				.filter((o) => !selected.includes(o.value))
				.filter(
					(o) =>
						o.label.toLowerCase().includes(query) ||
						o.value.toLowerCase().includes(query) ||
						(o.sublabel ?? '').toLowerCase().includes(query)
				)
				.map((o) => ({ id: o.value, label: o.label, sublabel: o.sublabel }))
		);
	}

	function fetchAudienceInitial(selected: string[]): Promise<AutocompleteItem[]> {
		return fetchAudience('', selected);
	}

	function fetchCountries(q: string, selected: string[]): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return Promise.resolve(
			countryOptions
				.filter((o) => !selected.includes(o.value))
				.filter((o) => o.label.toLowerCase().includes(query) || o.value.toLowerCase().includes(query))
				.map((o) => ({ id: o.value, label: o.label, sublabel: o.sublabel }))
		);
	}

	function fetchCountriesInitial(selected: string[]): Promise<AutocompleteItem[]> {
		return fetchCountries('', selected);
	}

	// Presets (spec §6.2): "Beta testers + staff" scopes the current surface
	// to the beta/staff groups; "Apply to all surfaces" copies the current
	// surface's full rule to every surface.
	function presetBetaStaff(surface: string) {
		const r = ruleFor(surface);
		if (!r) return;
		// Resolve group slugs against the loaded vocabulary so renamed or
		// missing groups never produce dangling entries.
		const slugs = new Set(groupOptions.map((g) => g.value));
		const candidates = groupOptions.some((g) => g.value === 'group:beta_testers')
			? ['group:beta_testers']
			: [];
		for (const s of ['group:staff', 'group:admin', 'group:super_admin']) {
			if (slugs.has(s)) { candidates.push(s); break; }
		}
		r.audience = candidates.filter((c) => slugs.has(c));
		dirtySurfaces = new Set([...dirtySurfaces, surface]);
	}

	function presetApplyAll(surface: string) {
		if (!draft) return;
		const src = draft.surfaces[surface as SurfaceName];
		if (!src) return;
		for (const s of SURFACES) {
			draft.surfaces[s] = {
				state: src.state,
				audience: [...src.audience],
				geo_allow: [...src.geo_allow],
				geo_deny: [...src.geo_deny]
			};
		}
		dirtySurfaces = new Set(SURFACES);
	}

	function restrictionSummary(r: DraftSurface): string {
		const parts: string[] = [];
		if (r.audience.length) parts.push(`${r.audience.length} ${t('extensions.audience_count', 'audience')}`);
		if (r.geo_allow.length) parts.push(`${t('extensions.geo_allow_short', 'allow')}: ${r.geo_allow.join(', ')}`);
		if (r.geo_deny.length) parts.push(`${t('extensions.geo_deny_short', 'deny')}: ${r.geo_deny.join(', ')}`);
		return parts.join(' · ');
	}

	async function save() {
		if (!ext || !draft || saving) return;
		// Pre-save validation — the backend 400s on these, but failing fast
		// keeps the sheet open with the error instead of a bare rejection.
		if (!draft.name.trim()) {
			saveError = t('extensions.name_required', 'Name is required.');
			return;
		}
		const rollout = Math.floor(Number(draft.rollout));
		if (!Number.isFinite(rollout) || rollout < 0 || rollout > 100) {
			saveError = t('extensions.rollout_invalid', 'Rollout must be a whole number between 0 and 100.');
			return;
		}
		if (!PHASES.includes(draft.phase as (typeof PHASES)[number])) {
			saveError = t('extensions.phase_invalid', 'Invalid phase.');
			return;
		}
		saving = true;
		saveError = '';

		// Only send surfaces the operator touched or that carry a
		// non-default rule — untouched plain-visible surfaces stay absent so
		// registry DefaultSurfaces keeps applying (§42 surface merge).
		const surfaces: Record<string, { state: string; audience?: string[]; geo_allow?: string[]; geo_deny?: string[] }> = {};
		for (const s of SURFACES) {
			const r = draft.surfaces[s];
			const nonDefault = r.state !== 'visible' || r.audience.length > 0 || r.geo_allow.length > 0 || r.geo_deny.length > 0;
			// Stored rules are always re-sent — omitting them on a whole-map
			// replace would silently revert an explicit override to the
			// registry default (e.g. stored visible over a hidden default).
			if (!dirtySurfaces.has(s) && !nonDefault && !storedSurfaces.has(s)) continue;
			surfaces[s] = {
				state: r.state,
				...(r.audience.length ? { audience: r.audience } : {}),
				...(r.geo_allow.length ? { geo_allow: r.geo_allow } : {}),
				...(r.geo_deny.length ? { geo_deny: r.geo_deny } : {})
			};
		}

		const patch: Partial<Extension> = {
			name: draft.name.trim(),
			description: draft.description.trim(),
			phase: draft.phase,
			show_on_roadmap: draft.showOnRoadmap,
			internal: draft.internal,
			rollout_percentage: draft.rollout,
			surfaces: surfaces as Extension['surfaces']
		};

	// Deep-clone the revert snapshot: prev.surfaces is a live reference to
	// the extension row, and a background poll can replace the extensions
	// array between onOptimistic and onRevert — a shallow snapshot would
	// revert to already-mutated state.
		const prev: Partial<Extension> = JSON.parse(
			JSON.stringify({
				name: ext.name,
				description: ext.description,
				phase: ext.phase,
				show_on_roadmap: ext.show_on_roadmap,
				internal: ext.internal,
				rollout_percentage: ext.rollout_percentage,
				surfaces: ext.surfaces
			})
		);

		onOptimistic?.(ext.key, patch);

		try {
			const result = await api<{ key: string; status: string; enabled: boolean; warning?: string }>(
				`/control/extensions/${ext.key}`,
				{ method: 'PATCH', body: patch }
			);
			if (result.ok) {
				handleOpenChange(false);
				onSaved?.(ext.key, result.data?.warning);
			} else {
				onRevert?.(ext.key, prev);
				saveError = result.error || t('extensions.save_error', 'Failed to save changes.');
			}
		} catch {
			onRevert?.(ext.key, prev);
			saveError = t('extensions.save_error', 'Failed to save changes.');
		} finally {
			saving = false;
		}
	}

	// remove deletes the stored feature_flags row via DELETE
	// /control/extensions/{key}. For registry keys the flag reverts to its
	// Registry default; custom keys disappear from the catalog entirely.
	async function remove() {
		if (!ext || deleting) return;
		if (!window.confirm(t('extensions.delete_confirm', 'Delete the stored configuration for this flag?')))
			return;
		deleting = true;
		saveError = '';
		try {
			const result = await api(`/control/extensions/${ext.key}`, { method: 'DELETE' });
			if (result.ok) {
				handleOpenChange(false);
				onDeleted?.(ext.key);
			} else {
				saveError = result.error || t('extensions.delete_error', 'Failed to delete flag.');
			}
		} catch {
			saveError = t('extensions.delete_error', 'Failed to delete flag.');
		} finally {
			deleting = false;
		}
	}
</script>

<Dialog.Root bind:open onOpenChange={handleOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 fixed inset-0 z-50 bg-black/60"
		/>
		<Dialog.Content
			data-slideover-content
			class="border-border-card bg-background-alt data-[state=open]:animate-in data-[state=open]:slide-in-from-right data-[state=closed]:animate-out data-[state=closed]:slide-out-to-right fixed inset-y-0 end-0 z-50 flex h-full w-full max-w-[560px] flex-col border-s shadow-xl duration-300"
			onInteractOutside={(e) => {
				// Autocomplete dropdowns portal to document.body to escape this
				// panel's overflow clipping — don't let option clicks dismiss it.
				if ((e.target as HTMLElement | null)?.closest?.('.autocomplete-dropdown')) {
					e.preventDefault();
				}
			}}
		>
			<div class="border-border-card flex items-start justify-between border-b px-5 py-4">
				<div class="min-w-0 pe-6">
					<Dialog.Title class="text-foreground text-base font-semibold">
						{t('extensions.manage_title', 'Manage extension')}
					</Dialog.Title>
					<Dialog.Description class="text-muted-foreground mt-1 truncate text-sm">
						{ext?.name}
						<span class="font-mono text-xs">({ext?.key})</span>
					</Dialog.Description>
				</div>
				<Dialog.Close
					class="text-muted-foreground hover:bg-muted rounded-md p-1 transition-colors"
					aria-label={t('common.close', 'Close')}
				>
					<X class="size-4" />
				</Dialog.Close>
			</div>

			{#if draft && ext}
				<div class="flex-1 overflow-y-auto px-5 py-4">
					<!-- General -->
					<section class="mb-6">
						<h4 class="text-muted-foreground mb-3 text-xs font-semibold uppercase tracking-wide">
							{t('extensions.section_general', 'General')}
						</h4>
						<div class="flex items-center justify-between">
							<span class="text-foreground text-sm font-medium">
								{ext.enabled ? t('extensions.status_enabled', 'Enabled') : t('extensions.status_disabled', 'Disabled')}
							</span>
							<Switch
								id="ext-manage-{ext.key}"
								checked={ext.enabled}
								onCheckedChange={(checked) => onToggle?.(ext, checked)}
							/>
						</div>
						<div class="mt-4">
							<label for="ext-manage-name" class="text-foreground block text-sm font-medium">
								{t('extensions.field_name', 'Name')}
							</label>
							<input
								id="ext-manage-name"
								type="text"
								bind:value={draft.name}
								class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors"
							/>
						</div>
						<div class="mt-4">
							<label for="ext-manage-desc" class="text-foreground block text-sm font-medium">
								{t('extensions.field_description', 'Description')}
							</label>
							<textarea
								id="ext-manage-desc"
								bind:value={draft.description}
								rows="3"
								class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors"
							></textarea>
						</div>
						<p class="text-muted-foreground mt-3 text-xs">
							{ext.category}
						</p>
						<div class="mt-4 flex items-center justify-between gap-4">
							<div class="min-w-0">
								<span class="text-foreground block text-sm font-medium">
									{t('extensions.field_internal', 'Internal flag')}
								</span>
								<p class="text-muted-foreground mt-0.5 text-xs">
									{t('extensions.internal_hint', 'Hide from the public flag map — gating still applies.')}
								</p>
							</div>
							<Switch
								id="ext-manage-internal"
								checked={draft?.internal ?? false}
								onCheckedChange={(checked) => { if (draft) draft.internal = checked; }}
							/>
						</div>
					</section>

					<!-- Rollout -->
					<section class="border-border-card mb-6 border-t pt-5">
						<h4 class="text-muted-foreground mb-3 text-xs font-semibold uppercase tracking-wide">
							{t('extensions.section_rollout', 'Rollout')}
						</h4>
						<div>
							<span class="text-foreground block text-sm font-medium">
								{t('extensions.phase', 'Phase')}
							</span>
							<AutocompleteSelect
								bind:value={draft.phase}
								options={phaseOptions}
								placeholder={t('extensions.phase', 'Phase')}
								ariaLabel={t('extensions.phase', 'Phase')}
								class="mt-1 w-full"
							/>
						</div>
						<div class="mt-4">
							<label for="ext-manage-rollout" class="text-foreground block text-sm font-medium">
								{t('extensions.rollout', 'Rollout')} %
							</label>
							<input
								id="ext-manage-rollout"
								type="number"
								min="1"
								max="100"
								bind:value={draft.rollout}
								class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus mt-1 block w-28 rounded-9px border px-3 py-2 text-sm transition-colors"
							/>
							<p class="text-muted-foreground mt-1 text-xs">
								{t('extensions.rollout_hint', '1-99 = % of users in rollout, 100 = fully on — disable to turn off.')}
							</p>
						</div>
					</section>

					<!-- Visibility -->
					<section class="border-border-card mb-6 border-t pt-5">
						<h4 class="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
							{t('extensions.section_visibility', 'Visibility')}
						</h4>
						<p class="text-muted-foreground mb-3 text-xs">
							{t('extensions.visibility_hint', 'Per-surface visibility and optional audience/geo restrictions.')}
						</p>
						<div class="space-y-2">
							{#each SURFACES as surface (surface)}
								{@const rule = draft.surfaces[surface]}
								{@const summary = restrictionSummary(rule)}
								<div class="border-border-card rounded-9px border">
									<div class="flex flex-wrap items-center gap-2 px-3 py-2.5">
										<span class="text-foreground w-20 shrink-0 font-mono text-sm font-medium">
											{surface}
										</span>
										<div class="bg-muted inline-flex rounded-7px p-0.5">
											{#each SURFACE_STATES as st (st)}
												<button
													type="button"
													class="rounded-5px px-2.5 py-1 text-xs font-medium transition-all {rule.state === st
														? 'bg-background text-foreground shadow-mini'
														: 'text-muted-foreground hover:text-foreground'}"
													onclick={() => setState(surface, st)}
												>
													{t(`extensions.surface_${st}`, st)}
												</button>
											{/each}
										</div>
										<div class="flex-1"></div>
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs transition-colors"
											onclick={() => toggleExpanded(surface)}
											aria-expanded={expanded.has(surface)}
										>
											{t('extensions.surface_restrictions', 'Restrictions')}
											{#if expanded.has(surface)}
												<ChevronUp class="size-3.5" />
											{:else}
												<ChevronDown class="size-3.5" />
											{/if}
										</button>
									</div>
									{#if !expanded.has(surface) && summary}
										<p class="text-muted-foreground border-border-card border-t px-3 py-1.5 text-xs">
											{summary}
										</p>
									{/if}
									{#if expanded.has(surface)}
										<div class="border-border-card space-y-4 border-t px-3 py-3">
											<div>
												<span class="text-foreground flex items-center gap-1.5 text-xs font-medium">
													<Users class="size-3.5" />
													{t('extensions.audience', 'Audience')}
												</span>
												{#if rule.audience.length > 0}
													<div class="mt-1.5 flex flex-wrap gap-1.5">
														{#each rule.audience as entry (entry)}
															<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium">
																{optionLabel(audienceOptions, entry)}
																<button
																	type="button"
																	class="hover:text-destructive ms-0.5 rounded"
																	onclick={() => removeAudience(surface, entry)}
																	aria-label={t('common.remove', 'Remove')}
																>×</button>
															</span>
														{/each}
													</div>
												{/if}
												<div class="mt-1.5">
													<Autocomplete
														placeholder={t('extensions.audience_add', 'Add group or permission...')}
														emptyMessage={t('common.no_results', 'No results')}
														fetcher={(q) => fetchAudience(q, rule.audience)}
														initialFetcher={() => fetchAudienceInitial(rule.audience)}
														selectedId={null}
														onSelect={(item) => addAudience(surface, item)}
														compact
													/>
												</div>
												<p class="text-muted-foreground mt-1 text-xs">
													{t('extensions.audience_hint', 'Restrict to user groups (group:slug) or permission nodes (perm:node). Empty = everyone.')}
												</p>
											</div>

											<div>
												<span class="text-foreground flex items-center gap-1.5 text-xs font-medium">
													<Globe class="size-3.5" />
													{t('extensions.geo_allow', 'Only in countries')}
												</span>
												{#if rule.geo_allow.length > 0}
													<div class="mt-1.5 flex flex-wrap gap-1.5">
														{#each rule.geo_allow as code (code)}
															<span class="bg-muted text-foreground inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium">
																{optionLabel(countryOptions, code)} ({code})
																<button
																	type="button"
																	class="hover:text-destructive ms-0.5 rounded"
																	onclick={() => removeGeo(surface, 'geo_allow', code)}
																	aria-label={t('common.remove', 'Remove')}
																>×</button>
															</span>
														{/each}
													</div>
												{/if}
												<div class="mt-1.5">
													<Autocomplete
														placeholder={t('extensions.geo_add', 'Add country...')}
														emptyMessage={t('common.no_results', 'No results')}
														fetcher={(q) => fetchCountries(q, rule.geo_allow)}
														initialFetcher={() => fetchCountriesInitial(rule.geo_allow)}
														selectedId={null}
														onSelect={(item) => addGeo(surface, 'geo_allow', item)}
														allowFreeText
														compact
													/>
												</div>
											</div>

											<div>
												<span class="text-foreground flex items-center gap-1.5 text-xs font-medium">
													<Globe class="size-3.5" />
													{t('extensions.geo_deny', 'Blocked in countries')}
												</span>
												{#if rule.geo_deny.length > 0}
													<div class="mt-1.5 flex flex-wrap gap-1.5">
														{#each rule.geo_deny as code (code)}
															<span class="bg-destructive/10 text-destructive inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium">
																{optionLabel(countryOptions, code)} ({code})
																<button
																	type="button"
																	class="hover:text-destructive ms-0.5 rounded"
																	onclick={() => removeGeo(surface, 'geo_deny', code)}
																	aria-label={t('common.remove', 'Remove')}
																>×</button>
															</span>
														{/each}
													</div>
												{/if}
												<div class="mt-1.5">
													<Autocomplete
														placeholder={t('extensions.geo_add', 'Add country...')}
														emptyMessage={t('common.no_results', 'No results')}
														fetcher={(q) => fetchCountries(q, rule.geo_deny)}
														initialFetcher={() => fetchCountriesInitial(rule.geo_deny)}
														selectedId={null}
														onSelect={(item) => addGeo(surface, 'geo_deny', item)}
														allowFreeText
														compact
													/>
												</div>
												<p class="text-muted-foreground mt-1 text-xs">
													{t('extensions.geo_hint', 'ISO alpha-2 codes. Empty allow-list = all countries; deny always wins.')}
												</p>
											</div>

											<div class="border-border-card flex flex-wrap gap-2 border-t pt-3">
												<button
													type="button"
													class="border-border-input bg-background hover:bg-muted text-foreground rounded-7px border px-2.5 py-1 text-xs font-medium transition-colors"
													onclick={() => presetBetaStaff(surface)}
												>
													{t('extensions.preset_beta_staff', 'Beta testers + staff')}
												</button>
												<button
													type="button"
													class="border-border-input bg-background hover:bg-muted text-foreground rounded-7px border px-2.5 py-1 text-xs font-medium transition-colors"
													onclick={() => presetApplyAll(surface)}
												>
													{t('extensions.preset_apply_all', 'Apply to all surfaces')}
												</button>
											</div>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					</section>

					<!-- Roadmap -->
					<section class="border-border-card mb-2 border-t pt-5">
						<h4 class="text-muted-foreground mb-3 text-xs font-semibold uppercase tracking-wide">
							{t('extensions.section_roadmap', 'Roadmap')}
						</h4>
						<div class="flex items-center justify-between">
							<span class="text-foreground text-sm font-medium">
								{t('extensions.show_on_roadmap', 'Show on roadmap')}
							</span>
							<Switch id="ext-manage-roadmap" bind:checked={draft.showOnRoadmap} />
						</div>
						<p class="text-muted-foreground mt-1 text-xs">
							{t('extensions.show_on_roadmap_hint', 'List this feature on the public roadmap board.')}
						</p>
						<div class="mt-3 flex items-center gap-2 text-sm">
							<Route class="text-muted-foreground size-4" />
							{#if ext.linked_roadmap_slug}
								<span class="text-muted-foreground">{t('extensions.linked_roadmap', 'Linked roadmap item')}:</span>
								<code class="bg-muted text-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">
									{ext.linked_roadmap_slug}
								</code>
							{:else}
								<span class="text-muted-foreground">{t('extensions.no_roadmap_link', 'No roadmap item linked')}</span>
							{/if}
						</div>
						<p class="text-muted-foreground mt-1 text-xs">
							{t('extensions.roadmap_link_hint', 'Link or unlink items from the admin roadmap editor.')}
						</p>
					</section>
				</div>

				<div class="border-border-card border-t px-5 py-3">
					{#if optionsError}
						<p class="text-destructive mb-2 text-sm">{optionsError} <button type="button" class="underline" onclick={loadOptions}>{t('common.retry', 'Retry')}</button></p>
					{/if}
					{#if saveError}
						<p class="text-destructive mb-2 text-sm">{saveError}</p>
					{/if}
					<div class="flex items-center gap-3">
						<button
							type="button"
							class="text-destructive hover:bg-destructive/10 me-auto flex items-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
							disabled={saving || deleting}
							onclick={remove}
							title={ext.category === 'Custom'
								? t('extensions.delete_custom_hint', 'Delete this custom flag row')
								: t('extensions.reset_hint', 'Delete the stored row — reverts to the registry default')}
						>
							{#if deleting}
								<Loader2 class="size-4 animate-spin" />
							{/if}
							{ext.category === 'Custom'
								? t('extensions.delete_flag', 'Delete flag')
								: t('extensions.reset_default', 'Reset to default')}
						</button>
						<div class="flex-1"></div>
						<button
							type="button"
							class="border-border-input bg-background hover:bg-muted text-foreground rounded-md border px-4 py-2 text-sm font-medium transition-colors"
							disabled={saving}
							onclick={closeSheet}
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							class="bg-foreground text-background flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
							disabled={saving}
							onclick={save}
						>
							{#if saving}
								<Loader2 class="size-4 animate-spin" />
								{t('common.saving', 'Saving...')}
							{:else}
								{t('common.save', 'Save')}
							{/if}
						</button>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
