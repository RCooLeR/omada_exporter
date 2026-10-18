// Package omada exposes controller features to collectors and Home Assistant
// without making those consumers choose between Omada's Web API and OpenAPI.
// Feature operations share one session for authentication, cache coalescing and
// guarded controls; endpoint protocols remain implementation details here.
package omada
