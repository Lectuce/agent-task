package tool

import "fmt"

// requireString extracts a required string field from a tool input map.
// It returns errMsg when the field is missing or empty.
func requireString(input map[string]any, key, errMsg string) (string, error) {
	v, ok := input[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("%s", errMsg)
	}
	return v, nil
}

// optionalString extracts an optional string field, returning "" if absent
// or not a string.
func optionalString(input map[string]any, key string) string {
	v, _ := input[key].(string)
	return v
}

// optionalBool extracts an optional bool field, returning def when absent
// or not a bool.
func optionalBool(input map[string]any, key string, def bool) bool {
	v, ok := input[key].(bool)
	if !ok {
		return def
	}
	return v
}

// requireBool extracts a required bool field from a tool input map,
// returning errMsg when the field is missing or not a bool.
func requireBool(input map[string]any, key, errMsg string) (bool, error) {
	v, ok := input[key].(bool)
	if !ok {
		return false, fmt.Errorf("%s", errMsg)
	}
	return v, nil
}
