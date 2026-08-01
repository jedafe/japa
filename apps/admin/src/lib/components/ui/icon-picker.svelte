<script lang="ts">
	import {
		Home, Compass, Search, Bell, Bookmark, MessageSquare, Mail,
		Users, User, Settings, LogIn, LogOut, Newspaper, BookOpen,
		GraduationCap, Hash, Layers, Scale, Star, Heart, ThumbsUp,
		ThumbsDown, Eye, EyeOff, Lock, Unlock, Shield, ShieldCheck,
		Globe, MapPin, FileText, Folder, FolderOpen, Image, Camera,
		Video, Music, Play, Pause, Download, Upload, Trash2, Edit,
		Plus, Minus, Check, X, ChevronRight, ChevronLeft, ChevronDown,
		ChevronUp, ArrowRight, ArrowLeft, ArrowUp, ArrowDown, Menu,
		MoreHorizontal, MoreVertical, Filter, SortAsc, SortDesc, Grid,
		List, Calendar, Clock, Tag, Flag, Award, Gift, CreditCard,
		Wallet, ShoppingBag, ShoppingCart, Package, Truck, Building,
		Briefcase, Landmark, BookMarked, Library, FileCode, Terminal,
		Cpu, Database, Server, Cloud, Wifi, HardDrive, Monitor, Smartphone,
		Tablet, Watch, Headphones, Mic, Phone, Save,
		RefreshCw, Repeat, RotateCw, Undo, Redo, Copy, Clipboard, Link,
		Unlink, ExternalLink, Share, Share2, Send, Reply, Forward,
		Info, HelpCircle, AlertCircle, AlertTriangle, Ban,
		CheckCircle, XCircle, PlusCircle, MinusCircle, Dot, Circle,
		CircleDot, Square, Triangle, Diamond, Hexagon, Octagon, Cylinder,
	} from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		value = $bindable(''),
		onchange = (_v: string) => {},
	}: {
		value?: string;
		onchange?: (v: string) => void;
	} = $props();

	// Curated list of icons useful for navigation menus.
	// Keys are kebab-case names matching the backend menu_items.icon column.
	const icons: { name: string; component: typeof Home }[] = [
		{ name: 'home', component: Home },
		{ name: 'compass', component: Compass },
		{ name: 'search', component: Search },
		{ name: 'bell', component: Bell },
		{ name: 'bookmark', component: Bookmark },
		{ name: 'message-square', component: MessageSquare },
		{ name: 'mail', component: Mail },
		{ name: 'users', component: Users },
		{ name: 'user', component: User },
		{ name: 'settings', component: Settings },
		{ name: 'log-in', component: LogIn },
		{ name: 'log-out', component: LogOut },
		{ name: 'newspaper', component: Newspaper },
		{ name: 'book-open', component: BookOpen },
		{ name: 'graduation-cap', component: GraduationCap },
		{ name: 'hash', component: Hash },
		{ name: 'layers', component: Layers },
		{ name: 'scale', component: Scale },
		{ name: 'star', component: Star },
		{ name: 'heart', component: Heart },
		{ name: 'thumbs-up', component: ThumbsUp },
		{ name: 'thumbs-down', component: ThumbsDown },
		{ name: 'eye', component: Eye },
		{ name: 'eye-off', component: EyeOff },
		{ name: 'lock', component: Lock },
		{ name: 'unlock', component: Unlock },
		{ name: 'shield', component: Shield },
		{ name: 'shield-check', component: ShieldCheck },
		{ name: 'globe', component: Globe },
		{ name: 'map-pin', component: MapPin },
		{ name: 'file-text', component: FileText },
		{ name: 'folder', component: Folder },
		{ name: 'folder-open', component: FolderOpen },
		{ name: 'image', component: Image },
		{ name: 'camera', component: Camera },
		{ name: 'video', component: Video },
		{ name: 'music', component: Music },
		{ name: 'play', component: Play },
		{ name: 'pause', component: Pause },
		{ name: 'download', component: Download },
		{ name: 'upload', component: Upload },
		{ name: 'trash', component: Trash2 },
		{ name: 'edit', component: Edit },
		{ name: 'plus', component: Plus },
		{ name: 'minus', component: Minus },
		{ name: 'check', component: Check },
		{ name: 'x', component: X },
		{ name: 'chevron-right', component: ChevronRight },
		{ name: 'chevron-left', component: ChevronLeft },
		{ name: 'chevron-down', component: ChevronDown },
		{ name: 'chevron-up', component: ChevronUp },
		{ name: 'arrow-right', component: ArrowRight },
		{ name: 'arrow-left', component: ArrowLeft },
		{ name: 'arrow-up', component: ArrowUp },
		{ name: 'arrow-down', component: ArrowDown },
		{ name: 'menu', component: Menu },
		{ name: 'more-horizontal', component: MoreHorizontal },
		{ name: 'more-vertical', component: MoreVertical },
		{ name: 'filter', component: Filter },
		{ name: 'grid', component: Grid },
		{ name: 'list', component: List },
		{ name: 'calendar', component: Calendar },
		{ name: 'clock', component: Clock },
		{ name: 'tag', component: Tag },
		{ name: 'flag', component: Flag },
		{ name: 'award', component: Award },
		{ name: 'gift', component: Gift },
		{ name: 'credit-card', component: CreditCard },
		{ name: 'wallet', component: Wallet },
		{ name: 'shopping-bag', component: ShoppingBag },
		{ name: 'shopping-cart', component: ShoppingCart },
		{ name: 'package', component: Package },
		{ name: 'building', component: Building },
		{ name: 'briefcase', component: Briefcase },
		{ name: 'landmark', component: Landmark },
		{ name: 'book-marked', component: BookMarked },
		{ name: 'library', component: Library },
		{ name: 'file-code', component: FileCode },
		{ name: 'terminal', component: Terminal },
		{ name: 'cpu', component: Cpu },
		{ name: 'database', component: Database },
		{ name: 'server', component: Server },
		{ name: 'cloud', component: Cloud },
		{ name: 'wifi', component: Wifi },
		{ name: 'hard-drive', component: HardDrive },
		{ name: 'monitor', component: Monitor },
		{ name: 'smartphone', component: Smartphone },
		{ name: 'tablet', component: Tablet },
		{ name: 'headphones', component: Headphones },
		{ name: 'phone', component: Phone },
		{ name: 'refresh', component: RefreshCw },
		{ name: 'repeat', component: Repeat },
		{ name: 'copy', component: Copy },
		{ name: 'clipboard', component: Clipboard },
		{ name: 'link', component: Link },
		{ name: 'external-link', component: ExternalLink },
		{ name: 'share', component: Share },
		{ name: 'share-2', component: Share2 },
		{ name: 'send', component: Send },
		{ name: 'info', component: Info },
		{ name: 'help-circle', component: HelpCircle },
		{ name: 'alert-circle', component: AlertCircle },
		{ name: 'alert-triangle', component: AlertTriangle },
		{ name: 'ban', component: Ban },
		{ name: 'check-circle', component: CheckCircle },
		{ name: 'x-circle', component: XCircle },
		{ name: 'plus-circle', component: PlusCircle },
		{ name: 'circle', component: Circle },
		{ name: 'circle-dot', component: CircleDot },
		{ name: 'square', component: Square },
	];

	let search = $state('');
	let open = $state(false);

	const filtered = $derived(
		search.trim()
			? icons.filter((i) => i.name.includes(search.toLowerCase().trim()))
			: icons
	);

	function selectIcon(name: string) {
		value = name;
		onchange(name);
		open = false;
		search = '';
	}
</script>

<div class="relative">
	<button
		type="button"
		class="flex items-center gap-2 w-full rounded-lg border p-2 text-sm"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
		onmousedown={(e) => { e.preventDefault(); e.stopPropagation(); open = !open; }}
	>
		{#if value}
			{#each icons as icon (icon.name)}
				{#if icon.name === value}
					{@const IconComp = icon.component}
					<IconComp class="h-4 w-4 shrink-0" />
				{/if}
			{/each}
			<span class="font-mono text-xs">{value}</span>
		{:else}
			<span style="color: var(--color-muted)">{t('menus.pick_icon', 'Pick an icon...')}</span>
		{/if}
		<ChevronDown class="ml-auto h-4 w-4 shrink-0 opacity-50" />
	</button>

	<div
		class="fixed inset-0 z-40"
		style={open ? '' : 'display: none'}
		onclick={() => (open = false)}
		onkeydown={(e) => e.key === 'Escape' && (open = false)}
		role="presentation"
		tabindex="-1"
	></div>
	<div
		class="absolute z-50 mt-1 w-full rounded-lg border shadow-lg"
		style={open ? 'border-color: var(--color-border); background: var(--color-surface)' : 'display: none'}
	>
		<div class="p-2 border-b" style="border-color: var(--color-border)">
			<input
				type="text"
				bind:value={search}
				placeholder={t('menus.search_icons', 'Search icons...')}
				class="w-full rounded border px-2 py-1.5 text-sm bg-transparent outline-none"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			/>
		</div>
		<div class="max-h-48 overflow-auto p-2 grid grid-cols-6 gap-1">
			{#each filtered as icon (icon.name)}
				{@const IconComp = icon.component}
				{@const isSelected = value === icon.name}
				<button
					type="button"
					class="flex flex-col items-center justify-center gap-1 rounded-lg p-2 hover:opacity-80 transition-opacity"
					class:bg-primary={isSelected}
					style={isSelected ? 'background: color-mix(in srgb, var(--color-primary) 15%, transparent)' : ''}
					onmousedown={(e) => { e.preventDefault(); e.stopPropagation(); selectIcon(icon.name); }}
					title={icon.name}
				>
					<IconComp class="h-5 w-5" style="color: var(--color-foreground)" />
					<span class="text-[10px] truncate w-full text-center" style="color: var(--color-muted)">{icon.name}</span>
				</button>
			{/each}
			{#if filtered.length === 0}
				<p class="col-span-6 text-center text-sm py-4" style="color: var(--color-muted)">{t('menus.no_icons', 'No icons found')}</p>
			{/if}
		</div>
	</div>
</div>
