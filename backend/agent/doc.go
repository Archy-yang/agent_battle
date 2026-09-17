// Package agent hosts the Agent Runtime and the Agent Protocol adapters
// (docs/world_rule.md §11–13): a stable observation ↔ intent protocol
// above the engine, with LLM providers behind a plain HTTP adapter (M1).
//
// The engine never knows which model is attached; the runtime never
// sees the true World State — only Observation and Capability Manifest.
// Memory belongs here, not in the engine (world_rule §21).
package agent
