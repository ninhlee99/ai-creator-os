package webai

import (
	"encoding/json"
)

// BrowserCookie stores provider cookie scope and metadata alongside legacy
// Cookie headers so imported browser backups can be sent with browser rules.
type BrowserCookie struct {
	Name               string          `json:"name"`
	Value              string          `json:"value"`
	Domain             string          `json:"domain"`
	Path               string          `json:"path"`
	Expires            float64         `json:"expires,omitempty"`
	HTTPOnly           bool            `json:"httpOnly,omitempty"`
	Secure             bool            `json:"secure,omitempty"`
	Session            bool            `json:"session,omitempty"`
	SameSite           string          `json:"sameSite,omitempty"`
	Priority           string          `json:"priority,omitempty"`
	SourceScheme       string          `json:"sourceScheme,omitempty"`
	SourcePort         int             `json:"sourcePort,omitempty"`
	SameParty          bool            `json:"sameParty,omitempty"`
	Partitioned        bool            `json:"partitioned,omitempty"`
	PartitionKey       json.RawMessage `json:"partitionKey,omitempty"`
	PartitionKeyOpaque bool            `json:"partitionKeyOpaque,omitempty"`
}

type cdpCookie = BrowserCookie

// CookieSnapshotUpdater persists the flat compatibility header and scoped jar
// after a provider rotates cookies.
type CookieSnapshotUpdater func(cookieHeader string, cookies []BrowserCookie) error
