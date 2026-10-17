// Package brand holds the configured site (brand) name for process-wide use
// by services that don't receive cfg.SiteName through their constructor.
// Set once at startup from config; Get() returns the fallback when unset.
package brand

import "sync/atomic"

var name atomic.Value

// Default is the only place the literal brand name may appear.
const Default = "Japa.to"

// Set records the configured site name (config.Config.SiteName).
func Set(n string) {
	if n != "" {
		name.Store(n)
	}
}

// Get returns the configured site name, or Default when unset.
func Get() string {
	if v := name.Load(); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return Default
}
