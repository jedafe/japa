/**
 * Permission checks mirroring the backend RBAC HasPermission semantics
 * (internal/rbac/rbac.go): the "*" system wildcard held by super_admin
 * implicitly grants every node, so a check for any specific node must
 * test for the wildcard too. /me/permissions returns the resolved set —
 * for super-admin that is literally ["*"], not an enumerated node list.
 */
export function hasPermission(permissions: string[] | null | undefined, node: string): boolean {
	if (!permissions) return false;
	return permissions.includes('*') || permissions.includes(node);
}

/** Canonical docs editor node (§44) — used by every edit affordance. */
export function hasDocsAdmin(permissions: string[] | null | undefined): boolean {
	return hasPermission(permissions, 'docs.admin');
}
