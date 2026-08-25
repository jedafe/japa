/**
 * Publish or synchronise a Markdown file to JapaConnect.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createApiClient } from '$lib/services/api';
import { loadSettings, type StudioSettings } from '$lib/services/settings';
import { markdownToHtml } from '$lib/services/markdown';
import { hashFile } from '$lib/services/hash';
import { queueStore } from '$lib/stores/queue.svelte';
import type { Persona, ContentRecord } from '$lib/types/prompt';

const HIGH_STAKES_CATEGORIES = new Set([
	'visa',
	'remittance',
	'banking',
	'grants',
	'health_insurance',
	'admission'
]);

export interface PublishInput {
	relativePath: string;
	frontmatter: Record<string, unknown>;
	body: string;
	raw?: string;
	/**
	 * If true, ignore the high-stakes gate and send the requested status.
	 * The backend will still hold it for moderation if spam is flagged.
	 */
	force?: boolean;
}

export interface PublishResult {
	ok: boolean;
	contentId?: string;
	slug?: string;
	status?: string;
	error?: string;
}

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

function normalisePublishAt(value: string): string | undefined {
	if (!value) return undefined;
	// datetime-local values lack seconds/zone; normalise to RFC3339-ish.
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
		return `${value}:00Z`;
	}
	return value;
}

function isHighStakes(frontmatter: Record<string, unknown>): boolean {
	const category = getString(frontmatter, 'guide_category').toLowerCase();
	if (HIGH_STAKES_CATEGORIES.has(category)) return true;
	for (const tag of getTags(frontmatter)) {
		if (tag.toLowerCase() === 'immigration-advice' || tag.toLowerCase() === 'financial-advice') {
			return true;
		}
	}
	return false;
}

function determineStatus(
	frontmatter: Record<string, unknown>,
	forPublish: boolean,
	force: boolean
): string {
	const requested = getString(frontmatter, 'status') || (forPublish ? 'published' : 'draft');
	if (force) return requested;
	if (requested === 'scheduled' && getString(frontmatter, 'publish_at')) {
		return 'scheduled';
	}
	if (isHighStakes(frontmatter)) {
		return 'in_review';
	}
	return requested;
}

async function fetchPersonas(settings: StudioSettings): Promise<Persona[]> {
	const client = createApiClient(settings.baseUrl, settings.apiKey);
	const res = await client.getPersonas();
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

	const journeyStage = getOptionalString(frontmatter, 'journey_stage');
	if (journeyStage) payload.journey_stage = journeyStage;

	const visaRoute = getOptionalString(frontmatter, 'visa_route');
	if (visaRoute) payload.visa_route = visaRoute;

	const sourceCountry = getOptionalString(frontmatter, 'source_country');
	if (sourceCountry) payload.source_country = sourceCountry;

	const targetCountry = getOptionalString(frontmatter, 'target_country');
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

	return payload;
}

export async function publishContent(input: PublishInput): Promise<PublishResult> {
	const settings = loadSettings();
	if (!settings.baseUrl || !settings.apiKey) {
		return { ok: false, error: 'API settings are not configured.' };
	}

	const status = determineStatus(input.frontmatter, true, input.force ?? false);

	if ((status === 'published' || status === 'scheduled') && !getString(input.frontmatter, 'space_id')) {
		return { ok: false, error: 'A primary space is required to publish or schedule content.' };
	}

	const client = createApiClient(settings.baseUrl, settings.apiKey);
	const existing = queueStore.files.find((f) => f.relativePath === input.relativePath);
	const contentId = existing?.contentId;

	const payload = buildPayload(input.frontmatter, input.body, status);

	let record: ContentRecord | undefined;
	if (contentId) {
		const res = await client.updateContent(contentId, payload);
		if (!res.ok) {
			return { ok: false, error: res.error?.message ?? `Update failed (${res.error?.code ?? 'unknown'})` };
		}
		record = res.data;
	} else {
		const res = await client.createContent(payload);
		if (!res.ok) {
			return { ok: false, error: res.error?.message ?? `Create failed (${res.error?.code ?? 'unknown'})` };
		}
		record = res.data;
	}

	if (!record) {
		return { ok: false, error: 'API returned no content record.' };
	}

	const targetId = record.id;

	// Reassign authorship to the persona's backing user when a persona is selected.
	const personaId = getString(input.frontmatter, 'persona_id');
	if (personaId) {
		const personas = await fetchPersonas(settings);
		const newAuthorId = resolvePersonaUserId(personas, personaId);
		if (newAuthorId) {
			const reassignRes = await client.reassignAuthor(targetId, {
				new_author_id: newAuthorId,
				reason: 'Japa Content Studio persona authorship'
			});
			if (!reassignRes.ok) {
				// Non-fatal: the content exists; surface the warning but continue.
				console.warn('Authorship reassignment failed:', reassignRes.error);
			}
		}
	}

	// Apply tags from frontmatter.
	const tags = getTags(input.frontmatter);
	if (tags.length > 0) {
		const tagsRes = await client.addContentTags(targetId, { tag_slugs: tags });
		if (!tagsRes.ok) {
			console.warn('Tag assignment failed:', tagsRes.error);
		}
	}

	// If the user explicitly clicked publish and the resulting status is
	// review, we do NOT call /publish; the content is held for moderation.
	if (status === 'published' && record.status !== 'published') {
		const publishRes = await client.publishContent(targetId);
		if (!publishRes.ok) {
			console.warn('Explicit publish call failed:', publishRes.error);
		} else if (publishRes.data) {
			record = publishRes.data;
		}
	}

	// Recompute hash from the canonical raw Markdown so later edits can be detected.
	const raw = input.raw ?? '';
	const newHash = raw ? await hashFile(raw) : '';
	const syncState = record.status === 'published' ? 'published' : record.status === 'in_review' ? 'in_review' : 'modified';

	queueStore.updateFile(input.relativePath, {
		contentId: targetId,
		lastPublishedHash: newHash,
		lastPublishedAt: new Date().toISOString(),
		syncState,
		lastError: ''
	});

	return {
		ok: true,
		contentId: targetId,
		slug: record.slug,
		status: record.status
	};
}
