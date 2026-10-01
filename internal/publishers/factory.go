package publishers

// BuildPublishers returns the configured publishers for an account, each
// already filtered to the content kinds it handles. Unconfigured platforms
// are skipped silently — the ledger records what actually published.
//
// The signature takes primitives only (no internal/network import): the
// caller passes the account's username, its YouTube channel, and the
// content kinds that channel accepts. When kinds is non-empty, only
// publishers handling at least one of them are returned.
func BuildPublishers(username, youtubeChannel string, youtubeContentTypes []string, kinds ...string) []Publisher {
	pubs := []Publisher{
		NewTikTokPublisher(username),
		NewFacebookPublisher(username),
		NewYouTubePublisher(username, youtubeContentTypes, youtubeChannel),
	}
	var out []Publisher
	for _, p := range pubs {
		if !p.IsConfigured() {
			continue
		}
		if len(kinds) > 0 {
			handled := false
			for _, k := range kinds {
				if p.Handles(k) {
					handled = true
					break
				}
			}
			if !handled {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}
