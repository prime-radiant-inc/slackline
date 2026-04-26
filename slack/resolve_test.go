package slack

import (
	"io"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

// fakeAPI implements SlackAPI for testing channel resolution.
type fakeAPI struct {
	channels []goslack.Channel
}

func (f *fakeAPI) AuthTest() (*goslack.AuthTestResponse, error) {
	return &goslack.AuthTestResponse{}, nil
}

func (f *fakeAPI) PostMessage(channelID string, options ...goslack.MsgOption) (string, string, error) {
	return "", "", nil
}

func (f *fakeAPI) GetConversationHistory(params *goslack.GetConversationHistoryParameters) (*goslack.GetConversationHistoryResponse, error) {
	return &goslack.GetConversationHistoryResponse{}, nil
}

func (f *fakeAPI) GetConversationReplies(params *goslack.GetConversationRepliesParameters) ([]goslack.Message, bool, string, error) {
	return nil, false, "", nil
}

func (f *fakeAPI) GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error) {
	// Return all channels in one page (no pagination needed for tests unless overridden).
	return f.channels, "", nil
}

func (f *fakeAPI) AddReaction(_ string, _ goslack.ItemRef) error    { return nil }
func (f *fakeAPI) RemoveReaction(_ string, _ goslack.ItemRef) error { return nil }

func (f *fakeAPI) GetFileInfo(_ string, _, _ int) (*goslack.File, []goslack.Comment, *goslack.Paging, error) {
	return nil, nil, nil, nil
}

func (f *fakeAPI) GetFile(_ string, _ io.Writer) error { return nil }

func (f *fakeAPI) UploadFiles(_, _, _ string, _ []FileUpload) ([]goslack.FileSummary, error) {
	return nil, nil
}

// countingFakeAPI wraps fakeAPI and counts GetConversations calls.
type countingFakeAPI struct {
	fakeAPI
	getConversationsCount int
}

func (c *countingFakeAPI) GetConversations(params *goslack.GetConversationsParameters) ([]goslack.Channel, string, error) {
	c.getConversationsCount++
	return c.fakeAPI.GetConversations(params)
}

func makeChannel(id, name string, archived bool) goslack.Channel {
	ch := goslack.Channel{}
	ch.ID = id
	ch.Name = name
	ch.IsArchived = archived
	return ch
}

func TestResolveChannel_RawID(t *testing.T) {
	r := NewResolver(&fakeAPI{})

	tests := []struct {
		input string
		want  string
	}{
		{"C01ABC23DEF", "C01ABC23DEF"},
		{"G01ABC23DEF", "G01ABC23DEF"},
		{"D01ABC23DEF", "D01ABC23DEF"},
	}

	for _, tt := range tests {
		got, err := r.Resolve(tt.input)
		if err != nil {
			t.Errorf("Resolve(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveChannel_URL(t *testing.T) {
	r := NewResolver(&fakeAPI{})

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "archives URL",
			input: "https://myteam.slack.com/archives/C01ABC23DEF",
			want:  "C01ABC23DEF",
		},
		{
			name:  "messages URL with timestamp",
			input: "https://myteam.slack.com/archives/C01ABC23DEF/p1234567890",
			want:  "C01ABC23DEF",
		},
		{
			name:  "app.slack.com client URL",
			input: "https://app.slack.com/client/T01234567/C01ABC23DEF",
			want:  "C01ABC23DEF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Resolve(tt.input)
			if err != nil {
				t.Errorf("Resolve(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveChannel_InvalidURL(t *testing.T) {
	r := NewResolver(&fakeAPI{})

	_, err := r.Resolve("https://example.com/not-slack")
	if err == nil {
		t.Error("expected error for non-Slack URL, got nil")
	}
}

func TestResolveChannel_Name(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			makeChannel("C111", "general", false),
			makeChannel("C222", "random", false),
		},
	}
	r := NewResolver(api)

	got, err := r.Resolve("#general")
	if err != nil {
		t.Fatalf("Resolve(#general) unexpected error: %v", err)
	}
	if got != "C111" {
		t.Errorf("Resolve(#general) = %q, want %q", got, "C111")
	}
}

func TestResolveChannel_NameNotFound(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			makeChannel("C111", "general", false),
		},
	}
	r := NewResolver(api)

	_, err := r.Resolve("#nonexistent")
	if err == nil {
		t.Error("expected error for missing channel name, got nil")
	}
}

func TestResolveChannel_NameCached(t *testing.T) {
	api := &countingFakeAPI{
		fakeAPI: fakeAPI{
			channels: []goslack.Channel{
				makeChannel("C111", "general", false),
			},
		},
	}
	r := NewResolver(api)

	// First call populates cache.
	_, err := r.Resolve("#general")
	if err != nil {
		t.Fatalf("first Resolve(#general) unexpected error: %v", err)
	}

	// Second call should use cache, not call API again.
	_, err = r.Resolve("#general")
	if err != nil {
		t.Fatalf("second Resolve(#general) unexpected error: %v", err)
	}

	if api.getConversationsCount != 1 {
		t.Errorf("GetConversations called %d times, want 1", api.getConversationsCount)
	}
}

func TestResolveChannel_PrefersActiveOverArchived(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			makeChannel("C_ARCHIVED", "general", true),
			makeChannel("C_ACTIVE", "general", false),
		},
	}
	r := NewResolver(api)

	got, err := r.Resolve("#general")
	if err != nil {
		t.Fatalf("Resolve(#general) unexpected error: %v", err)
	}
	if got != "C_ACTIVE" {
		t.Errorf("Resolve(#general) = %q, want %q (active channel)", got, "C_ACTIVE")
	}
}

func TestResolveChannel_AmbiguousActiveChannels(t *testing.T) {
	api := &fakeAPI{
		channels: []goslack.Channel{
			makeChannel("C_ONE", "general", false),
			makeChannel("C_TWO", "general", false),
		},
	}
	r := NewResolver(api)

	_, err := r.Resolve("#general")
	if err == nil {
		t.Fatal("expected error for ambiguous channel name, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguity, got: %v", err)
	}
}
