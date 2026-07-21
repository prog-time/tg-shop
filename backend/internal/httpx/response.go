package httpx

import (
	"encoding/json"
	"net/http"
)

// dataEnvelope is the wire shape of the contract's unified success envelope
// for a single resource: { "data": ... }. List endpoints additionally carry
// a "meta" block — out of scope here since WriteData is only used by
// single-resource responses so far; a WriteList helper can be added
// alongside the first list endpoint that needs it.
type dataEnvelope struct {
	Data any `json:"data"`
}

// WriteData writes the unified `{ "data": ... }` success envelope with the
// given HTTP status. It is the single-resource counterpart to WriteError —
// every handler should use one or the other, never hand-roll the envelope.
func WriteData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dataEnvelope{Data: data})
}
