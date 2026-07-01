package httpapi

import "strings"

func splitIDAction(path, prefix string) (id string, action string, ok bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	remainder := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if remainder == "" {
		return "", "", false
	}
	parts := strings.Split(remainder, "/")
	if len(parts) > 2 {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return parts[0], "", true
}

func splitPathParts(path, prefix string) ([]string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return nil, false
	}
	remainder := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if remainder == "" {
		return nil, false
	}
	return strings.Split(remainder, "/"), true
}
