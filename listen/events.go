// Package listen defines event types for Slack events (mentions, DMs, reactions)
// and their JSON serialization for JSONL output.
package listen

// Event "type" field values emitted in JSONL output. These are wire format —
// downstream agents key on these strings, so changes are user-visible.
const (
	EventTypeMention        = "mention"
	EventTypeDM             = "dm"
	EventTypeThreadReply    = "thread_reply"
	EventTypeChannelMessage = "channel_message"
	EventTypeReaction       = "reaction"
)

// ReactionAction values for EventTypeReaction events.
const (
	ReactionActionAdded   = "added"
	ReactionActionRemoved = "removed"
)

// Event represents a Slack event to be serialized as JSONL output.
type Event struct {
	Type         string     `json:"type"`
	Action       string     `json:"action,omitempty"`
	Channel      string     `json:"channel"`
	User         string     `json:"user,omitempty"`
	Text         string     `json:"text,omitempty"`
	TS           string     `json:"ts,omitempty"`
	ThreadTS     string     `json:"thread_ts,omitempty"`
	Emoji        string     `json:"emoji,omitempty"`
	ItemTS       string     `json:"item_ts,omitempty"`
	ParentUserID string     `json:"parent_user_id,omitempty"`
	Files        []FileMeta `json:"files,omitempty"`
}

// FileMeta is the receive-side schema for attached files on message events.
// URLs and download tokens are intentionally absent — caller fetches via
// `slackline download --file ID --out PATH`.
type FileMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mimetype string `json:"mimetype,omitempty"`
	Size     int    `json:"size"`
	Title    string `json:"title,omitempty"`
}
