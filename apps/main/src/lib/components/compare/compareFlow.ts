import type { ComparisonType } from '$lib/types/jce';

export function scopeChainOf(type: ComparisonType | null | undefined): string[] {
	return (type?.config?.scope_chain as string[] | undefined) ?? [];
}

export function defaultItemRefType(type: ComparisonType | null | undefined): string {
	const configured = type?.config?.default_item_ref_type;
	if (typeof configured === 'string' && configured) return configured;
	return type?.config?.item_ref_types?.[0] ?? 'space';
}

export function requiredScopeStep(source: string, _chain: string[]): string | null {
	// Course search must be scoped to a university — same rule as +page.server.ts.
	if (source === 'course') return 'university';
	return null;
}

export function missingRequiredScope(
	source: string,
	chain: string[],
	selections: Record<string, string>
): string | null {
	const step = requiredScopeStep(source, chain);
	if (step && chain.includes(step) && !selections[step]) return step;
	return null;
}

export function scopeSummary(
	chain: string[],
	selections: Record<string, string>,
	labels: Record<string, string>
): string {
	return chain
		.filter((step) => selections[step] && labels[step])
		.map((step) => labels[step])
		.join(' · ');
}

export function sourceOptions(
	itemRefTypes: string[],
	labelFor: (key: string) => string
): { value: string; label: string }[] {
	return itemRefTypes.map((t) => ({ value: t, label: labelFor(t) }));
}
