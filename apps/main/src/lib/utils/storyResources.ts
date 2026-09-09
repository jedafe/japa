/**
 * Parse the joined `most_useful_resource` text stored in `story_meta` into a
 * list of display-ready title/URL pairs.
 *
 * The backend joins repeater items as `Name — Link; …` (§79). Items may also
 * be plain URLs or plain names if authored through a non-repeater path.
 *
 * Ref: japaconnect-engineering/sections/79-community-story-wall.md
 */

export interface StoryResourceLink {
	name: string;
	url: string;
}

const ITEM_SEPARATOR = '; ';
const NAME_LINK_SEPARATOR = ' — ';

function looksLikeUrl(value: string): boolean {
	return /^[a-z][a-z0-9+.-]*:/i.test(value) || value.startsWith('/') || value.startsWith('//');
}

function looksLikeBareHost(value: string): boolean {
	if (/\s/.test(value)) return false;
	if (/[/.]/.test(value) && !/[<>"{}|\\^`\[\]]/.test(value)) return true;
	if (/^[^\s/]+:\d+/.test(value)) return true;
	return false;
}

function normalizeResourceUrl(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return '';
	if (looksLikeUrl(trimmed)) return trimmed;
	// Bare domains such as "example.com" or "localhost:3000" should resolve
	// to a web URL; anything with spaces is treated as a name without a link.
	if (looksLikeBareHost(trimmed)) return `https://${trimmed}`;
	return '';
}

export function parseMostUsefulResource(raw: string | null | undefined): StoryResourceLink[] {
	if (!raw) return [];

	return raw
		.split(ITEM_SEPARATOR)
		.map((part) => part.trim())
		.filter(Boolean)
		.map((part) => {
			const sepIdx = part.lastIndexOf(NAME_LINK_SEPARATOR);

			if (sepIdx !== -1) {
				const name = part.slice(0, sepIdx).trim();
				const url = part.slice(sepIdx + NAME_LINK_SEPARATOR.length).trim();
				return {
					name: name || url,
					url: normalizeResourceUrl(url)
				};
			}

			const normalized = normalizeResourceUrl(part);
			if (normalized) {
				return { name: part, url: normalized };
			}

			return { name: part, url: '' };
		});
}
