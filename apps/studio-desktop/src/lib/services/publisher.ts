/**
 * Publish or synchronise a Markdown file to JapaConnect.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createApiClient } from '$lib/services/api';
import { ensureSettingsLoaded, loadSettings, type StudioSettings } from '$lib/services/settings';
import { markdownToHtml, parseFrontmatter } from '$lib/services/markdown';
import { hashFile } from '$lib/services/hash';
import { queueStore } from '$lib/stores/queue.svelte';
import { syncLogStore } from '$lib/stores/sync-log.svelte';
import { validateFrontmatter, validateBody, isHighStakes } from '$lib/services/validation';
import type { Persona, ContentRecord } from '$lib/types/prompt';
import type { ApiResponse } from '$lib/api/client';
import { writable } from 'svelte/store';
import { readTextFile, stat } from '@tauri-apps/plugin-fs';
import { join } from '@tauri-apps/api/path';

export interface PublishInput {
	relativePath: string;
	frontmatter: Record<string, unknown>;
	body: string;
	raw?: string;
	modifiedAt?: string;
}

export interface PublishResult {
	ok: boolean;
	contentId?: string;
	slug?: string;
	status?: string;
	error?: string;
}

// Configurable retry and throttling limits for the publish worker.
const PUBLISH_MAX_RETRIES = 3;
const PUBLISH_BASE_DELAY_MS = 500;
const PUBLISH_MAX_DELAY_MS = 16000;
const BATCH_DELAY_MS = 350; // pause between files in the worker queue

function getString(frontmatter: Record<string, unknown>, key: string): string {
	const value = frontmatter[key];
	return typeof value === 'string' ? value : '';
}

function getOptionalString(frontmatter: Record<string, unknown>, key: string): string | undefined {
	const value = frontmatter[key];
	return typeof value === 'string' && value.trim() !== '' ? value : undefined;
}

function getBoolean(frontmatter: Record<string, unknown>, key: string, defaultValue: boolean): boolean {
	const value = frontmatter[key];
	return typeof value === 'boolean' ? value : defaultValue;
}

function getTags(frontmatter: Record<string, unknown>): string[] {
	const value = frontmatter.tags;
	if (Array.isArray(value)) {
		return value.map((t) => String(t).trim()).filter((t) => t.length > 0);
	}
	if (typeof value === 'string') {
		return value
			.split(',')
			.map((t) => t.trim())
			.filter((t) => t.length > 0);
	}
	return [];
}

function getFieldValues(frontmatter: Record<string, unknown>): Record<string, unknown> | undefined {
	const value = frontmatter.field_values;
	if (value && typeof value === 'object' && !Array.isArray(value)) {
		return value as Record<string, unknown>;
	}
	return undefined;
}

function getJourneyField(
	frontmatter: Record<string, unknown>,
	key: string
): string | undefined {
	const top = getOptionalString(frontmatter, key);
	if (top) return top;
	const fieldValues = getFieldValues(frontmatter);
	if (fieldValues) {
		const v = fieldValues[key];
		if (typeof v === 'string' && v.trim() !== '') return v;
	}
	return undefined;
}

function normalisePublishAt(value: string): string | undefined {
	if (!value) return undefined;
	// datetime-local values lack seconds/zone; normalise to RFC3339-ish.
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
		return `${value}:00Z`;
	}
	return value;
}

function inferredContentType(relativePath: string): string {
	const match = relativePath.match(/^jc-content\/([^/]+)\//);
	return match?.[1] ?? '';
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

function determineStatus(frontmatter: Record<string, unknown>): string {
	const requested = getString(frontmatter, 'status');
	if (requested === 'scheduled' && getString(frontmatter, 'publish_at')) {
		if (isHighStakes(frontmatter)) return 'in_review';
		return 'scheduled';
	}
	if (isHighStakes(frontmatter)) return 'in_review';
	return requested || 'published';
}

function isRetryableStatus(status: number | undefined): boolean {
	return status === 429 || (status !== undefined && status >= 500);
}

async function withRetry<T>(name: string, call: () => Promise<ApiResponse<T>>): Promise<ApiResponse<T>> {
	for (let attempt = 0; attempt <= PUBLISH_MAX_RETRIES; attempt++) {
		try {
			const res = await call();
			if (!res.ok && isRetryableStatus(res.status)) {
				if (attempt < PUBLISH_MAX_RETRIES) {
					const delay = Math.min(PUBLISH_BASE_DELAY_MS * 2 ** attempt, PUBLISH_MAX_DELAY_MS);
					await sleep(delay);
					continue;
				}
			}
			return res;
		} catch (e) {
			if (attempt < PUBLISH_MAX_RETRIES) {
				const delay = Math.min(PUBLISH_BASE_DELAY_MS * 2 ** attempt, PUBLISH_MAX_DELAY_MS);
				await sleep(delay);
				continue;
			}
			return {
				ok: false,
				status: 0,
				error: { code: 'network_error', message: e instanceof Error ? e.message : String(e) }
			};
		}
	}
	return { ok: false, status: 0, error: { code: 'max_retries_exceeded', message: name } };
}

async function fetchPersonas(settings: StudioSettings): Promise<Persona[]> {
	const client = createApiClient(settings.baseUrl);
	const res = await withRetry('list personas', () => client.getPersonas());
	if (!res.ok || !res.data) {
		return [];
	}
	return res.data;
}

function resolvePersonaUserId(personas: Persona[], personaId: string): string | undefined {
	if (!personaId) return undefined;
	const persona = personas.find((p) => p.id === personaId);
	return persona?.user_id;
}

function buildPayload(
	frontmatter: Record<string, unknown>,
	body: string,
	status: string
): unknown {
	const title = getString(frontmatter, 'title') || 'Untitled';
	const slug = getString(frontmatter, 'slug') || '';
	const contentType = getString(frontmatter, 'content_type') || 'guide';
	const bodyHtml = markdownToHtml(body);
	const spaceId = getOptionalString(frontmatter, 'space_id');
	const publishAt = normalisePublishAt(getString(frontmatter, 'publish_at'));
	const fieldValues = getFieldValues(frontmatter);

	const payload: Record<string, unknown> = {
		content_type: contentType,
		title,
		slug,
		body,
		body_html: bodyHtml,
		status,
		visibility: getString(frontmatter, 'visibility') || 'public',
		seo_indexable: getBoolean(frontmatter, 'seo_indexable', true)
	};

	if (spaceId) payload.space_id = spaceId;
	if (publishAt) payload.publish_at = publishAt;

	const guideCategory = getOptionalString(frontmatter, 'guide_category');
	if (guideCategory) payload.guide_category = guideCategory;

	const journeyStage = getJourneyField(frontmatter, 'journey_stage');
	if (journeyStage) payload.journey_stage = journeyStage;

	const visaRoute = getJourneyField(frontmatter, 'visa_route');
	if (visaRoute) payload.visa_route = visaRoute;

	const sourceCountry = getJourneyField(frontmatter, 'source_country');
	if (sourceCountry) payload.source_country = sourceCountry;

	const targetCountry = getJourneyField(frontmatter, 'target_country');
	if (targetCountry) payload.target_country = targetCountry;

	const metaTitle = getOptionalString(frontmatter, 'meta_title');
	if (metaTitle) payload.meta_title = metaTitle;

	const metaDescription = getOptionalString(frontmatter, 'meta_description');
	if (metaDescription) payload.meta_description = metaDescription;

	const ogImageUrl = getOptionalString(frontmatter, 'og_image_url');
	if (ogImageUrl) payload.og_image_url = ogImageUrl;

	const canonicalUrl = getOptionalString(frontmatter, 'canonical_url');
	if (canonicalUrl) payload.canonical_url = canonicalUrl;

	if (fieldValues) payload.field_values = fieldValues;

	const headerImageId = getOptionalString(frontmatter, 'header_image_id');
	if (headerImageId) payload.header_image_id = headerImageId;

	const featuredImageId = getOptionalString(frontmatter, 'featured_image_id');
	if (featuredImageId) payload.featured_image_id = featuredImageId;

	return payload;
}

export function shouldAutoPublish(settings: StudioSettings, contentType: string): boolean {
	if (!settings.autoSync) return false;
	if (settings.autoSyncContentTypes.length === 0) return true;
	return settings.autoSyncContentTypes.includes(contentType);
}

export async function publishContent(input: PublishInput): Promise<PublishResult> {
	await ensureSettingsLoaded();
	const settings = loadSettings();
	if (!settings.baseUrl || !settings.apiKey) {
		return { ok: false, error: 'API settings are not configured.' };
	}

	const status = determineStatus(input.frontmatter);

	const inferredType = inferredContentType(input.relativePath);
	const declaredType = getString(input.frontmatter, 'content_type');
	if (inferredType && declaredType && inferredType !== declaredType) {
		return {
			ok: false,
			error: `Content type mismatch: file is under jc-content/${inferredType}/ but frontmatter says "${declaredType}".`
		};
	}

	const validationErrors = validateFrontmatter(input.frontmatter, status, settings.mode);
	const bodyError = validateBody(input.body, status, settings.contentMinLength ?? 10);
	if (Object.keys(validationErrors).length > 0 || bodyError) {
		const messages = [...Object.values(validationErrors), bodyError].filter(Boolean).join(' ');
		return { ok: false, error: messages };
	}

	const client = createApiClient(settings.baseUrl);
	const existing = queueStore.files.find((f) => f.relativePath === input.relativePath);
	const contentId = existing?.contentId;

	const fileMeta = {
		contentType: getString(input.frontmatter, 'content_type') || inferredType || 'guide',
		slug: getString(input.frontmatter, 'slug'),
		title: getString(input.frontmatter, 'title') || 'Untitled',
		spaceId: getOptionalString(input.frontmatter, 'space_id'),
		personaId: getString(input.frontmatter, 'persona_id'),
		guideCategory: getOptionalString(input.frontmatter, 'guide_category'),
		status,
		visibility: getString(input.frontmatter, 'visibility') || 'public',
		modifiedAt: input.modifiedAt ?? existing?.modifiedAt
	};

	const payload = buildPayload(input.frontmatter, input.body, status);

	let record: ContentRecord | undefined;
	const isUpdate = !!contentId;
	if (contentId) {
		const res = await withRetry('update content', () => client.updateContent(contentId, payload));
		if (!res.ok) {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				contentId,
				error: res.error?.message ?? `Update failed (${res.error?.code ?? 'unknown'})`
			});
			await queueStore.upsertFile(input.relativePath, {
				...fileMeta,
				lastError: res.error?.message ?? 'Update failed.',
				syncState: 'failed'
			});
			return { ok: false, error: res.error?.message ?? `Update failed (${res.error?.code ?? 'unknown'})` };
		}
		record = res.data;
	} else {
		const res = await withRetry('create content', () => client.createContent(payload));
		if (!res.ok) {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				error: res.error?.message ?? `Create failed (${res.error?.code ?? 'unknown'})`
			});
			await queueStore.upsertFile(input.relativePath, {
				...fileMeta,
				lastError: res.error?.message ?? 'Create failed.',
				syncState: 'failed'
			});
			return { ok: false, error: res.error?.message ?? `Create failed (${res.error?.code ?? 'unknown'})` };
		}
		record = res.data;
	}

	if (!record) {
		await syncLogStore.append({
			relativePath: input.relativePath,
			action: 'failed',
			error: 'API returned no content record.'
		});
		await queueStore.upsertFile(input.relativePath, {
			...fileMeta,
			lastError: 'API returned no content record.',
			syncState: 'failed'
		});
		return { ok: false, error: 'API returned no content record.' };
	}

	const targetId = record.id;

	// Reassign authorship to the persona's backing user when a persona is selected.
	// Member mode posts as the authenticated account, so this is skipped.
	const personaId = settings.mode !== 'member' ? getString(input.frontmatter, 'persona_id') : '';
	if (personaId) {
		const personas = await fetchPersonas(settings);
		const newAuthorId = resolvePersonaUserId(personas, personaId);
		if (!newAuthorId) {
			const msg = `Could not resolve persona "${personaId}" to a JapaConnect user.`;
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				contentId: targetId,
				error: msg
			});
			await queueStore.upsertFile(input.relativePath, {
				...fileMeta,
				contentId: targetId,
				lastError: msg,
				syncState: 'failed'
			});
			return { ok: false, error: msg };
		}
		const reassignRes = await withRetry('reassign author', () =>
			client.reassignAuthor(targetId, {
				new_author_id: newAuthorId,
				reason: 'Japa Content Studio persona authorship'
			})
		);
		if (reassignRes.ok) {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'reassign',
				contentId: targetId
			});
		} else {
			const msg = `Authorship reassignment failed: ${reassignRes.error?.message ?? reassignRes.error?.code ?? 'unknown'}`;
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				contentId: targetId,
				error: msg
			});
			await queueStore.upsertFile(input.relativePath, {
				...fileMeta,
				contentId: targetId,
				lastError: msg,
				syncState: 'failed'
			});
			return { ok: false, error: msg };
		}
		await sleep(250);
	}

	// Apply tags from frontmatter, diffing against existing tags on update.
	const tags = getTags(input.frontmatter);
	if (isUpdate) {
		const listRes = await withRetry('list content tags', () => client.getContentTags(targetId));
		if (listRes.ok && listRes.data) {
			const existingTags = new Set(listRes.data.map((t) => t.slug));
			const desiredTags = new Set(tags);
			const toAdd = tags.filter((t) => !existingTags.has(t));
			const toRemove = listRes.data.filter((t) => !desiredTags.has(t.slug));
			if (toAdd.length > 0) {
				const addRes = await withRetry('add content tags', () => client.addContentTags(targetId, { tag_slugs: toAdd }));
				if (addRes.ok) {
					await syncLogStore.append({
						relativePath: input.relativePath,
						action: 'tags',
						contentId: targetId
					});
				} else {
					await syncLogStore.append({
						relativePath: input.relativePath,
						action: 'failed',
						contentId: targetId,
						error: `Tag add failed: ${addRes.error?.message ?? addRes.error?.code ?? 'unknown'}`
					});
				}
			}
			for (const tag of toRemove) {
				const delRes = await withRetry('delete content tag', () => client.deleteContentTag(targetId, tag.slug));
				if (!delRes.ok) {
					await syncLogStore.append({
						relativePath: input.relativePath,
						action: 'failed',
						contentId: targetId,
						error: `Tag remove failed: ${delRes.error?.message ?? delRes.error?.code ?? 'unknown'}`
					});
				}
			}
		}
	} else if (tags.length > 0) {
		const tagsRes = await withRetry('add content tags', () => client.addContentTags(targetId, { tag_slugs: tags }));
		if (tagsRes.ok) {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'tags',
				contentId: targetId
			});
		} else {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				contentId: targetId,
				error: `Tag add failed: ${tagsRes.error?.message ?? tagsRes.error?.code ?? 'unknown'}`
			});
		}
	}

	// If the user explicitly clicked publish and the resulting status is
	// review, we do NOT call /publish; the content is held for moderation.
	if (status === 'published' && record.status !== 'published') {
		const publishRes = await withRetry('publish content', () => client.publishContent(targetId));
		if (publishRes.ok && publishRes.data) {
			record = publishRes.data;
		} else {
			await syncLogStore.append({
				relativePath: input.relativePath,
				action: 'failed',
				contentId: targetId,
				error: `Publish call failed: ${publishRes.error?.message ?? publishRes.error?.code ?? 'unknown'}`
			});
		}
	}

	// Recompute hash from the canonical raw Markdown so later edits can be detected.
	const raw = input.raw ?? '';
	const newHash = raw ? await hashFile(raw) : '';
	const syncState =
		record.status === 'published'
			? 'published'
			: record.status === 'in_review'
				? 'in_review'
				: record.status === 'archived'
					? 'archived'
					: 'modified';

	await queueStore.upsertFile(input.relativePath, {
		...fileMeta,
		contentId: targetId,
		lastHash: newHash,
		lastPublishedHash: newHash,
		lastPublishedAt: new Date().toISOString(),
		syncState,
		lastError: ''
	});

	await syncLogStore.append({
		relativePath: input.relativePath,
		action: isUpdate ? 'update' : 'create',
		contentId: targetId
	});

	return {
		ok: true,
		contentId: targetId,
		slug: record.slug,
		status: record.status
	};
}

// --- Publish worker queue ---
// Serialises manual single/bulk publishes and watcher auto-publishes so that
// only one file is processed at a time and brief pauses separate files.
// Ref: sections/105-japa-content-studio-desktop.md (Concurrency and Throttling)

interface PublishJob {
	relativePath: string;
	source: 'manual' | 'auto';
	projectRoot?: string;
}

const publishQueue: PublishJob[] = [];
let workerRunning = false;
let currentJob: PublishJob | null = null;

export const isPublishing = writable(false);

export const publishWorker = {
	enqueue(relativePaths: string | string[], source: 'manual' | 'auto' = 'manual', projectRoot?: string) {
		const paths = Array.isArray(relativePaths) ? relativePaths : [relativePaths];
		for (const relativePath of paths) {
			publishQueue.push({ relativePath, source, projectRoot });
		}
		ensureWorkerRunning();
	},
	get current() {
		return currentJob;
	},
	get pending() {
		return publishQueue.length;
	}
};

function ensureWorkerRunning() {
	if (workerRunning) return;
	workerRunning = true;
	isPublishing.set(true);
	void processPublishQueue();
}

async function processPublishQueue() {
	while (publishQueue.length > 0) {
		currentJob = publishQueue.shift()!;
		try {
			await publishJob(currentJob);
		} catch (e) {
			const msg = e instanceof Error ? e.message : String(e);
			await syncLogStore.append({
				relativePath: currentJob.relativePath,
				action: 'failed',
				error: msg
			});
			await queueStore.upsertFile(currentJob.relativePath, {
				syncState: 'failed',
				lastError: msg,
				localStatus: 'failed'
			});
		}
		currentJob = null;
		if (publishQueue.length > 0) {
			await sleep(BATCH_DELAY_MS);
		}
	}
	workerRunning = false;
	isPublishing.set(false);
}

async function publishJob(job: PublishJob) {
	await ensureSettingsLoaded();
	const settings = loadSettings();
	const projectRoot = job.projectRoot || settings.projectRoot;
	if (!projectRoot) throw new Error('Project root not set.');

	const filePath = await join(projectRoot, job.relativePath);
	const raw = await readTextFile(filePath);
	const { frontmatter, body } = parseFrontmatter(raw);

	if (job.source === 'auto') {
		const row = queueStore.files.find((f) => f.relativePath === job.relativePath);
		if (!row) return;
		if (row.syncState !== 'new' && row.syncState !== 'modified') return;
		if (row.lastError) return;
		if (!shouldAutoPublish(settings, row.contentType)) return;
	}

	let modifiedAt: string | undefined;
	try {
		const info = await stat(filePath);
		if (info.mtime) {
			modifiedAt = info.mtime instanceof Date ? info.mtime.toISOString() : String(info.mtime);
		}
	} catch {
		// best-effort
	}

	const result = await publishContent({
		relativePath: job.relativePath,
		frontmatter,
		body,
		raw,
		modifiedAt
	});
	if (!result.ok) {
		throw new Error(result.error || 'Publish failed.');
	}
}
