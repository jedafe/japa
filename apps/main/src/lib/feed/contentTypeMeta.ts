import {
	FileText,
	Compass,
	BookOpen,
	MessagesSquare,
	Play,
	Scale,
	Calendar,
	Briefcase,
	Wrench,
	Trophy,
	MessageCircle
} from 'lucide-svelte';
import { SvelteComponent } from 'svelte';
import { t } from '$lib/i18n';

/**
 * Content-type identity for the `daily` feed card. Each type gets a Lucide
 * icon and a CSS variable accent role so blog/guide/video/story/etc. are
 * scannable at a glance without forking the card layout.
 */
export interface ContentTypeMeta {
	icon: typeof SvelteComponent<any>;
	/** CSS custom property used for the accent color. */
	accentVar: string;
	/** i18n key for the action label: t(key, fallback). */
	readKey: string;
	/** Plain-English fallback label. */
	readFallback: string;
}

const DEFAULT_META: ContentTypeMeta = {
	icon: FileText,
	accentVar: '--color-primary',
	readKey: 'feed.read_post',
	readFallback: 'Read post'
};

const META: Record<string, ContentTypeMeta> = {
	blog_post: {
		icon: FileText,
		accentVar: '--color-content-blog',
		readKey: 'feed.read_blog',
		readFallback: 'Read blog'
	},
	guide: {
		icon: Compass,
		accentVar: '--color-content-guide',
		readKey: 'feed.read_guide',
		readFallback: 'Read guide'
	},
	story: {
		icon: BookOpen,
		accentVar: '--color-content-story',
		readKey: 'feed.read_story',
		readFallback: 'Read story'
	},
	discussion: {
		icon: MessagesSquare,
		accentVar: '--color-content-discussion',
		readKey: 'feed.view_discussion',
		readFallback: 'View discussion'
	},
	forum_thread: {
		icon: MessageCircle,
		accentVar: '--color-content-discussion',
		readKey: 'feed.view_discussion',
		readFallback: 'View discussion'
	},
	video: {
		icon: Play,
		accentVar: '--color-content-video',
		readKey: 'feed.watch_video',
		readFallback: 'Watch video'
	},
	comparison: {
		icon: Scale,
		accentVar: '--color-content-comparison',
		readKey: 'feed.view_comparison',
		readFallback: 'View comparison'
	},
	event: {
		icon: Calendar,
		accentVar: '--color-content-event',
		readKey: 'feed.view_event',
		readFallback: 'View event'
	},
	job_listing: {
		icon: Briefcase,
		accentVar: '--color-content-job',
		readKey: 'feed.view_job',
		readFallback: 'View job'
	},
	service_listing: {
		icon: Wrench,
		accentVar: '--color-content-service',
		readKey: 'feed.view_service',
		readFallback: 'View service'
	},
	achievement: {
		icon: Trophy,
		accentVar: '--color-content-achievement',
		readKey: 'feed.view_achievement',
		readFallback: 'View achievement'
	},
	question: {
		icon: MessagesSquare,
		accentVar: '--color-content-discussion',
		readKey: 'feed.view_question',
		readFallback: 'View question'
	}
};

export function contentTypeMeta(contentType: string): ContentTypeMeta {
	return META[contentType] ?? DEFAULT_META;
}

export function readPostLabel(contentType: string): string {
	const meta = contentTypeMeta(contentType);
	return t(meta.readKey, meta.readFallback);
}
