// Package openapi holds the code generated from the API contract
// (docs/api/openapi.yaml) via oapi-codegen (ADR-005). The contract is the
// source of truth: edit the YAML first, then regenerate.
//
// Generation is two steps because oapi-codegen v2 does not yet support the
// OpenAPI 3.1 nullable idioms the contract uses throughout
// (oapi-codegen/oapi-codegen#373): specprep first rewrites a throwaway copy
// of the spec into 3.0-compatible equivalents (`type: X` + `nullable: true`,
// `allOf: [<schema>]` + `nullable: true`), then oapi-codegen runs against
// that copy. docs/api/openapi.yaml itself is never modified — see specprep's
// package doc for exactly which idioms are rewritten and the policy for any
// it doesn't recognize. See docs/adr/005-go-library-stack.md (addendum) for
// why this step exists.
//
// One consequence worth knowing before relying on it: GetSwagger() below
// (and the embedded spec behind it) serves specprep's *normalized* copy, not
// the literal 3.1 contract bytes. Harmless today since nothing calls it yet;
// worth remembering if a later issue wires request validation or serves
// /openapi.json from it.
//
//go:generate go run ./specprep ../../../docs/api/openapi.yaml openapi.normalized.yaml
//go:generate go tool oapi-codegen -config ../../oapi-codegen.yaml openapi.normalized.yaml
package openapi
