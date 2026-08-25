/**
 * Parse a list of batch topics and build a single Claude prompt that
 * creates multiple jc-content markdown files.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { slugify } from '$lib/services/prompt';
import type { BatchTopic } from '$lib/types/batch';
import type { StudioSettings } from '$lib/services/settings';
import type { PromptLookups } from '$lib/services/prompt';

export { type BatchTopic };

function parseCSV(text: string): BatchTopic[] {
	const lines = text
		.split(/\r?\n/)
		.map((l) => l.trim())
		.filter((l) => l.length > 0);
	if (lines.length === 0) return [];

	const first = lines[0].toLowerCase();
	const hasHeader = first.includes('title');
	const start = hasHeader ? 1 : 0;

	const parseLine = (line: string): BatchTopic => {
		const cols = line.split(',').map((c) => c.trim());
		if (hasHeader) {
			const header = lines[0].split(',').map((c) => c.trim().toLowerCase());
			const title = cols[header.indexOf('title')] ?? '';
			const contentType = cols[header.indexOf('content_type')] ?? '';
			const guideCategory = cols[header.indexOf('guide_category')] ?? '';
			return { title, content_type: contentType, guide_category: guideCategory };
		}
		return {
			title: cols[0] ?? '',
			content_type: cols[1] ?? '',
			guide_category: cols[2] ?? ''
		};
	};

	return lines
		.slice(start)
		.map(parseLine)
		.filter((t) => t.title.length > 0);
}

function parseJSON(text: string): BatchTopic[] | null {
	try {
		const parsed = JSON.parse(text);
		if (!Array.isArray(parsed)) return null;
		return parsed
			.filter((t) => t && typeof t === 'object')
			.map((t) => ({
				title: String(t.title ?? ''),
				content_type: t.content_type ? String(t.content_type) : '',
				guide_category: t.guide_category ? String(t.guide_category) : ''
			}))
			.filter((t) => t.title.length > 0);
	} catch {
		return null;
	}
}

export function parseBatchTopics(text: string): BatchTopic[] {
	const fromJSON = parseJSON(text);
	if (fromJSON) return fromJSON;
	return parseCSV(text);
}

function yamlMultiline(value: string): string {
	if (!value) return '';
	return `"${value.replace(/"/g, '\\"')}"`;
}

function frontmatterLine(key: string, value: unknown): string {
	if (value === '' || value === null || value === undefined) return '';
	if (typeof value === 'boolean') return `${key}: ${value}`;
	if (typeof value === 'number') return `${key}: ${value}`;
	if (typeof value === 'string') return `${key}: ${yamlMultiline(value)}`;
	return '';
}

function topicFrontmatter(
	topic: BatchTopic,
	settings: StudioSettings,
	lookups: PromptLookups
): string {
	const type = topic.content_type || settings.default_content_type || 'guide';
	const slug = slugify(topic.title);
	const tags = settings.default_tags?.length ? settings.default_tags : [];

	const lines = [
		'---',
		frontmatterLine('title', topic.title),
		frontmatterLine('slug', slug),
		frontmatterLine('content_type', type)
	];
	if (topic.guide_category) {
		lines.push(frontmatterLine('guide_category', topic.guide_category));
	}
	if (settings.default_persona_id) {
		lines.push(frontmatterLine('persona_id', settings.default_persona_id));
	}
	if (settings.default_voice_id) {
		lines.push(frontmatterLine('voice_id', settings.default_voice_id));
	}
	if (settings.default_space_id) {
		lines.push(frontmatterLine('space_id', settings.default_space_id));
	}
	lines.push(
		frontmatterLine('status', settings.default_status || 'draft'),
		frontmatterLine('visibility', settings.default_visibility || 'public')
	);
	if (tags.length) {
		lines.push('tags:');
		for (const tag of tags) {
			lines.push(`  - ${yamlMultiline(tag)}`);
		}
	} else {
		lines.push('tags: []');
	}
	lines.push('field_values:', '  estimated_read_time:');
	lines.push('---');
	return lines.join('\n');
}

export function buildBatchPrompt(
	topics: BatchTopic[],
	settings: StudioSettings,
	lookups: PromptLookups
): string {
	const defaultType = settings.default_content_type || 'guide';

	const fileList = topics
		.map((t, i) => {
			const type = t.content_type || defaultType;
			const slug = slugify(t.title);
			const filePath = `jc-content/${type}/${slug}.md`;
			const fm = topicFrontmatter(t, settings, lookups);
			return `## ${i + 1}. ${t.title}

Save to: ${filePath}

Required frontmatter:

\`\`\`yaml
${fm}
\`\`\``;
		})
		.join('\n\n');

	return `You are writing as the JapaConnect persona "${lookups.personaName || 'a JapaConnect author'}".

Create ${topics.length} markdown article(s) under \`jc-content/[content-type]/[slug].md\`. Use the exact frontmatter and file path provided for each topic below.

Use Markdown. H1 for the title, H2 for each major section. Include 3-5 H2 sections. End with key takeaways. Include 1-2 internal links as \`[text](/content/[slug])\` and 1-2 real, authoritative external URLs where relevant. Do not invent internal slugs; omit the link if you do not know a real one.

Tone: professional
Be specific, practical, and human. Do not sound generic or AI-like.

${fileList}
`;
}
