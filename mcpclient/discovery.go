package mcpclient

import "slices"

func toolAllowed(name string, allowTools []string, denyTools []string) bool {

	if slices.Contains(denyTools, name) {
		return false
	}

	if len(allowTools) == 0 {
		return true
	}

	return slices.Contains(allowTools, name)
}
