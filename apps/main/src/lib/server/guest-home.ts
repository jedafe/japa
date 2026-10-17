import type { ServerLoadEvent } from '@sveltejs/kit';
import { apiList, apiContext } from '$lib/server/api';
import { DEFAULT_SITE_NAME } from '$lib/brand';
import type {
	GuestHomeData,
	GuestHomeOpportunity,
	GuestHomeStory,
	GuestHomeTool,
	GuestHomeStat,
	GuestHomeCategory,
	SpaceSuggestion,
	Story
} from '$lib/types';

export const emptyGuestHome: GuestHomeData = {
	topDestinations: [],
	opportunities: [],
	stories: [],
	tools: [],
	stats: [],
	categories: []
};

function asArray<T>(value: unknown): T[] {
	if (Array.isArray(value)) return value as T[];
	if (value && typeof value === 'object' && !Array.isArray(value)) {
		const maybe = (value as Record<string, unknown>).data;
		if (Array.isArray(maybe)) return maybe as T[];
	}
	return [];
}

function mapStoryToGuestHome(story: Story, siteName = DEFAULT_SITE_NAME): GuestHomeStory {
	const quote =
		story.best_advice?.trim() ||
		story.most_useful_resource?.trim() ||
		(story.body ? story.body.replace(/<[^>]+>/g, '').slice(0, 160) : story.title);
	const role = [story.profession, story.destination_space_name].filter(Boolean).join(' • ');

	return {
		id: story.id,
		slug: story.slug,
		title: story.title,
		quote,
		author_name:
			story.author_display_name ??
			story.author_name ??
			`A ${siteName} Member`,
		author_role: role || undefined,
		author_avatar_url: story.author_avatar ?? story.featured_image_url ?? null,
		destination: story.destination_space_name
	};
}

/**
 * Build the guest-home context data consumed by `guest_*` landing blocks in
 * context mode (stats, categories, opportunities, tools, destinations,
 * stories). Sourced from public settings and live API data (Rule 16).
 * Shared by the home route and published-page loader so standalone guest
 * blocks get context data on any page, not just `guest_home`.
 */
export async function buildGuestHome(
	ctx: ReturnType<typeof apiContext>,
	event: ServerLoadEvent
): Promise<GuestHomeData> {
	const parent = await event.parent();
	const siteSettings = (parent?.siteSettings as Record<string, unknown> | undefined) ?? {};

	const stats = asArray<GuestHomeStat>(siteSettings['landing.stats']);
	const categories = asArray<GuestHomeCategory>(siteSettings['landing.categories']);
	const opportunities = asArray<GuestHomeOpportunity>(siteSettings['landing.featured_opportunities']);
	const tools = asArray<GuestHomeTool>(siteSettings['landing.tools']);
	let topDestinations = asArray<SpaceSuggestion>(siteSettings['landing.top_destinations']);

	// If no curated top destinations are configured, try the directory API.
	if (topDestinations.length === 0) {
		try {
			const spaceRes = await apiList<SpaceSuggestion>(ctx, `/spaces?level=country&sort=members&page_size=4`);
			topDestinations = spaceRes.items.slice(0, 4);
		} catch (err) {
			console.error('[HOME] top destinations load failed', err);
		}
	}

	let stories: GuestHomeStory[] = [];
	try {
		const storyRes = await apiList<Story>(ctx, '/stories?limit=10&sort=top');
		const site = (siteSettings['general.site_name'] as string | undefined) ?? DEFAULT_SITE_NAME;
		stories = storyRes.items.slice(0, 10).map((s) => mapStoryToGuestHome(s, site));
	} catch (err) {
		console.error('[HOME] stories load failed', err);
	}

	return {
		topDestinations,
		opportunities,
		stories,
		tools,
		stats,
		categories
	};
}
