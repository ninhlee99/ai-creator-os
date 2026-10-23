package webai

// WebSessionAuth contains saved provider credentials used to validate a web session.
type WebSessionAuth struct {
	Platform               string
	SessionValue           string
	CookieHeader           string
	CookieJar              []BrowserCookie
	Email                  string
	OnCookieUpdate         func(string) error
	OnCookieSnapshotUpdate CookieSnapshotUpdater
	cookieSnapshotJar      *cookieSnapshotJar
}
