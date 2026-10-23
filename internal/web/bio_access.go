package web

import (
	"net/http"
	"strconv"
	"strings"
)

// isPublicBioRequest is the complete anonymous surface for a public
// storefront. New endpoints must be added here deliberately; broad /bio/*
// prefix access would also expose admin or analytics routes.
func isPublicBioRequest(r *http.Request) bool {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method == http.MethodGet && len(parts) == 1 && parts[0] == "bio" {
		return true
	}
	if r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "bio" {
		return validPublicBioChannel(parts[1])
	}
	if r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "bio" && parts[1] == "google" &&
		(parts[2] == "login" || parts[2] == "callback") {
		return true
	}
	if r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "bio" && parts[1] == "google" && parts[2] == "logout" {
		return true
	}
	if r.Method == http.MethodGet && len(parts) == 4 && parts[0] == "bio" &&
		validPublicBioChannel(parts[1]) && (parts[2] == "product" || parts[2] == "go") {
		return positivePathID(parts[3])
	}
	if r.Method == http.MethodGet && len(parts) == 6 && parts[0] == "bio" &&
		validPublicBioChannel(parts[1]) && parts[2] == "product" && positivePathID(parts[3]) &&
		parts[4] == "image" && validBioProductImageFile(parts[5]) {
		return true
	}
	if r.Method == http.MethodPost && len(parts) == 5 && parts[0] == "bio" &&
		validPublicBioChannel(parts[1]) && parts[2] == "products" && positivePathID(parts[3]) &&
		(parts[4] == "try-on" || parts[4] == "voucher-copy") {
		return true
	}
	if r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "bio" && parts[1] == "try-on" {
		return validOpaqueBioToken(parts[2])
	}
	if r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "bio" && parts[1] == "try-on-result" {
		return validOpaqueBioToken(parts[2])
	}
	if r.Method == http.MethodGet && len(parts) == 1 && parts[0] == "bio-go" {
		return positivePathID(r.URL.Query().Get("id"))
	}
	// Keep old affiliate links working only for valid product IDs. Their
	// handler independently verifies that the product is publicly listed.
	if r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "bio-go" {
		return positivePathID(parts[1])
	}
	return false
}

func validPublicBioChannel(raw string) bool {
	channel := strings.TrimPrefix(raw, "@")
	if channel == "" {
		return false
	}
	switch strings.ToLower(channel) {
	case "try-on", "admin", "analytics", "pin", "go", "product", "products":
		return false
	}
	for _, r := range channel {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func positivePathID(raw string) bool {
	id, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && id > 0
}

func validOpaqueBioToken(raw string) bool {
	if len(raw) < 24 || len(raw) > 128 {
		return false
	}
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
