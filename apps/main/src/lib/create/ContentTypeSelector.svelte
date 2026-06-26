<script lang="ts">
	import {
		FileText,
		MessageSquare,
		BookOpen,
		Calendar,
		HelpCircle,
		Users,
		Compass,
		ClipboardList,
		GraduationCap,
		Briefcase,
		Heart,
		Scale,
		Home,
		CreditCard,
		Star,
		type Icon as IconType,
	} from 'lucide-svelte';
	import { ArrowRight } from 'lucide-svelte';

	interface ContentType {
		slug: string;
		icon: string;
		label: string;
		description: string;
	}

	let { types = [] }: { types?: ContentType[] } = $props();

	const ICON_MAP: Record<string, typeof IconType> = {
		'file-text': FileText,
		'message-square': MessageSquare,
		'book-open': BookOpen,
		calendar: Calendar,
		'help-circle': HelpCircle,
		users: Users,
		compass: Compass,
		'clipboard-list': ClipboardList,
		'graduation-cap': GraduationCap,
		briefcase: Briefcase,
		heart: Heart,
		scale: Scale,
		home: Home,
		'credit-card': CreditCard,
		star: Star,
	};

	function getIcon(iconName: string): typeof IconType {
		return ICON_MAP[iconName] ?? FileText;
	}
</script>

<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
	{#each types as type}
		{@const Icon = getIcon(type.icon)}
		<a
			href="/create/{type.slug}"
			class="group relative flex flex-col rounded-xl border border-gray-200 bg-white p-5 shadow-sm transition-all duration-200 hover:border-blue-300 hover:shadow-md dark:border-gray-700 dark:bg-gray-900 dark:hover:border-blue-600"
		>
			<div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-950 dark:text-blue-400">
				<Icon class="h-5 w-5" />
			</div>

			<h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-gray-100">
				{type.label}
			</h3>

			<p class="mb-4 flex-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
				{type.description}
			</p>

			<span class="inline-flex items-center gap-1 text-xs font-medium text-blue-600 transition-colors group-hover:text-blue-700 dark:text-blue-400 dark:group-hover:text-blue-300">
				Create
				<ArrowRight class="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
			</span>
		</a>
	{/each}
</div>
