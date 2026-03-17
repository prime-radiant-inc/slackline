// Package listen defines event types for Slack events (mentions, DMs, reactions)
// and their JSON serialization for JSONL output.
package listen

// Event represents a Slack event to be serialized as JSONL output.
type Event struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	User     string `json:"user,omitempty"`
	Text     string `json:"text,omitempty"`
	TS       string `json:"ts,omitempty"`
	ThreadTS string `json:"thread_ts,omitempty"`
	Emoji    string `json:"emoji,omitempty"`
	ItemTS   string `json:"item_ts,omitempty"`
}
