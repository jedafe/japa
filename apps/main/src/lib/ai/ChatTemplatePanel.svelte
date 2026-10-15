<script lang="ts">
	// ChatTemplatePanel — the /ai right rail: saved generation-spec presets
	// plus a Prompt|Structure editor, in the PromptRightPanel idiom.
	//
	// Ownership split: the panel owns editor UI + list interactions and
	// binds `values`/`activeId`/`active` upward; the parent (JapaAIChat)
	// owns persistence (onpersist → PUT /me/ai-preferences) and send-time
	// spec injection.
	//
	// "active" means the spec auto-attaches to every outgoing message;
	// "Insert" drops a readable version into the composer instead and
	// detaches so a send doesn't carry the spec twice.
	import { Tabs } from 'bits-ui';
	import {
		Plus,
		Trash2,
		X,
		Check,
		Loader2,
		Search,
		Ellipsis,
		Pencil,
		CornerDownLeft,
		Folder,
		ChevronDown,
		ChevronRight
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { AutocompleteSelect, Modal } from '$lib/components/ui';
	// Shared option vocabularies + the composer's own Journey/Settings
	// pickers — same components the studio prompt rail embeds (Rule 16).
	import { TONE_OPTIONS, FRAMEWORK_OPTIONS, HUMANIZE_OPTIONS, type SelectOption } from '$lib/studio/types';
	import type { MeSpace, BlogSeries, ContentTypeMeta, CustomFieldGroup } from '$lib/types';
	import { isHighStakesCategory } from '$lib/studio/validation';
	import { api } from '$lib/api/client';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import SpaceSelector from '$lib/components/create/SpaceSelector.svelte';
	import TagInput from '$lib/components/create/TagInput.svelte';
	import SeriesSelector from '$lib/components/create/panel/SeriesSelector.svelte';
	import CreateJourneyPanel from '$lib/components/create/panel/CreateJourneyPanel.svelte';
	import {
		DEFAULT_CHAT_TEMPLATE_VALUES,
		normalizeTemplateFolder,
		normalizeTemplateValues,
		templateFingerprint,
		type ChatTemplate,
		type ChatTemplateFramework,
		type ChatTemplateHumanize,
		type ChatTemplateValues
	} from './chatTemplate';

	interface Props {
		templates: ChatTemplate[];
		values: ChatTemplateValues;
		activeId: string;
		active: boolean;
		/** GET /content-types option list (has_category/categories drive
		 * the guide-category selector) — empty renders a text input. */
		contentTypes: ContentTypeMeta[];
		/** Member spaces + blog series for the Settings tab selectors —
		 * bindable because the selectors append freshly created rows. */
		spaces: MeSpace[];
		series: BlogSeries[];
		/** GET /assessment dimensions — Journey tab's roadmap pickers. */
		assessmentDimensions: string[];
		/** /ai/voices catalog — the persona voice the reply renders with;
		 * empty hides the Voice field (same rule as the composer picker). */
		voices: { id: string; name: string }[];
		maxTags: number;
		/** /content/{status,visibility}-options — live catalogs (Rule 16). */
		statusOptions: SelectOption[];
		visibilityOptions: SelectOption[];
		/** Persist a full template list server-side; resolves to an error
		 * message or null on success. The parent reassigns `templates`. */
		onpersist: (next: ChatTemplate[]) => Promise<string | null>;
		/** Insert a readable version of a spec into the composer input —
		 * row menus pass their own template; the editor footer passes
		 * nothing and the parent uses the working copy. */
		oninsert: (tpl?: ChatTemplate) => void;
		onclose?: () => void;
	}

	let {
		templates,
		values = $bindable(),
		activeId = $bindable(),
		active = $bindable(),
		contentTypes,
		spaces = $bindable(),
		series = $bindable(),
		assessmentDimensions,
		voices,
		maxTags,
		statusOptions,
		visibilityOptions,
		onpersist,
		oninsert,
		onclose
	}: Props = $props();

	let saveName = $state('');
	let saving = $state(false);
	let saveErr = $state('');

	// The rail switches between two surfaces — the searchable template
	// list and the spec editor. Picking a row or starting a new template
	// lands on the editor; "Templates" switches back to browse.
	let surface = $state<'list' | 'edit'>('list');
	let templateQuery = $state('');
	// Bound (not fixed) so a Structure/Settings/Journey tab survives a
	// list↔edit surface switch — Tabs.Root remounts, the state doesn't.
	let editTab = $state('prompt');
	const filteredTemplates = $derived.by(() => {
		const q = templateQuery.trim().toLowerCase();
		if (!q) return templates;
		return templates.filter(
			(tpl) =>
				tpl.name.toLowerCase().includes(q) ||
				(tpl.folder ?? '').toLowerCase().includes(q) ||
				(tpl.values?.content_type ?? '').toLowerCase().includes(q)
		);
	});

	// Folder grouping — folders exist only as template.folder paths, so
	// the tree is derived (a rename/delete is a path rewrite across the
	// member rows). Templates on an intermediate path render under that
	// folder's header, above its subfolders.
	interface TplFolderNode {
		path: string;
		name: string;
		templates: ChatTemplate[];
		children: Map<string, TplFolderNode>;
	}
	const folderTree = $derived.by(() => {
		const roots = new Map<string, TplFolderNode>();
		const ungrouped: ChatTemplate[] = [];
		for (const tpl of filteredTemplates) {
			if (!tpl.folder) {
				ungrouped.push(tpl);
				continue;
			}
			let level = roots;
			let node: TplFolderNode | undefined;
			let prefix = '';
			for (const seg of tpl.folder.split('/')) {
				const full = prefix ? `${prefix}/${seg}` : seg;
				node = level.get(full);
				if (!node) {
					node = { path: full, name: seg, templates: [], children: new Map() };
					level.set(full, node);
				}
				level = node.children;
				prefix = full;
			}
			node?.templates.push(tpl);
		}
		return { roots, ungrouped };
	});
	const sortedNodes = (m: Map<string, TplFolderNode>) =>
		[...m.values()].sort((a, b) => a.name.localeCompare(b.name));

	// Folder header count covers the whole subtree, not just direct rows.
	function countTemplates(node: TplFolderNode): number {
		let n = node.templates.length;
		for (const c of node.children.values()) n += countTemplates(c);
		return n;
	}

	// Collapsed folders keyed by path; menus tracked by template id /
	// folder path (same click-away idiom as the conversation sidebar).
	let collapsedFolders = $state<Set<string>>(new Set());
	let tplMenuFor = $state<string | null>(null);
	let folderMenuFor = $state<string | null>(null);
	// Menus are position:fixed anchored to the trigger's viewport rect —
	// an absolute menu inside the overflow-y-auto list clips at the
	// scroll edge (same flyout approach as the sidebar's project menu).
	let menuPos = $state<{ top: number; left: number } | null>(null);
	function anchorMenu(e: MouseEvent, h = 116) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		menuPos = {
			top: r.bottom + h + 8 > window.innerHeight ? r.top - h - 4 : r.bottom + 4,
			left: Math.max(8, r.right - 176)
		};
	}
	function closeMenus() {
		tplMenuFor = null;
		folderMenuFor = null;
		menuPos = null;
	}
	// Root element for in-rail checks (Esc fallback for the mobile overlay).
	let rootEl: HTMLElement | undefined;
	// Rename dialog — path being renamed + the working path input.
	let renameFolderFor = $state<string | null>(null);
	let renameName = $state('');
	// The folder the editor's save targets — synced from the loaded
	// template, normalized on save (typed paths are allowed).
	let saveFolder = $state('');
	const folderPaths = $derived(
		[...new Set(templates.map((x) => x.folder).filter(Boolean))].sort((a, b) =>
			a.localeCompare(b)
		)
	);
	const folderItems = $derived(folderPaths.map((p) => ({ id: p, label: p })));
	const folderFetcher = $derived(staticAutocompleteFetcher(folderItems));

	function toggleFolder(path: string) {
		const next = new Set(collapsedFolders);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		collapsedFolders = next;
	}

	function folderParent(path: string): string {
		const i = path.lastIndexOf('/');
		return i > 0 ? path.slice(0, i) : '';
	}

	// Rewrite the folder path prefix on member templates — rename swaps
	// it, delete replaces it with the parent path (nothing is lost).
	function rebaseFolders(from: string, to: string): ChatTemplate[] {
		const now = new Date().toISOString();
		return templates.map((tpl) => {
			const f = tpl.folder ?? '';
			if (f === from) return { ...tpl, folder: to, updated_at: now };
			if (f.startsWith(from + '/')) {
				return {
					...tpl,
					folder: to ? to + f.slice(from.length) : f.slice(from.length + 1),
					updated_at: now
				};
			}
			return tpl;
		});
	}

	function renameFolder(path: string) {
		folderMenuFor = null;
		renameFolderFor = path;
		renameName = path;
	}

	async function doFolderRename() {
		const path = renameFolderFor;
		const to = normalizeTemplateFolder(renameName);
		renameFolderFor = null;
		if (!path || to === path || saving) return;
		await run(rebaseFolders(path, to));
	}

	function deleteFolder(path: string) {
		folderMenuFor = null;
		if (saving) return;
		const parent = folderParent(path);
		confirmDialog = {
			title: t('ai.templates.folder_delete', 'Delete folder'),
			body: t(
				'ai.templates.folder_delete_confirm',
				'Delete folder "{name}"? Templates inside move to {parent}.',
				{
					name: path,
					parent: parent || t('ai.templates.ungrouped', 'Ungrouped')
				}
			),
			confirmLabel: t('common.delete', 'Delete'),
			run: () => void run(rebaseFolders(path, parent))
		};
	}

	const activeTemplate = $derived(templates.find((tpl) => tpl.id === activeId));
	// Working-copy divergence — edits to a loaded template that haven't
	// been saved back yet. Drives the "unsaved changes" hint.
	const dirty = $derived(
		!!activeTemplate &&
			templateFingerprint(values) !==
				templateFingerprint(normalizeTemplateValues(activeTemplate.values))
	);
	// Broader than `dirty` — also true for an unsaved custom spec (no
	// activeId), so switching templates can't silently drop typed edits.
	const editsPresent = $derived(
		templateFingerprint(values) !==
			templateFingerprint(
				activeTemplate
					? normalizeTemplateValues(activeTemplate.values)
					: DEFAULT_CHAT_TEMPLATE_VALUES
			)
	);
	// In-app confirm dialog (bits-ui Dialog via the shared Modal) —
	// replaces window.confirm so delete/discard prompts render in the
	// app's own UI instead of a native browser popup.
	let confirmDialog = $state<{
		title: string;
		body: string;
		confirmLabel: string;
		run: () => void;
	} | null>(null);

	// When unsaved edits exist, route the action through the dialog;
	// otherwise run it immediately.
	function requestDiscard(proceed: () => void) {
		if (!editsPresent) {
			proceed();
			return;
		}
		confirmDialog = {
			title: t('ai.templates.discard_title', 'Discard changes?'),
			body: t('ai.templates.discard_changes', 'Discard unsaved template changes?'),
			confirmLabel: t('ai.templates.discard', 'Discard'),
			run: proceed
		};
	}

	const toneOptions = $derived(
		TONE_OPTIONS.map((o) => ({ ...o, label: t(`ai.body.tone_${o.value}`, o.label) }))
	);
	const frameworkOptions = $derived(
		FRAMEWORK_OPTIONS.map((o) =>
			o.value === '' ? { ...o, label: t('ai.body.framework_none', 'No framework') } : o
		)
	);
	const humanizeOptions = $derived(
		HUMANIZE_OPTIONS.map((o) => ({
			...o,
			label: t(`ai.body.humanize_${o.value}`, o.label)
		}))
	);

	// Voices are named for the tone they embody ("Professional",
	// "Witty Warm", ...) — the label prefix maps a voice back to its
	// tone and the picker only offers voices matching the selected
	// Tone. Same mapping as PromptRightPanel and the composer:
	// unlabelled voices count as 'natural'.
	function voiceToneFromLabel(label: string): string {
		const lower = label.toLowerCase();
		for (const tone of TONE_OPTIONS) {
			if (lower === tone.value || lower.startsWith(tone.value + ' ')) {
				return tone.value;
			}
		}
		return 'natural';
	}

	// Same "Default voice" prepend rule as the composer's voiceOptions —
	// the backend may already emit an empty-id row (/ai/voices). The
	// stored pick stays visible even when the selected tone filters it
	// out — same escape the project picker uses, so loading a template
	// whose voice doesn't match its tone doesn't silently drop the pin.
	// A saved voice no longer in the catalog also stays visible as an
	// extra option instead of rendering a blank pick.
	const voiceOptions = $derived([
		...(voices.some((v) => v.id === '')
			? []
			: [{ value: '', label: t('ai.chat.voice_default', 'Default voice') }]),
		...voices
			.filter(
				(v) =>
					!values.tone ||
					voiceToneFromLabel(v.name) === values.tone ||
					v.id === values.voice_id
			)
			.map((v) => ({ value: v.id, label: v.name })),
		...(values.voice_id && !voices.some((v) => v.id === values.voice_id)
			? [
					{
						value: values.voice_id,
						label: t('ai.templates.voice_unavailable', '{id} (no longer available)', {
							id: values.voice_id
						})
					}
				]
			: [])
	]);
	// The Voice field only renders when there's a real voice to pick —
	// a lone "Default voice" option is dead UI. A set voice_id keeps it
	// visible even when the catalog is empty (the orphan option above).
	const hasVoiceField = $derived(voices.some((v) => v.id !== '') || !!values.voice_id);

	const typeOptions = $derived(
		contentTypes.map((ct) => ({ value: ct.value, label: ct.label }))
	);
	const emptyMsg = t('common.no_results', 'No results found');

	// Guide category — the selector only renders when the chosen content
	// type declares categories (same has_category rule the studio builder
	// uses). Free-text stays allowed: an admin may not have configured
	// categories yet, and a typed slug still lands in the spec.
	const selectedType = $derived(contentTypes.find((ct) => ct.value === values.content_type));
	const categories = $derived(selectedType?.categories ?? []);
	const hasCategory = $derived(selectedType?.has_category === true || categories.length > 0);
	const guideCategoryItems = $derived(categories.map((c) => ({ id: c.Slug, label: c.Label })));
	const guideCategoryFetcher = $derived(staticAutocompleteFetcher(guideCategoryItems));
	const guideCategoryLabel = $derived(
		categories.find((c) => c.Slug === values.guide_category)?.Label ?? values.guide_category
	);
	$effect(() => {
		// Switching to a type that declares no categories clears a stale
		// slug — but only when the type actually resolved (a free-text or
		// unloaded type must not wipe a saved template's category).
		if (selectedType && !hasCategory && values.guide_category) {
			values.guide_category = '';
		}
	});

	// High-stakes guide categories can't publish/schedule — same rule as
	// the studio rail: filter the status options and coerce a stale pick.
	const highStakes = $derived(isHighStakesCategory(values.guide_category));
	const statusOpts = $derived(
		statusOptions.filter(
			(s) => !highStakes || (s.value !== 'published' && s.value !== 'scheduled')
		)
	);
	$effect(() => {
		if (highStakes && (values.status === 'published' || values.status === 'scheduled')) {
			values.status = 'in_review';
		}
	});

	// Category-specific custom-field schema — same /custom-field-groups
	// endpoint the composer uses (§14). Rendered as a hint under
	// field_values so the template's keys match what the category expects.
	let categoryFieldKeys = $state<string[]>([]);
	let fieldGroupRequest = 0;
	$effect(() => {
		const cat = values.guide_category;
		const type = values.content_type;
		if (!cat || !hasCategory) {
			categoryFieldKeys = [];
			return;
		}
		const token = ++fieldGroupRequest;
		void (async () => {
			const res = await api.get<CustomFieldGroup[]>(
				`/custom-field-groups?entity_class=${encodeURIComponent(`content_type:${type}:${cat}`)}`
			);
			if (token !== fieldGroupRequest) return;
			categoryFieldKeys = res.error
				? []
				: (res.data ?? []).flatMap((g) => g.fields.map((f) => f.key));
		})();
	});

	// Journey tab's roadmap dimension/purpose live inside field_values,
	// matching the composer contract (§71).
	const dimension = $derived(values.field_values.roadmap_dimension || null);
	const purpose = $derived(values.field_values.roadmap_purpose || null);

	// field_values edits live in a local string: parsing on every
	// keystroke would eat lines that don't have a colon yet, and binding
	// straight to values would lose the draft when the Settings tab
	// unmounts. Commits happen on blur, on save, and before any other
	// code path writes field_values (setFieldValue below).
	let fieldValuesText = $state('');
	const serializeFieldValues = (fv: Record<string, string>) =>
		Object.entries(fv)
			.map(([k, v]) => `${k}: ${v}`)
			.join('\n');
	function commitFieldValues(text: string) {
		const fv: Record<string, string> = {};
		for (const line of text.split('\n')) {
			const idx = line.indexOf(':');
			if (idx > 0) fv[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
		}
		values.field_values = fv;
	}
	$effect(() => {
		fieldValuesText = serializeFieldValues(values.field_values);
	});

	function setFieldValue(key: string, v: string | null) {
		commitFieldValues(fieldValuesText);
		if (v) values.field_values[key] = v;
		else delete values.field_values[key];
	}

	// Picking a row loads the spec for editing — it does NOT attach.
	// `active` is the user's explicit choice via the footer checkbox;
	// flipping it here would silently change what every send carries.
	function select(tpl: ChatTemplate) {
		requestDiscard(() => {
			values = normalizeTemplateValues(tpl.values);
			activeId = tpl.id;
			saveName = '';
			saveFolder = tpl.folder ?? '';
			saveErr = '';
			surface = 'edit';
		});
	}

	function newTemplate() {
		requestDiscard(() => {
			values = structuredClone(DEFAULT_CHAT_TEMPLATE_VALUES);
			activeId = '';
			saveName = '';
			saveFolder = '';
			saveErr = '';
			surface = 'edit';
		});
	}

	// JSON round-trip clone — required because `values` is a $state proxy
	// and structuredClone can't clone proxies (values are JSON-safe by
	// construction: they persist to ai_prefs verbatim).
	function cloneValues(v: ChatTemplateValues): ChatTemplateValues {
		return JSON.parse(JSON.stringify(v)) as ChatTemplateValues;
	}

	async function run(next: ChatTemplate[]): Promise<boolean> {
		saving = true;
		saveErr = '';
		const err = await onpersist(next);
		saving = false;
		if (err) {
			saveErr = err;
			return false;
		}
		return true;
	}

	// Two templates with the same name are indistinguishable in the
	// picker — reject the collision before persisting.
	function nameTaken(name: string, excludeId = ''): boolean {
		const n = name.trim().toLowerCase();
		return templates.some((x) => x.id !== excludeId && x.name.trim().toLowerCase() === n);
	}

	async function save() {
		if (saving) return;
		commitFieldValues(fieldValuesText);
		const folder = normalizeTemplateFolder(saveFolder);
		const now = new Date().toISOString();
		if (activeId && activeTemplate) {
			// Overwrite the loaded template — a typed name renames it.
			const name = saveName.trim() || activeTemplate.name;
			if (nameTaken(name, activeId)) {
				saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
				return;
			}
			const next = templates.map((tpl) =>
				tpl.id === activeId
					? { ...tpl, name, folder, values: cloneValues(values), updated_at: now }
					: tpl
			);
			if (await run(next)) {
				saveName = '';
				saveFolder = folder;
			}
			return;
		}
		const name = saveName.trim();
		if (!name) {
			saveErr = t('ai.templates.name_required', 'Name the template first.');
			return;
		}
		if (nameTaken(name)) {
			saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
			return;
		}
		const tpl: ChatTemplate = {
			id: crypto.randomUUID(),
			name,
			folder,
			values: cloneValues(values),
			created_at: now,
			updated_at: now
		};
		if (await run([tpl, ...templates])) {
			activeId = tpl.id;
			saveName = '';
			saveFolder = folder;
		}
	}

	async function saveAsNew() {
		if (saving) return;
		commitFieldValues(fieldValuesText);
		const folder = normalizeTemplateFolder(saveFolder);
		const name =
			saveName.trim() ||
			(activeTemplate ? `${activeTemplate.name} (copy)` : t('ai.templates.untitled', 'Untitled'));
		if (nameTaken(name)) {
			saveErr = t('ai.templates.name_taken', 'A template with this name already exists.');
			return;
		}
		const now = new Date().toISOString();
		const tpl: ChatTemplate = {
			id: crypto.randomUUID(),
			name,
			folder,
			values: cloneValues(values),
			created_at: now,
			updated_at: now
		};
		if (await run([tpl, ...templates])) {
			activeId = tpl.id;
			saveName = '';
			saveFolder = folder;
		}
	}

	function remove(id: string) {
		if (saving) return;
		const tpl = templates.find((x) => x.id === id);
		if (!tpl) return;
		confirmDialog = {
			title: t('ai.templates.delete', 'Delete template'),
			body: t('ai.templates.delete_confirm', 'Delete template "{name}"? This cannot be undone.', {
				name: tpl.name
			}),
			confirmLabel: t('common.delete', 'Delete'),
			run: () => void doRemove(id)
		};
	}

	async function doRemove(id: string) {
		const next = templates.filter((x) => x.id !== id);
		if (await run(next) && activeId === id) {
			// Keep the working values as an ad-hoc spec — deleting the saved
			// row shouldn't silently change what the next send carries.
			activeId = '';
		}
	}
</script>

{#snippet numField(label: string, value: number, onInput: (v: number) => void)}
	<div class="tpl-field">
		<span class="tpl-label">{label}</span>
		<input
			type="number"
			min="0"
			class="tpl-input"
			{value}
			oninput={(e) => onInput(Math.max(0, Math.floor(Number(e.currentTarget.value) || 0)))}
		/>
	</div>
{/snippet}

{#snippet tplRow(tpl: ChatTemplate)}
	<li class="group relative">
		<button
			type="button"
			onclick={() => select(tpl)}
			class="w-full rounded-[var(--radius)] px-2.5 py-1.5 pr-8 text-left text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {activeId ===
			tpl.id
				? 'bg-[var(--color-primary)]/10 font-medium'
				: ''}"
		>
			<span class="flex items-center gap-1.5">
				<span class="truncate">{tpl.name}</span>
				{#if activeId === tpl.id && active}
					<Check size={12} class="shrink-0 text-[var(--color-primary)]" />
				{/if}
			</span>
		</button>
		<div
			class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
		>
			<button
				type="button"
				onclick={(e) => {
					e.stopPropagation();
					folderMenuFor = null;
					if (tplMenuFor === tpl.id) {
						closeMenus();
						return;
					}
					anchorMenu(e);
					tplMenuFor = tpl.id;
				}}
				class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
				aria-label={t('ai.templates.actions', 'Template actions')}
				title={t('ai.templates.actions', 'Template actions')}
			>
				<Ellipsis size={12} />
			</button>
		</div>
		{#if tplMenuFor === tpl.id && menuPos}
			<!-- Fixed-position flyout rendered outside the translated kebab
			     wrapper — a transform on any ancestor makes position:fixed
			     resolve against that ancestor, not the viewport. -->
			<div
				class="fixed z-50 w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
				style="top: {menuPos.top}px; left: {menuPos.left}px"
				role="menu"
			>
				<button
					type="button"
					onclick={(e) => {
						e.stopPropagation();
						closeMenus();
						oninsert(tpl);
					}}
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<CornerDownLeft size={12} class="shrink-0 text-[var(--color-muted)]" />
					<span class="flex-1 truncate"
						>{t('ai.templates.insert', 'Insert as editable prompt')}</span
					>
				</button>
				<button
					type="button"
					onclick={(e) => {
						e.stopPropagation();
						closeMenus();
						select(tpl);
					}}
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Pencil size={12} class="shrink-0 text-[var(--color-muted)]" />
					<span class="flex-1 truncate">{t('ai.templates.tab_edit', 'Edit template')}</span>
				</button>
				<button
					type="button"
					onclick={(e) => {
						e.stopPropagation();
						closeMenus();
						remove(tpl.id);
					}}
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-[var(--color-danger)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Trash2 size={12} class="shrink-0" />
					<span class="flex-1 truncate">{t('common.delete', 'Delete')}</span>
				</button>
			</div>
		{/if}
	</li>
{/snippet}

{#snippet folderNode(node: TplFolderNode)}
	<li>
		<div class="group/f relative">
			<button
				type="button"
				onclick={() => toggleFolder(node.path)}
				class="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-2 py-1.5 text-left text-xs font-semibold text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				{#if collapsedFolders.has(node.path)}
					<ChevronRight size={12} />
				{:else}
					<ChevronDown size={12} />
				{/if}
				<Folder size={13} class="shrink-0" />
				<span class="truncate">{node.name}</span>
				<span class="ml-auto shrink-0 pr-5">{countTemplates(node)}</span>
			</button>
			<div
				class="absolute right-1 top-1 opacity-0 transition-opacity group-hover/f:opacity-100 group-focus-within/f:opacity-100"
			>
				<button
					type="button"
					onclick={(e) => {
						e.stopPropagation();
						tplMenuFor = null;
						if (folderMenuFor === node.path) {
							closeMenus();
							return;
						}
						anchorMenu(e, 84);
						folderMenuFor = node.path;
					}}
					class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
					aria-label={t('ai.templates.folder_actions', 'Folder actions')}
					title={t('ai.templates.folder_actions', 'Folder actions')}
				>
					<Ellipsis size={12} />
				</button>
				{#if folderMenuFor === node.path && menuPos}
					<div
						class="fixed z-50 w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
						style="top: {menuPos.top}px; left: {menuPos.left}px"
						role="menu"
					>
						<button
							type="button"
							onclick={(e) => {
								e.stopPropagation();
								renameFolder(node.path);
							}}
							class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Pencil size={12} class="shrink-0 text-[var(--color-muted)]" />
							<span class="flex-1 truncate"
								>{t('ai.templates.folder_rename', 'Rename folder')}</span
							>
						</button>
						<button
							type="button"
							onclick={(e) => {
								e.stopPropagation();
								deleteFolder(node.path);
							}}
							class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-[var(--color-danger)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Trash2 size={12} class="shrink-0" />
							<span class="flex-1 truncate"
								>{t('ai.templates.folder_delete', 'Delete folder')}</span
							>
						</button>
					</div>
				{/if}
			</div>
		</div>
		{#if !collapsedFolders.has(node.path)}
			<ul class="pl-4">
				{#each node.templates as tpl (tpl.id)}
					{@render tplRow(tpl)}
				{/each}
				{#each sortedNodes(node.children) as child (child.path)}
					{@render folderNode(child)}
				{/each}
			</ul>
		{/if}
	</li>
{/snippet}

<aside
	bind:this={rootEl}
	class="flex h-full w-full flex-col overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface)] rounded-l-[var(--radius-card)] lg:rounded-[var(--radius-card)]"
>
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-2.5">
		<h2 class="text-sm font-semibold text-[var(--color-text)]">
			{t('ai.templates.title', 'Chat templates')}
		</h2>
		<div class="flex items-center gap-1">
			<button
				type="button"
				onclick={newTemplate}
				class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
			>
				<Plus size={13} />
				{t('ai.templates.new', 'New')}
			</button>
			{#if onclose}
				<button
					type="button"
					onclick={onclose}
					class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)] lg:hidden"
					aria-label={t('common.close', 'Close')}
				>
					<X size={15} />
				</button>
			{/if}
		</div>
	</div>

	<!-- Surface switch — the rail is either the searchable template
	     list or the spec editor (same pill-switch idiom as
	     BookingListPane's sub-filters). -->
	<div
		class="flex gap-1.5 border-b border-[var(--color-border)] px-3 py-2"
		role="group"
		aria-label={t('ai.templates.title', 'Chat templates')}
	>
		<button
			type="button"
			aria-pressed={surface === 'list'}
			onclick={() => (surface = 'list')}
			class="flex-1 rounded-full border px-3 py-1 text-center text-xs font-medium transition-colors {surface ===
			'list'
				? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
				: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{t('ai.templates.tab_list', 'Chat templates')}
		</button>
		<button
			type="button"
			aria-pressed={surface === 'edit'}
			onclick={() => (surface = 'edit')}
			class="flex-1 rounded-full border px-3 py-1 text-center text-xs font-medium transition-colors {surface ===
			'edit'
				? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
				: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{t('ai.templates.tab_edit', 'Edit template')}
		</button>
	</div>

	{#if surface === 'list'}
		<div class="border-b border-[var(--color-border)] p-2">
			<div class="relative">
				<Search
					size={13}
					class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
				/>
				<input
					type="search"
					bind:value={templateQuery}
					onkeydown={(e) => {
						if (e.key === 'Escape') templateQuery = '';
					}}
					class="tpl-input tpl-search"
					placeholder={t('ai.templates.search', 'Search templates')}
					aria-label={t('ai.templates.search', 'Search templates')}
				/>
			</div>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto" onscroll={closeMenus}>
			{#if filteredTemplates.length > 0}
				<ul class="p-2">
					{#each sortedNodes(folderTree.roots) as node (node.path)}
						{@render folderNode(node)}
					{/each}
					{#if folderTree.ungrouped.length > 0}
						{#if folderTree.roots.size > 0}
							<li class="px-2 pb-0.5 pt-2 text-xs font-semibold text-[var(--color-muted)]">
								{t('ai.templates.ungrouped', 'Ungrouped')}
							</li>
						{/if}
						{#each folderTree.ungrouped as tpl (tpl.id)}
							{@render tplRow(tpl)}
						{/each}
					{/if}
				</ul>
			{:else}
				<p class="px-4 py-8 text-center text-xs text-[var(--color-muted)]">
					{templates.length === 0
						? t(
								'ai.templates.empty',
								'No templates yet — create one to reuse a generation spec.'
							)
						: t('common.no_results', 'No results found')}
				</p>
			{/if}
		</div>

		<div class="border-t border-[var(--color-border)] p-2.5">
			<button
				type="button"
				onclick={newTemplate}
				class="flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] px-2 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				<Plus size={13} />
				{t('ai.templates.new_template', 'New template')}
			</button>
			{#if saveErr}
				<p class="mt-2 text-xs text-[var(--color-danger)]">{saveErr}</p>
			{/if}
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-y-auto">
		<Tabs.Root bind:value={editTab}>
			<Tabs.List class="tpl-tabs">
				<Tabs.Trigger value="prompt" class="tpl-tab">
					{t('studio.prompts.tab.prompt', 'Prompt')}
				</Tabs.Trigger>
				<Tabs.Trigger value="structure" class="tpl-tab">
					{t('studio.prompts.tab.structure', 'Structure')}
				</Tabs.Trigger>
				<Tabs.Trigger value="journey" class="tpl-tab">
					{t('create.panel.journey', 'Journey')}
				</Tabs.Trigger>
				<Tabs.Trigger value="settings" class="tpl-tab">
					{t('create.panel.settings', 'Settings')}
				</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="prompt" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-field">
						<span class="tpl-label">{t('ai.templates.content_type', 'Content type')}</span>
						{#if typeOptions.length > 0}
							<AutocompleteSelect
								value={values.content_type}
								onValueChange={(v) => {
									values.content_type = v;
									values.guide_category = '';
								}}
								options={typeOptions}
								ariaLabel={t('ai.templates.content_type', 'Content type')}
							/>
						{:else}
							<input type="text" class="tpl-input" bind:value={values.content_type} />
						{/if}
					</div>

					{#if hasCategory}
						<div class="tpl-field">
							<span class="tpl-label"
								>{t('studio.prompts.guide_category', 'Guide category')}</span
							>
							<Autocomplete
								id="tpl-guide-category"
								placeholder={t('studio.prompts.guide_category', 'Guide category')}
								emptyMessage={emptyMsg}
								fetcher={guideCategoryFetcher}
								initialFetcher={guideCategoryFetcher}
								selectedId={values.guide_category || null}
								selectedLabel={guideCategoryLabel}
								onSelect={(item: AutocompleteItem) => {
									values.guide_category = item.id;
								}}
								onClear={() => {
									values.guide_category = '';
								}}
								allowFreeText
							/>
						</div>
					{/if}

					<div class:tpl-pair={hasVoiceField}>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.tone', 'Tone')}</span>
							<AutocompleteSelect
								bind:value={values.tone}
								options={toneOptions}
								ariaLabel={t('studio.prompts.tone', 'Tone')}
							/>
						</div>
						{#if hasVoiceField}
							<div class="tpl-field">
								<span class="tpl-label">{t('ai.templates.voice', 'Voice')}</span>
								<AutocompleteSelect
									bind:value={values.voice_id}
									options={voiceOptions}
									ariaLabel={t('ai.templates.voice', 'Voice')}
								/>
							</div>
						{/if}
					</div>

					<div class="tpl-pair">
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.framework', 'Framework')}</span>
							<AutocompleteSelect
								value={values.framework}
								onValueChange={(v) => (values.framework = v as ChatTemplateFramework)}
								options={frameworkOptions}
								ariaLabel={t('studio.prompts.framework', 'Framework')}
							/>
						</div>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.humanize', 'Humanize')}</span>
							<AutocompleteSelect
								value={values.humanize_level}
								onValueChange={(v) => (values.humanize_level = v as ChatTemplateHumanize)}
								options={humanizeOptions}
								ariaLabel={t('studio.prompts.humanize', 'Humanize')}
							/>
						</div>
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.audience', 'Audience')}</span>
						<input
							type="text"
							class="tpl-input"
							bind:value={values.audience}
							placeholder={t(
								'studio.prompts.audience_placeholder',
								'e.g. Nigerians relocating to Canada — defaults to general JapaConnect readers'
							)}
						/>
					</div>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-instructions"
							>{t('ai.templates.instructions', 'Extra instructions')}</label
						>
						<textarea
							id="tpl-instructions"
							rows="3"
							class="tpl-input tpl-textarea"
							bind:value={values.instructions}
							placeholder={t(
								'ai.templates.instructions_placeholder',
								'e.g. Always cite official sources; end with next steps'
							)}
						></textarea>
					</div>
				</div>
			</Tabs.Content>

			<Tabs.Content value="structure" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.min_words', 'Min words'), values.min_words, (v) => (values.min_words = v))}
						{@render numField(t('studio.prompts.heading_count', 'H2 headings'), values.heading_count, (v) => (values.heading_count = v))}
					</div>
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.paragraphs_per_heading', 'Paras / heading'), values.min_paragraphs_per_heading, (v) => (values.min_paragraphs_per_heading = v))}
						{@render numField(t('studio.prompts.paragraph_count', 'Total paras'), values.paragraph_count, (v) => (values.paragraph_count = v))}
					</div>
					<div class="tpl-pair">
						{@render numField(t('studio.prompts.internal_links', 'Internal links'), values.internal_links, (v) => (values.internal_links = v))}
						{@render numField(t('studio.prompts.external_links', 'External links'), values.external_links, (v) => (values.external_links = v))}
					</div>

					<div class="tpl-toggles">
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_intro} />
							{t('studio.prompts.inc_intro', 'Intro')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_conclusion} />
							{t('studio.prompts.inc_conclusion', 'Conclusion')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_faqs} />
							{t('studio.prompts.inc_faqs', 'FAQs')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_steps} />
							{t('studio.prompts.inc_steps', 'Steps')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_key_takeaways} />
							{t('studio.prompts.inc_takeaways', 'Key takeaways')}</label
						>
						<label class="tpl-toggle"
							><input type="checkbox" bind:checked={values.include_inline_image} />
							{t('studio.prompts.inc_images', 'Image suggestions')}</label
						>
					</div>
				</div>
			</Tabs.Content>

			<Tabs.Content value="journey" class="pt-2">
				<div class="tpl-stack">
					<CreateJourneyPanel
						journeyStage={values.journey_stage || null}
						visaRoute={values.visa_route || null}
						sourceCountry={values.source_country || null}
						targetCountry={values.target_country || null}
						{dimension}
						{purpose}
						{assessmentDimensions}
						onjourney={(v) => (values.journey_stage = v ?? '')}
						onvisa={(v) => (values.visa_route = v ?? '')}
						onsource={(v) => (values.source_country = v ?? '')}
						ontarget={(v) => (values.target_country = v ?? '')}
						ondimension={(v) => setFieldValue('roadmap_dimension', v)}
						onpurpose={(v) => setFieldValue('roadmap_purpose', v)}
						onedit={() => {}}
					/>
				</div>
			</Tabs.Content>

			<Tabs.Content value="settings" class="pt-2">
				<div class="tpl-stack">
					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.series', 'Series')}</span>
						<SeriesSelector
							bind:series
							seriesId={values.series_id || null}
							onselect={(id) => (values.series_id = id ?? '')}
						/>
					</div>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-publish-at"
							>{t('studio.prompts.publish_at', 'Publish at')}</label
						>
						<input
							id="tpl-publish-at"
							type="datetime-local"
							class="tpl-input"
							bind:value={values.publish_at}
						/>
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.space', 'Space')}</span>
						<SpaceSelector
							bind:spaces
							spaceIds={values.space_id ? [values.space_id] : []}
							onChange={(ids) => (values.space_id = ids[0] ?? '')}
						/>
					</div>

					<div class="tpl-field">
						<span class="tpl-label">{t('studio.prompts.tags_label', 'Tags')}</span>
						<TagInput tags={values.tags} onTags={(next) => (values.tags = next)} {maxTags} />
					</div>

					<div class="tpl-pair">
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.status', 'Status')}</span>
							<AutocompleteSelect
								bind:value={values.status}
								options={statusOpts}
								ariaLabel={t('studio.prompts.status', 'Status')}
							/>
						</div>
						<div class="tpl-field">
							<span class="tpl-label">{t('studio.prompts.visibility', 'Visibility')}</span>
							<AutocompleteSelect
								bind:value={values.visibility}
								options={visibilityOptions}
								ariaLabel={t('studio.prompts.visibility', 'Visibility')}
							/>
						</div>
					</div>
					{#if highStakes}
						<p class="tpl-hint tpl-hint-warn">
							{t(
								'studio.prompts.high_stakes',
								'High-stakes category — saved as in_review regardless of selected status.'
							)}
						</p>
					{/if}

					<label class="tpl-toggle"
						><input type="checkbox" bind:checked={values.seo_indexable} />
						{t('studio.prompts.seo_indexable', 'Indexable')}</label
					>

					<div class="tpl-field">
						<label class="tpl-label" for="tpl-field-values">
							{t('studio.prompts.field_values', 'Field values (key: value, one per line)')}
						</label>
						<textarea
							id="tpl-field-values"
							rows="3"
							class="tpl-input tpl-textarea"
							bind:value={fieldValuesText}
							onchange={() => commitFieldValues(fieldValuesText)}
						></textarea>
						{#if categoryFieldKeys.length > 0}
							<p class="tpl-hint">
								{t(
									'ai.templates.category_fields',
									'This category expects these field keys:'
								)}
								{categoryFieldKeys.join(', ')}
							</p>
						{/if}
					</div>
				</div>
			</Tabs.Content>
		</Tabs.Root>
	</div>

	<!-- Footer: attach toggle + insert + save -->
	<div class="space-y-2 border-t border-[var(--color-border)] p-3">
		<label class="tpl-toggle">
			<input type="checkbox" bind:checked={active} />
			{t('ai.templates.attach', 'Attach spec to every message')}
		</label>

		<button
			type="button"
			onclick={() => oninsert()}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			{t('ai.templates.insert', 'Insert as editable prompt')}
		</button>

		<div class="tpl-field">
			<span class="tpl-label">{t('ai.templates.folder', 'Folder')}</span>
			<Autocomplete
				id="tpl-folder"
				placeholder={t('ai.templates.folder_placeholder', 'e.g. visa/guides')}
				emptyMessage={emptyMsg}
				fetcher={folderFetcher}
				initialFetcher={folderFetcher}
				selectedId={saveFolder || null}
				selectedLabel={saveFolder}
				onSelect={(item: AutocompleteItem) => {
					saveFolder = item.id;
				}}
				onClear={() => {
					saveFolder = '';
				}}
				allowFreeText
			/>
		</div>

		<div class="flex gap-2">
			<input
				type="text"
				bind:value={saveName}
				maxlength="200"
				class="tpl-input flex-1"
				placeholder={activeTemplate
					? activeTemplate.name
					: t('ai.templates.name_placeholder', 'Template name')}
				aria-label={t('ai.templates.name_placeholder', 'Template name')}
			/>
			<button
				type="button"
				onclick={save}
				disabled={saving || (!activeId && !saveName.trim())}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if saving}<Loader2 size={12} class="animate-spin" />{/if}
				{t('common.save', 'Save')}
			</button>
		</div>
		{#if activeId}
			<button
				type="button"
				onclick={saveAsNew}
				disabled={saving}
				class="w-full text-center text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)] disabled:opacity-50"
			>
				{t('ai.templates.save_as_new', 'Save as new template')}
			</button>
		{/if}

		{#if dirty}
			<p class="text-[10px] text-[var(--color-warning)]">
				{t('ai.templates.unsaved', 'Unsaved changes — the live spec is what sends, save to keep it.')}
			</p>
		{/if}
		{#if saveErr}
			<p class="text-xs text-[var(--color-danger)]">{saveErr}</p>
		{/if}
	</div>
	{/if}
</aside>

<Modal
	open={!!confirmDialog}
	onOpenChange={(o) => {
		if (!o) confirmDialog = null;
	}}
	title={confirmDialog?.title ?? ''}
	description={confirmDialog?.body ?? ''}
	size="sm"
>
	{#snippet children(close)}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={close}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				onclick={() => {
					const pending = confirmDialog;
					close();
					confirmDialog = null;
					pending?.run();
				}}
				class="rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-white"
				style="background-color: var(--color-danger)"
			>
				{confirmDialog?.confirmLabel}
			</button>
		</div>
	{/snippet}
</Modal>

<Modal
	open={renameFolderFor !== null}
	onOpenChange={(o) => {
		if (!o) renameFolderFor = null;
	}}
	title={t('ai.templates.folder_rename', 'Rename folder')}
	description={t(
		'ai.templates.folder_rename_help',
		'Use "/" for nesting — e.g. visa/guides. Applies to every template and subfolder inside.'
	)}
	size="sm"
>
	{#snippet children(close)}
		<div class="space-y-3">
			<input
				type="text"
				class="tpl-input"
				bind:value={renameName}
				placeholder={t('ai.templates.folder_placeholder', 'e.g. visa/guides')}
				aria-label={t('ai.templates.folder', 'Folder')}
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						void doFolderRename();
						close();
					}
				}}
			/>
			<div class="flex justify-end gap-2">
				<button
					type="button"
					onclick={close}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					onclick={() => {
						void doFolderRename();
						close();
					}}
					class="rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-white"
					style="background-color: var(--color-primary)"
				>
					{t('ai.templates.rename', 'Rename')}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>

<!-- Click-away / Escape closes an open row/folder menu — same idiom as
     the conversation sidebar. Menu buttons stopPropagation so the click
     handler only fires on real outside clicks; the parent's Esc chain
     skips in-rail targets (data-template-rail), so this only dismisses
     the menus. -->
<svelte:window
	onclick={closeMenus}
	onkeydown={(e) => {
		if (e.key !== 'Escape') return;
		if (tplMenuFor || folderMenuFor) {
			closeMenus();
			return;
		}
		// No menu open: the parent skips rail dismissal for in-rail Esc
		// presses, so close the mobile overlay from here instead.
		if (
			onclose &&
			!window.matchMedia('(min-width: 1024px)').matches &&
			e.target instanceof Node &&
			rootEl?.contains(e.target)
		)
			onclose();
	}}
/>

<style>
	/* Bits UI applies the class props to its rendered elements, so these
	 * must be global — scoped selectors would be flagged unused and
	 * dropped (same idiom as PromptRightPanel's right-panel-tab). */
	:global(.tpl-tabs) {
		display: flex;
		border-bottom: 1px solid var(--color-border);
		/* Four tabs overflow the w-72 rail at lg — scroll rather than clip. */
		overflow-x: auto;
		scrollbar-width: none;
	}
	:global(.tpl-tab) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 40px;
		padding: 8px 12px;
		white-space: nowrap;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s;
	}
	:global(.tpl-tab:hover) {
		color: var(--color-text);
	}
	:global(.tpl-tab[data-state='active']) {
		color: var(--color-text);
		border-bottom: 2px solid var(--color-primary);
	}
	.tpl-stack {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem 0.75rem 0.75rem;
	}
	.tpl-pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
		align-items: start;
	}
	.tpl-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.tpl-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.tpl-input {
		width: 100%;
		height: 2.125rem;
		padding: 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		outline: none;
		transition:
			border-color 0.15s,
			box-shadow 0.15s;
	}
	.tpl-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	/* Scoped so it always beats .tpl-input's padding shorthand — a
	   Tailwind pl-* utility can lose to unlayered component CSS. */
	.tpl-search {
		padding-left: 2.375rem;
	}
	.tpl-textarea {
		height: auto;
		padding: 8px 12px;
		font-family: inherit;
		resize: vertical;
	}
	.tpl-toggles {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px 12px;
	}
	.tpl-toggle {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.8125rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.tpl-hint {
		font-size: 0.75rem;
		line-height: 1.4;
		color: var(--color-muted);
	}
	.tpl-hint-warn {
		color: var(--color-warning);
	}
</style>
