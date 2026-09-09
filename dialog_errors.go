package main

import "strings"

func isDialogCancelled(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "cancelled by user") || strings.Contains(message, "canceled by user")
}
