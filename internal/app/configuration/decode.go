package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// FieldError is one validation problem: where and why.
type FieldError struct {
	Path   string
	Reason string
}

// decodeStrict decodes JSON into v rejecting unknown fields (CONV-5).
// Problems are returned as FieldErrors; the path is "$" when unknown.
func decodeStrict(data []byte, v any) []FieldError {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		if _, err2 := dec.Token(); err2 != io.EOF {
			return []FieldError{{"$", "unexpected content after the JSON object"}}
		}
		return nil
	}
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		return []FieldError{{"$", fmt.Sprintf("invalid JSON at byte %d: %s", syn.Offset, syn.Error())}}
	case errors.As(err, &typ):
		path := typ.Field
		if path == "" {
			path = "$"
		}
		return []FieldError{{path, fmt.Sprintf("expected %s, got %s", typ.Type, typ.Value)}}
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		name := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return []FieldError{{strings.Trim(name, `"`), "unknown field"}}
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return []FieldError{{"$", "file is empty or truncated JSON"}}
	}
	return []FieldError{{"$", err.Error()}}
}
