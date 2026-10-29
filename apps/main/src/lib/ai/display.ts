// Presentation helpers for the Japa AI chat surface — extracted from
// JapaAIChat.svelte so the 2600-line component shrinks and the helpers stay
// testable/shareable (BUG-AI-006, incremental decomposition step 1).
import { t } from '$lib/i18n';
import {
	BookOpen,
	FileText,
	Globe,
	GraduationCap,
	LayoutGrid,
	Tag,
	Users as UsersIcon,
	Wrench
} from 'lucide-svelte';

export type ToolStatus =
	| 'pending'
	| 'success'
	| 'error'
	| 'unauthorized'
	| 'privacy_blocked'
	| 'hitl_queued';

export interface ToolCallEntry {
	id: number;
	tool_name: string;
	arguments?: string;
	status: ToolStatus;
	result?: string;
	error?: string;
	iteration: number;
}

const toolIcon: Record<string, typeof Wrench> = {
	content: FileText,
	rag: BookOpen,
	external: Globe,
	custom_field: Tag,
	comparison: LayoutGrid,
	spaces: Globe,
	academy: GraduationCap,
	docs: FileText,
	tools: Wrench,
	menu: LayoutGrid,
	user: UsersIcon
};

export function iconForTool(name: string): typeof Wrench {
	return toolIcon[name.split('.')[0] ?? ''] ?? Wrench;
}

export function statusToToolStatus(s: string | undefined): ToolStatus {
	switch (s) {
		case 'success':
			return 'success';
		case 'error':
			return 'error';
		case 'unauthorized':
			return 'unauthorized';
		case 'privacy_blocked':
			return 'privacy_blocked';
		case 'hitl_queued':
			return 'hitl_queued';
		default:
			return 'pending';
	}
}

export function statusColor(status: ToolStatus): string {
	switch (status) {
		case 'success':
			return 'var(--color-success, #16a34a)';
		case 'error':
			return 'var(--color-danger, #dc2626)';
		case 'unauthorized':
			return 'var(--color-warning, #d97706)';
		case 'privacy_blocked':
			return 'var(--color-warning, #d97706)';
		case 'hitl_queued':
			return 'var(--color-info, #2563eb)';
		default:
			return 'var(--color-muted)';
	}
}

// Content items a successful tool call produced — create_draft /
// create / bulk_create (and status transitions) return
// {content_id, title, slug, status}, so the chat can link the created
// item instead of leaving the result buried in the JSON details.
export function createdItemLinks(tc: ToolCallEntry): { href: string; label: string }[] {
	if (tc.status !== 'success' || !tc.result) return [];
	let parsed: unknown;
	try {
		parsed = JSON.parse(tc.result);
	} catch {
		return [];
	}
	const links: { href: string; label: string }[] = [];
	const push = (o: unknown) => {
		if (!o || typeof o !== 'object') return;
		const rec = o as Record<string, unknown>;
		const slug = typeof rec.slug === 'string' ? rec.slug : '';
		if (!slug || links.some((l) => l.href === `/content/${slug}`)) return;
		const title = typeof rec.title === 'string' && rec.title.trim() ? rec.title.trim() : slug;
		links.push({ href: `/content/${slug}`, label: title });
	};
	if (Array.isArray(parsed)) parsed.forEach(push);
	else if (parsed && typeof parsed === 'object') {
		push(parsed);
		for (const key of ['items', 'results', 'created', 'contents']) {
			const arr = (parsed as Record<string, unknown>)[key];
			if (Array.isArray(arr)) arr.forEach(push);
		}
	}
	return links;
}

export function statusLabel(status: ToolStatus): string {
	switch (status) {
		case 'success':
			return t('ai.agent.tool.success', 'Success');
		case 'error':
			return t('ai.agent.tool.error', 'Error');
		case 'unauthorized':
			return t('ai.agent.tool.unauthorized', 'Denied');
		case 'privacy_blocked':
			return t('ai.agent.tool.privacy_blocked', 'Privacy blocked');
		case 'hitl_queued':
			return t('ai.agent.tool.hitl_queued', 'Awaiting review');
		case 'pending':
			return t('ai.agent.tool.pending', 'Running');
	}
}

export function fmtCost(c: number | undefined): string {
	if (!c) return '';
	return `$${c.toFixed(4)}`;
}

export function fmtTokens(n: number | undefined): string {
	if (!n) return '';
	return n.toLocaleString();
}
