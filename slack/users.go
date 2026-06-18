package slack

import (
	"regexp"
	"strings"

	goslack "github.com/slack-go/slack"
)

// UserDirectory resolves between Slack user references and IDs, backed by a
// single cached users.list fetch. Forward resolution turns an @handle (or a
// display/real name) into a user ID for linkifying mentions; reverse lookup
// turns a user ID into a handle for rendering names in read output.
type UserDirectory struct {
	api       SlackAPI
	users     []goslack.User
	byID      map[string]goslack.User
	byHandle  map[string]string   // lowercased handle (name) -> ID; handles are unique per workspace
	byName    map[string][]string // lowercased display/real name -> candidate IDs
	populated bool
}

// NewUserDirectory creates a directory backed by the given SlackAPI.
func NewUserDirectory(api SlackAPI) *UserDirectory {
	return &UserDirectory{api: api}
}

// Load eagerly fetches and caches the user list. It is idempotent; subsequent
// calls (and lazy resolution methods) reuse the cache.
func (d *UserDirectory) Load() error {
	return d.populate()
}

func (d *UserDirectory) populate() error {
	if d.populated {
		return nil
	}
	users, err := d.api.GetUsers()
	if err != nil {
		return err
	}
	d.users = users
	d.byID = make(map[string]goslack.User, len(users))
	d.byHandle = make(map[string]string)
	d.byName = make(map[string][]string)
	for _, u := range users {
		d.byID[u.ID] = u
		// Deactivated users can't be mentioned; keep them in byID for reverse
		// rendering of historical mentions, but don't offer them for resolution.
		if u.Deleted {
			continue
		}
		if u.Name != "" {
			d.byHandle[strings.ToLower(u.Name)] = u.ID
		}
		d.addName(u.Profile.DisplayName, u.ID)
		d.addName(u.RealName, u.ID)
	}
	d.populated = true
	return nil
}

// addName records a display/real name as a resolution candidate, deduping IDs.
func (d *UserDirectory) addName(name, id string) {
	if name == "" {
		return
	}
	key := strings.ToLower(name)
	for _, existing := range d.byName[key] {
		if existing == id {
			return
		}
	}
	d.byName[key] = append(d.byName[key], id)
}

// ResolveHandle resolves a mention token (the text after '@', without the '@')
// to a user ID. Handles match first (unique per workspace); display and real
// names are a fallback. ok is false when the token matches no user or matches
// more than one — the caller should leave the text literal and warn. err is
// non-nil only on API failure.
func (d *UserDirectory) ResolveHandle(token string) (id string, ok bool, err error) {
	if err := d.populate(); err != nil {
		return "", false, err
	}
	key := strings.ToLower(token)
	if id, found := d.byHandle[key]; found {
		return id, true, nil
	}
	if ids := d.byName[key]; len(ids) == 1 {
		return ids[0], true, nil
	}
	return "", false, nil
}

// Name returns the handle for a user ID, for rendering <@ID|handle> in output.
// ok is false when the ID is unknown.
func (d *UserDirectory) Name(id string) (handle string, ok bool, err error) {
	if err := d.populate(); err != nil {
		return "", false, err
	}
	u, found := d.byID[id]
	if !found {
		return "", false, nil
	}
	return userHandle(u), true, nil
}

// All returns every user in the workspace, fetching users.list once.
func (d *UserDirectory) All() ([]goslack.User, error) {
	if err := d.populate(); err != nil {
		return nil, err
	}
	return d.users, nil
}

// userHandle returns the best short name for a user: the handle, falling back
// to display then real name (only relevant for records missing a username).
func userHandle(u goslack.User) string {
	if u.Name != "" {
		return u.Name
	}
	if u.Profile.DisplayName != "" {
		return u.Profile.DisplayName
	}
	return u.RealName
}

// mentionTokenRE matches an @mention: '@' at the start of the string or after
// whitespace (so emails like drew@example.com are left alone), capturing the
// leading boundary and the handle token. Trailing sentence punctuation is
// trimmed at resolve time.
var mentionTokenRE = regexp.MustCompile(`(^|\s)@([A-Za-z0-9][A-Za-z0-9._-]*)`)

// LinkifyMentions rewrites @handle tokens in text to Slack <@ID> mention
// syntax so the mentioned users are actually notified. Tokens that don't
// resolve to a unique user are left literal and returned in unresolved, so the
// caller can warn. Returns an error only on API failure (which happens only
// when the text contains at least one @mention to resolve).
func (d *UserDirectory) LinkifyMentions(text string) (out string, unresolved []string, err error) {
	var apiErr error
	result := replaceAllSubmatchFunc(mentionTokenRE, text, func(groups []string) string {
		lead, token := groups[1], groups[2]
		core, trailing := splitTrailingDots(token)
		id, ok, rerr := d.ResolveHandle(core)
		if rerr != nil {
			apiErr = rerr
			return groups[0]
		}
		if !ok {
			unresolved = append(unresolved, core)
			return groups[0]
		}
		return lead + "<@" + id + ">" + trailing
	})
	if apiErr != nil {
		return text, nil, apiErr
	}
	return result, unresolved, nil
}

// userMentionRE matches a Slack-encoded user mention <@ID> or <@ID|label>.
// Slack user IDs start with U (members) or W (enterprise grid members).
var userMentionRE = regexp.MustCompile(`<@([UW][A-Z0-9]+)(\|[^>]*)?>`)

// EnrichMentions rewrites bare <@ID> mentions in text to the labeled
// <@ID|handle> form so output is human-readable while still carrying the ID.
// Mentions that already have a label, or whose ID is unknown, are left as-is.
func (d *UserDirectory) EnrichMentions(text string) (string, error) {
	var apiErr error
	out := replaceAllSubmatchFunc(userMentionRE, text, func(groups []string) string {
		id, label := groups[1], groups[2]
		if label != "" {
			return groups[0]
		}
		handle, ok, err := d.Name(id)
		if err != nil {
			apiErr = err
			return groups[0]
		}
		if !ok {
			return groups[0]
		}
		return "<@" + id + "|" + handle + ">"
	})
	if apiErr != nil {
		return text, apiErr
	}
	return out, nil
}

// splitTrailingDots separates trailing '.' characters (e.g. a sentence period
// after an @mention) from a handle token so "drew." resolves as "drew" while
// preserving the period in the output.
func splitTrailingDots(token string) (core, trailing string) {
	i := len(token)
	for i > 0 && token[i-1] == '.' {
		i--
	}
	return token[:i], token[i:]
}

// replaceAllSubmatchFunc replaces each match of re in s with the result of
// repl, which receives the full match in groups[0] and capture groups in
// groups[1:]. The standard library has no submatch-aware ReplaceAll, and the
// mention rewrites need the captured groups (boundary, token, label).
func replaceAllSubmatchFunc(re *regexp.Regexp, s string, repl func(groups []string) string) string {
	var b strings.Builder
	last := 0
	for _, idx := range re.FindAllStringSubmatchIndex(s, -1) {
		groups := make([]string, len(idx)/2)
		for i := range groups {
			start, end := idx[2*i], idx[2*i+1]
			if start >= 0 {
				groups[i] = s[start:end]
			}
		}
		b.WriteString(s[last:idx[0]])
		b.WriteString(repl(groups))
		last = idx[1]
	}
	b.WriteString(s[last:])
	return b.String()
}
