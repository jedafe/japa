<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { untrack, tick } from 'svelte';
	import {
		Lock, CheckCircle2, CircleCheckBig, Check, ArrowRight, ArrowLeft, TrendingUp, X,
		MapPin, Plane, Calendar, CalendarDays, Flag, Briefcase, Globe, IdCard, Heart, Route,
		Wallet, FileCheck, Compass, Languages, HeartHandshake, ClipboardList, GraduationCap,
		ChartNoAxesColumn, Camera, Users, Lightbulb, Package, Send, House, Sun, Smile, CircleDashed,
		PiggyBank, Landmark, ScrollText, ShieldCheck, Luggage, BadgeCheck, Baby, FileText,
		ListChecks, School, Stethoscope, PlaneTakeoff, Banknote, Receipt, Repeat, PawPrint,
		Hospital, Car, Building2, LifeBuoy, MessageCircleHeart, Info
	} from 'lucide-svelte';
	import { LineChart } from 'layerchart';
	import { EmptyState, LoadingSkeleton, AutocompleteSelect, type IconComponent } from '$lib/components/ui';
	import ReadinessHeroArt from '$lib/components/tools/ReadinessHeroArt.svelte';
	import UserDropdown from '$lib/components/shell/UserDropdown.svelte';
	import { api } from '$lib/api/client';
	import { completePendingNextAction } from '$lib/api/nextActions';
	import type { AssessmentForm, AssessmentResult, AssessmentField, FeedItem, SessionUser } from '$lib/types';
	import type { PageData } from './$types';

	interface JourneyContext {
		origin_country: string;
		destination_country: string;
		visa_route: string;
		journey_stage: string;
		purpose: string;
		travel_date: string;
	}

	interface RoadmapResponse {
		context: JourneyContext;
		items: FeedItem[];
	}

	interface CountryOption {
		value: string;
		label: string;
	}

	// AssessmentHistoryEntry is exported from +page.server.ts and inferred by
	// $types via PageData, but we redeclare it here for direct use in
	// component state typing.
	interface AssessmentHistoryEntry {
		id: string;
		total_score: number;
		threshold_label?: string;
		submission_id?: string;
		created_at: string;
	}

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.readiness', 'Readiness Assessment — {site}'));
	const description = $derived(
		t('tools.readiness_desc', 'Score your japa readiness across key dimensions.')
	);

	const form = $derived<AssessmentForm | null>(data.form);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);
	// Session user comes from the (app) layout load, merged into page.data.
	const user = $derived((page.data.user ?? null) as SessionUser | null);

	// Local history so we can prepend new results after a submit without
	// waiting for a full page reload.
	let history = $state<AssessmentHistoryEntry[]>(untrack(() => data.history ?? []));
	let answers = $state<Record<string, string | string[] | number | boolean>>({});
	let currentStep = $state(0);
	let submitting = $state(false);
	let scoring = $state(false);
	let submitError = $state<string | null>(null);
	let result = $state<AssessmentResult | null>(untrack(() => data.result ?? null));
	let loadingResult = $state(false);
	let showRegisterPrompt = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let destroyed = $state(false);

	// Track which result id has already been requested so a failed load (401,
	// 404, network) does not trigger an infinite retry loop (§71 bug fix).
	let attemptedResultId = $state<string | null>(null);
	let resultLoadError = $state<{ message: string; status: number } | null>(null);

	// Geo-aware roadmap and the journey context used to rank it.
	let roadmap = $state<RoadmapResponse | null>(null);
	let roadmapLoading = $state(false);
	let roadmapError = $state<string | null>(null);
	let roadmapAttemptedId = $state<string | null>(null);

	// Load a shared/past result from the URL ?result= query param.
	$effect(() => {
		const resultId = page.url.searchParams.get('result');
		if (resultId && !loadingResult && result?.id !== resultId && attemptedResultId !== resultId) {
			attemptedResultId = resultId;
			loadingResult = true;
			resultLoadError = null;
			roadmap = null;
			roadmapAttemptedId = null;
			api.get<AssessmentResult>(`/assessment/result/${resultId}`).then((res) => {
				loadingResult = false;
				if (!res.error && res.data) {
					result = res.data;
				} else {
					resultLoadError = {
						message: res.error?.message ?? t('assessment.result_load_error', 'This result could not be loaded.'),
						status: res.error?.status ?? 0
					};
					// If the result belongs to a signed-in user, prompt them to sign in.
					if (res.error?.status === 401 && !authed) {
						showRegisterPrompt = true;
					}
				}
			}).catch(() => {
				loadingResult = false;
				resultLoadError = {
					message: t('assessment.result_load_error', 'This result could not be loaded.'),
					status: 0
				};
			});
		}
	});

	const fields = $derived(form?.fields ?? []);
	const requiredFields = $derived(fields.filter((f) => f.is_required));

	/**
	 * Group the flat field list by dimension for the multi-step UX. Dimensions
	 * are ordered by the position of their first field. Fields without a
	 * dimension fall into a final "general" step. When the form isn't flagged
	 * is_multi_step, all fields render in a single step per the §06 contract.
	 */
	const steps = $derived.by(() => {
		if (!form) return [] as { name: string; fields: AssessmentField[] }[];
		if (!form.is_multi_step) return [{ name: 'assessment', fields: form.fields }];
		const order: string[] = [];
		const map = new Map<string, AssessmentField[]>();
		for (const f of form.fields) {
			const dim = f.config?.dimension || 'general';
			if (!map.has(dim)) {
				map.set(dim, []);
				order.push(dim);
			}
			map.get(dim)!.push(f);
		}
		return order.map((name) => ({ name, fields: map.get(name)! }));
	});

	// Dimension copy for the step rail and card headers — keys live in the
	// locale files; the literals here are the English fallback.
	const DIM_LABELS: Record<string, string> = {
		financial: 'Finances',
		logistical: 'Documents',
		professional: 'Career & study',
		language: 'Language',
		destination_knowledge: 'Destination',
		personal: 'Personal readiness'
	};

	const DIM_META: Record<string, { icon: IconComponent; title: string; sub: string }> = {
		general: { icon: Route, title: "Let's map out your journey", sub: 'Tell us a bit about your plans so we can personalize your readiness report.' },
		financial: { icon: Wallet, title: "Let's talk finances", sub: 'Savings, income and safety nets — the money side of your move.' },
		logistical: { icon: FileCheck, title: 'Documents & logistics', sub: 'Visas, paperwork and the practical pieces of the journey.' },
		professional: { icon: Briefcase, title: 'Career & study', sub: 'How work, business or study is shaping up at your destination.' },
		language: { icon: Languages, title: 'Language readiness', sub: 'How you will communicate in the language of your destination.' },
		destination_knowledge: { icon: Compass, title: 'Know your destination', sub: 'How well you understand the place you are moving to.' },
		personal: { icon: HeartHandshake, title: 'The personal side', sub: 'Final stretch — family, support and emotional readiness.' }
	};

	const STEP_TIPS: Record<string, { title: string; body: string }> = {
		general: { title: 'One step at a time', body: 'Your answers shape a personal action plan.' },
		financial: { title: 'Honesty beats optimism', body: 'A lower score here just means a clearer plan to close the gap.' },
		logistical: { title: 'Paperwork first', body: 'Most delays are document delays — check expiry dates early.' },
		professional: { title: 'Proof over promises', body: 'Demand, credentials and connections all count toward the move.' },
		language: { title: 'Small steps compound', body: 'Even basic proficiency changes your first 90 days.' },
		destination_knowledge: { title: 'Local knowledge pays', body: 'Knowing costs, areas and systems saves money on arrival.' },
		personal: { title: 'Almost done', body: 'The human side of the move matters as much as the paperwork.' }
	};

	// Field-key → label icon. Keys not in the map (e.g. a new form shipped from
	// the backend) fall back to a neutral info glyph.
	const FIELD_ICONS: Record<string, IconComponent> = {
		destination_country: Globe, origin_country: MapPin, visa_type: IdCard,
		journey_stage: Flag, purpose: Heart, travel_date: CalendarDays,
		savings: PiggyBank, income_source: Briefcase, proof_of_funds: Landmark,
		relocation_budget: Wallet, visa_stage: ScrollText, documents_ready: FileCheck,
		health_insurance: ShieldCheck, travel_logistics: Luggage, accommodation: House,
		skill_demand: TrendingUp, credential_evaluation: BadgeCheck, job_search_progress: Send,
		emotional_readiness: HeartHandshake, support_network: Users, dependents_plan: Baby,
		language_proficiency: Languages, language_test: FileText,
		destination_research: Compass, settlement_plan: ListChecks,
		study_funding: GraduationCap, emergency_buffer: LifeBuoy, admission_status: School,
		visa_medical: Stethoscope, police_clearance: BadgeCheck, exit_plan: PlaneTakeoff,
		banking_setup: Banknote, tax_obligations: Receipt, remittance_commitments: Repeat,
		belongings_shipping: Package, pets_relocation: PawPrint, healthcare_continuity: Hospital,
		driving_transport: Car, business_readiness: ChartNoAxesColumn,
		professional_network: Building2, spouse_career: Heart, contingency_plan: LifeBuoy,
		mentor_connection: MessageCircleHeart
	};

	// Option-value keyword → chip icon, so choice pills carry a meaningful glyph
	// without the backend storing icon metadata.
	// Option values are snake_case slugs — anchor patterns to the start so
	// 'note', 'know', 'applied_for' etc. don't false-match (BUG-UX-011).
	const OPTION_ICON_RULES: [RegExp, IconComponent][] = [
		[/^stud|^academ|^school|universit/i, GraduationCap],
		[/^work|^job|^employ|^skilled/i, Briefcase],
		[/^business|^invest|^entrepren/i, ChartNoAxesColumn],
		[/^tourist|^visit|^vacation|^holiday|^transit/i, Camera],
		[/^spouse|^partner|^family|^reunion|^dependent|^child/i, Users],
		[/^consider|^research/i, Lightbulb],
		[/^plan/i, CalendarDays],
		[/^prepar|^gather|^in_progress/i, Package],
		[/^appli|^submit/i, Send],
		[/^approv|^ready|^sorted|^secured|^yes|^complete|^done/i, CircleCheckBig],
		[/^relocat|^settle|^moved|^living/i, House],
		[/^lifestyle|^personal/i, Sun],
		[/^other/i, Smile],
		[/^not|^none|^never|^no$/i, CircleDashed]
	];

	function optionIcon(value: string): IconComponent {
		const hit = OPTION_ICON_RULES.find(([re]) => re.test(value))?.[1];
		return hit ?? CircleDashed;
	}

	// Roving tabindex for the pill radiogroups: arrow keys move selection and
	// focus together; Home/End jump to the ends (BUG-UX-002).
	function radioGroupKeydown(e: KeyboardEvent, field: AssessmentField) {
		const opts = fieldOptions(field);
		if (!opts.length || !['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
		e.preventDefault();
		const cur = typeof answers[field.key] === 'string' ? (answers[field.key] as string) : '';
		let idx = opts.findIndex((o) => o.value === cur);
		if (e.key === 'Home') {
			idx = 0;
		} else if (e.key === 'End') {
			idx = opts.length - 1;
		} else {
			const delta = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : -1;
			idx = idx < 0 ? (delta > 0 ? 0 : opts.length - 1) : (idx + delta + opts.length) % opts.length;
		}
		const next = opts[idx];
		if (!next) return;
		setAnswer(field.key, next.value);
		tick().then(() => document.getElementById(`${field.key}-opt-${idx}`)?.focus());
	}

	function boolGroupKeydown(e: KeyboardEvent, field: AssessmentField) {
		if (!['ArrowRight', 'ArrowDown', 'ArrowLeft', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
		e.preventDefault();
		const next = e.key === 'Home' ? true : e.key === 'End' ? false : !(answers[field.key] === true);
		setAnswer(field.key, next);
		tick().then(() => document.getElementById(`${field.key}-bool-${next}`)?.focus());
	}

	function fieldIcon(key: string): IconComponent {
		return FIELD_ICONS[key] ?? Info;
	}

	function stepIcon(name: string): IconComponent {
		return DIM_META[name]?.icon ?? ListChecks;
	}

	/** Friendly display name for a step's raw dimension key. */
	function stepLabel(name: string): string {
		if (name === 'general') return t('assessment.journey_step', 'Your journey');
		const pretty = name.replace(/_/g, ' ');
		const fallback = DIM_LABELS[name] ?? pretty.charAt(0).toUpperCase() + pretty.slice(1);
		return t(`assessment.dim.${name}`, fallback);
	}

	function stepTitle(name: string): string {
		return t(`assessment.step_title.${name}`, DIM_META[name]?.title ?? stepLabel(name));
	}

	function stepSub(name: string): string {
		return t(`assessment.step_sub.${name}`, DIM_META[name]?.sub ?? '');
	}

	function stepTip(name: string): { title: string; body: string } {
		const fallback = STEP_TIPS[name] ?? STEP_TIPS.general;
		return {
			title: t(`assessment.tip_title.${name}`, fallback.title),
			body: t(`assessment.tip_body.${name}`, fallback.body)
		};
	}

	// Hero headline: the final word carries the lime underline flourish, so
	// split the translated string rather than hardcoding "here.".
	const heroTitle = $derived(t('tools.readiness_hero', 'Your next chapter starts here.'));
	const heroParts = $derived.by(() => {
		const words = heroTitle.split(' ');
		return { head: words.slice(0, -1).join(' '), tail: words.at(-1) ?? heroTitle };
	});

	const totalSteps = $derived(steps.length);
	const currentStepData = $derived(steps[currentStep] ?? null);
	const nextStepName = $derived(
		currentStep < totalSteps - 1 ? stepLabel(steps[currentStep + 1].name).toLocaleLowerCase(i18n.locale) : ''
	);

	function isFieldAnswered(f: AssessmentField): boolean {
		const a = answers[f.key];
		if (a === undefined || a === null) return false;
		if (Array.isArray(a)) return a.length > 0;
		if (typeof a === 'string') return a.trim().length > 0;
		return true;
	}

	/**
	 * Fields rendered as a group of inputs (pill choices, checkboxes) have no
	 * single labeled control — the question renders as a heading span rather
	 * than a <label for> pointing at a non-existent id (a11y).
	 */
	function isGroupField(f: AssessmentField): boolean {
		if (isCountryField(f) && data.countryOptions.length > 0) return false;
		if (f.field_type === 'multiselect' || f.field_type === 'boolean') return true;
		return (
			(f.field_type === 'select' || f.field_type === 'radio' || f.field_type === 'suggest') &&
			fieldOptions(f).length > 0
		);
	}

	/** Full-width fields vs the two-per-row inputs in the step grid. */
	function isWideField(f: AssessmentField): boolean {
		if (isCountryField(f)) return false;
		if (f.field_type === 'multiselect' || f.field_type === 'textarea' || f.field_type === 'rich_text') return true;
		return (
			(f.field_type === 'select' || f.field_type === 'radio' || f.field_type === 'suggest') &&
			fieldOptions(f).length > 0
		);
	}

	function stepComplete(): boolean {
		if (!currentStepData) return false;
		return currentStepData.fields.every((f) => !f.is_required || isFieldAnswered(f));
	}

	const requiredAnsweredCount = $derived(
		requiredFields.filter((f) => isFieldAnswered(f)).length
	);

	function setAnswer(key: string, value: string | string[] | number | boolean) {
		answers = { ...answers, [key]: value };
	}

	function toggleMulti(key: string, value: string, checked: boolean) {
		const current = Array.isArray(answers[key]) ? (answers[key] as string[]) : [];
		setAnswer(key, checked ? [...current, value] : current.filter((v) => v !== value));
	}

	const countryKeys = new Set(['origin_country', 'destination_country']);

	function isCountryField(field: AssessmentField): boolean {
		return countryKeys.has(field.key) || field.key.endsWith('_country');
	}

	function fieldOptions(field: AssessmentField): CountryOption[] {
		if (isCountryField(field) && data.countryOptions.length > 0) {
			return data.countryOptions;
		}
		return (field.options ?? []) as CountryOption[];
	}

	function countryLabel(code: string): string {
		if (!code) return '';
		return data.countryOptions.find((c) => c.value === code)?.label ?? code;
	}

	function journeyOptionLabel(list: CountryOption[], code: string): string {
		if (!code) return '';
		return list.find((o) => o.value === code)?.label ?? code;
	}

	function buildFieldValues(): Record<string, unknown> {
		const out: Record<string, unknown> = {};
		for (const f of form?.fields ?? []) {
			const a = answers[f.key];
			if (a !== undefined && a !== null) out[f.key] = a;
		}
		return out;
	}

	function nextStep() {
		if (currentStep < totalSteps - 1) currentStep++;
	}

	function prevStep() {
		if (currentStep > 0) currentStep--;
	}

	function resetResult() {
		result = null;
		roadmap = null;
		roadmapError = null;
		roadmapAttemptedId = null;
		submitError = null;
		answers = {};
		currentStep = 0;
		showRegisterPrompt = false;
		resultLoadError = null;
		// Keep attemptedResultId as the current result id while the URL is being
		// cleared so the load effect does not re-fetch the same result.
		const url = new URL(page.url);
		url.searchParams.delete('result');
		replaceState(url, {});
	}

	// Persist the in-flight submission so a scoring timeout never orphans the
	// result — anonymous users can resume polling from the pending card
	// (BUG-UX-001).
	const PENDING_KEY = 'jc.readiness.pending_submission';
	let pendingSubmission = $state<string | null>(null);

	async function runPoll(submissionId: string, persist: boolean) {
		scoring = true;
		try {
			result = await pollForResult(submissionId);
			pendingSubmission = null;
			if (typeof window !== 'undefined') sessionStorage.removeItem(PENDING_KEY);
			// §122 — completing the assessment fulfills a pending suggestion.
			completePendingNextAction();
		} catch (e) {
			if (persist) {
				pendingSubmission = submissionId;
				if (typeof window !== 'undefined') sessionStorage.setItem(PENDING_KEY, submissionId);
			} else {
				pendingSubmission = null;
				if (typeof window !== 'undefined') sessionStorage.removeItem(PENDING_KEY);
				submitError =
					e instanceof Error && e.message !== 'cancelled'
						? e.message
						: t('assessment.result_expired', 'This result is no longer available.');
			}
		} finally {
			scoring = false;
		}
	}

	// Resume an unfinished poll on load — the result-by-submission endpoint is
	// public for anonymous results, so the pending card works for everyone.
	// Any displayed result (new or shared via ?result=) also clears the marker.
	$effect(() => {
		if (typeof window === 'undefined') return;
		if (result) {
			pendingSubmission = null;
			sessionStorage.removeItem(PENDING_KEY);
			return;
		}
		if (scoring || pendingSubmission) return;
		const stored = sessionStorage.getItem(PENDING_KEY);
		if (stored) {
			pendingSubmission = stored;
			runPoll(stored, true);
		}
	});

	async function submit() {
		if (!form || submitting || scoring) return;
		submitting = true;
		submitError = null;
		result = null;
		roadmap = null;
		roadmapAttemptedId = null;
		pendingSubmission = null;
		const res = await api.post<{ submission_id: string; status: string }>('/assessment/submit', {
			field_values: buildFieldValues()
		});
		if (res.error) {
			submitting = false;
			submitError = res.error.message;
			return;
		}
		const submissionId = res.data?.submission_id;
		if (!submissionId) {
			submitting = false;
			submitError = t('assessment.submit_error', 'Submission failed: no submission id returned.');
			return;
		}
		submitting = false;
		await runPoll(submissionId, true);
	}

	async function pollForResult(submissionId: string): Promise<AssessmentResult> {
		const maxAttempts = 30;
		const delayMs = 1000;
		for (let attempt = 0; attempt < maxAttempts; attempt++) {
			await new Promise<void>((resolve) => {
				pollTimer = setTimeout(resolve, delayMs);
			});
			// Stop polling after unmount — clearing the pending timer alone
			// does not break the loop (§4.3).
			if (destroyed) throw new Error('cancelled');
			const res = await api.get<AssessmentResult | { status: string }>(
				`/assessment/result/by-submission/${submissionId}`
			);
			if (res.error) throw new Error(res.error.message);
			const payload = res.data as AssessmentResult | { status: string };
			if (payload && typeof payload === 'object' && 'total_score' in payload) {
				return payload as AssessmentResult;
			}
		}
		throw new Error(t('assessment.scoring_timeout', 'Scoring is taking too long. Please try again.'));
	}

	// Clear the polling timer on destroy so a mid-poll navigation does not
	// fire a pending setTimeout on an unmounted component (§4.3).
	$effect(() => {
		return () => {
			destroyed = true;
			if (pollTimer) {
				clearTimeout(pollTimer);
				pollTimer = null;
			}
		};
	});

	// After a result is scored, persist the result ID in the URL and update
	// local state: authenticated users get the new entry prepended to history;
	// anonymous users are shown a registration prompt (§71).
	$effect(() => {
		if (!result) return;
		const url = new URL(page.url);
		url.searchParams.set('result', result.id);
		replaceState(url, {});

		// Only the signed-in user's own results belong in their history —
		// anonymous (shared) results have an empty user_id and must not be
		// prepended.
		if (authed && result.user_id) {
			const exists = history.some((h) => h.id === result!.id);
			if (!exists) {
				const newItem: AssessmentHistoryEntry = {
					id: result.id,
					total_score: result.total_score,
					threshold_label: result.threshold_label,
					submission_id: result.submission_id,
					created_at: result.created_at
				};
				history = [newItem, ...history];
			}
		} else {
			showRegisterPrompt = true;
		}
	});

	// Fetch the geo-aware roadmap and journey context once per result.
	$effect(() => {
		if (!result) {
			roadmap = null;
			roadmapAttemptedId = null;
			return;
		}
		const id = result.id;
		if (roadmapAttemptedId === id || roadmapLoading) return;
		roadmapLoading = true;
		roadmapAttemptedId = id;
		roadmapError = null;

		api.get<RoadmapResponse>(`/assessment/result/${id}/roadmap`).then((res) => {
			roadmapLoading = false;
			if (!res.error && res.data) {
				roadmap = res.data;
			} else {
				roadmapError = res.error?.message ?? t('assessment.roadmap_load_error', 'Your roadmap could not be loaded.');
			}
		}).catch(() => {
			roadmapLoading = false;
			roadmapError = t('assessment.roadmap_load_error', 'Your roadmap could not be loaded.');
		});
	});

	async function viewHistoryResult(item: AssessmentHistoryEntry) {
		loadingResult = true;
		resultLoadError = null;
		roadmap = null;
		roadmapAttemptedId = null;
		const res = await api.get<AssessmentResult>(`/assessment/result/${item.id}`);
		loadingResult = false;
		if (!res.error && res.data) {
			result = res.data;
		} else {
			resultLoadError = {
				message: res.error?.message ?? t('assessment.result_load_error', 'This result could not be loaded.'),
				status: res.error?.status ?? 0
			};
		}
	}

	function resolveThresholdColor(color?: string | null): string {
		if (!color) return 'var(--r-ink-2)';
		if (color.startsWith('#')) return color;
		if (color.startsWith('chart-')) return `var(--${color})`;
		return color;
	}

	const resultThreshold = $derived.by(() => {
		if (!result || !form) return null;
		const label = result.threshold_label;
		return form.scoring_config.thresholds.find((th) => th.label === label) ?? null;
	});

	const gaugeColor = $derived.by(() => {
		return resolveThresholdColor(resultThreshold?.color);
	});

	const scorePct = $derived(
		result && form
			? Math.round((result.total_score / Math.max(form.scoring_config.max_score, 1)) * 100)
			: 0
	);

	const maxScore = $derived(form?.scoring_config.max_score ?? 1);
	const historySeries = $derived(
		history
			.slice()
			.reverse()
			.map((h) => ({
				date: new Date(h.created_at),
				score: Math.round((h.total_score / Math.max(maxScore, 1)) * 100),
				label: h.created_at
			}))
	);

	const nextReturnPath = $derived(
		encodeURIComponent(page.url.pathname + (page.url.searchParams.toString() ? `?${page.url.searchParams.toString()}` : ''))
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

{#snippet historyPanel()}
	<!-- Score history analytics — rendered in the aside of both form and
	     result views (§33.02). -->
	{#if authed && historySeries.length > 0}
		<div class="r-card p-5">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold" style="color: var(--r-ink)">
				<TrendingUp size={16} style="color: var(--r-ink-2)" />
				{t('assessment.history', 'Score history')}
			</h2>
			<div class="h-48">
				<LineChart
					data={historySeries}
					x="date"
					y="score"
					props={{
						line: { class: 'stroke-[var(--r-ink-2)]' },
						area: { class: 'fill-[var(--r-lime)]/20' }
					}}
				/>
			</div>
			<ul class="mt-3 space-y-1">
				{#each history as item (item.id)}
					<li>
						<button
							type="button"
							onclick={() => viewHistoryResult(item)}
							disabled={loadingResult}
							class="w-full rounded-lg px-2 py-1.5 text-start text-xs hover:bg-[var(--r-soft)] disabled:opacity-50"
							style="color: var(--r-ink)"
						>
							<span class="font-medium">{new Date(item.created_at).toLocaleDateString()}</span>
							<span class="ms-2" style="color: var(--r-muted)">
								{Math.round((item.total_score / Math.max(maxScore, 1)) * 100)}%
								{#if item.threshold_label}
									— {item.threshold_label}
								{/if}
							</span>
						</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
{/snippet}

<div class="readiness">
	<div class="relative mx-auto w-full max-w-6xl px-4 pb-16 pt-8 sm:px-6 lg:px-10 lg:pt-12">
		<!-- Account avatar — pinned to the page's top-right corner, the same
		     spot it occupies in the shell sidebar topbar on other pages. -->
		<div class="absolute end-4 top-8 sm:end-6 lg:end-10 lg:top-12">
			<UserDropdown {user} />
		</div>
		{#if disabled}
			<div class="r-card p-8">
				<EmptyState
					icon={Lock}
					title={t('tools.disabled_title', 'This tool is currently unavailable')}
					body={t('tools.disabled_body', 'Check back soon or contact support.')}
				/>
			</div>
		{:else if !form}
			<div class="r-card p-8">
				<LoadingSkeleton variant="lines" />
			</div>
		{:else if result}
			<!-- Results view -->
			<header class="flex flex-wrap items-start justify-between gap-4">
				<div class="max-w-2xl">
					<h1 class="font-serif text-4xl font-semibold leading-[1.08] tracking-tight sm:text-5xl" style="color: var(--r-ink)">
						{t('assessment.report_title', 'Your readiness report.')}
					</h1>
					<p class="mt-3 text-base" style="color: var(--r-muted)">
						{t('assessment.report_sub', 'Here is how ready you are — and what to work on next.')}
					</p>
				</div>
				<div class="ms-auto flex items-center gap-3 pe-16">
					<button
						type="button"
						onclick={resetResult}
						class="r-back"
					>
						{t('assessment.retake', 'Retake assessment')}
					</button>
				</div>
			</header>

			{#if resultLoadError}
				<div class="r-alert mt-6">
					{resultLoadError.message}
				</div>
			{/if}

			<div class="mt-8 grid gap-6 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
				<div class="space-y-6">
					<!-- Score gauge -->
					<div class="r-card p-6 sm:p-8">
						<div class="flex items-center gap-6">
							<div class="relative grid h-28 w-28 shrink-0 place-items-center">
								<svg class="h-28 w-28 -rotate-90" viewBox="0 0 80 80">
									<circle cx="40" cy="40" r="34" fill="none" stroke="var(--r-soft)" stroke-width="7" />
									<circle
										cx="40" cy="40" r="34" fill="none"
										stroke={gaugeColor}
										stroke-width="7"
										stroke-dasharray={2 * Math.PI * 34}
										stroke-dashoffset={2 * Math.PI * 34 * (1 - scorePct / 100)}
										stroke-linecap="round"
										class="transition-all duration-700"
									/>
								</svg>
								<span class="absolute font-serif text-2xl font-semibold" style="color: var(--r-ink)">{scorePct}%</span>
							</div>
							<div class="min-w-0 flex-1">
								<span class="block font-serif text-2xl font-semibold" style="color: {gaugeColor};">
									{result.threshold_label}
								</span>
								<span class="mt-1 block text-sm" style="color: var(--r-muted)">
									{t('assessment.score', 'Score')}: {result.total_score} / {form.scoring_config.max_score}
								</span>
							</div>
						</div>

						{#if Object.keys(result.dimension_scores ?? {}).length > 0}
							<div class="mt-8 border-t pt-6" style="border-color: var(--r-line)">
								<h3 class="mb-4 text-sm font-semibold" style="color: var(--r-ink)">
									{t('assessment.dimensions', 'Dimension breakdown')}
								</h3>
								<div class="space-y-3">
									{#each Object.entries(result.dimension_scores ?? {}) as [dim, score] (dim)}
										<div>
											<div class="flex items-center justify-between text-xs" style="color: var(--r-muted)">
												<span class="capitalize">{stepLabel(dim)}</span>
												<span>{score}</span>
											</div>
											<div class="mt-1.5 h-2 overflow-hidden rounded-full" style="background: var(--r-soft)">
												<div
													class="h-full rounded-full transition-all"
													style="width: {Math.min(100, Math.max(0, Math.round(score)))}%; background: var(--r-lime)"
												></div>
											</div>
										</div>
									{/each}
								</div>
							</div>
						{/if}
					</div>

					<!-- Journey context -->
					{#if roadmap?.context}
						{@const ctx = roadmap.context}
						<div class="r-card p-6">
							<h3 class="mb-3 text-sm font-semibold" style="color: var(--r-ink)">
								{t('assessment.your_journey', 'Your journey context')}
							</h3>
							<div class="flex flex-wrap gap-2">
								{#if ctx.destination_country}
									<span class="r-chip"><MapPin size={12} /> {countryLabel(ctx.destination_country)}</span>
								{/if}
								{#if ctx.origin_country}
									<span class="r-chip"><Flag size={12} /> {countryLabel(ctx.origin_country)}</span>
								{/if}
								{#if ctx.journey_stage}
									<span class="r-chip"><Plane size={12} /> {journeyOptionLabel(data.journeyOptions.stages, ctx.journey_stage)}</span>
								{/if}
								{#if ctx.visa_route}
									<span class="r-chip"><Briefcase size={12} /> {journeyOptionLabel(data.journeyOptions.visa_routes, ctx.visa_route)}</span>
								{/if}
								{#if ctx.purpose}
									<span class="r-chip">{t('assessment.purpose', 'Purpose')}: {journeyOptionLabel(data.journeyOptions.purposes, ctx.purpose)}</span>
								{/if}
								{#if ctx.travel_date}
									<span class="r-chip">
										<Calendar size={12} /> {new Date(ctx.travel_date).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' })}
									</span>
								{/if}
							</div>
						</div>
					{/if}

					<!-- Roadmap -->
					{#if roadmapLoading}
						<div class="r-card p-6">
							<h3 class="mb-3 text-sm font-semibold" style="color: var(--r-ink)">
								{t('assessment.roadmap', 'Recommended next steps')}
							</h3>
							<LoadingSkeleton variant="lines" />
						</div>
					{:else if roadmapError}
						<div class="r-alert">{roadmapError}</div>
					{:else if roadmap?.items && roadmap.items.length > 0}
						<div class="r-card p-6">
							<h3 class="mb-4 text-sm font-semibold" style="color: var(--r-ink)">
								{t('assessment.roadmap', 'Recommended next steps')}
							</h3>
							<ol class="space-y-4">
								{#each roadmap.items as item, i (item.slug)}
									<li class="flex items-start gap-3">
										<span class="r-stepnum">{i + 1}</span>
										<span class="min-w-0 flex-1">
											<a href="/content/{item.slug}" class="block text-sm font-medium hover:underline" style="color: var(--r-ink)">
												{item.title}
											</a>
											{#if item.excerpt}
												<span class="mt-0.5 block text-xs" style="color: var(--r-muted)">{item.excerpt}</span>
											{/if}
											<div class="mt-1.5 flex flex-wrap items-center gap-2 text-xs" style="color: var(--r-muted)">
												{#if item.target_country}
													<span class="r-chip"><MapPin size={10} /> {countryLabel(item.target_country)}</span>
												{/if}
												{#if item.journey_stage}
													<span class="r-chip"><Plane size={10} /> {journeyOptionLabel(data.journeyOptions.stages, item.journey_stage)}</span>
												{/if}
												{#if item.visa_route}
													<span class="r-chip"><Briefcase size={10} /> {journeyOptionLabel(data.journeyOptions.visa_routes, item.visa_route)}</span>
												{/if}
											</div>
											<a href="/content/{item.slug}" class="mt-1 inline-flex items-center gap-1 text-xs font-medium hover:underline" style="color: var(--r-ink-2)">
												{t('assessment.read_more', 'Read more')} <ArrowRight class="rtl:-scale-x-100" size={12} />
											</a>
										</span>
									</li>
								{/each}
							</ol>
						</div>
					{:else if result.roadmap?.length > 0}
						<div class="r-card p-6">
							<h3 class="mb-4 text-sm font-semibold" style="color: var(--r-ink)">
								{t('assessment.roadmap', 'Recommended next steps')}
							</h3>
							<ol class="space-y-4">
								{#each result.roadmap as step, i (i)}
									<li class="flex items-start gap-3">
										<span class="r-stepnum">{i + 1}</span>
										<span class="min-w-0 flex-1">
											<span class="block text-sm font-medium" style="color: var(--r-ink)">{step.title}</span>
											{#if step.description}
												<span class="mt-0.5 block text-xs" style="color: var(--r-muted)">{step.description}</span>
											{/if}
											{#if step.content_slug}
												<a href="/content/{step.content_slug}" class="mt-1 inline-flex items-center gap-1 text-xs font-medium hover:underline" style="color: var(--r-ink-2)">
													{t('assessment.read_more', 'Read more')} <ArrowRight class="rtl:-scale-x-100" size={12} />
												</a>
											{/if}
										</span>
									</li>
								{/each}
							</ol>
						</div>
					{/if}

					<!-- Anonymous registration prompt -->
					{#if !authed && showRegisterPrompt}
						<div class="r-card p-6" style="border-color: var(--r-ink-2)">
							<div class="flex items-start justify-between gap-3">
								<div>
									<h3 class="font-serif text-lg font-semibold" style="color: var(--r-ink)">
										{t('assessment.save_results_title', 'Save your results')}
									</h3>
									<p class="mt-1 text-sm" style="color: var(--r-muted)">
										{t('assessment.save_results_desc', 'Create a free account to save your results, track your progress over time, and get your personalized roadmap.')}
									</p>
									<div class="mt-4 flex items-center gap-2">
										<a href="/signup?next={nextReturnPath}" class="r-continue">
											{t('assessment.create_account', 'Create free account')}
										</a>
										<a href="/login?next={nextReturnPath}" class="r-back">
											{t('assessment.sign_in', 'Sign in')}
										</a>
									</div>
								</div>
								<button
									type="button"
									onclick={() => (showRegisterPrompt = false)}
									class="hover:opacity-70"
									style="color: var(--r-muted)"
									aria-label={t('common.close', 'Close')}
								>
									<X size={16} />
								</button>
							</div>
						</div>
					{/if}
				</div>

				<!-- Aside -->
				<aside class="space-y-6">
					<ReadinessHeroArt class="h-auto w-full" />
					<div class="r-card p-5">
						<div class="flex items-center gap-3">
							<span class="r-tick"><Compass size={20} /></span>
							<div>
								<p class="font-semibold" style="color: var(--r-ink)">{stepTip(steps[0]?.name ?? 'general').title}</p>
								<div class="mt-0.5 h-0.5 w-10 rounded-full" style="background: var(--r-lime)"></div>
							</div>
						</div>
						<p class="mt-2 text-sm" style="color: var(--r-muted)">
							{stepTip(steps[0]?.name ?? 'general').body}
						</p>
					</div>
					{@render historyPanel()}
				</aside>
			</div>
		{:else if loadingResult}
			<!-- Shared/history result loading -->
			<div class="r-card p-8">
				<LoadingSkeleton variant="lines" />
			</div>
		{:else}
			<!-- Form view -->
			<header class="flex flex-wrap items-start justify-between gap-x-8 gap-y-5">
				<div class="max-w-2xl">
					<h1 class="font-serif text-4xl font-semibold leading-[1.08] tracking-tight sm:text-5xl" style="color: var(--r-ink)">
						{#if heroParts.head}{heroParts.head}&nbsp;{/if}<span class="relative inline-block">{heroParts.tail}<svg class="absolute -bottom-1 start-0 h-[0.28em] w-full" viewBox="0 0 120 12" preserveAspectRatio="none" aria-hidden="true"><path d="M3 9 C 34 2, 78 2, 117 7" stroke="var(--r-lime)" stroke-width="5" fill="none" stroke-linecap="round" /></svg></span>
					</h1>
					<p class="mt-3 max-w-lg text-base" style="color: var(--r-muted)">
						{t('tools.readiness_hero_sub', 'Understand your readiness to move abroad — and what to do next.')}
					</p>
				</div>

				<div class="ms-auto flex items-center gap-3 pe-16">
					<div class="r-card flex items-stretch gap-4 px-5 py-3.5">
						<div class="flex items-center gap-3">
							<span class="r-glyph-sm"><ClipboardList size={18} /></span>
							<p class="text-[11px] font-bold uppercase leading-tight tracking-[0.14em]" style="color: var(--r-ink)">
								{t('assessment.step_counter', 'Step {current} of {total}', { current: currentStep + 1, total: totalSteps })}
							</p>
						</div>
						<div class="w-px self-stretch" style="background: var(--r-line)"></div>
						<div class="flex flex-col justify-center">
							<p class="text-sm font-bold leading-tight" style="color: var(--r-ink)">
								{t('assessment.answered_count', '{answered} of {total}', { answered: requiredAnsweredCount, total: requiredFields.length })}
							</p>
							<p class="text-xs" style="color: var(--r-muted)">{t('assessment.questions_answered', 'questions answered')}</p>
						</div>
					</div>
				</div>
			</header>

			{#if !authed}
				<div class="mt-5 flex items-center gap-2 rounded-xl border px-3 py-2 text-xs" style="border-color: var(--r-line); background: var(--r-soft); color: var(--r-muted)">
					<Info size={14} class="shrink-0" />
					{t('assessment.anon_hint', 'Sign in to save your score to history. Anonymous results are not saved.')}
				</div>
			{/if}

			{#if resultLoadError}
				<div class="r-alert mt-5">
					<p>{resultLoadError.message}</p>
					{#if resultLoadError.status === 401 && !authed}
						<a href="/login?next={nextReturnPath}" class="mt-1 inline-block text-xs font-medium underline">
							{t('assessment.sign_in_to_view', 'Sign in to view this result')}
						</a>
					{/if}
				</div>
			{/if}

			<!-- Step rail -->
			{#if totalSteps > 1}
				<nav class="mt-8" aria-label={t('assessment.progress', 'Progress')}>
					<ol class="flex items-start">
						{#each steps as s, i (s.name)}
							<li class="flex min-w-0 flex-1 flex-col items-center gap-2">
								<div class="flex w-full items-center">
									<div
										class="h-0.5 flex-1 rounded-full"
										style="background: {i === 0 ? 'transparent' : i <= currentStep ? 'var(--r-ink-2)' : 'var(--r-line)'}"
									></div>
									<span
										class="r-marker"
										class:r-marker-current={i === currentStep}
										class:r-marker-done={i < currentStep}
										aria-current={i === currentStep ? 'step' : undefined}
									>
										{#if i < currentStep}<Check size={15} strokeWidth={3} />{:else}{i + 1}{/if}
									</span>
									<div
										class="h-0.5 flex-1 rounded-full"
										style="background: {i === steps.length - 1 ? 'transparent' : i < currentStep ? 'var(--r-ink-2)' : 'var(--r-line)'}"
									></div>
								</div>
								<span
									class="hidden truncate text-center text-xs font-medium sm:block"
									style="color: {i <= currentStep ? 'var(--r-ink)' : 'var(--r-muted)'}"
								>
									{stepLabel(s.name)}
								</span>
							</li>
						{/each}
					</ol>
					<p class="mt-2 text-xs sm:hidden" style="color: var(--r-muted)">
						{t('assessment.section_progress', 'Section {current} of {total}', { current: currentStep + 1, total: totalSteps })}
						— {stepLabel(currentStepData?.name ?? '')}
					</p>
				</nav>
			{/if}

			<div class="mt-8 grid items-start gap-8 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
				<!-- Form card -->
				<section class="r-card p-6 sm:p-8">
					{#if currentStepData}
						{@const StepGlyph = stepIcon(currentStepData.name)}
						<div class="flex items-start gap-4">
							<span class="r-glyph"><StepGlyph size={26} /></span>
							<div class="min-w-0">
								<h2 class="font-serif text-3xl font-semibold leading-snug" style="color: var(--r-ink)">
									{stepTitle(currentStepData.name)}
								</h2>
								{#if stepSub(currentStepData.name)}
									<p class="mt-1 text-sm" style="color: var(--r-muted)">{stepSub(currentStepData.name)}</p>
								{/if}
							</div>
						</div>

						<hr class="my-6" style="border-color: var(--r-line)" />

						<div class="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
							{#each currentStepData.fields as field (field.key)}
								{@const FieldGlyph = fieldIcon(field.key)}
								<div class={isWideField(field) ? 'sm:col-span-2' : ''}>
									{#if isGroupField(field)}
										<span class="mb-2.5 flex items-center gap-2.5 text-sm font-bold" style="color: var(--r-ink)">
											<span class="r-glyph-field"><FieldGlyph size={18} /></span>
											{field.label}
											{#if field.is_required}<span style="color: var(--r-pin)">*</span>{/if}
										</span>
									{:else}
										<label class="mb-2.5 flex items-center gap-2.5 text-sm font-bold" style="color: var(--r-ink)" for={field.key}>
											<span class="r-glyph-field"><FieldGlyph size={18} /></span>
											{field.label}
											{#if field.is_required}<span style="color: var(--r-pin)">*</span>{/if}
										</label>
									{/if}

									{#if isCountryField(field) && data.countryOptions.length > 0}
										<AutocompleteSelect
											id={field.key}
											value={typeof answers[field.key] === 'string' ? (answers[field.key] as string) : ''}
											options={[
												{ value: '', label: field.placeholder || t('assessment.select_country', 'Select a country') },
												...data.countryOptions.map((o) => ({ value: o.value, label: o.label }))
											]}
											onValueChange={(v) => setAnswer(field.key, v)}
										/>
									{:else if (field.field_type === 'select' || field.field_type === 'radio' || field.field_type === 'suggest') && fieldOptions(field).length > 0}
										<div class="flex flex-wrap gap-2" role="radiogroup" aria-label={field.label} tabindex={-1} onkeydown={(e) => radioGroupKeydown(e, field)}>
											{#each fieldOptions(field) as opt, i (opt.value)}
												{@const OptGlyph = optionIcon(opt.value)}
												{@const selected = answers[field.key] === opt.value}
												<button
													type="button"
													role="radio"
													aria-checked={selected}
													id="{field.key}-opt-{i}"
													tabindex={selected || (answers[field.key] === undefined && i === 0) ? 0 : -1}
													onclick={() => setAnswer(field.key, opt.value)}
													class="r-pill"
													class:r-pill-on={selected}
												>
													<OptGlyph size={16} class="shrink-0" />
													<span>{opt.label}</span>
												</button>
											{/each}
										</div>
									{:else if field.field_type === 'multiselect' && fieldOptions(field).length > 0}
										<div class="flex flex-wrap gap-2">
											{#each fieldOptions(field) as opt (opt.value)}
												{@const OptGlyph = optionIcon(opt.value)}
												{@const selected = Array.isArray(answers[field.key]) && (answers[field.key] as string[]).includes(opt.value)}
												<button
													type="button"
													aria-pressed={selected}
													onclick={() => toggleMulti(field.key, opt.value, !selected)}
													class="r-pill"
													class:r-pill-on={selected}
												>
													<OptGlyph size={16} class="shrink-0" />
													<span>{opt.label}</span>
												</button>
											{/each}
										</div>
									{:else if field.field_type === 'boolean'}
										<div class="flex gap-2" role="radiogroup" aria-label={field.label} tabindex={-1} onkeydown={(e) => boolGroupKeydown(e, field)}>
											<button
												type="button"
												role="radio"
												aria-checked={answers[field.key] === true}
												id="{field.key}-bool-true"
												tabindex={answers[field.key] !== false ? 0 : -1}
												onclick={() => setAnswer(field.key, true)}
												class="r-pill"
												class:r-pill-on={answers[field.key] === true}
											>
												<CircleCheckBig size={16} />
												{t('assessment.yes', 'Yes')}
											</button>
											<button
												type="button"
												role="radio"
												aria-checked={answers[field.key] === false}
												id="{field.key}-bool-false"
												tabindex={answers[field.key] === false ? 0 : -1}
												onclick={() => setAnswer(field.key, false)}
												class="r-pill"
												class:r-pill-on={answers[field.key] === false}
											>
												<CircleDashed size={16} />
												{t('assessment.no', 'No')}
											</button>
										</div>
									{:else if field.field_type === 'rating'}
										<div class="flex items-center gap-3">
											<input
												type="range"
												min="0"
												max="100"
												value={(answers[field.key] as number) ?? 0}
												oninput={(e) => setAnswer(field.key, parseInt(e.currentTarget.value))}
												class="r-range flex-1"
												aria-label={field.label}
											/>
											<span class="w-8 text-center text-sm font-semibold" style="color: var(--r-ink)">
												{(answers[field.key] as number) ?? 0}
											</span>
										</div>
									{:else if field.field_type === 'number' || field.field_type === 'currency' || field.field_type === 'percentage' || field.field_type === 'price_range'}
										<input
											id={field.key}
											type="number"
											placeholder={field.placeholder || ''}
											value={(answers[field.key] as number) ?? ''}
											oninput={(e) => {
												const raw = (e.currentTarget as HTMLInputElement).value;
												setAnswer(field.key, raw === '' ? '' : parseFloat(raw));
											}}
											class="r-input"
										/>
									{:else if field.field_type === 'date' || field.field_type === 'datetime' || field.field_type === 'time'}
										<input
											id={field.key}
											type={field.field_type === 'datetime' ? 'datetime-local' : field.field_type}
											value={(answers[field.key] as string) ?? ''}
											oninput={(e) => setAnswer(field.key, (e.currentTarget as HTMLInputElement).value)}
											class="r-input"
										/>
									{:else if field.field_type === 'textarea' || field.field_type === 'rich_text'}
										<textarea
											id={field.key}
											rows="3"
											placeholder={field.placeholder || ''}
											value={(answers[field.key] as string) ?? ''}
											oninput={(e) => setAnswer(field.key, (e.currentTarget as HTMLTextAreaElement).value)}
											class="r-input"
										></textarea>
									{:else}
										<input
											id={field.key}
											type="text"
											placeholder={field.placeholder || ''}
											value={(answers[field.key] as string) ?? ''}
											oninput={(e) => setAnswer(field.key, (e.target as HTMLInputElement).value)}
											class="r-input"
										/>
									{/if}
								</div>
							{/each}
						</div>
					{/if}

					{#if submitError}
						<p class="r-alert mt-6">{submitError}</p>
					{/if}

					{#if pendingSubmission}
						<div class="r-alert mt-6" role="status">
							<p>{t('assessment.still_scoring', 'Your answers are still being scored — this can take a moment.')}</p>
							<button
								type="button"
								onclick={() => pendingSubmission && runPoll(pendingSubmission, false)}
								disabled={scoring}
								class="mt-2 inline-flex items-center gap-1 text-xs font-medium underline"
							>
								{scoring ? t('assessment.scoring', 'Scoring…') : t('assessment.check_now', 'Check again')}
							</button>
						</div>
					{/if}

					<div class="mt-8 flex items-center justify-between gap-4 border-t pt-6" style="border-color: var(--r-line)">
						<button
							type="button"
							onclick={prevStep}
							disabled={currentStep === 0}
							class="r-back"
						>
							<ArrowLeft class="rtl:-scale-x-100" size={16} />
							{t('assessment.back', 'Back')}
						</button>

						{#if currentStep < totalSteps - 1}
							<button
								type="button"
								onclick={nextStep}
								disabled={!stepComplete()}
								class="r-continue"
							>
								{t('assessment.continue_to', 'Continue to {step}', { step: nextStepName })}
								<ArrowRight class="rtl:-scale-x-100" size={17} />
							</button>
						{:else}
							<button
								type="button"
								onclick={submit}
								disabled={submitting || scoring || !stepComplete()}
								class="r-continue"
							>
								{#if scoring}
									{t('assessment.scoring', 'Scoring…')}
								{:else if submitting}
									{t('common.submitting', 'Submitting…')}
								{:else}
									<CheckCircle2 size={17} /> {form.submit_label || t('assessment.submit', 'Submit')}
								{/if}
							</button>
						{/if}
					</div>
				</section>

				<!-- Aside: illustration + tip -->
				<aside class="space-y-6">
					<ReadinessHeroArt class="h-auto w-full max-w-md" />
					{#if currentStepData}
						{@const tip = stepTip(currentStepData.name)}
						<div class="r-card relative overflow-hidden p-5">
							<div class="flex items-center gap-3">
								<span class="r-tick"><Compass size={20} /></span>
								<div>
									<p class="font-semibold" style="color: var(--r-ink)">{tip.title}</p>
									<div class="mt-0.5 h-0.5 w-10 rounded-full" style="background: var(--r-lime)"></div>
								</div>
							</div>
							<p class="mt-2 text-sm" style="color: var(--r-muted)">{tip.body}</p>
							<Flag size={44} class="absolute -bottom-2 -end-1 opacity-10" style="color: var(--r-ink-2)" />
						</div>
					{/if}
					{@render historyPanel()}
				</aside>
			</div>
		{/if}

	</div>
</div>

<style>
	/* Mockup structure, site palette — every --r-* var aliases the app's theme
	   tokens so the wizard flips with light/dark like the rest of the shell.
	   --radius-input is the only override: the Autocomplete wrapper reads it
	   to round the country selectors like the other inputs. */
	.readiness {
		--r-cream: var(--color-bg);
		--r-card: var(--color-surface);
		--r-ink: var(--color-text);
		--r-ink-2: var(--color-primary);
		--r-lime: var(--color-secondary);
		--r-lime-hover: var(--color-secondary-hover);
		--r-accent: var(--color-primary);
		--r-accent-hover: var(--color-primary-hover);
		--r-on-accent: var(--color-primary-contrast);
		--r-muted: var(--color-muted);
		--r-line: var(--color-border);
		--r-soft: var(--color-surface-subtle);
		--r-pin: var(--color-warning);

		--radius-input: 12px;

		min-height: calc(100dvh - 3.5rem);
		background: var(--r-cream);
		color: var(--r-ink);
	}
	@media (min-width: 768px) {
		.readiness {
			min-height: 100dvh;
		}
	}

	.r-card {
		border: 1px solid var(--r-line);
		border-radius: 20px;
		background: var(--r-card);
		box-shadow: 0 1px 2px rgb(0 0 0 / 0.04);
	}

	.r-input {
		width: 100%;
		border: 1px solid var(--r-line);
		border-radius: 12px;
		background: var(--r-card);
		color: var(--r-ink);
		padding: 0.7rem 0.95rem;
		font-size: 0.9rem;
	}
	.r-input::placeholder {
		color: var(--r-muted);
	}
	.r-input:focus-visible {
		outline: 2px solid var(--r-lime);
		outline-offset: 1px;
		border-color: transparent;
	}

	.r-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border: 1px solid var(--r-line);
		border-radius: 10px;
		background: var(--r-card);
		color: var(--r-ink);
		padding: 0.55rem 0.9rem;
		font-size: 0.875rem;
		font-weight: 500;
		text-align: start;
		transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
	}
	.r-pill:hover {
		border-color: var(--r-ink-2);
	}
	.r-pill:focus-visible {
		outline: 2px solid var(--r-lime);
		outline-offset: 2px;
	}
	.r-pill-on,
	.r-pill-on:hover {
		border-color: var(--r-ink-2);
		background: var(--r-ink-2);
		color: var(--r-on-accent);
	}

	.r-marker {
		display: grid;
		place-items: center;
		height: 2.5rem;
		width: 2.5rem;
		flex-shrink: 0;
		border: 2px solid var(--r-line);
		border-radius: 9999px;
		background: var(--r-card);
		color: var(--r-muted);
		font-size: 0.875rem;
		font-weight: 600;
		transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
	}
	.r-marker-current {
		border-color: var(--r-ink-2);
		background: var(--r-ink-2);
		color: var(--r-on-accent);
	}
	.r-marker-done {
		border-color: var(--r-ink-2);
		background: var(--r-ink-2);
		color: var(--r-on-accent);
	}

	.r-continue {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		border-radius: 12px;
		background: var(--r-accent);
		color: var(--r-on-accent);
		padding: 0.75rem 1.5rem;
		font-size: 0.9rem;
		font-weight: 700;
		transition: background-color 0.15s ease, transform 0.15s ease;
	}
	.r-continue:hover:not(:disabled) {
		background: var(--r-accent-hover);
	}
	.r-continue:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.r-continue:focus-visible {
		outline: 2px solid var(--r-ink-2);
		outline-offset: 2px;
	}

	.r-back {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		border: 1px solid var(--r-line);
		border-radius: 12px;
		background: transparent;
		color: var(--r-muted);
		padding: 0.7rem 1.25rem;
		font-size: 0.9rem;
		font-weight: 500;
		transition: border-color 0.15s ease, color 0.15s ease;
	}
	.r-back:hover:not(:disabled) {
		border-color: var(--r-ink-2);
		color: var(--r-ink);
	}
	.r-back:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.r-back:focus-visible {
		outline: 2px solid var(--r-lime);
		outline-offset: 2px;
	}

	.r-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		border: 1px solid var(--r-line);
		border-radius: 9999px;
		background: var(--r-soft);
		color: var(--r-ink);
		padding: 0.3rem 0.7rem;
		font-size: 0.75rem;
	}

	.r-stepnum {
		display: grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		flex-shrink: 0;
		border-radius: 9999px;
		background: var(--r-ink-2);
		color: var(--r-on-accent);
		font-size: 0.75rem;
		font-weight: 600;
	}

	/* Tinted glyph chips — same convention as LeftNav's .nav-icon-chip:
	   14% primary tint disc + saturated primary glyph (no accent-on-primary
	   colour clash). */
	.r-glyph,
	.r-glyph-sm,
	.r-tick {
		display: grid;
		place-items: center;
		flex-shrink: 0;
		border-radius: 9999px;
		background: color-mix(in srgb, var(--r-ink-2) 14%, transparent);
		color: var(--r-ink-2);
	}
	.r-glyph {
		height: 3.5rem;
		width: 3.5rem;
	}
	.r-glyph-sm {
		height: 2.5rem;
		width: 2.5rem;
	}
	.r-glyph-field {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		flex-shrink: 0;
		border-radius: 9999px;
		background: color-mix(in srgb, var(--r-ink-2) 14%, transparent);
		color: var(--r-ink-2);
	}
	.r-tick {
		height: 2.75rem;
		width: 2.75rem;
	}

	.r-alert {
		border: 1px solid color-mix(in srgb, var(--r-pin) 30%, transparent);
		border-radius: 12px;
		background: color-mix(in srgb, var(--r-pin) 10%, transparent);
		color: var(--r-pin);
		padding: 0.75rem 1rem;
		font-size: 0.875rem;
	}

	.r-range {
		accent-color: var(--r-lime);
	}
</style>
