/**
 * Extract unique dimension names from a comparison matrix.
 */
export function getDimensions(matrix: Record<string, Record<string, string>>): string[] {
	const dims = new Set<string>();
	for (const item of Object.values(matrix)) {
		for (const key of Object.keys(item)) {
			dims.add(key);
		}
	}
	return Array.from(dims);
}
