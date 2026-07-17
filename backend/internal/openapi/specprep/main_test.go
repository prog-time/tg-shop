package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- nullableOtherType -------------------------------------------------

func TestNullableOtherType(t *testing.T) {
	tests := []struct {
		name      string
		types     []any
		wantOther string
		wantOK    bool
	}{
		{"nullable string, null last", []any{"string", "null"}, "string", true},
		{"nullable integer, null first", []any{"null", "integer"}, "integer", true},
		{"nullable object", []any{"object", "null"}, "object", true},

		// Adjacent, currently unhandled shapes: normalize must leave these
		// exactly as decoded, not guess.
		{"three-element union with null", []any{"string", "integer", "null"}, "", false},
		{"two-element union without null", []any{"string", "integer"}, "", false},
		{"duplicate null", []any{"null", "null"}, "", false},
		{"single element", []any{"string"}, "", false},
		{"empty", []any{}, "", false},
		{"non-string element", []any{"string", map[string]any{"foo": "bar"}}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other, ok := nullableOtherType(tt.types)
			if ok != tt.wantOK || other != tt.wantOther {
				t.Fatalf("nullableOtherType(%v) = (%q, %v), want (%q, %v)",
					tt.types, other, ok, tt.wantOther, tt.wantOK)
			}
		})
	}
}

// --- isBareNullSchema ----------------------------------------------------

func TestIsBareNullSchema(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"bare null", map[string]any{"type": "null"}, true},
		{"null with sibling key", map[string]any{"type": "null", "description": "x"}, false},
		{"non-null type", map[string]any{"type": "string"}, false},
		{"empty object", map[string]any{}, false},
		{"type as array", map[string]any{"type": []any{"null"}}, false},
		{"type as non-string scalar", map[string]any{"type": 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBareNullSchema(tt.m); got != tt.want {
				t.Fatalf("isBareNullSchema(%v) = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

// --- collapseNullableUnion (shared by oneOf and anyOf) -------------------

func TestCollapseNullableUnion(t *testing.T) {
	ref := map[string]any{"$ref": "#/components/schemas/Media"}
	nullSchema := map[string]any{"type": "null"}
	nullWithSibling := map[string]any{"type": "null", "description": "x"}
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}

	tests := []struct {
		name      string
		items     []any
		wantOther any
		wantOK    bool
	}{
		{"ref then bare null", []any{ref, nullSchema}, ref, true},
		{"bare null then ref", []any{nullSchema, ref}, ref, true},
		{"inline schema then bare null", []any{str, nullSchema}, str, true},

		// Adjacent, currently unhandled shapes.
		{"three-element union with null", []any{ref, nullSchema, str}, nil, false},
		{"bare single-element union", []any{nullSchema}, nil, false},
		{"neither branch is null", []any{str, num}, nil, false},
		{"both branches are bare null", []any{nullSchema, nullSchema}, nil, false},
		{"null schema with sibling key is not bare (unhandled)", []any{ref, nullWithSibling}, nil, false},
		{"non-object element", []any{ref, "null"}, nil, false},
		{"empty union", []any{}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other, ok := collapseNullableUnion(tt.items)
			if ok != tt.wantOK {
				t.Fatalf("collapseNullableUnion(%v) ok = %v, want %v", tt.items, ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(other, tt.wantOther) {
				t.Fatalf("collapseNullableUnion(%v) other = %v, want %v", tt.items, other, tt.wantOther)
			}
		})
	}
}

// --- normalize end-to-end, for both oneOf and anyOf -----------------------

func TestNormalize_HandlesOneOfNullUnion(t *testing.T) {
	doc := map[string]any{
		"image": map[string]any{
			"oneOf": []any{
				map[string]any{"$ref": "#/components/schemas/Media"},
				map[string]any{"type": "null"},
			},
			"description": "keep me",
		},
	}
	normalize(doc)

	image := doc["image"].(map[string]any)
	if _, present := image["oneOf"]; present {
		t.Fatalf("expected oneOf to be removed, got %v", image)
	}
	if image["nullable"] != true {
		t.Fatalf("expected nullable: true, got %v", image)
	}
	if image["description"] != "keep me" {
		t.Fatalf("expected sibling key to survive, got %v", image)
	}
	allOf, ok := image["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		t.Fatalf("expected a single-element allOf, got %v", image["allOf"])
	}
}

func TestNormalize_HandlesAnyOfNullUnion(t *testing.T) {
	doc := map[string]any{
		"image": map[string]any{
			"anyOf": []any{
				map[string]any{"$ref": "#/components/schemas/Media"},
				map[string]any{"type": "null"},
			},
		},
	}
	normalize(doc)

	image := doc["image"].(map[string]any)
	if _, present := image["anyOf"]; present {
		t.Fatalf("expected anyOf to be removed, got %v", image)
	}
	if image["nullable"] != true {
		t.Fatalf("expected nullable: true, got %v", image)
	}
	allOf, ok := image["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		t.Fatalf("expected a single-element allOf, got %v", image["allOf"])
	}
}

func TestNormalize_LeavesUnhandledShapesUntouched(t *testing.T) {
	// A hypothetical future contract edit: a three-element type union. This
	// must survive normalize verbatim rather than being guessed at — the
	// guard test below is what turns "survives verbatim" into "caught before
	// codegen runs".
	doc := map[string]any{
		"weird": map[string]any{
			"type": []any{"string", "integer", "null"},
		},
	}
	normalize(doc)

	weird := doc["weird"].(map[string]any)
	types, ok := weird["type"].([]any)
	if !ok || len(types) != 3 {
		t.Fatalf("expected the unhandled type array to survive untouched, got %v", weird["type"])
	}
}

// --- guard: the real contract has no unhandled nullable shape -----------

// contractPath is relative to this package's directory, since `go test`
// (like `go generate`) runs with cwd set to the package directory.
const contractPath = "../../../../docs/api/openapi.yaml"

// TestNormalize_RealContractHasNoUnhandledNullableShapes loads the actual
// contract, runs the same normalize() used by `go generate`, and fails
// loudly if any `type` array or unhandled `oneOf`/`anyOf` null-union
// survives. Its purpose is to catch a *future* contract edit that
// introduces a nullable idiom normalize doesn't yet know how to rewrite —
// today that would otherwise pass silently through to oapi-codegen, which
// might error unhelpfully deep in schema resolution, or worse, mis-generate
// without erroring at all (see package doc, "Policy for unrecognized
// shapes").
func TestNormalize_RealContractHasNoUnhandledNullableShapes(t *testing.T) {
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s: %v", contractPath, err)
	}

	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", contractPath, err)
	}

	// Sanity check the test itself isn't vacuous: the contract must still
	// actually use the idioms normalize exists to rewrite, otherwise "zero
	// unhandled shapes" would trivially (and uselessly) pass.
	before := countNullableIdioms(doc)
	if before == 0 {
		t.Fatal("expected the real contract to contain at least one nullable-union idiom; " +
			"this test would otherwise pass vacuously")
	}

	normalize(doc)

	var unhandled []string
	findUnhandledNullableShapes(doc, "$", &unhandled)
	if len(unhandled) > 0 {
		t.Fatalf(
			"normalize left %d unhandled nullable shape(s) in %s — a new OpenAPI 3.1 nullable idiom "+
				"was likely introduced; extend specprep's normalize (and this guard) before trusting "+
				"generated code:\n%s",
			len(unhandled), contractPath, strings.Join(unhandled, "\n"),
		)
	}
}

// countNullableIdioms counts `type` arrays and `oneOf`/`anyOf` arrays
// containing at least one bare-null branch, pre-normalization. Used only to
// keep the guard test honest (see above).
func countNullableIdioms(node any) int {
	count := 0
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v["type"].([]any); ok {
			count++
		}
		for _, key := range nullableUnionKeys {
			if items, ok := v[key].([]any); ok {
				for _, it := range items {
					if m, ok := it.(map[string]any); ok && isBareNullSchema(m) {
						count++
						break
					}
				}
			}
		}
		for _, child := range v {
			count += countNullableIdioms(child)
		}
	case []any:
		for _, child := range v {
			count += countNullableIdioms(child)
		}
	}
	return count
}

// findUnhandledNullableShapes walks a document already passed through
// normalize and appends a human-readable description of every surviving
// `type` array and every surviving `oneOf`/`anyOf` array that still
// contains a bare-null branch (normalize should have deleted the key
// entirely when it successfully collapsed one).
func findUnhandledNullableShapes(node any, path string, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if types, ok := v["type"].([]any); ok {
			*out = append(*out, fmt.Sprintf("%s.type: unhandled type union %v", path, types))
		}
		for _, key := range nullableUnionKeys {
			items, ok := v[key].([]any)
			if !ok {
				continue
			}
			for i, it := range items {
				if m, ok := it.(map[string]any); ok && isBareNullSchema(m) {
					*out = append(*out, fmt.Sprintf(
						"%s.%s[%d]: unhandled bare-null branch in a %d-element %s union",
						path, key, i, len(items), key,
					))
				}
			}
		}
		for k, child := range v {
			findUnhandledNullableShapes(child, path+"."+k, out)
		}
	case []any:
		for i, child := range v {
			findUnhandledNullableShapes(child, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}
