package tool

import "fmt"

func requireString(input map[string]any, key, errMsg string) (string, error) {
	v, ok := input[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("%s", errMsg)
	}
	return v, nil
}

func requireBool(input map[string]any, key, errMsg string) (bool, error) {
	v, ok := input[key].(bool)
	if !ok {
		return false, fmt.Errorf("%s", errMsg)
	}
	return v, nil
}
