package control

// BuildVersion is the deployment version reported by GET /control/deployments.
// It is injected at build time via -ldflags and defaults to "1.0.0" for
// unversioned builds.
// Ref: .jeos/sections/21-control-panel.md.
var BuildVersion = "1.0.0"
