package server

// Route registration is now handled directly in server.go's NewServer.
// This file remains as a placeholder for future route registration helpers
// as the API surface grows. Infrastructure routes (/health, /ready) and
// tenant-scoped API routes (/api/v1/...) are wired in NewServer.
