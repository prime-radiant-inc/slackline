package slack

import (
	"fmt"
	"net/url"
	"strings"

	goslack "github.com/slack-go/slack"
)

// Resolver translates channel references (#name, C... ID, or Slack URL) into
// a canonical Slack channel ID.
type Resolver struct {
	api            SlackAPI
	cache          map[string]string // name -> channel ID (or "AMBIGUOUS:id1,id2" sentinel)
	cachePopulated bool
}

// NewResolver creates a Resolver backed by the given SlackAPI.
func NewResolver(api SlackAPI) *Resolver {
	return &Resolver{
		api:   api,
		cache: make(map[string]string),
	}
}

// Resolve takes a channel reference and returns the canonical channel ID.
// Accepted formats:
//   - Raw ID: C..., G..., D... (returned as-is)
//   - Slack URL: https://*.slack.com/archives/C... or https://app.slack.com/client/T.../C...
//   - Channel name: #name (looked up via API, cached)
func (r *Resolver) Resolve(ref string) (string, error) {
	if isChannelID(ref) {
		return ref, nil
	}

	if strings.Contains(ref, "://") {
		return resolveURL(ref)
	}

	name := strings.TrimPrefix(ref, "#")
	return r.resolveName(name)
}

// isChannelID returns true if s looks like a Slack channel/group/DM ID
// (starts with C, G, or D followed by uppercase alphanumeric characters).
func isChannelID(s string) bool {
	if len(s) < 2 {
		return false
	}
	prefix := s[0]
	if prefix != 'C' && prefix != 'G' && prefix != 'D' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

const ambiguousPrefix = "AMBIGUOUS:"

// resolveURL extracts a channel ID from a Slack URL.
func resolveURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, ".slack.com") && host != "slack.com" {
		return "", fmt.Errorf("not a Slack URL: %s", rawURL)
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, seg := range segments {
		if isChannelID(seg) {
			return seg, nil
		}
	}
	return "", fmt.Errorf("no channel ID found in Slack URL: %s", rawURL)
}

// resolveName looks up a channel name (without leading #) from the cache,
// populating the cache on first call.
func (r *Resolver) resolveName(name string) (string, error) {
	if !r.cachePopulated {
		if err := r.populateCache(); err != nil {
			return "", fmt.Errorf("listing channels: %w", err)
		}
	}

	id, ok := r.cache[name]
	if !ok {
		return "", fmt.Errorf("channel not found: #%s", name)
	}

	if strings.HasPrefix(id, ambiguousPrefix) {
		ids := strings.TrimPrefix(id, ambiguousPrefix)
		return "", fmt.Errorf("ambiguous channel name #%s matches multiple active channels: %s", name, ids)
	}

	return id, nil
}

// populateCache fetches all conversations and builds the name-to-ID cache.
// Active channels take priority over archived ones. If multiple active channels
// share a name, a sentinel value is stored so Resolve can report the ambiguity.
func (r *Resolver) populateCache() error {
	// Track whether cached IDs are archived, so cross-page duplicates are
	// handled correctly.
	archived := make(map[string]bool) // channel ID -> is archived

	cursor := ""
	for {
		channels, nextCursor, err := r.api.GetConversations(&goslack.GetConversationsParameters{
			Cursor: cursor,
			Limit:  1000,
			Types:  []string{"public_channel", "private_channel"},
		})
		if err != nil {
			return err
		}

		for _, ch := range channels {
			archived[ch.ID] = ch.IsArchived
			name := ch.Name
			existing, exists := r.cache[name]

			if !exists {
				r.cache[name] = ch.ID
				continue
			}

			// If the existing entry is already ambiguous, add this ID if active.
			if strings.HasPrefix(existing, ambiguousPrefix) {
				if !ch.IsArchived {
					r.cache[name] = existing + "," + ch.ID
				}
				continue
			}

			// Both exist. Determine which to keep.
			existingIsArchived := archived[existing]
			if ch.IsArchived && !existingIsArchived {
				// Existing is active, new is archived: keep existing.
				continue
			}
			if !ch.IsArchived && existingIsArchived {
				// New is active, existing is archived: replace.
				r.cache[name] = ch.ID
				continue
			}
			if !ch.IsArchived && !existingIsArchived {
				// Both active: ambiguous.
				r.cache[name] = ambiguousPrefix + existing + "," + ch.ID
				continue
			}
			// Both archived: keep first one seen (arbitrary).
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	r.cachePopulated = true
	return nil
}
