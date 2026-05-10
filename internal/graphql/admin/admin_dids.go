package admin

import "strings"

// ParseAdminDIDs parses a comma-separated admin DID list from configuration.
func ParseAdminDIDs(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	adminDIDs := make([]string, 0, len(parts))
	for _, did := range parts {
		did = strings.TrimSpace(did)
		if did == "" {
			continue
		}
		adminDIDs = append(adminDIDs, did)
	}

	return adminDIDs
}

func isAdminDID(userDID string, adminDIDs []string) bool {
	for _, adminDID := range adminDIDs {
		if adminDID == userDID {
			return true
		}
	}

	return false
}
