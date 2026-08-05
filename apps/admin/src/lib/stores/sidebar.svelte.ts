const STORAGE_KEY = 'jc.admin.sidebar.collapsed';

let collapsed = $state(false);

export function sidebarCollapsed(): boolean {
	return collapsed;
}

export function setSidebarCollapsed(v: boolean): void {
	collapsed = v;
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem(STORAGE_KEY, v ? '1' : '0');
	}
}

export function toggleSidebar(): void {
	setSidebarCollapsed(!collapsed);
}

export function initSidebarFromStorage(): void {
	if (typeof localStorage === 'undefined') return;
	collapsed = localStorage.getItem(STORAGE_KEY) === '1';
}
