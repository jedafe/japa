let enabledFlags = $state<Record<string, boolean>>({});

export function setFeatureFlags(flags: Record<string, boolean>) {
	enabledFlags = flags;
}

export function isFeatureEnabled(flag: string): boolean {
	return enabledFlags[flag] ?? false;
}
