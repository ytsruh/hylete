// Package ai_coach implements Coach, the user-facing name for Hylete's
// AI features. It lives in directory internal/ai (imported as
// aicoach "hylete/internal/ai") to avoid the generic "ai" identifier
// colliding with local variable names.
//
// Layout: client.go is the dumb Cloudflare Workers AI transport
// (OpenAI-compatible HTTP, stdlib only); service.go is the domain
// orchestrator (load data → trainingstats → prompt → validate →
// persist); render.go substitutes the versioned prompt file. The
// service depends on the Client interface, so tests inject a fake
// and never touch the network.
package ai_coach
