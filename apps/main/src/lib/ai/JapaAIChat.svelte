<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// JapaAIChat — the ADR-130 unified chat surface. One transcript, one
	// composer, one history: the resolved tool catalog is what differs
	// between "chat" and "agent" turns (an empty catalog degenerates the
	// backend loop to JapaBot-class chat).
	//
	// Eligible users (agent.use) get a composer tools toggle + a per-tool/
	// category popover; everyone else gets plain chat. Streams over
	// POST /api/v1/ai/chat with Accept: text/event-stream (fetch SSE —
	// form actions cannot stream). Tool calls, progress, citations, and
	// token/cost footers render inline per turn.
	//
	// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
	// Ref: japaconnect-engineering/sections/20-ai-core.md
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md

	import { t } from '$lib/i18n';
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import {
		ArrowUp,
		Sparkles,
		MessageSquare,
		Loader2,
		AlertCircle,
		ShieldCheck,
		Menu,
		X,
		Plus,
		Trash2,
		Wrench,
		SlidersHorizontal,
		CheckCircle2,
		XCircle,
		Search,
		Cpu,
		Database,
		BookOpen,
		Globe,
		FileText,
		Tag,
		Users as UsersIcon,
		GraduationCap,
		LayoutGrid,
		Square,
		Copy,
		Check,
		Folder,
		FolderPlus,
		FolderInput,
		FolderOutput,
		Ellipsis,
		Pencil,
		ChevronDown,
		ChevronRight,
		History,
		PanelRight
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { AgentRequestError } from './agentTypes';
	import { streamChat, type ToolSelectionOverride } from './unifiedStream';
	import AiModelVoicePicker from './AiModelVoicePicker.svelte';
	import ChatTemplatePanel from './ChatTemplatePanel.svelte';
	import {
		DEFAULT_CHAT_TEMPLATE_VALUES,
		buildTemplateBlock,
		buildInsertTail,
		buildTemplateInsertText,
		buildTemplateSpec,
		normalizeTemplateFolder,
		normalizeTemplateValues,
		splitComposerForInsert,
		templateFingerprint,
		type ChatTemplate,
		type ChatTemplatePostMode,
		type ChatTemplatePostTarget,
		type ChatTemplateValues
	} from './chatTemplate';
	import { chatTemplateRail } from './chatTemplateRail.svelte';
	import { invalidateAll } from '$app/navigation';
	import { AutocompleteSelect, Modal } from '$lib/components/ui';
	import AddAiKeyDialog from './AddAiKeyDialog.svelte';
	import AiSourceCitations from './AiSourceCitations.svelte';
	import type { AiCitationSource } from './types';
	import type { MeSpace, BlogSeries, ContentTypeMeta } from '$lib/types';
	import type {
		AgentCapabilities,
		AgentEvent,
		AIPrefs,
		ConversationMessage,
		ConversationToolCall,
		UnifiedConversationSummary
	} from './agentTypes';
	import type { AiProject } from './agentTypes';

	interface Props {
		conversations: UnifiedConversationSummary[];
		models: { id: string; label: string }[];
		byokProviders?: import('$lib/ai/agentTypes').ByokProviderOption[];
		voices: { id: string; name: string }[];
		projects: AiProject[];
		capabilities: AgentCapabilities | null;
		prefs: AIPrefs | null;
		/** GET /content-types — options for the template editor's content
		 * type field (has_category/categories drive the guide-category
		 * selector). Empty renders a plain text input there. */
		contentTypes: ContentTypeMeta[];
		/** Settings/Journey tab data for the template panel — same catalogs
		 * the studio prompt rail gets (best-effort; empty degrades the tab). */
		spaces: MeSpace[];
		series: BlogSeries[];
		assessmentDimensions: string[];
		maxTags: number;
		statusOptions: { value: string; label: string }[];
		visibilityOptions: { value: string; label: string }[];
		/** Actor holds agent.skip_hitl — high-stakes categories don't force
		 * the in_review status restriction in the template editor (§103). */
		canSkipHitl?: boolean;
		/** §115: open this thread on mount — the panel's "Open in Japa AI"
		 * CTA deep-links /ai?c={conversation_id}. */
		initialConversationId?: string;
	}

	let {
		conversations,
		models,
		byokProviders = [],
		voices,
		projects,
		capabilities,
		prefs,
		contentTypes,
		spaces,
		series,
		assessmentDimensions,
		maxTags,
		statusOptions,
		visibilityOptions,
		canSkipHitl = false,
		initialConversationId = undefined
	}: Props = $props();

	type ToolStatus = 'pending' | 'success' | 'error' | 'unauthorized' | 'privacy_blocked' | 'hitl_queued';

	interface ToolCallEntry {
		id: number;
		tool_name: string;
		arguments?: string;
		status: ToolStatus;
		result?: string;
		error?: string;
		iteration: number;
	}

	interface UnifiedTurn {
		id: number;
		role: 'user' | 'assistant';
		content: string;
		// ADR-130 Phase 2: accumulated `delta` fragments for the in-flight
		// reply — rendered in place of content while non-empty; cleared on
		// final_response which carries the authoritative text.
		streamed?: string;
		sources?: AiCitationSource[];
		tool_calls?: ToolCallEntry[];
		session_id?: string;
		presearch_level?: string;
		tokens_used?: number;
		estimated_cost?: number;
		error?: string;
		in_progress?: boolean;
		/** Transient agent-loop status line ("LLM iteration 2") — shown in
		 * the in-progress indicator only; never persists as turn content. */
		progress?: string;
		/** Set when the turn was sent with a chat template attached — the
		 * bubble shows the template name under the user's text. */
		template_name?: string;
	}

	// Local copy of the sidebar list — a conversation created mid-session is
	// prepended when its `conversation` event arrives.
	let convos = $state<UnifiedConversationSummary[]>([]);
	$effect(() => {
		convos = [...conversations];
	});

	// --- §103 AI Projects ---------------------------------------------------
	// Local copy of the projects prop — create/update/delete/assign mutate
	// it in place rather than re-fetching the list.
	let projs = $state<AiProject[]>([]);
	$effect(() => {
		projs = [...projects];
	});

	// Collapsed project groups in the sidebar (expanded by default).
	let collapsedProjects = $state<Set<string>>(new Set());
	function toggleProjectCollapsed(id: string) {
		const next = new Set(collapsedProjects);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		collapsedProjects = next;
	}

	// Sidebar grouping: each project owns its member threads; threads with
	// no project (or pointing at a deleted one) fall under "ungrouped".
	const ungroupedConvos = $derived(
		convos.filter((c) => !c.project_id || !projs.some((p) => p.id === c.project_id))
	);
	function convsInProject(id: string): UnifiedConversationSummary[] {
		return convos.filter((c) => c.project_id === id);
	}

	// Apply a project's pinned defaults on entry — empty pins leave the
	// current composer selections untouched.
	function applyProjectPins(p: AiProject) {
		if (p.default_model) selectedModel = p.default_model;
		if (p.default_tone) selectedTone = p.default_tone;
		if (p.default_voice_id) selectedVoice = p.default_voice_id;
		const tools = p.default_tools as ToolSelectionOverride | undefined;
		if (tools && typeof tools === 'object' && toolsAvailable) {
			if ('enabled' in tools) toolsOn = tools.enabled !== false;
			// Pinned tool/category whitelists narrow the composer selection;
			// prefs-disabled tools still stay off. Only apply after the
			// per-tool map has been seeded — an early project entry before
			// capabilities arrive must not lock every tool off.
			const hasWhitelist = Array.isArray(tools.tools) || Array.isArray(tools.categories);
			if (toolsOn && hasWhitelist && Object.keys(toolEnabled).length > 0) {
				const pinTools = new Set(tools.tools ?? []);
				const pinCats = new Set(tools.categories ?? []);
				const next = { ...toolEnabled };
				for (const cat of categories) {
					for (const tool of cat.tools) {
						next[tool.name] =
							prefsAllows(tool.name, cat.name) &&
							(pinTools.has(tool.name) || pinCats.has(cat.name));
					}
				}
				toolEnabled = next;
			}
		}
	}

	// Restore the composer's global defaults — empty pickers + prefs-derived
	// tool state. Runs when entering an ungrouped thread or starting a
	// plain new chat so project pins don't leak across contexts.
	function resetComposerDefaults() {
		selectedModel = '';
		selectedVoice = '';
		selectedTone = '';
		if (toolsAvailable) {
			toolsOn = prefs?.default_mode !== 'chat';
			const next: Record<string, boolean> = {};
			for (const cat of categories) {
				for (const tool of cat.tools) {
					next[tool.name] = prefsAllows(tool.name, cat.name);
				}
			}
			toolEnabled = next;
		}
	}

	// Start a fresh chat stamped into a project — pins apply immediately and
	// the first send carries project_id so the thread is created inside it.
	function startNewChatInProject(p: AiProject) {
		startNewChat();
		pendingProjectId = p.id;
		applyProjectPins(p);
	}

	// --- Project dialog + membership ----------------------------------------
	let projectDialog = $state<{ mode: 'create' } | { mode: 'edit'; project: AiProject } | null>(null);
	let projName = $state('');
	let projInstructions = $state('');
	let projModel = $state('');
	let projTone = $state('');
	let projVoice = $state('');
	let projToolsOn = $state(true);
	let projSaving = $state(false);
	let projError = $state('');

	// Voice pin options follow the dialog's tone pin — same mapping as the
	// composer's tone→voice filter. The currently stored pin is always kept
	// in the list (v.id === projVoice) so editing a project whose stored
	// voice doesn't map to the stored tone doesn't silently drop the pin —
	// the user can still clear it explicitly via "Default voice".
	const projVoiceOptions = $derived([
		{ value: '', label: t('ai.chat.voice_default', 'Default voice') },
		...voices
			.filter((v) => !projTone || voiceToneFromLabel(v.name) === projTone || v.id === projVoice)
			.map((v) => ({ value: v.id, label: v.name }))
	]);
	$effect(() => {
		if (projVoice !== '' && !projVoiceOptions.some((v) => v.value === projVoice)) {
			projVoice = '';
		}
	});

	function openProjectDialog(mode: 'create' | AiProject) {
		const editing = typeof mode === 'object' ? mode : null;
		projectDialog = editing ? { mode: 'edit', project: editing } : { mode: 'create' };
		projName = editing?.name ?? '';
		projInstructions = editing?.instructions ?? '';
		projModel = editing?.default_model ?? '';
		projTone = editing?.default_tone ?? '';
		projVoice = editing?.default_voice_id ?? '';
		const tools = editing?.default_tools as ToolSelectionOverride | undefined;
		projToolsOn = tools?.enabled !== false;
		projError = '';
	}

	async function saveProject() {
		const name = projName.trim();
		if (!name || projSaving) return;
		projSaving = true;
		projError = '';
		// Merge `enabled` over the stored tools object rather than replacing
		// it — a project whose default_tools carries a tools/categories
		// whitelist (set via API) must not lose it on an unrelated edit.
		const existingTools =
			projectDialog?.mode === 'edit' && typeof projectDialog.project.default_tools === 'object'
				? (projectDialog.project.default_tools as Record<string, unknown>)
				: {};
		const body = {
			name,
			instructions: projInstructions,
			default_model: projModel,
			default_tone: projTone,
			default_voice_id: projVoice,
			default_tools: { ...existingTools, enabled: projToolsOn }
		};
		try {
			if (projectDialog?.mode === 'edit') {
				const res = await api.patch<AiProject>(
					`/ai/projects/${projectDialog.project.id}`,
					body
				);
				if (res.error) {
					projError =
						res.error.code === 'project_name_taken'
							? t('ai.projects.name_taken', 'You already have a project with this name.')
							: res.error.message || 'save failed';
					return;
				}
				if (res.data) {
					const updated = res.data;
					projs = projs.map((p) => (p.id === updated.id ? { ...p, ...updated } : p));
				}
			} else {
				const res = await api.post<AiProject>('/ai/projects', body);
				if (res.error) {
					projError =
						res.error.code === 'project_name_taken'
							? t('ai.projects.name_taken', 'You already have a project with this name.')
							: res.error.message || 'save failed';
					return;
				}
				// List is updated_at-DESC — prepend so the new group
				// matches server ordering without a reload.
				if (res.data) projs = [res.data, ...projs];
			}
			projectDialog = null;
		} catch (e) {
			projError = e instanceof Error ? e.message : 'save failed';
		} finally {
			projSaving = false;
		}
	}

	// In-app confirm dialog (bits-ui Dialog via the shared Modal) —
	// replaces window.confirm for destructive actions (delete project /
	// conversation). `run` may be async; the dialog closes on confirm.
	let confirmDialog = $state<{
		title: string;
		body: string;
		confirmLabel: string;
		run: () => void;
	} | null>(null);

	function deleteProject(p: AiProject) {
		// Destructive-ish: instructions and pins are lost (threads survive
		// un-grouped) — confirm via the in-app modal before the API call.
		confirmDialog = {
			title: t('ai.projects.delete_title', 'Delete project?'),
			body: t(
				'ai.projects.delete_confirm',
				'Delete this project? Its conversations are kept, but the instructions and defaults are lost.'
			),
			confirmLabel: t('common.delete', 'Delete'),
			run: async () => {
				const res = await api.delete(`/ai/projects/${p.id}`);
				if (res.error) return;
				projs = projs.filter((x) => x.id !== p.id);
				// The backend clears membership (SET NULL) — mirror it locally.
				convos = convos.map((c) => (c.project_id === p.id ? { ...c, project_id: undefined } : c));
				// If the deleted project was driving the composer, drop its pins.
				if (activeConvo?.project_id === p.id || pendingProjectId === p.id) {
					pendingProjectId = null;
					resetComposerDefaults();
				}
			}
		};
	}

	// Overflow-menu state — which row's menu is open and whether the
	// "Move to project" flyout is expanded inside it. projMenuFor tracks
	// the overflow menu on a project group header.
	let moveMenuFor = $state<string | null>(null);
	let moveSubOpen = $state(false);
	// The "Move to project" flyout is position:fixed — the sidebar's
	// overflow-y-auto would clip an absolutely-positioned submenu, so we
	// anchor it to the trigger's viewport rect instead.
	let flyoutPos = $state<{ top: number; left: number } | null>(null);
	let projMenuFor = $state<string | null>(null);

	function toggleMoveSub(e: MouseEvent) {
		e.stopPropagation();
		if (!moveSubOpen) {
			const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
			flyoutPos = { top: r.top, left: r.right - 8 };
		}
		moveSubOpen = !moveSubOpen;
	}

	async function assignProject(convID: string, projectID: string | null) {
		const res = await api.put(`/ai/conversations/${convID}/project`, {
			project_id: projectID
		});
		if (res.error) {
			moveMenuFor = null;
			moveSubOpen = false;
			error =
				res.error.message ||
				t('ai.projects.assign_failed', 'Could not update the project assignment.');
			return;
		}
		convos = convos.map((c) =>
			c.conversation_id === convID ? { ...c, project_id: projectID ?? undefined } : c
		);
		moveMenuFor = null;
		moveSubOpen = false;
		if (convID === conversationId) {
			if (projectID) {
				const p = projs.find((x) => x.id === projectID);
				if (p) applyProjectPins(p);
			} else {
				// Just un-grouped the open thread — restore global defaults.
				resetComposerDefaults();
			}
		}
	}

	let turns = $state<UnifiedTurn[]>([]);
	let input = $state('');
	let inputEl: HTMLTextAreaElement | undefined = $state();
	let conversationId = $state('');
	// The project the open thread belongs to — its pins pre-fill the
	// composer pickers (defaults, not locks). On a fresh chat, a pending
	// project selection (new-chat-in-project) supplies the same context.
	let pendingProjectId = $state<string | null>(null);
	const activeConvo = $derived(convos.find((c) => c.conversation_id === conversationId));
	const activeProject = $derived(
		conversationId
			? activeConvo?.project_id
				? projs.find((p) => p.id === activeConvo.project_id)
				: undefined
			: pendingProjectId
				? projs.find((p) => p.id === pendingProjectId)
				: undefined
	);
	let sending = $state(false);
	let error = $state('');
	let scrollEl: HTMLElement | undefined = $state();
	let loadingConversation = $state(false);
	let sidebarOpen = $state(false);
	// True while viewing a read-only legacy (Honcho-era) thread — the
	// composer is locked so a reply can't silently fork it into a new
	// unified thread that has none of the legacy history.
	let viewingLegacy = $state(false);
	let turnSeq = 0;
	let toolSeq = 0;

	let activeAbort: AbortController | null = $state(null);
	let activeSessionId: string | null = $state(null);
	let cancelling = $state(false);

	// Detached turns keep running server-side after a disconnect — when a
	// conversation carries a running session (or a live stream drops), poll
	// the session endpoint until terminal, then refetch so the persisted
	// assistant message (or failure notice) lands.
	let bgSessionId: string | null = $state(null);
	let bgPollTimer: ReturnType<typeof setInterval> | null = null;
	const BG_POLL_MS = 3000;
	const BG_POLL_CAP_MS = 10 * 60 * 1000;

	function stopSessionPoll() {
		if (bgPollTimer) clearInterval(bgPollTimer);
		bgPollTimer = null;
		bgSessionId = null;
		bgCancelling = false;
	}

	// A tools-on stream that drops before session_start still leaves a
	// detached session running server-side — the conversation detail
	// exposes it so the poll can latch on without the SSE event.
	async function findRunningSessionId(convId: string): Promise<string> {
		const res = await api.get<{ running_session_id?: string }>(`/ai/conversations/${convId}`);
		return res.data?.running_session_id ?? '';
	}

	let bgCancelling = $state(false);

	// The composer's Stop only exists while a stream is attached — a
	// detached/polled turn needs its own cancel path to the same endpoint.
	async function cancelBackgroundSession() {
		const sid = bgSessionId;
		if (!sid || bgCancelling) return;
		bgCancelling = true;
		try {
			await api.post(`/ai/agent/sessions/${sid}/cancel`);
		} finally {
			bgCancelling = false;
		}
		// Keep polling — the next tick observes 'cancelled' and lands the
		// persisted cancellation notice.
	}

	function startSessionPoll(sessionId: string) {
		if (!sessionId || bgSessionId === sessionId) return;
		stopSessionPoll();
		bgSessionId = sessionId;
		const sid = sessionId;
		const startedAt = Date.now();
		let failures = 0;
		let inFlight = false;
		bgPollTimer = setInterval(() => {
			if (bgSessionId !== sid || inFlight) return;
			if (Date.now() - startedAt > BG_POLL_CAP_MS) {
				stopSessionPoll();
				return;
			}
			inFlight = true;
			void (async () => {
				try {
					const res = await api.get<{
						session: { status: string; conversation_id?: string };
					}>(`/ai/agent/sessions/${sid}`);
					// A stop/re-arm mid-request must not act on a stale session.
					if (bgSessionId !== sid) return;
					const status = res.data?.session?.status;
					if (res.error) {
						// api error shape is { code, message, status } — a 404 means
						// the session row is gone; anything else gets one retry.
						if (++failures >= 2 || res.error.code === 'session_not_found') stopSessionPoll();
						return;
					}
					failures = 0;
					if (status && status !== 'running') {
						stopSessionPoll();
						// The turn finished — clear any live placeholder spinner so a
						// failed refetch can't leave it stuck, and skip the reload
						// entirely while a fresh send is streaming (it would
						// clobber the live turn; the result persists regardless).
						const stuck = turns.find((tn) => tn.in_progress && tn.session_id === sid);
						if (stuck) {
							stuck.in_progress = false;
							stuck.progress = '';
						}
						// A fresh chat whose stream dropped before the `conversation`
						// event never set conversationId — the session payload's
						// conversation_id is authoritative for which thread owns it.
						const convId = res.data?.session?.conversation_id || conversationId || '';
						if (convId && !convos.some((c) => c.conversation_id === convId)) {
							// Mirror the `conversation` event's sidebar insert so the
							// thread isn't invisible until the next list refresh.
							const lastUser = turns.filter((tn) => tn.role === 'user').pop();
							convos = [
								{
									conversation_id: convId,
									project_id: pendingProjectId || undefined,
									turn_count: 1,
									legacy: false,
									preview: lastUser?.content?.slice(0, 80) ?? ''
								},
								...convos
							];
						}
						// Land the result only when the finished session belongs to
						// the thread on screen — or to the fresh chat that spawned it
						// (conversationId empty). A different conversationId means the
						// user moved on; the sidebar insert above is enough.
						if (!sending && convId && (!conversationId || conversationId === convId)) {
							await loadConversation(convId);
						}
					}
				} finally {
					inFlight = false;
				}
			})();
		}, BG_POLL_MS);
	}

	onDestroy(stopSessionPoll);

	// --- Tools state -------------------------------------------------------
	// Categories/tools come from GET /ai/agent/capabilities (already
	// platform+RBAC filtered). The composer can only narrow further — tools
	// the user's prefs disable render locked.
	const categories = $derived(capabilities?.categories ?? []);
	const toolsAvailable = $derived(categories.length > 0);

	// Per-tool enabled state keyed by function_name. Initialized from prefs
	// (disabled_tools / disabled_categories with enabled_tools exceptions —
	// mirrors AIPrefs.AllowsTool in internal/ai/prefs.go).
	function prefsAllows(name: string, category: string): boolean {
		if (!prefs) return true;
		if (prefs.disabled_tools?.includes(name)) return false;
		if (prefs.disabled_categories?.includes(category)) {
			return prefs.enabled_tools?.includes(name) ?? false;
		}
		return true;
	}

	let toolEnabled = $state<Record<string, boolean>>({});
	let toolsOn = $state(false);
	let toolsOpen = $state(false);
	// The tools button + popover — the window click handler ignores
	// clicks inside it so the popover only closes on a true outside click.
	let toolsBoxEl = $state<HTMLDivElement | null>(null);
	let prefsSaved = $state(false);

	$effect(() => {
		// Seed once capabilities/prefs arrive (first non-empty categories).
		if (categories.length > 0 && Object.keys(toolEnabled).length === 0) {
			const next: Record<string, boolean> = {};
			for (const cat of categories) {
				for (const tool of cat.tools) {
					next[tool.name] = prefsAllows(tool.name, cat.name);
				}
			}
			toolEnabled = next;
			// Tools default ON for eligible users — an explicit 'chat'
			// preference is the only thing that starts them off.
			toolsOn = prefs?.default_mode !== 'chat';
		}
	});

	const enabledToolCount = $derived(
		Object.values(toolEnabled).filter(Boolean).length
	);
	const totalToolCount = $derived(
		categories.reduce((n, c) => n + c.tools.length, 0)
	);
	// Tools the composer may toggle on — prefs-disabled tools are locked.
	const selectableCount = $derived(
		categories.reduce(
			(n, c) => n + c.tools.filter((tl) => prefsAllows(tl.name, c.name)).length,
			0
		)
	);

	function toolCategory(name: string): string {
		for (const cat of categories) {
			if (cat.tools.some((tl) => tl.name === name)) return cat.name;
		}
		return '';
	}

	function setCategory(name: string, on: boolean) {
		const cat = categories.find((c) => c.name === name);
		if (!cat) return;
		const next = { ...toolEnabled };
		for (const tool of cat.tools) {
			// Narrowing only — a prefs-disabled tool stays off.
			next[tool.name] = on && prefsAllows(tool.name, cat.name);
		}
		toolEnabled = next;
	}

	function categoryState(cat: { name: string; tools: { name: string }[] }): 'all' | 'some' | 'none' {
		const enabled = cat.tools.filter((tl) => toolEnabled[tl.name]).length;
		if (enabled === 0) return 'none';
		return enabled === cat.tools.length ? 'all' : 'some';
	}

	// Build the per-message `tools` override. Returns undefined when the
	// composer state matches prefs defaults (inherit), {enabled:false} when
	// the master switch is off, or a tool whitelist when narrowed.
	function toolsOverride(): ToolSelectionOverride | undefined {
		if (!toolsAvailable) return undefined;
		if (!toolsOn) return { enabled: false };
		const selected = Object.entries(toolEnabled)
			.filter(([, on]) => on)
			.map(([name]) => name);
		// FilterCatalog treats an empty whitelist as "no whitelist" — an
		// all-unchecked composer must send enabled:false, not tools:[].
		if (selected.length === 0) return { enabled: false };
		if (selected.length === selectableCount) return undefined;
		return { enabled: true, tools: selected };
	}

	// Serialize composer state back to AIPrefs without flattening category
	// semantics: a fully-off category stays a category (so future tools added
	// to it stay off); a partially-off category keeps the minority side as
	// exceptions — mostly-off → disabled_categories + enabled_tools,
	// mostly-on → disabled_tools.
	async function savePrefsAsDefault() {
		const disabledCats: string[] = [];
		const disabledTools: string[] = [];
		const enabledTools: string[] = [];
		for (const cat of categories) {
			const on = cat.tools.filter((tl) => toolEnabled[tl.name]).map((tl) => tl.name);
			const off = cat.tools.filter((tl) => !toolEnabled[tl.name]).map((tl) => tl.name);
			if (on.length === 0) {
				disabledCats.push(cat.name);
			} else if (off.length === 0) {
				// fully on — no pref entries
			} else if (on.length <= off.length) {
				disabledCats.push(cat.name);
				enabledTools.push(...on);
			} else {
				disabledTools.push(...off);
			}
		}
		const body = {
			default_mode: (toolsOn ? 'agent' : 'chat') as 'agent' | 'chat',
			disabled_categories: disabledCats,
			disabled_tools: disabledTools,
			enabled_tools: enabledTools,
			// Carry the template list through every prefs PUT — the endpoint
			// replaces the whole ai_prefs blob, so omitting it would wipe them.
			chat_templates: templates
		};
		const res = await api.put('/me/ai-preferences', body);
		if (!res.error) {
			savedPrefs = body;
			prefsSaved = true;
			setTimeout(() => (prefsSaved = false), 2000);
		}
	}

	// --- Chat templates ----------------------------------------------------
	// Latest persisted prefs snapshot — PUT merges over it so a template
	// save never silently reverts a tools-defaults save made earlier in the
	// session (and vice versa). untrack seeds once from load data; the prop
	// doesn't change after mount.
	let savedPrefs = $state<AIPrefs | null>(untrack(() => prefs));
	let templates = $state<ChatTemplate[]>(
		untrack(() =>
			(prefs?.chat_templates ?? []).map((tpl) => ({
				...tpl,
				folder: normalizeTemplateFolder(tpl.folder),
				values: normalizeTemplateValues(tpl.values)
			}))
		)
	);
	// Working editor copy — what the right rail edits and what an active
	// send actually carries (WYSIWYG: unsaved edits apply to sends, Save
	// only persists).
	let tplValues = $state<ChatTemplateValues>(structuredClone(DEFAULT_CHAT_TEMPLATE_VALUES));
	// Local copies for bind: — the panel's space/series selectors append
	// newly created rows, and load-data props aren't valid bind targets.
	let tplSpaces = $state(untrack(() => spaces));
	let tplSeries = $state(untrack(() => series));
	let activeTemplateId = $state('');
	let templateActive = $state(false);
	// SSR + first client paint render the rail closed — opening it here
	// would emit a full-height overlay into mobile SSR HTML that covers
	// the transcript until hydration flips it. onMount opens it on lg+.
	let templatePanelOpen = $state(false);

	const activeTemplate = $derived(templates.find((tpl) => tpl.id === activeTemplateId));
	const templateDirty = $derived(
		!!activeTemplate &&
			templateFingerprint(tplValues) !==
				templateFingerprint(normalizeTemplateValues(activeTemplate.values))
	);
	const activeTemplateLabel = $derived(
		activeTemplate?.name || t('ai.templates.custom', 'Custom template')
	);

	// Persist the full template list inside ai_prefs. Returns an error
	// message or null — the panel renders the error inline.
	async function persistTemplates(next: ChatTemplate[]): Promise<string | null> {
		// PUT replaces the whole ai_prefs blob. If the snapshot never
		// loaded (initial GET failed), refetch before writing — sending
		// empty fields here would silently wipe default_mode + tool prefs.
		if (!savedPrefs) {
			const res = await api.get<AIPrefs>('/me/ai-preferences');
			if (res.error || !res.data) {
				return (
					res.error?.message || t('ai.templates.save_failed', 'Could not save templates.')
				);
			}
			savedPrefs = res.data;
		}
		const body = {
			default_mode: (savedPrefs?.default_mode ?? '') as AIPrefs['default_mode'],
			disabled_categories: savedPrefs?.disabled_categories ?? [],
			disabled_tools: savedPrefs?.disabled_tools ?? [],
			enabled_tools: savedPrefs?.enabled_tools ?? [],
			chat_templates: next
		};
		const res = await api.put('/me/ai-preferences', body);
		if (res.error) {
			return res.error.message || t('ai.templates.save_failed', 'Could not save templates.');
		}
		templates = next;
		savedPrefs = body;
		return null;
	}

	// "Insert as editable prompt" opens the Posts modal — post targeting
	// (single title / counted set / bulk titles) is per-insert input, not
	// part of the saved template. The composer's existing tail seeds the
	// modal so a re-insert brings the typed title/topics back instead of
	// clobbering them; the spec body is rebuilt from the template.
	let insertModalOpen = $state(false);
	let insertValues = $state<ChatTemplateValues | null>(null);
	let insertMode = $state<ChatTemplatePostMode>('single');
	let insertTitle = $state('');
	let insertCount = $state(3);
	let insertTopics = $state('');

	function insertTemplateText(tpl?: ChatTemplate) {
		insertValues = tpl ? normalizeTemplateValues(tpl.values) : tplValues;
		const tail = splitComposerForInsert(input).trim();
		insertMode = 'single';
		insertTitle = tail && !tail.includes('\n') ? tail.replace(/^[-*•]\s*/, '') : '';
		insertCount = 3;
		insertTopics = tail;
		insertModalOpen = true;
	}

	function confirmInsert() {
		const v = insertValues;
		if (!v) return;
		// Resolve spec ids to display names so the prose reads as names —
		// same catalogs the panel's selectors draw from.
		const labels = {
			space: tplSpaces.find((s) => s.id === v.space_id)?.name,
			series: tplSeries.find((s) => s.id === v.series_id)?.title,
			voice: voices.find((x) => x.id === v.voice_id)?.name
		};
		const posts: ChatTemplatePostTarget = {
			mode: insertMode,
			title: insertTitle.trim(),
			count: Math.max(1, Math.floor(insertCount) || 1),
			topics: insertTopics
				.split('\n')
				.map((s) => s.trim().replace(/^[-*•]\s*/, ''))
				.filter(Boolean)
		};
		input = buildTemplateInsertText(v, labels, posts) + '\n' + buildInsertTail(posts);
		templateActive = false;
		insertModalOpen = false;
		tick().then(() => {
			resizeInput();
			inputEl?.focus();
		});
	}

	onMount(() => {
		// §115: ?c= deep link from the quick panel — land on the anchored
		// thread. A bad/foreign id fails soft via loadConversation's error
		// path, same as a bad sidebar click.
		if (initialConversationId) void loadConversation(initialConversationId);
		// Open the rail on desktop (it's a static column there); on small
		// screens it stays closed as an on-demand overlay.
		chatTemplateRail.mounted = true;
		templatePanelOpen = window.matchMedia('(min-width: 1024px)').matches;
		return () => {
			chatTemplateRail.open = false;
			chatTemplateRail.mounted = false;
		};
	});

	// The layout hides the widget sidebar only while this rail owns the
	// right edge — closing the panel brings the widgets back.
	$effect(() => {
		chatTemplateRail.open = templatePanelOpen;
	});

	// Templates are an agent-mode feature — activating tools opens the
	// rail by default (desktop only: on mobile it's a covering overlay,
	// so it stays on-demand); switching tools off closes it. The effect
	// never reads templatePanelOpen, so a manual toggle isn't overridden.
	$effect(() => {
		if (!toolsOn) templatePanelOpen = false;
		else if (window.matchMedia('(min-width: 1024px)').matches) templatePanelOpen = true;
	});

	// --- Composer pickers ---------------------------------------------------
	let selectedModel = $state('');
	let selectedVoice = $state('');
	let selectedTone = $state('');
	// The backend already emits an empty-id "Default" entry for models
	// (/ai/models), so only prepend our own when none exists — two value:''
	// options would collide on the picker's __none__ key.
	// BYOK models encode as "byok|<keyId>|<modelId>" so a single flat option
	// value carries the key reference — the send path splits on '|'. The
	// "🔑" prefix visually separates user-key models from platform ones.
	const modelOptions = $derived([
		...(models.some((m) => m.id === '')
			? []
			: [{ value: '', label: t('ai.chat.model_default', 'Default model') }]),
		...models.map((m) => ({ value: m.id, label: m.label })),
		...byokProviders.flatMap((bp) =>
			bp.keys.flatMap((k) =>
				bp.models.map((m) => ({
					value: `byok|${k.id}|${m.id}`,
					label: `🔑 ${k.label || bp.display_name} · ${m.label || m.id}`
				}))
			)
		)
	]);
	// Composer-only: the "Add API key" sentinel opens the BYOK dialog instead
	// of selecting a model. Kept off `modelOptions` because the project
	// default pickers share it and must not offer key management.
	const ADD_BYOK_OPTION = '__byok_add_key';
	let addKeyOpen = $state(false);
	const composerModelOptions = $derived([
		...modelOptions,
		...(byokProviders.length > 0
			? [{ value: ADD_BYOK_OPTION, label: `+ ${t('ai.byok.add_key', 'Add API key')}` }]
			: [])
	]);
	$effect(() => {
		if (selectedModel === ADD_BYOK_OPTION) {
			selectedModel = '';
			addKeyOpen = true;
		}
	});
	// Voice option sets populate from the selected tone — the persona catalog
	// prefixes each voice label with its tone ("Witty ...", "Academic ...").
	// Same mapping as AiDraftPanel: unlabelled voices count as 'natural';
	// no tone selected shows the full catalog.
	function voiceToneFromLabel(label: string): string {
		const lower = label.toLowerCase();
		for (const value of TONE_VALUES) {
			if (value === 'natural') continue;
			if (lower.startsWith(value + ' ')) {
				return value;
			}
		}
		return 'natural';
	}
	const voiceOptions = $derived([
		...(voices.some((v) => v.id === '')
			? []
			: [{ value: '', label: t('ai.chat.voice_default', 'Default voice') }]),
		...voices
			.filter((v) => !selectedTone || voiceToneFromLabel(v.name) === selectedTone)
			.map((v) => ({ value: v.id, label: v.name }))
	]);
	// A previously selected voice that the new tone filters out falls back
	// to the default — never send a voice the picker no longer offers.
	$effect(() => {
		const available = new Set(voiceOptions.map((v) => v.value));
		if (selectedVoice !== '' && !available.has(selectedVoice)) {
			selectedVoice = '';
		}
	});
	// Response-style steering sent via context.tone (merged into the AI
	// context payload — same allowlisted key used by the assist endpoints).
	// Empty value = no override; the server default applies.
	const toneOptions = $derived([
		{ value: '', label: t('ai.picker.tone_default', 'Default tone') },
		{ value: 'natural', label: t('ai.body.tone_natural', 'Natural') },
		{ value: 'friendly', label: t('ai.body.tone_friendly', 'Friendly') },
		{ value: 'witty', label: t('ai.body.tone_witty', 'Witty') },
		{ value: 'empathetic', label: t('ai.body.tone_empathetic', 'Empathetic') },
		{ value: 'direct', label: t('ai.body.tone_direct', 'Direct') },
		{ value: 'eloquent', label: t('ai.body.tone_eloquent', 'Eloquent') },
		{ value: 'professional', label: t('ai.body.tone_professional', 'Professional') },
		{ value: 'persuasive', label: t('ai.body.tone_persuasive', 'Persuasive') },
		{ value: 'motivational', label: t('ai.body.tone_motivational', 'Motivational') },
		{ value: 'promoter', label: t('ai.body.tone_promoter', 'Promoter') },
		{ value: 'influencer', label: t('ai.body.tone_influencer', 'Influencer') },
		{ value: 'marketer', label: t('ai.body.tone_marketer', 'Marketer') },
		{ value: 'concise', label: t('ai.body.tone_concise', 'Concise') },
		{ value: 'academic', label: t('ai.body.tone_academic', 'Academic') },
		{ value: 'casual', label: t('ai.body.tone_casual', 'Casual') }
	]);
	const TONE_VALUES = $derived(
		toneOptions.map((o) => o.value).filter((v) => v !== '')
	);

	const suggestions = $derived([
		t('ai.chat.suggestion_1', 'What documents do I need for a UK student visa?'),
		t('ai.chat.suggestion_2', 'How do I find scholarships for masters?'),
		t('ai.chat.suggestion_3', "What's the cost of living in Toronto vs Lagos?"),
		t('ai.chat.suggestion_4', 'How do I send money home cheaply?')
	]);

	// Auto-scroll on transcript growth or tool-call updates.
	$effect(() => {
		void turns.length;
		void turns[turns.length - 1]?.tool_calls?.length;
		if (scrollEl) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});

	// --- Conversation load/delete ------------------------------------------
	function turnFromToolCall(tc: ConversationToolCall): ToolCallEntry {
		return {
			id: ++toolSeq,
			tool_name: tc.tool_name,
			arguments: tc.arguments !== undefined ? JSON.stringify(tc.arguments) : undefined,
			status: statusToToolStatus(tc.status),
			result: tc.result !== undefined ? JSON.stringify(tc.result) : undefined,
			error: tc.error_message,
			iteration: 0
		};
	}

	async function loadConversation(id: string) {
		stopSessionPoll(); // a stale poll must not fire mid-load
		loadingConversation = true;
		const res = await api.get<{
			conversation_id: string;
			project_id?: string;
			running_session_id?: string;
			messages?: ConversationMessage[];
			entries?: { id: string; content: string; metadata?: Record<string, unknown>; created_at: string }[];
		}>(`/ai/conversations/${id}`);
		loadingConversation = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? t('ai.chat.error', 'Failed to load conversation');
			return;
		}
		conversationId = res.data.conversation_id;
		error = '';
		// §103 AI Projects — keep the sidebar entry's membership in sync with
		// the detail response in BOTH directions (a membership changed on
		// another device must un-group here too) and pre-fill the composer
		// from the project's pinned defaults (overridable, per §103).
		const detailProject = res.data.project_id || undefined;
		convos = convos.map((c) =>
			c.conversation_id === conversationId ? { ...c, project_id: detailProject } : c
		);
		pendingProjectId = null;
		if (detailProject) {
			const p = projs.find((x) => x.id === detailProject);
			if (p) applyProjectPins(p);
		} else {
			// Ungrouped thread — restore global composer defaults so a
			// previous project's pins don't leak into it.
			resetComposerDefaults();
		}
		// Unified threads return `messages`; legacy Honcho threads return
		// `entries` and are read-only.
		viewingLegacy = res.data.messages === undefined;
		if (res.data.messages) {
			// Unified thread — assistant messages may carry tool_calls.
			turns = res.data.messages.map((m) => ({
				id: ++turnSeq,
				role: m.role === 'user' ? 'user' : 'assistant',
				content: m.content,
				session_id: m.agent_session_id,
				tool_calls: m.tool_calls?.map(turnFromToolCall)
			}));
		} else {
			// Legacy Honcho thread — entries carry role in metadata.
			turns = (res.data.entries ?? []).map((e) => ({
				id: ++turnSeq,
				role: (e.metadata?.role === 'user' ? 'user' : 'assistant') as 'user' | 'assistant',
				content: e.content
			}));
		}
		sidebarOpen = false;
		// A detached turn may still be running server-side for this thread —
		// poll it and land the persisted result when it finishes.
		if (res.data.running_session_id && !sending) {
			startSessionPoll(res.data.running_session_id);
		}
	}

	function deleteConversation(id: string) {
		// Destructive: the thread is gone for good — same in-app modal
		// confirm the project delete uses.
		confirmDialog = {
			title: t('ai.chat.delete_title', 'Delete conversation?'),
			body: t('ai.chat.delete_confirm', 'Delete this conversation? This cannot be undone.'),
			confirmLabel: t('common.delete', 'Delete'),
			run: async () => {
				const res = await api.delete(`/ai/conversations/${id}`);
				if (!res.error) {
					convos = convos.filter((c) => c.conversation_id !== id);
					if (conversationId === id) startNewChat();
				}
			}
		};
	}

	function startNewChat() {
		stopSessionPoll();
		conversationId = '';
		turns = [];
		viewingLegacy = false;
		pendingProjectId = null;
		// Plain new chat returns to global defaults — a previous project's
		// pins don't carry over.
		resetComposerDefaults();
		sidebarOpen = false;
	}

	// Copy-to-clipboard feedback — which turn id most recently copied; the
	// check icon resets after a short window.
	let copiedTurn = $state<number | null>(null);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	async function copyTurn(turn: UnifiedTurn) {
		const text = turn.streamed || turn.content;
		if (!text) return;
		try {
			await navigator.clipboard.writeText(text);
			copiedTurn = turn.id;
			clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copiedTurn = null), 1500);
		} catch {
			// Clipboard API unavailable (insecure context / denied) — no-op.
		}
	}

	// --- Send / stream -------------------------------------------------------
	async function send() {
		const message = input.trim();
		if (!message || sending || viewingLegacy) return;

		// A poll armed for THIS thread means a detached session is still
		// running server-side — the send would 409 (one running turn per
		// conversation). An empty conversationId means the running session
		// belongs to a thread this client never adopted; a fresh send then
		// creates a different conversation and is allowed.
		if (bgSessionId && conversationId) {
			error = t(
				'ai.agent.bg_busy',
				'An agent task is still running for this conversation — wait for it to finish or cancel it.'
			);
			return;
		}

		error = '';
		input = '';
		sending = true;

		// Active chat template: the serialized spec rides along inside the
		// message — the model maps it onto content.generate_draft's arguments
		// in agent mode or honors it as drafting constraints in plain chat.
		const spec = templateActive ? buildTemplateSpec(tplValues) : null;
		const outgoing = spec
			? message + buildTemplateBlock(activeTemplateLabel, tplValues)
			: message;

		const userTurn: UnifiedTurn = {
			id: ++turnSeq,
			role: 'user',
			content: message,
			template_name: spec ? activeTemplateLabel : undefined
		};
		const assistantTurn: UnifiedTurn = {
			id: ++turnSeq,
			role: 'assistant',
			content: '',
			in_progress: true
		};
		turns = [...turns, userTurn, assistantTurn];
		// Mutate via the $state proxy — writes to the raw object bypass reactivity.
		const liveTurn = turns[turns.length - 1];

		// Merge template fields into the allowlisted context keys the
		// endpoint accepts (tone/audience/content_type/space_id/tags) — the
		// composer's own tone picker wins when set.
		const ctxPayload: Record<string, unknown> = {};
		if (selectedTone) ctxPayload.tone = selectedTone;
		if (spec) {
			if (!ctxPayload.tone && tplValues.tone) ctxPayload.tone = tplValues.tone;
			if (tplValues.audience) ctxPayload.audience = tplValues.audience;
			if (tplValues.content_type) ctxPayload.content_type = tplValues.content_type;
			if (tplValues.space_id) ctxPayload.space_id = tplValues.space_id;
			if (tplValues.tags.length) ctxPayload.tags = tplValues.tags;
		}

		const abort = new AbortController();
		activeAbort = abort;
		activeSessionId = null;
		cancelling = false;
		// Decide detach-vs-discard by what was SENT, not what the toggle
		// shows now — it could flip while the stream is live.
		const sentWithTools = toolsOn;

		// BYOK selection: composite "byok|<keyId>|<modelId>" → real fields.
		let byokKeyId: string | undefined;
		let outgoingModel = selectedModel || undefined;
		if (selectedModel.startsWith('byok|')) {
			const parts = selectedModel.split('|');
			byokKeyId = parts[1] || undefined;
			outgoingModel = parts[2] || undefined;
		}

		try {
			for await (const ev of streamChat({
				message: outgoing,
				byokKeyId,
				conversationId: conversationId || undefined,
				// §103 AI Projects: a pending project stamps the NEW thread —
				// existing conversations keep their own membership.
				projectId: conversationId ? undefined : (pendingProjectId ?? undefined),
				model: outgoingModel,
				// Template voice fills in when the composer picker is unset —
				// same precedence as the tone merge above.
				voiceId: selectedVoice || (spec ? tplValues.voice_id : '') || undefined,
				context: Object.keys(ctxPayload).length > 0 ? ctxPayload : undefined,
				// An active template addresses humanization explicitly — 'off'
				// sends humanize:false, any level sends the request-level value.
				humanize: spec ? tplValues.humanize_level !== 'off' : undefined,
				humanizeLevel:
					spec && tplValues.humanize_level !== 'off' ? tplValues.humanize_level : undefined,
				tools: toolsOverride(),
				signal: abort.signal
			})) {
				handleEvent(ev, liveTurn);
			}
			if (liveTurn.in_progress) {
				// A drop before session_start leaves no session id — recover the
				// detached session from the conversation detail when there is one.
				let sid = liveTurn.session_id ?? '';
				if (!sid && sentWithTools && conversationId) {
					sid = await findRunningSessionId(conversationId);
				}
				if (sid && sentWithTools) {
					// Detached turn — the run continues server-side; poll the
					// session and land the persisted message on completion.
					liveTurn.progress = t('ai.agent.bg_running', 'Working in the background…');
					liveTurn.session_id = sid;
					startSessionPoll(sid);
				} else {
					liveTurn.in_progress = false;
					if (!liveTurn.content && !liveTurn.error) {
						liveTurn.content = t('ai.agent.stream_closed', 'Stream closed.');
					}
				}
				turns = [...turns];
			}
		} catch (e) {
			liveTurn.in_progress = false;
			if (e instanceof AgentRequestError) {
				// The pending project vanished elsewhere (deleted in another
				// tab) — drop it so a retry creates a plain ungrouped thread
				// instead of 404ing forever.
				if (e.code === 'project_not_found' && pendingProjectId) {
					projs = projs.filter((p) => p.id !== pendingProjectId);
					pendingProjectId = null;
					resetComposerDefaults();
				}
				liveTurn.error = e.message;
				error = e.message;
				if (!liveTurn.content && !liveTurn.tool_calls?.length) {
					turns = turns.filter((tn) => tn.id !== liveTurn.id);
				}
			} else if (e instanceof DOMException && e.name === 'AbortError') {
				liveTurn.error = t('ai.agent.cancelled', 'Cancelled.');
			} else if (sentWithTools) {
				// The reader threw mid-stream (network drop) — a detached
				// turn keeps running server-side; poll to completion instead
				// of erroring out a live background task. A drop before
				// session_start may leave session_id unset — recover it
				// from the conversation detail.
				let sid = liveTurn.session_id ?? '';
				if (!sid && conversationId) sid = await findRunningSessionId(conversationId);
				if (sid) {
					liveTurn.in_progress = true;
					liveTurn.progress = t('ai.agent.bg_running', 'Working in the background…');
					liveTurn.session_id = sid;
					startSessionPoll(sid);
				} else {
					liveTurn.error = e instanceof Error ? e.message : String(e);
					error = liveTurn.error;
				}
			} else {
				liveTurn.error = e instanceof Error ? e.message : String(e);
				error = liveTurn.error;
			}
			turns = [...turns];
		} finally {
			sending = false;
			activeAbort = null;
			activeSessionId = null;
			cancelling = false;
		}
	}

	function handleEvent(ev: AgentEvent, turn: UnifiedTurn) {
		switch (ev.type) {
			case 'session_start': {
				turn.session_id = ev.session_id;
				turn.presearch_level = ev.presearch_level;
				activeSessionId = ev.session_id ?? null;
				turns = [...turns];
				break;
			}
			case 'conversation': {
				// ADR-130: carries the thread id — adopt it so follow-ups and the
				// sidebar resolve to the persisted conversation.
				if (ev.conversation_id) {
					conversationId = ev.conversation_id;
					if (!convos.some((c) => c.conversation_id === ev.conversation_id)) {
						// Latest user turn, not the first — the first message of
						// the transcript is stale when this event re-inserts a
						// row for a conversation the sidebar didn't have loaded.
						const lastUser = turns.filter((tn) => tn.role === 'user').pop();
						convos = [
							{
								conversation_id: ev.conversation_id,
								project_id: ev.project_id || pendingProjectId || undefined,
								turn_count: ev.turn_count ?? 1,
								legacy: false,
								preview: lastUser?.content?.slice(0, 80) ?? ''
							},
							...convos
						];
					}
					pendingProjectId = null;
				}
				break;
			}
			case 'sources': {
				turn.sources = ev.sources;
				turns = [...turns];
				break;
			}
			case 'tool_call_start': {
				const entry: ToolCallEntry = {
					id: ++toolSeq,
					tool_name: ev.tool_name ?? '',
					arguments: ev.arguments,
					status: 'pending',
					iteration: ev.iteration ?? 0
				};
				turn.tool_calls = [...(turn.tool_calls ?? []), entry];
				// Any deltas accumulated before this call were preamble text
				// from an intermediate iteration — transient, never persisted.
				// Clear so the final iteration streams into a clean bubble.
				turn.streamed = '';
				turns = [...turns];
				break;
			}
			case 'tool_call_result': {
				const calls = turn.tool_calls ?? [];
				let matched = false;
				for (let i = calls.length - 1; i >= 0; i--) {
					if (calls[i].status === 'pending' && (!ev.tool_name || calls[i].tool_name === ev.tool_name)) {
						calls[i].status = statusToToolStatus(ev.status);
						calls[i].result = ev.content;
						calls[i].error = ev.error;
						matched = true;
						break;
					}
				}
				if (!matched) {
					const last = calls[calls.length - 1];
					if (last) {
						last.status = statusToToolStatus(ev.status);
						last.result = ev.content;
						last.error = ev.error;
					}
				}
				turn.tool_calls = [...calls];
				turns = [...turns];
				break;
			}
			case 'delta': {
				// Token-level fragment (ADR-130 Phase 2) — accumulate; the
				// template renders streamed text over the progress placeholder.
				turn.streamed = (turn.streamed ?? '') + (ev.content ?? '');
				turns = [...turns];
				break;
			}
			case 'progress': {
				// Progress is a transient status line — keep it out of
				// `content` so an error/stream-close can't leave a stale
				// "LLM iteration 1" bubble that reads like a reply.
				if (ev.message) {
					turn.progress = ev.message;
					turns = [...turns];
				}
				break;
			}
			case 'final_response': {
				turn.content = ev.content ?? '';
				turn.streamed = '';
				turn.progress = '';
				turn.tokens_used = ev.tokens_used;
				turn.estimated_cost = ev.estimated_cost;
				if (ev.conversation_id) conversationId = ev.conversation_id;
				turn.in_progress = false;
				// Keep the sidebar row fresh — turn_count comes from the
				// server (authoritative persisted count); fall back to +2 for
				// streams that predate the field.
				const conv = convos.find((c) => c.conversation_id === conversationId);
				if (conv) {
					conv.turn_count = ev.turn_count ?? (conv.turn_count ?? 0) + 2;
					conv.preview = (ev.content ?? '').slice(0, 80);
					convos = [...convos];
				}
				turns = [...turns];
				break;
			}
			case 'error': {
				// Promote any streamed partial into committed content so the
				// bubble shows what arrived before the failure, then the error.
				// The partial wins over the transient progress placeholder
				// that progress events write into content.
				if (turn.streamed) {
					turn.content = turn.streamed;
				}
				turn.streamed = '';
				turn.progress = '';
				turn.error = ev.error ?? 'Unknown error';
				turn.in_progress = false;
				turns = [...turns];
				break;
			}
		}
	}

	async function cancel() {
		if (!activeAbort || cancelling) return;
		cancelling = true;
		stopSessionPoll();
		activeAbort.abort();
		if (activeSessionId) {
			try {
				await api.post(`/ai/agent/sessions/${activeSessionId}/cancel`);
			} catch {
				// Suppress — the stream is already aborted.
			}
		}
	}

	function resizeInput() {
		if (!inputEl) return;
		inputEl.style.height = 'auto';
		inputEl.style.height = Math.min(inputEl.scrollHeight, 160) + 'px';
	}

	$effect(() => {
		// Keep the composer height in sync with its content (incl. reset
		// after send and wrapped placeholder text). Measure after paint —
		// on mount the flex width may not be resolved yet.
		input;
		const id = requestAnimationFrame(resizeInput);
		window.addEventListener('resize', resizeInput);
		return () => {
			cancelAnimationFrame(id);
			window.removeEventListener('resize', resizeInput);
		};
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	// --- Presentation helpers ------------------------------------------------
	const toolIcon: Record<string, typeof Wrench> = {
		content: FileText,
		rag: BookOpen,
		external: Globe,
		custom_field: Tag,
		comparison: LayoutGrid,
		spaces: Globe,
		academy: GraduationCap,
		docs: FileText,
		tools: Wrench,
		menu: LayoutGrid,
		user: UsersIcon
	};

	function iconForTool(name: string): typeof Wrench {
		return toolIcon[name.split('.')[0] ?? ''] ?? Wrench;
	}

	function statusToToolStatus(s: string | undefined): ToolStatus {
		switch (s) {
			case 'success': return 'success';
			case 'error': return 'error';
			case 'unauthorized': return 'unauthorized';
			case 'privacy_blocked': return 'privacy_blocked';
			case 'hitl_queued': return 'hitl_queued';
			default: return 'pending';
		}
	}

	function statusColor(status: ToolStatus): string {
		switch (status) {
			case 'success': return 'var(--color-success, #16a34a)';
			case 'error': return 'var(--color-danger, #dc2626)';
			case 'unauthorized': return 'var(--color-warning, #d97706)';
			case 'privacy_blocked': return 'var(--color-warning, #d97706)';
			case 'hitl_queued': return 'var(--color-info, #2563eb)';
			default: return 'var(--color-muted)';
		}
	}

	// Content items a successful tool call produced — create_draft /
	// create / bulk_create (and status transitions) return
	// {content_id, title, slug, status}, so the chat can link the created
	// item instead of leaving the result buried in the JSON details.
	function createdItemLinks(tc: ToolCallEntry): { href: string; label: string }[] {
		if (tc.status !== 'success' || !tc.result) return [];
		let parsed: unknown;
		try {
			parsed = JSON.parse(tc.result);
		} catch {
			return [];
		}
		const links: { href: string; label: string }[] = [];
		const push = (o: unknown) => {
			if (!o || typeof o !== 'object') return;
			const rec = o as Record<string, unknown>;
			const slug = typeof rec.slug === 'string' ? rec.slug : '';
			if (!slug || links.some((l) => l.href === `/content/${slug}`)) return;
			const title = typeof rec.title === 'string' && rec.title.trim() ? rec.title.trim() : slug;
			links.push({ href: `/content/${slug}`, label: title });
		};
		if (Array.isArray(parsed)) parsed.forEach(push);
		else if (parsed && typeof parsed === 'object') {
			push(parsed);
			for (const key of ['items', 'results', 'created', 'contents']) {
				const arr = (parsed as Record<string, unknown>)[key];
				if (Array.isArray(arr)) arr.forEach(push);
			}
		}
		return links;
	}

	function statusLabel(status: ToolStatus): string {
		switch (status) {
			case 'success': return t('ai.agent.tool.success', 'Success');
			case 'error': return t('ai.agent.tool.error', 'Error');
			case 'unauthorized': return t('ai.agent.tool.unauthorized', 'Denied');
			case 'privacy_blocked': return t('ai.agent.tool.privacy_blocked', 'Privacy blocked');
			case 'hitl_queued': return t('ai.agent.tool.hitl_queued', 'Awaiting review');
			case 'pending': return t('ai.agent.tool.pending', 'Running');
		}
	}

	function fmtCost(c: number | undefined): string {
		if (!c) return '';
		return `$${c.toFixed(4)}`;
	}

	function fmtTokens(n: number | undefined): string {
		if (!n) return '';
		return n.toLocaleString();
	}
</script>

<div class="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col gap-4 lg:flex-row">
	<!-- Conversation sidebar -->
	<aside
		class="w-full lg:w-64 lg:shrink-0 {sidebarOpen
			? 'flex'
			: 'hidden lg:flex'} lg:ms-4 max-h-[50vh] lg:max-h-full flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
	>
		<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('ai.chat.history', 'Conversations')}</h2>
			<div class="flex items-center gap-2">
				<button
					type="button"
					onclick={() => openProjectDialog('create')}
					class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
					aria-label={t('ai.projects.new', 'New project')}
					title={t('ai.projects.new', 'New project')}
				>
					<FolderPlus size={14} />
				</button>
				<button
					type="button"
					onclick={startNewChat}
					class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
				>
					<Plus size={14} />
					{t('ai.chat.new', 'New')}
				</button>
			</div>
		</div>
		<!-- Scrolling detaches a fixed-position flyout from its anchor —
			close any open menus instead of leaving a floating submenu. -->
		<div
			class="flex-1 overflow-y-auto p-2"
			onscroll={() => {
				moveMenuFor = null;
				moveSubOpen = false;
				projMenuFor = null;
			}}
		>
			{#if convos.length === 0 && projs.length === 0}
				<p class="p-2 text-center text-xs text-[var(--color-muted)]">{t('ai.chat.no_history', 'No previous conversations.')}</p>
			{:else}
				<ul class="space-y-1">
					{#each projs as proj (proj.id)}
						{@const members = convsInProject(proj.id)}
						<li>
							<div class="group/proj relative">
								<button
									type="button"
									onclick={() => toggleProjectCollapsed(proj.id)}
									class="flex w-full items-center gap-1.5 rounded-[var(--radius)] px-2 py-1.5 text-start text-xs font-semibold text-[var(--color-muted)] hover:text-[var(--color-text)]"
								>
									{#if collapsedProjects.has(proj.id)}
										<ChevronRight class="rtl:-scale-x-100" size={12} />
									{:else}
										<ChevronDown size={12} />
									{/if}
									<Folder size={13} class="shrink-0" />
									<span class="truncate">{proj.name}</span>
									<span class="ms-auto shrink-0 pe-5">{members.length}</span>
								</button>
								<div class="absolute end-1 top-1 opacity-0 transition-opacity group-hover/proj:opacity-100 group-focus-within/proj:opacity-100">
									<button
										type="button"
										onclick={(e) => {
											e.stopPropagation();
											projMenuFor = projMenuFor === proj.id ? null : proj.id;
										}}
										class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
										aria-label={t('ai.chat.more_actions', 'More actions')}
										title={t('ai.chat.more_actions', 'More actions')}
									>
										<Ellipsis size={12} />
									</button>
									{#if projMenuFor === proj.id}
										<div
											class="absolute end-0 top-6 z-30 w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
											role="menu"
										>
											<button
												type="button"
												onclick={(e) => {
													e.stopPropagation();
													projMenuFor = null;
													startNewChatInProject(proj);
												}}
												class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
											>
												<Plus size={12} class="shrink-0 text-[var(--color-muted)]" />
												<span class="flex-1 truncate">{t('ai.projects.new_chat', 'New chat in project')}</span>
											</button>
											<button
												type="button"
												onclick={(e) => {
													e.stopPropagation();
													projMenuFor = null;
													openProjectDialog(proj);
												}}
												class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
											>
												<Pencil size={12} class="shrink-0 text-[var(--color-muted)]" />
												<span class="flex-1 truncate">{t('ai.projects.edit', 'Edit project')}</span>
											</button>
											<div class="my-1 border-t border-[var(--color-border)]"></div>
											<button
												type="button"
												onclick={(e) => {
													e.stopPropagation();
													projMenuFor = null;
													deleteProject(proj);
												}}
												class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-danger)] hover:bg-[var(--color-surface-subtle)]"
											>
												<Trash2 size={12} class="shrink-0" />
												<span class="flex-1 truncate">{t('ai.projects.delete', 'Delete project')}</span>
											</button>
										</div>
									{/if}
								</div>
							</div>
							{#if !collapsedProjects.has(proj.id)}
								{#if members.length > 0}
									<ul class="ms-3 space-y-1 border-s border-[var(--color-border)] ps-2">
										{#each members as conv (conv.conversation_id)}
											{@render convItem(conv)}
										{/each}
									</ul>
								{/if}
							{/if}
						</li>
					{/each}
					{#if ungroupedConvos.length > 0}
						{#if projs.length > 0}
							<li class="px-2 pb-1 pt-2 text-xs font-semibold text-[var(--color-muted)]">
								{t('ai.projects.ungrouped', 'Ungrouped')}
							</li>
						{/if}
						{#each ungroupedConvos as conv (conv.conversation_id)}
							{@render convItem(conv)}
						{/each}
					{/if}
				</ul>
			{/if}
		</div>
	</aside>

	<!-- One conversation card — rendered inside each project group and the
	     ungrouped section so both share the row markup. -->
	{#snippet convItem(conv: UnifiedConversationSummary)}
		<li class="group relative">
			<button
				type="button"
				onclick={() => loadConversation(conv.conversation_id)}
				class="w-full rounded-[var(--radius)] px-3 py-2 text-start text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] {conversationId === conv.conversation_id ? 'bg-[var(--color-primary)]/10 font-medium' : ''}"
			>
				<div class="flex items-center gap-1.5">
					<span class="truncate">{conv.title || conv.preview || t('ai.chat.unnamed', 'Conversation')}</span>
					{#if conv.legacy}
						<span class="shrink-0 rounded-full border px-1.5 py-px text-[10px]" style="border-color: var(--color-border); color: var(--color-muted)">
							{t('ai.chat.legacy', 'legacy')}
						</span>
					{/if}
				</div>
				<div class="mt-0.5 text-xs text-[var(--color-muted)]">
					{conv.turn_count} {conv.turn_count === 1 ? t('ai.chat.turn', 'turn') : t('ai.chat.turns', 'turns')}
				</div>
			</button>
			{#if !conv.legacy}
				<!-- §103 — overflow menu (ChatGPT-style): row actions plus a
					"Move to project" flyout listing the catalog. -->
				<button
					type="button"
					onclick={(e) => {
						e.stopPropagation();
						moveSubOpen = false;
						moveMenuFor = moveMenuFor === conv.conversation_id ? null : conv.conversation_id;
					}}
					class="absolute end-1.5 top-1.5 rounded p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:text-[var(--color-text)] focus:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100"
					aria-label={t('ai.chat.more_actions', 'More actions')}
					title={t('ai.chat.more_actions', 'More actions')}
				>
					<Ellipsis size={13} />
				</button>
				{#if moveMenuFor === conv.conversation_id}
					<div
						class="absolute end-0 top-8 z-30 w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
						role="menu"
					>
						<button
							type="button"
							onclick={toggleMoveSub}
							class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							aria-haspopup="menu"
							aria-expanded={moveSubOpen}
						>
							<FolderInput size={12} class="shrink-0 text-[var(--color-muted)]" />
							<span class="flex-1 truncate">{t('ai.projects.move', 'Move to project')}</span>
							<ChevronRight size={11} class="shrink-0 text-[var(--color-muted)] rtl:-scale-x-100" />
						</button>
						{#if conv.project_id}
							<button
								type="button"
								onclick={(e) => {
									e.stopPropagation();
									assignProject(conv.conversation_id, null);
								}}
								class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								<FolderOutput size={12} class="shrink-0 text-[var(--color-muted)]" />
								<span class="flex-1 truncate">{t('ai.projects.remove', 'Remove from project')}</span>
							</button>
						{/if}
						<div class="my-1 border-t border-[var(--color-border)]"></div>
						<button
							type="button"
							onclick={(e) => {
								e.stopPropagation();
								deleteConversation(conv.conversation_id);
							}}
							class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-danger)] hover:bg-[var(--color-surface-subtle)]"
						>
							<Trash2 size={12} class="shrink-0" />
							<span class="flex-1 truncate">{t('ai.chat.delete', 'Delete conversation')}</span>
						</button>
					</div>
					{#if moveSubOpen}
						<div
							class="fixed z-40 w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
							style="top: {flyoutPos?.top ?? 0}px; inset-inline-start: {flyoutPos?.left ?? 0}px"
							role="menu"
						>
							<button
								type="button"
								onclick={(e) => {
									e.stopPropagation();
									moveMenuFor = null;
									moveSubOpen = false;
									openProjectDialog('create');
								}}
								class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								<Plus size={12} class="shrink-0 text-[var(--color-muted)]" />
								<span class="flex-1 truncate">{t('ai.projects.new', 'New project')}</span>
							</button>
							{#if projs.length > 0}
								<div class="my-1 border-t border-[var(--color-border)]"></div>
							{/if}
							{#each projs as p (p.id)}
								<button
									type="button"
									onclick={(e) => {
										e.stopPropagation();
										assignProject(conv.conversation_id, p.id);
									}}
									class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-start text-xs hover:bg-[var(--color-surface-subtle)] {conv.project_id === p.id ? 'font-medium text-[var(--color-primary)]' : 'text-[var(--color-text)]'}"
								>
									<Folder size={11} class="shrink-0 text-[var(--color-muted)]" />
									<span class="flex-1 truncate">{p.name}</span>
								</button>
							{/each}
						</div>
					{/if}
				{/if}
			{/if}
		</li>
	{/snippet}
	<!-- Main chat — min-w-0 lets the column shrink below its content's
	     min-content width so the transcript/composer never overflow the
	     shell's center column (a <pre> or wide grid would otherwise push
	     the Send button under the right rail). -->
	<div class="flex min-w-0 flex-1 flex-col min-h-0">
		<header class="mb-4 flex items-center gap-3">
			<button
				type="button"
				onclick={() => (sidebarOpen = !sidebarOpen)}
				class="me-2 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] lg:hidden"
				aria-label={t('ai.chat.toggle_history', 'Toggle conversation history')}
			>
				{#if sidebarOpen}
					<X size={18} />
				{:else}
					<Menu size={18} />
				{/if}
			</button>
			<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background-color: var(--color-primary)">
				<Sparkles class="h-5 w-5 text-white" />
			</div>
			<div>
				<h1 class="text-xl font-bold tracking-tight">{t('ai.page.title', 'Japa AI')}</h1>
				<p class="text-xs" style="color: var(--color-muted)">
					{toolsAvailable
						? t('ai.page.subtitle_agent', 'Chat or use tools — switch anytime')
						: t('ai.chat.subtitle', 'Your AI assistant')}
				</p>
			</div>
			<div class="ms-auto flex items-center gap-2">
				{#if activeProject}
					<span
						class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-muted)]"
						title={activeProject.name}
					>
						<Folder size={12} />
						<span class="max-w-32 truncate">{activeProject.name}</span>
					</span>
				{/if}
				{#if toolsAvailable}
					<!-- §103: every tool-enabled turn writes an ai_agent_sessions
					     audit row — the history list lives at /ai/sessions. Hidden for
					     plain-chat users (the sessions API shares the agent gate). -->
					<a
						href="/ai/sessions"
						class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors"
						style="border-color: var(--color-border); color: var(--color-muted)"
						title={t('ai.agent.session_history', 'Session history')}
					>
						<History size={13} />
						<span class="hidden sm:inline">{t('ai.agent.session_history', 'Session history')}</span>
					</a>
				{/if}
				{#if toolsOn}
					<button
						type="button"
						onclick={() => (templatePanelOpen = !templatePanelOpen)}
						class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors"
						style={templatePanelOpen
							? 'border-color: var(--color-primary); color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent)'
							: 'border-color: var(--color-border); color: var(--color-muted)'}
						title={t('ai.templates.toggle', 'Chat templates')}
					>
						<PanelRight class="rtl:-scale-x-100" size={13} />
						<span class="hidden sm:inline">{t('ai.templates.title', 'Templates')}</span>
					</button>
				{/if}
			</div>
		</header>

		<!-- Transcript — borderless ChatGPT/Claude-style stream: a centered
		     max-w-3xl column, user turns as soft gray bubbles on the right,
		     assistant turns as plain full-width prose on the left. -->
		<div bind:this={scrollEl} class="relative flex-1 overflow-y-auto">
			{#if turns.length === 0 && !loadingConversation}
				<div class="mx-auto flex h-full w-full max-w-3xl flex-col items-center justify-center gap-2 p-6 text-center">
					<MessageSquare class="h-8 w-8" style="color: var(--color-muted)" />
					<p class="text-sm" style="color: var(--color-muted)">{t('ai.page.empty', 'Start a conversation with Japa AI')}</p>
					<div class="mt-3 grid w-full max-w-md grid-cols-1 gap-2 sm:grid-cols-2">
						{#each suggestions as suggestion (suggestion)}
							<button
								type="button"
								onclick={async () => {
									input = suggestion;
									await tick();
									send();
								}}
								class="rounded-lg border p-3 text-start text-sm transition-colors hover:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-surface-subtle); color: var(--color-muted)"
							>
								{suggestion}
							</button>
						{/each}
					</div>
				</div>
			{:else}
				{#if loadingConversation}
					<div class="absolute inset-0 z-10 flex items-center justify-center bg-[var(--color-surface)]/80">
						<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-primary)" />
					</div>
				{/if}
				<div class="mx-auto w-full max-w-3xl space-y-6 px-1 py-3 sm:px-4">
					{#each turns as turn (turn.id)}
						<div class="flex {turn.role === 'user' ? 'justify-end' : ''}">
							<div
								class="group relative min-w-0 {turn.role === 'user'
									? 'w-fit max-w-[85%] sm:max-w-[75%]'
									: 'w-full'}"
							>
								{#if turn.role === 'user' && (turn.streamed || turn.content)}
									<button
										type="button"
										onclick={() => copyTurn(turn)}
										class="absolute -start-9 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-[var(--color-muted)] opacity-0 transition-opacity hover:bg-[color-mix(in_srgb,var(--color-muted)_15%,transparent)] hover:text-[var(--color-text)] focus-visible:opacity-100 group-hover:opacity-100"
										aria-label={t('common.copy', 'Copy')}
										title={t('common.copy', 'Copy')}
									>
										{#if copiedTurn === turn.id}
											<Check size={14} />
										{:else}
											<Copy size={14} />
										{/if}
									</button>
								{/if}
								<div
									class="whitespace-pre-wrap text-[15px] leading-7 text-[var(--color-text)] {turn.role ===
									'user'
										? 'w-fit rounded-3xl bg-[var(--color-surface-subtle)] px-4 py-2.5'
										: 'px-1 sm:px-2'}"
								>
									{turn.streamed || turn.content}
									{#if turn.in_progress && !turn.content && !turn.streamed}
										<span class="inline-flex items-center gap-1 text-xs" style="color: var(--color-muted)">
											<Loader2 size={12} class="animate-spin" />
											{turn.progress || t('ai.agent.thinking', 'Working...')}
											{#if bgSessionId && turn.session_id === bgSessionId}
												<button
													type="button"
													class="rounded border px-1.5 py-0.5 text-[10px]"
													style="border-color: var(--color-border); color: var(--color-muted)"
													disabled={bgCancelling}
													onclick={cancelBackgroundSession}
												>
													{t('ai.agent.cancel', 'Cancel')}
												</button>
											{/if}
										</span>
									{/if}
								</div>

								{#if turn.role === 'user' && turn.template_name}
									<div class="mt-1 flex justify-end text-[10px]" style="color: var(--color-muted)">
										{t('ai.templates.sent_with', 'Template')}: {turn.template_name}
									</div>
								{/if}

								{#if turn.role === 'assistant' && turn.presearch_level}
									<div class="mt-1.5 flex items-center gap-1.5 text-xs" style="color: var(--color-muted)">
										<Search size={11} />
										{t('ai.agent.presearch', 'Pre-search')}: <span class="font-medium">{turn.presearch_level}</span>
									</div>
								{/if}

								<!-- Tool call log -->
								{#if turn.tool_calls && turn.tool_calls.length > 0}
									<div class="mt-2 space-y-1.5">
										{#each turn.tool_calls as tc (tc.id)}
											{@const Icon = iconForTool(tc.tool_name)}
											{@const created = createdItemLinks(tc)}
											<div
												class="rounded-md border px-2.5 py-1.5 text-xs"
												style="border-color: var(--color-border); background-color: var(--color-background)"
											>
												<div class="flex items-center gap-2">
													<Icon size={12} style="color: var(--color-muted)" />
													<span class="font-mono font-medium">{tc.tool_name}</span>
													<span
														class="ms-auto inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-xs font-medium"
														style="background-color: color-mix(in srgb, {statusColor(tc.status)} 12%, transparent); color: {statusColor(tc.status)}"
													>
														{#if tc.status === 'pending'}
															<Loader2 size={10} class="animate-spin" />
														{:else if tc.status === 'success'}
															<CheckCircle2 size={10} />
														{:else if tc.status === 'error'}
															<XCircle size={10} />
														{/if}
														{statusLabel(tc.status)}
													</span>
												</div>
												{#if created.length > 0}
													<div class="mt-1 flex flex-wrap items-center gap-1.5">
														<span style="color: var(--color-muted)"
															>{t('ai.agent.created', 'Created')}:</span
														>
														{#each created as l (l.href)}
															<a
																href={l.href}
																class="rounded-full border px-1.5 py-0.5 underline-offset-2 hover:underline"
																style="border-color: var(--color-border); color: var(--color-primary)"
																target="_blank"
																rel="noopener noreferrer">{l.label}</a
															>
														{/each}
													</div>
												{/if}
												{#if tc.arguments}
													<details class="mt-1">
														<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">
															{t('ai.agent.tool.arguments', 'Arguments')}
														</summary>
														<pre class="mt-1 overflow-x-auto rounded p-1.5 text-xs font-mono" style="background-color: var(--color-surface)">{tc.arguments}</pre>
													</details>
												{/if}
												{#if tc.result}
													<details class="mt-1">
														<summary class="cursor-pointer text-xs" style="color: var(--color-muted)">
															{t('ai.agent.tool.result', 'Result')}
														</summary>
														<pre class="mt-1 max-h-48 overflow-auto rounded p-1.5 text-xs font-mono" style="background-color: var(--color-surface)">{tc.result}</pre>
													</details>
												{/if}
												{#if tc.error}
													<p class="mt-1 text-xs" style="color: var(--color-danger)">{tc.error}</p>
												{/if}
											</div>
										{/each}
									</div>
								{/if}

								<!-- RAG source citations (Section 69, ADR-069) -->
								{#if turn.role === 'assistant' && turn.sources && turn.sources.length > 0}
									<AiSourceCitations sources={turn.sources} />
								{/if}

								<!-- Token + cost footer -->
								{#if turn.role === 'assistant' && (turn.tokens_used || turn.estimated_cost) && !turn.in_progress}
									<div class="mt-1.5 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
										{#if turn.tokens_used}
											<span class="inline-flex items-center gap-1"><Cpu size={11} /> {fmtTokens(turn.tokens_used)} {t('ai.agent.tokens', 'tokens')}</span>
										{/if}
										{#if turn.estimated_cost}
											<span class="inline-flex items-center gap-1"><Database size={11} /> {fmtCost(turn.estimated_cost)}</span>
										{/if}
									</div>
								{/if}

								{#if turn.error}
									<div class="mt-1.5 flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">
										<AlertCircle size={12} />
										{turn.error}
									</div>
								{/if}

								<!-- Immigration/financial content disclaimer (Section 69) —
								     the assistant copy action lives in this meta row so a
								     full-width turn doesn't overflow the centered column. -->
								{#if turn.role === 'assistant' && turn.content && !turn.in_progress}
									<p class="mt-1 flex items-center gap-1 text-xs" style="color: var(--color-muted)">
										<ShieldCheck size={11} />
										{t('ai.chat.disclaimer', 'AI-generated. Verify immigration/financial details with official sources.')}
										<button
											type="button"
											onclick={() => copyTurn(turn)}
											class="ms-1 rounded-md p-1 text-[var(--color-muted)] opacity-0 transition-opacity hover:bg-[color-mix(in_srgb,var(--color-muted)_15%,transparent)] hover:text-[var(--color-text)] focus-visible:opacity-100 group-hover:opacity-100"
											aria-label={t('common.copy', 'Copy')}
											title={t('common.copy', 'Copy')}
										>
											{#if copiedTurn === turn.id}
												<Check size={13} />
											{:else}
												<Copy size={13} />
											{/if}
										</button>
									</p>
								{/if}
							</div>
						</div>
					{/each}
					{#if bgSessionId && !turns.some((tn) => tn.in_progress)}
						<div class="flex items-center gap-2 px-1 py-2 text-xs sm:px-2" style="color: var(--color-muted)">
							<Loader2 size={12} class="animate-spin" />
							<span class="flex-1">{t('ai.agent.bg_running', 'Agent is working in the background — the result will appear here.')}</span>
							<button
								type="button"
								class="rounded border px-1.5 py-0.5 text-[10px]"
								style="border-color: var(--color-border); color: var(--color-muted)"
								disabled={bgCancelling}
								onclick={cancelBackgroundSession}
							>
								{t('ai.agent.cancel', 'Cancel')}
							</button>
						</div>
					{/if}
				</div>
			{/if}
		</div>

		{#if error}
			<div class="mx-auto mt-2 flex w-full max-w-3xl items-center gap-2 rounded-md border p-2 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
				<AlertCircle class="h-4 w-4" />
				{error}
			</div>
		{/if}

		<!-- Composer column — shares the transcript's centered max-w-3xl so
		     the stream and the input read as one column (ChatGPT/Claude). -->
		<div class="mx-auto w-full max-w-3xl shrink-0">
		{#if viewingLegacy}
			<p class="mb-2 rounded-lg border px-3 py-2 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
				{t('ai.chat.legacy_readonly', 'This is an older conversation — it’s view-only. Start a new chat to keep going.')}
			</p>
		{/if}

		{#if templateActive}
			<!-- Active chat template — every send carries the spec until detached. -->
			<div class="mb-2 flex items-center gap-2 text-xs">
				<span
					class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1"
					style="border-color: var(--color-primary); color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent)"
				>
					{t('ai.templates.sent_with', 'Template')}: {activeTemplateLabel}
					<button
						type="button"
						onclick={() => (templateActive = false)}
						class="hover:opacity-70"
						aria-label={t('ai.templates.detach', 'Detach template')}
						title={t('ai.templates.detach', 'Detach template')}
					>
						<X size={11} />
					</button>
				</span>
				{#if templateDirty}
					<span style="color: var(--color-muted)">{t('ai.templates.live_edits', 'with unsaved edits')}</span>
				{/if}
			</div>
		{/if}

		<!-- Composer — rounded input box in the ChatGPT/Claude idiom: the
		     textarea grows inside, controls sit on a bottom bar, and the
		     send button is a circle. Picker dropdowns at this viewport
		     edge auto-flip upward via the shared Autocomplete. -->
		<div class="rounded-[26px] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 pb-2 pt-2.5 shadow-sm transition-shadow focus-within:border-[var(--color-primary)]/50 focus-within:shadow-md">
			<textarea
				bind:this={inputEl}
				bind:value={input}
				rows="1"
				placeholder={toolsOn
					? t('ai.agent.placeholder', 'Ask the agent to search, draft, or analyze...')
					: t('ai.page.placeholder', 'Ask Japa AI anything...')}
				aria-label={toolsOn
					? t('ai.agent.placeholder', 'Ask the agent to search, draft, or analyze...')
					: t('ai.page.placeholder', 'Ask Japa AI anything...')}
				class="max-h-40 w-full resize-none overflow-y-auto bg-transparent px-1 text-[15px] leading-6 text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
				oninput={resizeInput}
				onkeydown={onKeydown}
				disabled={sending || viewingLegacy}
			></textarea>
			<div class="mt-1 flex flex-wrap items-center gap-2 sm:flex-nowrap">
			<!-- Tools controls — only for users whose catalog is non-empty -->
			{#if toolsAvailable}
				<div class="relative shrink-0" bind:this={toolsBoxEl}>
					<button
						type="button"
						onclick={() => (toolsOpen = !toolsOpen)}
						class="flex h-8 items-center gap-1.5 rounded-full border px-2.5 text-xs transition-colors {toolsOn ? 'font-medium' : ''}"
						style={toolsOn
							? 'border-color: var(--color-primary); color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent)'
							: 'border-color: var(--color-border); color: var(--color-muted)'}
						title={t('ai.tools.configure', 'Configure tools')}
					>
						<Wrench size={14} />
						{#if toolsOn}
							{enabledToolCount}
						{:else}
							{t('ai.tools.off', 'Off')}
						{/if}
					</button>

					{#if toolsOpen}
						<!-- Anchored to the trigger (bottom-full), not a fixed
						     offset — the button height changed with the composer
						     restyle, so a hard-coded bottom-* would drift. Width is
						     capped so the popover can't overflow the composer on
						     narrow viewports. -->
						<div
							class="absolute bottom-full start-0 z-20 mb-2 w-72 max-w-[calc(100vw-2.5rem)] rounded-lg border p-3 shadow-lg"
							style="border-color: var(--color-border); background-color: var(--color-surface)"
						>
							<div class="mb-2 flex items-center justify-between">
								<span class="text-xs font-semibold" style="color: var(--color-text)">
									{t('ai.tools.title', 'Tools')}
								</span>
								<!-- Master per-conversation switch -->
								<button
									type="button"
									role="switch"
									aria-checked={toolsOn}
									onclick={() => (toolsOn = !toolsOn)}
									class="relative h-5 w-9 rounded-full transition-colors"
									style="background-color: {toolsOn ? 'var(--color-primary)' : 'var(--color-border)'}"
									aria-label={t('ai.tools.toggle', 'Toggle tools')}
								>
									<span
										class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-transform"
										style="left: 2px; transform: translateX({toolsOn ? '16px' : '0'})"
									></span>
								</button>
							</div>

							{#if toolsOn}
								<div class="max-h-56 space-y-2 overflow-y-auto">
									{#each categories as cat (cat.name)}
										<div>
											<button
												type="button"
												onclick={() => setCategory(cat.name, categoryState(cat) !== 'all')}
												class="flex w-full items-center gap-1.5 text-xs font-medium"
												style="color: var(--color-text)"
											>
												<SlidersHorizontal size={11} style="color: var(--color-muted)" />
												{cat.name}
												<span class="ms-auto" style="color: var(--color-muted)">
													{cat.tools.filter((tl) => toolEnabled[tl.name]).length}/{cat.tools.length}
												</span>
											</button>
											<div class="mt-1 space-y-0.5 ps-4">
												{#each cat.tools as tool (tool.name)}
													{@const locked = !prefsAllows(tool.name, cat.name)}
													<label class="flex items-center gap-1.5 text-xs {locked ? 'opacity-50' : 'cursor-pointer'}" style="color: var(--color-muted)">
														<input
															type="checkbox"
															checked={toolEnabled[tool.name] ?? false}
															disabled={locked}
															onchange={(e) => (toolEnabled = { ...toolEnabled, [tool.name]: e.currentTarget.checked })}
															class="h-3 w-3"
														/>
														<span class="font-mono">{tool.name}</span>
														{#if locked}
															<span class="text-[10px]">({t('ai.tools.disabled_in_settings', 'off in settings')})</span>
														{/if}
													</label>
												{/each}
											</div>
										</div>
									{/each}
								</div>

								<button
									type="button"
									onclick={savePrefsAsDefault}
									class="mt-2 w-full rounded-md border px-2 py-1 text-xs transition-colors hover:opacity-80"
									style="border-color: var(--color-border); color: var(--color-muted)"
								>
									{prefsSaved ? t('ai.tools.saved', 'Saved') : t('ai.tools.save_default', 'Use these as my defaults')}
								</button>
							{:else}
							{/if}
						</div>
					{/if}
				</div>
			{/if}

			<div class="min-w-0 basis-full order-last sm:order-none sm:basis-auto sm:flex-1">
				<AiModelVoicePicker
					bind:model={selectedModel}
					bind:voice={selectedVoice}
					bind:tone={selectedTone}
					modelOptions={composerModelOptions}
					{voiceOptions}
					{toneOptions}
					idPrefix="japaai"
					compact
				/>
			</div>
			{#if sending}
				<button
					type="button"
					onclick={cancel}
					disabled={cancelling}
					class="ms-auto grid h-8 w-8 shrink-0 place-items-center rounded-full border transition-colors disabled:opacity-50 sm:ms-0"
					style="border-color: var(--color-danger); color: var(--color-danger)"
					aria-label={t('ai.agent.cancel', 'Cancel')}
					title={t('ai.agent.cancel', 'Cancel')}
				>
					{#if cancelling}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<Square class="h-3.5 w-3.5" />
					{/if}
				</button>
			{:else}
				<button
					type="button"
					onclick={send}
					disabled={!input.trim() || viewingLegacy}
					class="ms-auto grid h-8 w-8 shrink-0 place-items-center rounded-full text-white transition-colors disabled:opacity-40 sm:ms-0"
					style="background-color: var(--color-primary)"
					aria-label={t('ai.chat.send', 'Send')}
				>
					<ArrowUp class="h-4 w-4" />
				</button>
			{/if}
			</div>
		</div>
	</div>
	</div>

	<!-- Chat template rail — a column on lg+, a right-edge overlay on
	     smaller viewports (close button lives in the panel header).
	     Agent-mode only: hidden while tools are off. -->
	{#if templatePanelOpen && toolsOn}
		<div
			data-template-rail
			class="fixed inset-y-0 end-0 z-40 flex w-[21rem] max-w-[90vw] lg:static lg:z-auto lg:me-4 lg:w-72 lg:shrink-0 xl:w-80"
		>
			<ChatTemplatePanel
				{templates}
				bind:values={tplValues}
				bind:activeId={activeTemplateId}
				bind:active={templateActive}
				{canSkipHitl}
				{contentTypes}
				bind:spaces={tplSpaces}
				bind:series={tplSeries}
				{assessmentDimensions}
				{voices}
				{maxTags}
				{statusOptions}
				{visibilityOptions}
				onpersist={persistTemplates}
				oninsert={insertTemplateText}
				onclose={() => (templatePanelOpen = false)}
			/>
		</div>
	{/if}
</div>

<!-- Click-away / Escape closes an open "Move to project" menu, the
     tools popover, or the project dialog. Clicks inside the tools box
     (trigger button + popover) are ignored via the element check. -->
<svelte:window
	onclick={(e) => {
		moveMenuFor = null;
		moveSubOpen = false;
		projMenuFor = null;
		if (!toolsBoxEl?.contains(e.target as Node)) toolsOpen = false;
	}}
	onkeydown={(e) => {
		if (e.key !== 'Escape') return;
		if (projectDialog) projectDialog = null;
		else if (toolsOpen) toolsOpen = false;
		else if (moveSubOpen) moveSubOpen = false;
		else if (moveMenuFor) moveMenuFor = null;
		else if (projMenuFor) projMenuFor = null;
		// The template rail is a fixed overlay below lg — Esc dismisses it,
		// except when the keypress came from inside the rail (its own
		// menus handle Esc first) or while a dialog is open (the dialog's
		// own Esc dismissal wins).
		else if (
			templatePanelOpen &&
			!window.matchMedia('(min-width: 1024px)').matches &&
			!(e.target instanceof Element && e.target.closest('[data-template-rail]')) &&
			!document.querySelector('[role="dialog"]')
		)
			templatePanelOpen = false;
	}}
/>

<!-- §103 AI Projects — create/edit dialog. -->
{#if projectDialog}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="presentation"
		onclick={(e) => {
			if (e.target === e.currentTarget) projectDialog = null;
		}}
	>
		<div
			class="w-full max-w-md space-y-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-xl"
			role="dialog"
			aria-modal="true"
			aria-label={projectDialog.mode === 'create'
				? t('ai.projects.new', 'New project')
				: t('ai.projects.edit', 'Edit project')}
		>
			<div class="flex items-center justify-between">
				<h3 class="text-sm font-semibold text-[var(--color-text)]">
					{projectDialog.mode === 'create'
						? t('ai.projects.new', 'New project')
						: t('ai.projects.edit', 'Edit project')}
				</h3>
				<button
					type="button"
					onclick={() => (projectDialog = null)}
					class="rounded p-1 text-[var(--color-muted)] hover:text-[var(--color-text)]"
					aria-label={t('common.close', 'Close')}
				>
					<X size={15} />
				</button>
			</div>

			<label class="block">
				<span class="text-xs text-[var(--color-muted)]">{t('ai.projects.name', 'Project name')}</span>
				<input
					type="text"
					bind:value={projName}
					maxlength="200"
					class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] px-3 py-2 text-sm text-[var(--color-text)]"
					placeholder={t('ai.projects.name_placeholder', 'e.g. UK student visa')}
				/>
			</label>

			<label class="block">
				<span class="text-xs text-[var(--color-muted)]">{t('ai.projects.instructions', 'Instructions')}</span>
				<textarea
					bind:value={projInstructions}
					rows="3"
					maxlength="4000"
					class="mt-1 w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] px-3 py-2 text-sm text-[var(--color-text)]"
					placeholder={t('ai.projects.instructions_placeholder', 'Context the AI applies to every conversation in this project')}
				></textarea>
				<HintIcon text={t('ai.projects.instructions_hint', 'Sent with each message in this project’s conversations.')} />
			</label>

			<div>
				<span class="text-xs text-[var(--color-muted)]">{t('ai.projects.pins', 'Default pickers')}</span>
				<div class="mt-1 grid grid-cols-1 gap-2 sm:grid-cols-3">
					<AutocompleteSelect
						bind:value={projModel}
						options={modelOptions}
						ariaLabel={t('ai.picker.model_label', 'Model')}
					/>
					<AutocompleteSelect
						bind:value={projTone}
						options={toneOptions}
						ariaLabel={t('ai.picker.tone_label', 'Tone')}
					/>
					<AutocompleteSelect
						bind:value={projVoice}
						options={projVoiceOptions}
						ariaLabel={t('ai.picker.voice_label', 'Voice')}
					/>
				</div>
			</div>

			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input type="checkbox" bind:checked={projToolsOn} class="rounded border-[var(--color-border)]" />
				{t('ai.projects.tools_default', 'Enable tools by default')}
			</label>

			{#if projError}
				<p class="text-xs" style="color: var(--color-danger)">{projError}</p>
			{/if}

			<div class="flex justify-end gap-2 pt-1">
				<button
					type="button"
					onclick={() => (projectDialog = null)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					onclick={saveProject}
					disabled={projSaving || !projName.trim()}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm text-white disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if projSaving}<Loader2 size={13} class="animate-spin" />{/if}
					{t('common.save', 'Save')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Destructive-action confirm (delete project / conversation) — the
     same shared Modal the template panel uses, styled like its dialog. -->
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

<!-- Posts modal — per-insert targeting for the template prompt. -->
<Modal
	open={insertModalOpen}
	onOpenChange={(o) => {
		if (!o) insertModalOpen = false;
	}}
	title={t('ai.templates.insert_title', 'Insert template as prompt')}
	description={t('ai.templates.insert_hint', 'Choose what the prompt should generate.')}
	size="lg"
>
	{#snippet children(close)}
		<div class="space-y-3">
			<div class="flex overflow-hidden rounded-[var(--radius)] border" style="border-color: var(--color-border)">
				{#each [
					['single', t('ai.templates.mode_single', 'Single post')],
					['count', t('ai.templates.mode_count', 'Counted set')],
					['bulk', t('ai.templates.mode_bulk', 'Bulk list')]
				] as [val, label] (val)}
					<button
						type="button"
						onclick={() => (insertMode = val as ChatTemplatePostMode)}
						class="flex-1 px-2 py-1.5 text-xs {insertMode === val
							? 'font-medium text-white'
							: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
						style={insertMode === val ? 'background-color: var(--color-primary)' : ''}
					>
						{label}
					</button>
				{/each}
			</div>

			{#if insertMode !== 'bulk'}
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="insert-title"
						>{t('ai.templates.post_title', 'Title')}</label
					>
					<input
						id="insert-title"
						type="text"
						class="w-full rounded-[var(--radius)] border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-surface)"
						bind:value={insertTitle}
						placeholder={t('ai.templates.title_placeholder', 'e.g. UK student visa checklist')}
					/>
				</div>
			{/if}
			{#if insertMode === 'count'}
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="insert-count"
						>{t('ai.templates.post_count', 'Related posts')}</label
					>
					<input
						id="insert-count"
						type="number"
						min="1"
						class="w-full rounded-[var(--radius)] border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-surface)"
						bind:value={insertCount}
					/>
				</div>
			{/if}
			{#if insertMode === 'bulk'}
				<div>
					<label class="mb-1 block text-xs" style="color: var(--color-muted)" for="insert-topics"
						>{t('ai.templates.titles', 'Titles')}</label
					>
					<textarea
						id="insert-topics"
						rows="7"
						class="w-full rounded-[var(--radius)] border px-2.5 py-1.5 text-sm"
						style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-surface)"
						bind:value={insertTopics}
						placeholder={t('ai.templates.paste_titles_placeholder', 'Title one\nTitle two\nTitle three')}
					></textarea>
					<HintIcon text={t('ai.templates.paste_titles_hint', 'One title per line — each becomes a post.')} />
				</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button
					type="button"
					onclick={close}
					class="rounded-[var(--radius)] border px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					style="border-color: var(--color-border)"
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					onclick={() => {
						confirmInsert();
						close();
					}}
					class="rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-white"
					style="background-color: var(--color-primary)"
				>
					{t('ai.templates.insert_apply', 'Insert')}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>

<!-- BYOK add-key flow from the composer picker (spec §7): saving refetches
     the page so the new key's models appear under "Your keys". -->
<AddAiKeyDialog bind:open={addKeyOpen} providers={byokProviders} onsaved={() => invalidateAll()} />
