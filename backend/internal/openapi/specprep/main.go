// Command specprep normalizes the OpenAPI 3.1 nullable-type idioms
// (`type: [X, "null"]` and `oneOf`/`anyOf: [<schema>, {type: null}]`) into
// the OpenAPI 3.0-style form oapi-codegen v2 understands (`type: X` /
// `allOf: [<schema>]` plus `nullable: true`). oapi-codegen does not yet
// support 3.1 type unions (oapi-codegen/oapi-codegen#373), and
// docs/api/openapi.yaml — the contract — is 3.1 and uses both idioms
// throughout for nullable fields. See docs/adr/005-go-library-stack.md
// (addendum) for why this step exists and the policy below.
//
// This tool never touches docs/api/openapi.yaml: it reads it and writes a
// throwaway, gitignored copy that only go generate consumes. The contract
// file itself remains the single source of truth, unmodified.
//
// Policy for unrecognized shapes: normalize only rewrites the exact two
// idioms above. Anything else — a three-element type union, a nullable
// enum wrapped some other way, a bare single-element `oneOf: [{type:
// null}]`, etc. — is left untouched rather than guessed at. main_test.go's
// TestNormalize_RealContractHasNoUnhandledNullableShapes guards this: it
// runs normalize against the real contract and fails loudly if any `type`
// array or unhandled `oneOf`/`anyOf` null-union survives, so a future
// contract edit introducing a new nullable idiom is caught here instead of
// silently mis-generating (or failing deep inside oapi-codegen).
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: specprep <in.yaml> <out.yaml>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "specprep:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("read %s: %w", in, err)
	}

	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", in, err)
	}

	normalize(doc)

	normalized, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("re-encode normalized spec: %w", err)
	}

	if err := os.WriteFile(out, normalized, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}

// nullableUnionKeys are the JSON Schema keywords that express an untagged
// union; the contract only ever uses them for a two-branch nullable ref
// (`oneOf`/`anyOf: [<schema>, {type: null}]`), which is semantically
// identical for either keyword in that shape.
var nullableUnionKeys = [...]string{"oneOf", "anyOf"}

// normalize walks the decoded document and rewrites the OpenAPI 3.1
// nullable idioms the contract uses into their 3.0-compatible equivalents
// that oapi-codegen's schema resolver handles:
//
//   - `type: [X, "null"]`                  -> `type: X` + `nullable: true`
//   - `oneOf: [<schema>, {type: null}]`     -> `allOf: [<schema>]` + `nullable: true`
//   - `anyOf: [<schema>, {type: null}]`     -> `allOf: [<schema>]` + `nullable: true`
//
// Any other shape (see the package doc's "Policy for unrecognized shapes")
// is left exactly as decoded.
func normalize(node any) {
	switch v := node.(type) {
	case map[string]any:
		if types, ok := v["type"].([]any); ok {
			if other, ok := nullableOtherType(types); ok {
				v["type"] = other
				v["nullable"] = true
			}
		}
		for _, key := range nullableUnionKeys {
			items, ok := v[key].([]any)
			if !ok {
				continue
			}
			if other, ok := collapseNullableUnion(items); ok {
				delete(v, key)
				v["allOf"] = []any{other}
				v["nullable"] = true
			}
		}
		for _, child := range v {
			normalize(child)
		}
	case []any:
		for _, child := range v {
			normalize(child)
		}
	}
}

// nullableOtherType reports whether types is exactly a two-element JSON
// Schema type union containing "null", returning the other member. Any
// other shape (wrong length, a non-string element, no "null" member, or
// "null" paired with another "null") is reported as unhandled.
func nullableOtherType(types []any) (string, bool) {
	if len(types) != 2 {
		return "", false
	}
	var other string
	sawNull, sawOther := false, false
	for _, t := range types {
		s, ok := t.(string)
		if !ok {
			return "", false
		}
		if s == "null" {
			sawNull = true
			continue
		}
		other = s
		sawOther = true
	}
	if !sawNull || !sawOther {
		return "", false
	}
	return other, true
}

// collapseNullableUnion reports whether items (a decoded `oneOf` or `anyOf`
// list) is exactly a two-element union where one element is a bare
// `{type: null}` schema, returning the other element (a $ref or inline
// schema). Any other shape (wrong length, neither/both elements bare-null,
// or a non-object element) is reported as unhandled.
func collapseNullableUnion(items []any) (any, bool) {
	if len(items) != 2 {
		return nil, false
	}
	var other any
	sawNull, sawOther := false, false
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, false
		}
		if isBareNullSchema(m) {
			sawNull = true
			continue
		}
		other = m
		sawOther = true
	}
	if !sawNull || !sawOther {
		return nil, false
	}
	return other, true
}

// isBareNullSchema reports whether m is exactly `{type: "null"}` with no
// other keys.
func isBareNullSchema(m map[string]any) bool {
	if len(m) != 1 {
		return false
	}
	s, ok := m["type"].(string)
	return ok && s == "null"
}
