<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Curriculum editor — Japa Academy creator authors modules, lessons, and
	// quiz questions on their own track (ADR-063). Platform admins may edit
	// any track's curriculum (the backend bypasses ownership for them).
	// CSR ONLY: /academy/my/* endpoints.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { uploadLessonFile, listMyMedia, uploadMediaImage, stockApi } from '$lib/api/media';
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { onMount } from 'svelte';
	import { RichTextEditor, MediaPickerModal } from '@japaconnect/ui';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import {
		ArrowLeft,
		Plus,
		Pencil,
		Trash2,
		ChevronUp,
		ChevronDown,
		FileText,
		Video,
		CircleHelp,
		ListChecks,
		FileDown,
		MonitorPlay,
		Link as LinkIcon,
		Music,
		Send,
		ExternalLink
	} from 'lucide-svelte';
	import type { Curriculum, CurriculumModule, CurriculumLesson, QuizEditQuestion, LessonType } from '$lib/types';

	const trackId = page.params.id;

	let curriculum = $state<Curriculum | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let notice = $state<string | null>(null);
	let submittingTrack = $state(false);

	// ---------------- module editor state ----------------
	let addingModule = $state(false);
	let newModuleTitle = $state('');
	let editingModuleId = $state<string | null>(null);
	let editModuleTitle = $state('');
	let editModuleDescription = $state('');
	let busyModule = $state(false);

	// ---------------- lesson editor state ----------------
	// Which lesson is open in the editor panel (null = creating a new lesson in
	// `lessonModuleId`).
	let lessonModuleId = $state<string | null>(null);
	let editingLessonId = $state<string | null>(null);
	let lessonForm = $state({
		title: '',
		slug: '',
		lesson_type: 'article' as LessonType,
		content: '',
		external_url: '',
		duration_minutes: 0,
		sort_order: 0,
		is_published: false,
		is_preview: false
	});
	let lessonMediaId = $state<string | null>(null);
	let uploadingFile = $state(false);
	let busyLesson = $state(false);
	let busyMove = $state(false);
	// Snapshot of the article markdown at editor-open time — seeds the
	// RichTextEditor once; edits flow back via onmarkdown → lessonForm.content.
	let articleSeed = $state('');
	// Content is tracked via a flag (the rich editor normalizes its seed at
	// mount, which would false-positive a baseline string compare). The
	// baseline snapshot covers every other field + the attached media id.
	let lessonBaseline = $state<string | null>(null);
	let lessonContentEdited = $state(false);
	let openedLessonType = $state<LessonType | null>(null);
	// Media picker for article images — RichTextEditor hands us its insert
	// callback so the picked asset lands at the current cursor position.
	let mediaPickerOpen = $state(false);
	let pendingImageInsert: ((src: string, alt?: string) => void) | null = null;

	// ---------------- quiz builder state ----------------
	let quizQuestions = $state<QuizEditQuestion[]>([]);
	let quizDirty = $state(false);
	let busyQuiz = $state(false);

	const lessonTypeOptions: { value: LessonType; label: string; icon: typeof FileText }[] = [
		{ value: 'article', label: t('curriculum.type_article', 'Article'), icon: FileText },
		{ value: 'quiz', label: t('curriculum.type_quiz', 'Quiz'), icon: CircleHelp },
		{ value: 'video', label: t('curriculum.type_video', 'Video'), icon: Video },
		{ value: 'youtube', label: t('curriculum.type_youtube', 'YouTube embed'), icon: MonitorPlay },
		{ value: 'screencast', label: t('curriculum.type_screencast', 'Screencast'), icon: MonitorPlay },
		{ value: 'document', label: t('curriculum.type_document', 'Document'), icon: FileDown },
		{ value: 'audio', label: t('curriculum.type_audio', 'Audio'), icon: Music },
		{ value: 'checklist', label: t('curriculum.type_checklist', 'Checklist'), icon: ListChecks },
		{ value: 'link', label: t('curriculum.type_link', 'External link'), icon: LinkIcon }
	];

	function typeMeta(type: string) {
		return lessonTypeOptions.find((o) => o.value === type) ?? lessonTypeOptions[0];
	}

	// ---------------- data loading ----------------

	async function load() {
		loading = true;
		error = null;
		const r = await api.get<Curriculum>(`/academy/my/tracks/${trackId}/curriculum`);
		if (r.error) {
			error = r.error.message;
		} else {
			curriculum = r.data;
		}
		loading = false;
	}

	onMount(load);

	onMount(() => {
		const handler = (e: BeforeUnloadEvent) => {
			if (lessonDirty) e.preventDefault();
		};
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
	});

	beforeNavigate((nav) => {
		if (lessonDirty && nav.type !== 'leave') {
			if (!confirm(t('curriculum.unsaved_confirm', 'You have unsaved lesson changes. Discard them?'))) {
				nav.cancel();
			}
		}
	});

	function flash(msg: string) {
		notice = msg;
		setTimeout(() => (notice = null), 4000);
	}

	// ---------------- track submit-for-review ----------------

	async function submitTrack() {
		if (submittingTrack) return;
		submittingTrack = true;
		const r = await api.post(`/academy/my/tracks/${trackId}/submit`);
		submittingTrack = false;
		if (r.error) {
			error = r.error.message;
			return;
		}
		flash(t('curriculum.submitted', 'Track submitted for review'));
		await load();
	}

	// ---------------- module ops ----------------

	async function createModule() {
		if (!newModuleTitle.trim() || busyModule) return;
		busyModule = true;
		const r = await api.post(`/academy/my/tracks/${trackId}/modules`, { title: newModuleTitle.trim() });
		busyModule = false;
		if (r.error) {
			error = r.error.message;
			return;
		}
		newModuleTitle = '';
		addingModule = false;
		await load();
	}

	function startEditModule(m: CurriculumModule) {
		editingModuleId = m.id;
		editModuleTitle = m.title;
		editModuleDescription = m.description ?? '';
	}

	async function saveModule() {
		if (!editingModuleId || busyModule) return;
		busyModule = true;
		const r = await api.patch(`/academy/my/modules/${editingModuleId}`, {
			title: editModuleTitle.trim(),
			description: editModuleDescription
		});
		busyModule = false;
		if (r.error) {
			error = r.error.message;
			return;
		}
		editingModuleId = null;
		await load();
	}

	async function deleteModule(m: CurriculumModule) {
		if (!confirm(t('curriculum.confirm_delete_module', `Delete module "${m.title}" and all its lessons?`))) return;
		const r = await api.delete(`/academy/my/modules/${m.id}`);
		if (r.error) {
			error = r.error.message;
			return;
		}
		if (lessonModuleId === m.id) resetLessonEditor();
		await load();
	}

	// Move a module one step up/down by swapping sort_order with its neighbour.
	async function moveModule(mods: CurriculumModule[], idx: number, dir: -1 | 1) {
		if (busyMove) return;
		const other = mods[idx + dir];
		if (!other) return;
		const cur = mods[idx];
		busyMove = true;
		const r1 = await api.patch(`/academy/my/modules/${cur.id}`, { sort_order: other.sort_order });
		const r2 = r1.error ? r1 : await api.patch(`/academy/my/modules/${other.id}`, { sort_order: cur.sort_order });
		busyMove = false;
		if (r1.error || r2.error) {
			error = (r1.error ?? r2.error)?.message ?? t('common.error', 'Error');
			return;
		}
		await load();
	}

	// ---------------- lesson ops ----------------

	function lessonEditorOpenFor(moduleId: string): boolean {
		return lessonModuleId === moduleId;
	}

	// Baseline snapshot of the editable fields — `content` is excluded (the
	// rich editor emits a normalized seed at mount; `lessonContentEdited`
	// tracks real edits instead). sort_order is reorder-owned, not a field.
	function lessonFieldsSnapshot(): string {
		return JSON.stringify({
			title: lessonForm.title,
			slug: lessonForm.slug,
			lesson_type: lessonForm.lesson_type,
			external_url: lessonForm.external_url,
			duration_minutes: lessonForm.duration_minutes ?? 0,
			is_published: lessonForm.is_published,
			is_preview: lessonForm.is_preview,
			media: lessonMediaId,
			// Checklist edits write lessonForm.content through a plain textarea
			// — include it for non-article types where no editor emits.
			content: lessonForm.lesson_type === 'article' ? '' : lessonForm.content
		});
	}

	const lessonDirty = $derived(
		lessonBaseline !== null &&
			(lessonContentEdited || quizDirty || lessonFieldsSnapshot() !== lessonBaseline)
	);

	function confirmDiscardLesson(): boolean {
		if (!lessonDirty) return true;
		return confirm(t('curriculum.unsaved_confirm', 'You have unsaved lesson changes. Discard them?'));
	}

	async function openLessonEditor(moduleId: string, lesson: CurriculumLesson | null) {
		if (!confirmDiscardLesson()) return;
		lessonModuleId = moduleId;
		editingLessonId = lesson?.id ?? null;
		// Seed only for article lessons — other types don't use the editor.
		articleSeed = lesson?.lesson_type === 'article' ? (lesson.content ?? '') : '';
		quizDirty = false;
		lessonContentEdited = false;
		openedLessonType = lesson?.lesson_type ?? null;

		if (lesson) {
			lessonForm = {
				title: lesson.title,
				slug: lesson.slug,
				lesson_type: lesson.lesson_type,
				content: lesson.content ?? '',
				external_url: '',
				duration_minutes: lesson.duration_minutes ?? 0,
				sort_order: lesson.sort_order,
				is_published: lesson.is_published,
				is_preview: lesson.is_preview
			};
			// For link/embed types the URL lives in content.
			if (['youtube', 'screencast', 'video', 'document', 'audio', 'link'].includes(lesson.lesson_type)) {
				lessonForm.external_url = lesson.content ?? '';
				lessonForm.content = '';
			}
			lessonMediaId = lesson.media_id ?? null;
			if (lesson.lesson_type === 'quiz') {
				const qr = await api.get<QuizEditQuestion[]>(`/academy/my/lessons/${lesson.id}/questions`);
				quizQuestions = qr.data ?? [];
			} else {
				quizQuestions = [];
			}
		} else {
			lessonForm = {
				title: '',
				slug: '',
				lesson_type: 'article',
				content: '',
				external_url: '',
				duration_minutes: 0,
				sort_order: 0,
				is_published: false,
				is_preview: false
			};
			lessonMediaId = null;
			quizQuestions = [];
		}

		lessonBaseline = lessonFieldsSnapshot();
	}

	// Internal teardown — callers that already know the edit is safe to lose
	// (successful save, module delete) pass force=true to skip the confirm.
	function resetLessonEditor() {
		lessonModuleId = null;
		editingLessonId = null;
		quizQuestions = [];
		quizDirty = false;
		lessonBaseline = null;
		lessonContentEdited = false;
		openedLessonType = null;
	}

	function cancelLessonEditor() {
		if (!confirmDiscardLesson()) return;
		resetLessonEditor();
	}

	function lessonPayload(): Record<string, unknown> {
		// URL/embed types store their URL in content; article/checklist/quiz
		// keep it in lessonForm.content. '' (not null) so clearing a field
		// actually clears it server-side — null means "keep" in the PATCH.
		const contentFields = ['youtube', 'screencast', 'video', 'document', 'audio', 'link'].includes(
			lessonForm.lesson_type
		)
			? { content: lessonForm.external_url.trim() }
			: { content: lessonForm.content };

		return {
			title: lessonForm.title.trim(),
			slug: lessonForm.slug.trim() || undefined,
			lesson_type: lessonForm.lesson_type,
			...contentFields,
			// '' detaches the file; null/omitted would preserve it.
			media_id: lessonMediaId ?? '',
			// 0 is a real value (no duration) — the || null form made it
			// impossible to reset a duration back to zero.
			duration_minutes: Math.max(0, lessonForm.duration_minutes ?? 0),
			// sort_order intentionally omitted — appending is the default on
			// create and reordering owns it on update.
			is_published: lessonForm.is_published,
			is_preview: lessonForm.is_preview
		};
	}

	function onLessonTypeChange() {
		// Switching type on an existing lesson drops the previous type's
		// payload (markdown body, URL, media) — warn before it happens.
		if (editingLessonId && openedLessonType && lessonForm.lesson_type !== openedLessonType) {
			const ok = confirm(
				t(
					'curriculum.type_change_confirm',
					'Changing the lesson type discards content saved for the previous type. Continue?'
				)
			);
			if (!ok) lessonForm.lesson_type = openedLessonType;
		}
	}

	async function saveLesson() {
		if (!lessonForm.title.trim() || busyLesson) return;
		error = null;

		// URL-bearing types need a real http(s) URL — type="url" never runs
		// because these are plain buttons, not a form submit.
		const url = lessonForm.external_url.trim();
		const urlTypes = ['youtube', 'screencast', 'video', 'document', 'audio', 'link'];
		if (urlTypes.includes(lessonForm.lesson_type)) {
			const required = lessonForm.lesson_type === 'youtube' || lessonForm.lesson_type === 'link';
			const ok = !url ? !required : /^https?:\/\/\S+$/i.test(url);
			if (!ok) {
				error = required
					? t('curriculum.url_required', 'A valid https:// URL is required for this lesson type.')
					: t('curriculum.url_invalid', 'The external URL must be a valid https:// URL.');
				return;
			}
		}

		busyLesson = true;
		let savedId = editingLessonId;

		if (editingLessonId) {
			const r = await api.patch(`/academy/my/lessons/${editingLessonId}`, lessonPayload());
			if (r.error) {
				error = r.error.message;
				busyLesson = false;
				return;
			}
		} else {
			const r = await api.post<{ id: string }>(`/academy/my/modules/${lessonModuleId}/lessons`, lessonPayload());
			if (r.error) {
				error = r.error.message;
				busyLesson = false;
				return;
			}
			savedId = r.data?.id ?? null;
		}

		// Quiz questions save alongside the lesson when dirty.
		if (lessonForm.lesson_type === 'quiz' && quizDirty && savedId) {
			const saved = await saveQuizQuestions(savedId);
			if (!saved) {
				busyLesson = false;
				return;
			}
		}

		busyLesson = false;
		flash(editingLessonId ? t('common.saved', 'Saved') : t('curriculum.lesson_created', 'Lesson created'));
		resetLessonEditor();
		await load();
	}

	async function deleteLesson(lesson: CurriculumLesson) {
		if (!confirm(t('curriculum.confirm_delete_lesson', `Delete lesson "${lesson.title}"?`))) return;
		const r = await api.delete(`/academy/my/lessons/${lesson.id}`);
		if (r.error) {
			error = r.error.message;
			return;
		}
		if (editingLessonId === lesson.id) resetLessonEditor();
		await load();
	}

	async function moveLesson(lessons: CurriculumLesson[], idx: number, dir: -1 | 1) {
		if (busyMove) return;
		const other = lessons[idx + dir];
		if (!other) return;
		const cur = lessons[idx];
		busyMove = true;
		const r1 = await api.patch(`/academy/my/lessons/${cur.id}`, { sort_order: other.sort_order });
		const r2 = r1.error ? r1 : await api.patch(`/academy/my/lessons/${other.id}`, { sort_order: cur.sort_order });
		busyMove = false;
		if (r1.error || r2.error) {
			error = (r1.error ?? r2.error)?.message ?? t('common.error', 'Error');
			return;
		}
		await load();
	}

	// ---------------- media upload ----------------

	async function onFilePicked(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		uploadingFile = true;
		const ctx = lessonForm.lesson_type === 'document' ? 'document' : 'media';
		const r = await uploadLessonFile(file, ctx);
		uploadingFile = false;
		if (r.error) {
			error = r.error;
			return;
		}
		lessonMediaId = r.mediaId;
		flash(t('curriculum.file_uploaded', 'File uploaded'));
		input.value = '';
	}

	// ---------------- quiz builder ----------------

	function addQuestion() {
		quizQuestions = [
			...quizQuestions,
			{ question: '', options: ['', ''], answer: '', explanation: '' }
		];
		quizDirty = true;
	}

	function removeQuestion(idx: number) {
		quizQuestions = quizQuestions.filter((_, i) => i !== idx);
		quizDirty = true;
	}

	function addOption(q: QuizEditQuestion) {
		q.options = [...q.options, ''];
		quizDirty = true;
	}

	function removeOption(q: QuizEditQuestion, idx: number) {
		const removed = q.options[idx];
		q.options = q.options.filter((_, i) => i !== idx);
		if (q.answer === removed) q.answer = '';
		quizDirty = true;
	}

	async function saveQuizQuestions(lessonId: string): Promise<boolean> {
		if (busyQuiz) return false;
		busyQuiz = true;
		const r = await api.put(`/academy/my/lessons/${lessonId}/questions`, {
			questions: quizQuestions.map((q, i) => ({
				question: q.question,
				options: q.options.filter((o) => o.trim() !== ''),
				answer: q.answer,
				explanation: q.explanation || null,
				position: i + 1
			}))
		});
		busyQuiz = false;
		if (r.error) {
			error = r.error.message;
			return false;
		}
		quizDirty = false;
		return true;
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'approved':
				return 'var(--color-progress)';
			case 'pending':
				return 'var(--color-warning, #d97706)';
			case 'rejected':
				return 'var(--color-destructive)';
			default:
				return 'var(--color-muted)';
		}
	}
</script>

<svelte:head>
	<title>{curriculum?.track.title ?? t('curriculum.title', 'Curriculum')} · {t('app.name')}</title>
</svelte:head>

<main class="mx-auto w-full max-w-5xl px-4 py-6">
	<a
		href="/dashboard/instructor"
		class="inline-flex items-center gap-1 text-sm text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
	>
		<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('common.back', 'Back')}
	</a>

	{#if loading}
		<div class="mt-6"><LoadingSkeleton /></div>
	{:else if error && !curriculum}
		<EmptyState title={t('common.error', 'Error')} description={error} />
	{:else if curriculum}
		<!-- Header -->
		<div class="mt-4 flex flex-wrap items-start justify-between gap-3">
			<div>
				<h1 class="text-2xl font-bold text-[var(--color-text)]">{curriculum.track.title}</h1>
				<div class="mt-1 flex items-center gap-3">
					<span class="text-xs font-medium" style="color: {statusColor(curriculum.track.academy_submission_status)}">
						{t(`academy.submission_${curriculum.track.academy_submission_status}`, curriculum.track.academy_submission_status)}
					</span>
					<a href="/tracks/{curriculum.track.slug}" class="inline-flex items-center gap-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]">
						<ExternalLink class="rtl:-scale-x-100" size={12} /> {t('curriculum.view_track', 'View public page')}
					</a>
					<a href="/dashboard/instructor/tracks/{trackId}/edit" class="inline-flex items-center gap-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]">
						<Pencil size={12} /> {t('curriculum.edit_details', 'Edit details')}
					</a>
				</div>
				{#if curriculum.track.academy_submission_status === 'rejected' && curriculum.track.academy_rejection_reason}
					<p class="mt-2 rounded-[var(--radius)] border border-[var(--color-destructive)] px-3 py-2 text-xs text-[var(--color-destructive)]">
						{curriculum.track.academy_rejection_reason}
					</p>
				{/if}
				{#if curriculum.track.academy_submission_status === 'approved'}
					<p class="mt-2 rounded-[var(--radius)] border border-[var(--color-warning,#d97706)] px-3 py-2 text-xs text-[var(--color-warning,#d97706)]">
						{t('curriculum.live_edit_warning', 'This course is live. Any edit takes it offline until it is re-approved.')}
					</p>
				{/if}
			</div>
			{#if curriculum.track.academy_submission_status === 'draft' || curriculum.track.academy_submission_status === 'rejected'}
				<button
					type="button"
					onclick={submitTrack}
					disabled={submittingTrack}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					<Send class="rtl:-scale-x-100" size={14} />
					{submittingTrack ? t('common.processing', 'Processing...') : t('academy.submit_course', 'Submit for review')}
				</button>
			{/if}
		</div>

		{#if notice}
			<p class="mt-4 rounded-[var(--radius)] border border-[var(--color-progress)] px-3 py-2 text-sm text-[var(--color-progress)]">{notice}</p>
		{/if}
		{#if error}
			<p class="mt-4 rounded-[var(--radius)] border border-[var(--color-destructive)] px-3 py-2 text-sm text-[var(--color-destructive)]">{error}</p>
		{/if}

		<!-- Modules -->
		<div class="mt-6 space-y-4">
			{#each curriculum.modules as m, mi (m.id)}
				<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<!-- Module header -->
					<div class="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
						<div class="flex flex-col">
							<button type="button" class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30" disabled={mi === 0} onclick={() => moveModule(curriculum!.modules, mi, -1)} aria-label={t('curriculum.move_module_up', 'Move module up')}>
								<ChevronUp size={16} />
							</button>
							<button type="button" class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30" disabled={mi === curriculum!.modules.length - 1} onclick={() => moveModule(curriculum!.modules, mi, 1)} aria-label={t('curriculum.move_module_down', 'Move module down')}>
								<ChevronDown size={16} />
							</button>
						</div>
						<div class="flex-1">
							{#if editingModuleId === m.id}
								<div class="space-y-2">
									<input
										type="text"
										bind:value={editModuleTitle}
										class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text)]"
										placeholder={t('curriculum.module_title', 'Module title')}
									/>
									<input
										type="text"
										bind:value={editModuleDescription}
										class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-xs text-[var(--color-text)]"
										placeholder={t('curriculum.module_description', 'Description (optional)')}
									/>
									<div class="flex gap-2">
										<button type="button" onclick={saveModule} disabled={busyModule} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] disabled:opacity-50">{t('common.save', 'Save')}</button>
										<button type="button" onclick={() => (editingModuleId = null)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)]">{t('common.cancel', 'Cancel')}</button>
									</div>
								</div>
							{:else}
								<button type="button" class="text-start" onclick={() => startEditModule(m)}>
									<span class="text-xs font-semibold text-[var(--color-muted)]">{t('academy.module', 'Module')} {mi + 1}</span>
									<h2 class="font-semibold text-[var(--color-text)]">{m.title}</h2>
									{#if m.description}<p class="text-xs text-[var(--color-muted)]">{m.description}</p>{/if}
								</button>
							{/if}
						</div>
						<div class="flex items-center gap-1">
							<button type="button" onclick={() => startEditModule(m)} class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]" aria-label={t('curriculum.edit_module', 'Edit module')}><Pencil size={15} /></button>
							<button type="button" onclick={() => deleteModule(m)} class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-destructive)]" aria-label={t('curriculum.delete_module', 'Delete module')}><Trash2 size={15} /></button>
						</div>
					</div>

					<!-- Lessons -->
					<ul class="divide-y divide-[var(--color-border)]">
						{#each m.lessons as lesson, li (lesson.id)}
							{@const meta = typeMeta(lesson.lesson_type)}
							{@const TypeIcon = meta.icon}
							<li class="flex items-center gap-3 px-4 py-2.5">
								<div class="flex flex-col">
									<button type="button" class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30" disabled={li === 0} onclick={() => moveLesson(m.lessons, li, -1)} aria-label={t('curriculum.move_lesson_up', 'Move lesson up')}><ChevronUp size={14} /></button>
									<button type="button" class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30" disabled={li === m.lessons.length - 1} onclick={() => moveLesson(m.lessons, li, 1)} aria-label={t('curriculum.move_lesson_down', 'Move lesson down')}><ChevronDown size={14} /></button>
								</div>
								<TypeIcon size={16} class="shrink-0 text-[var(--color-muted)]" />
								<div class="flex-1 min-w-0">
									<span class="block truncate text-sm text-[var(--color-text)]">{lesson.title}</span>
									<span class="text-xs text-[var(--color-muted)]">
										{meta.label}
										{#if lesson.lesson_type === 'quiz'}· {lesson.question_count} {t('curriculum.questions', 'questions')}{/if}
										{#if lesson.duration_minutes}· {lesson.duration_minutes}m{/if}
									</span>
								</div>
								{#if lesson.is_preview}
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[10px] font-medium text-[var(--color-muted)]">{t('curriculum.preview', 'PREVIEW')}</span>
								{/if}
								{#if !lesson.is_published}
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[10px] font-medium text-[var(--color-warning,#d97706)]">{t('curriculum.draft', 'DRAFT')}</span>
								{/if}
								<button type="button" onclick={() => openLessonEditor(m.id, lesson)} class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]" aria-label={t('curriculum.edit_lesson', 'Edit lesson')}><Pencil size={15} /></button>
								<button type="button" onclick={() => deleteLesson(lesson)} class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-destructive)]" aria-label={t('curriculum.delete_lesson', 'Delete lesson')}><Trash2 size={15} /></button>
							</li>
						{/each}
						{#if m.lessons.length === 0}
							<li class="px-4 py-3 text-xs text-[var(--color-muted)]">{t('curriculum.no_lessons', 'No lessons yet')}</li>
						{/if}
					</ul>

					<div class="border-t border-[var(--color-border)] px-4 py-2.5">
						<button
							type="button"
							onclick={() => openLessonEditor(m.id, null)}
							class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:text-[var(--color-primary-hover)]"
						>
							<Plus size={14} /> {t('curriculum.add_lesson', 'Add lesson')}
						</button>
					</div>

					<!-- Lesson editor panel -->
					{#if lessonEditorOpenFor(m.id)}
						<div class="border-t border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-4">
							<h3 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
								{editingLessonId ? t('curriculum.edit_lesson', 'Edit lesson') : t('curriculum.new_lesson', 'New lesson')}
							</h3>

							<div class="grid gap-3 sm:grid-cols-2">
								<label class="block">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.lesson_title', 'Title')} *</span>
									<input type="text" bind:value={lessonForm.title} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]" />
								</label>
								<label class="block">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.lesson_slug', 'Slug')}</span>
									<input type="text" bind:value={lessonForm.slug} placeholder={t('curriculum.slug_hint', 'auto from title')} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]" />
								</label>
								<label class="block">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.lesson_type', 'Type')}</span>
									<select bind:value={lessonForm.lesson_type} onchange={onLessonTypeChange} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]">
										{#each lessonTypeOptions as opt (opt.value)}
											<option value={opt.value}>{opt.label}</option>
										{/each}
									</select>
								</label>
								<label class="block">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.duration', 'Duration (minutes)')}</span>
									<input type="number" min="0" bind:value={lessonForm.duration_minutes} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]" />
								</label>
							</div>

							<!-- Type-specific fields -->
							{#if lessonForm.lesson_type === 'article'}
								<div class="mt-3">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.content', 'Content')}</span>
									<RichTextEditor
										content={articleSeed}
										{t}
										placeholder={t('curriculum.article_placeholder', 'Write the lesson…')}
										ariaLabel={t('curriculum.content', 'Content')}
										defaultMode="wysiwyg"
										minHeight={220}
										features={{ image: true, youtube: true, markdown: true }}
										onmarkdown={(md) => (lessonForm.content = md)}
										onPickImage={(insert) => {
											pendingImageInsert = insert;
											mediaPickerOpen = true;
										}}
										onedit={() => (lessonContentEdited = true)}
									/>
								</div>
							{:else if lessonForm.lesson_type === 'checklist'}
								<label class="mt-3 block">
									<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.checklist_items', 'Checklist items (one per line)')}</span>
									<textarea bind:value={lessonForm.content} oninput={() => (lessonContentEdited = true)} rows="6" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"></textarea>
								</label>
							{:else if lessonForm.lesson_type === 'quiz'}
								<div class="mt-3">
									<div class="mb-2 flex items-center justify-between">
										<span class="text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.questions', 'Questions')}</span>
										<button type="button" onclick={addQuestion} class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)]"><Plus size={13} /> {t('curriculum.add_question', 'Add question')}</button>
									</div>
									<div class="space-y-3">
										{#each quizQuestions as q, qi (qi)}
											<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
												<div class="mb-2 flex items-start justify-between gap-2">
													<span class="text-xs font-semibold text-[var(--color-muted)]">Q{qi + 1}</span>
													<button type="button" onclick={() => removeQuestion(qi)} class="text-[var(--color-muted)] hover:text-[var(--color-destructive)]" aria-label={t('curriculum.remove_question', 'Remove question')}><Trash2 size={14} /></button>
												</div>
												<input type="text" bind:value={q.question} placeholder={t('curriculum.question_text', 'Question')} oninput={() => (quizDirty = true)} class="mb-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-sm text-[var(--color-text)]" />
												<div class="space-y-1.5">
													{#each q.options as _opt, oi (oi)}
														<div class="flex items-center gap-2">
															<input
																type="radio"
																name="answer-{qi}"
																checked={q.answer !== '' && q.answer === q.options[oi]}
																onchange={() => { q.answer = q.options[oi]; quizDirty = true; }}
																class="shrink-0"
																title={t('curriculum.mark_correct', 'Mark as correct answer')}
															/>
															<input type="text" bind:value={q.options[oi]} oninput={() => (quizDirty = true)} placeholder={t('curriculum.option_ph', 'Option {n}').replace('{n}', String(oi + 1))} class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1 text-sm text-[var(--color-text)]" />
															<button type="button" onclick={() => removeOption(q, oi)} class="text-[var(--color-muted)] hover:text-[var(--color-destructive)]" aria-label={t('curriculum.remove_option', 'Remove option')}><Trash2 size={13} /></button>
														</div>
													{/each}
												</div>
												<button type="button" onclick={() => addOption(q)} class="mt-1.5 inline-flex items-center gap-1 text-xs text-[var(--color-primary)]"><Plus size={12} /> {t('curriculum.add_option', 'Add option')}</button>
												<input type="text" bind:value={q.explanation} oninput={() => (quizDirty = true)} placeholder={t('curriculum.explanation', 'Explanation shown after submit (optional)')} class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1 text-xs text-[var(--color-text)]" />
											</div>
										{/each}
										{#if quizQuestions.length === 0}
											<p class="text-xs text-[var(--color-muted)]">{t('curriculum.no_questions', 'No questions yet — add at least one for the quiz to work.')}</p>
										{/if}
									</div>
								</div>
							{:else}
								<!-- video / document / audio / screencast / youtube / link -->
								<div class="mt-3 grid gap-3">
									<label class="block">
										<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">
											{#if lessonForm.lesson_type === 'youtube'}{t('curriculum.youtube_url', 'YouTube URL')}
											{:else if lessonForm.lesson_type === 'link'}{t('curriculum.link_url', 'Link URL')}
											{:else}{t('curriculum.external_url', 'External URL (optional when a file is attached)')}{/if}
										</span>
										<input type="url" bind:value={lessonForm.external_url} placeholder="https://…" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)]" />
									</label>
									{#if lessonForm.lesson_type !== 'youtube' && lessonForm.lesson_type !== 'link'}
										<div class="block">
											<span class="mb-1 block text-xs font-medium text-[var(--color-text-subtle)]">{t('curriculum.attach_file', 'Attach file')}</span>
											<div class="flex items-center gap-3">
												<input type="file" onchange={onFilePicked} disabled={uploadingFile} class="text-xs text-[var(--color-text-subtle)] file:me-3 file:rounded-[var(--radius)] file:border-0 file:bg-[var(--color-surface-subtle)] file:px-3 file:py-1.5 file:text-xs file:font-medium file:text-[var(--color-text)]" />
												{#if uploadingFile}<span class="text-xs text-[var(--color-muted)]">{t('curriculum.uploading', 'Uploading…')}</span>{/if}
												{#if lessonMediaId}
													<span class="text-xs text-[var(--color-progress)]">✓ {t('curriculum.file_attached', 'File attached')}</span>
													<button type="button" onclick={() => (lessonMediaId = null)} class="text-xs text-[var(--color-destructive)]">{t('common.remove', 'Remove')}</button>
												{/if}
											</div>
										</div>
									{/if}
								</div>
							{/if}

							<div class="mt-3 flex flex-wrap items-center gap-4">
								<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
									<input type="checkbox" bind:checked={lessonForm.is_published} /> {t('curriculum.published', 'Published')}
								</label>
								<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
									<input type="checkbox" bind:checked={lessonForm.is_preview} /> {t('curriculum.free_preview', 'Free preview')}
								</label>
							</div>

							<div class="mt-4 flex gap-2">
								<button type="button" onclick={saveLesson} disabled={busyLesson || !lessonForm.title.trim()} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
									{busyLesson ? t('common.processing', 'Processing...') : editingLessonId ? t('common.save', 'Save') : t('curriculum.create_lesson', 'Create lesson')}
								</button>
								<button type="button" onclick={cancelLessonEditor} class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-1.5 text-sm text-[var(--color-text)]">{t('common.cancel', 'Cancel')}</button>
							</div>
						</div>
					{/if}
				</section>
			{/each}

			<!-- Add module -->
			{#if addingModule}
				<div class="flex items-center gap-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<input
						type="text"
						bind:value={newModuleTitle}
						placeholder={t('curriculum.module_title', 'Module title')}
						class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text)]"
						onkeydown={(e) => e.key === 'Enter' && createModule()}
					/>
					<button type="button" onclick={createModule} disabled={busyModule || !newModuleTitle.trim()} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50">{t('common.add', 'Add')}</button>
					<button type="button" onclick={() => (addingModule = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">{t('common.cancel', 'Cancel')}</button>
				</div>
			{:else}
				<button
					type="button"
					onclick={() => (addingModule = true)}
					class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] px-4 py-3 text-sm font-medium text-[var(--color-primary)] hover:bg-[var(--color-surface)]"
				>
					<Plus size={16} /> {t('curriculum.add_module', 'Add module')}
				</button>
			{/if}
		</div>
	{/if}
</main>

{#if mediaPickerOpen}
	<MediaPickerModal
		{t}
		allowedType="image"
		fetchItems={listMyMedia}
		onUpload={uploadMediaImage}
		stock={stockApi}
		onSelect={(item) => {
			pendingImageInsert?.(item.url, item.alt_text ?? item.filename);
			pendingImageInsert = null;
			mediaPickerOpen = false;
		}}
		onClose={() => {
			pendingImageInsert = null;
			mediaPickerOpen = false;
		}}
	/>
{/if}
