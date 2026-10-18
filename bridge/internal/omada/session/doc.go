// Package session owns shared HTTP transport, authentication, controller/site
// context and generation-scoped request caching for the Omada integration.
// Telemetry requests may reauthenticate and replay once; guarded controls use a
// separately pinned session that never retries a mutation. Domain endpoints and
// data enrichment belong to the parent omada package.
package session
