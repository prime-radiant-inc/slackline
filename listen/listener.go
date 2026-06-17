package listen

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const messageSubtypeFileShare = "file_share"

// Listener connects to Slack via Socket Mode and emits events to stdout.
type Listener struct {
	api            *goslack.Client
	sm             *socketmode.Client
	botUserID      string
	botID          string
	out            io.Writer
	status         io.Writer
	includeBotSelf bool
	threads        bool
	allMessages    bool
	types          map[string]bool
	outputFormat   string
}

// ListenerOptions bundles per-mode flags for NewListener.
type ListenerOptions struct {
	IncludeBotSelf bool
	Threads        bool
	AllMessages    bool
	Types          map[string]bool
	OutputFormat   string
}

// NewListener creates a Socket Mode listener.
// botToken is the xoxb- token; appToken is the xapp- token.
// botUserID and botID are used to filter self-messages.
func NewListener(botToken, appToken, botUserID, botID string, opts ListenerOptions, out, status io.Writer) *Listener {
	api := goslack.New(botToken, goslack.OptionAppLevelToken(appToken))
	sm := socketmode.New(api)
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = OutputFormatText
	}
	return &Listener{
		api:            api,
		sm:             sm,
		botUserID:      botUserID,
		botID:          botID,
		out:            out,
		status:         status,
		includeBotSelf: opts.IncludeBotSelf,
		threads:        opts.Threads,
		allMessages:    opts.AllMessages,
		types:          opts.Types,
		outputFormat:   outputFormat,
	}
}

// shouldFilterSelf reports whether an event from the given user should be
// suppressed because it was authored by the bot itself.
func (l *Listener) shouldFilterSelf(user, botID string) bool {
	if l.includeBotSelf {
		return false
	}
	return (user != "" && user == l.botUserID) || (botID != "" && botID == l.botID)
}

// Run starts the Socket Mode connection and blocks until interrupted.
// Events are written to l.out. Status messages go to l.status.
// Shuts down on SIGTERM, SIGINT, or when stdin is closed (parent process exits).
func (l *Listener) Run() error {
	stop := make(chan struct{}, 1)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		close(stop)
	}()

	// Monitor stdin for EOF — when parent process exits, stdin closes
	go func() {
		buf := make([]byte, 1)
		for {
			_, err := os.Stdin.Read(buf)
			if err != nil { // EOF or error
				close(stop)
				return
			}
		}
	}()

	go func() {
		for evt := range l.sm.Events {
			l.handleEvent(evt)
		}
	}()

	// Start Socket Mode in background goroutine.
	// "connected" status is emitted by handleEvent on EventTypeConnected,
	// not here — sm.Run() hasn't connected yet at this point.
	go func() { _ = l.sm.Run() }()

	// Block until shutdown signal
	<-stop
	_, _ = fmt.Fprintln(l.status, "disconnected")
	return nil
}

func (l *Listener) handleEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		l.sm.Ack(*evt.Request)
		l.handleEventsAPI(eventsAPIEvent)

	case socketmode.EventTypeConnectionError:
		_, _ = fmt.Fprintln(l.status, "reconnecting")

	case socketmode.EventTypeConnected:
		_, _ = fmt.Fprintln(l.status, "connected")

	case socketmode.EventTypeHello:
		// Slack's hello frame: the session is established and events will now
		// flow. "connected" is only the websocket open — "ready" is the signal
		// that the listener is actually subscribed and receiving.
		_, _ = fmt.Fprintln(l.status, "ready")
	}
}

func (l *Listener) handleEventsAPI(evt slackevents.EventsAPIEvent) {
	switch ev := evt.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		if l.shouldFilterSelf(ev.User, ev.BotID) {
			return // Self-filter
		}
		l.emit(Event{
			Type:     EventTypeMention,
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
		})

	case *slackevents.MessageEvent:
		if len(ev.Channel) == 0 {
			return
		}
		// DM channels (D...) flow through the DM path.
		if ev.Channel[0] == 'D' {
			if l.shouldFilterSelf(ev.User, ev.BotID) {
				return
			}
			// Allow file_share subtype since it carries Files; skip other subtypes.
			if ev.SubType != "" && ev.SubType != messageSubtypeFileShare {
				return
			}
			l.emit(Event{
				Type:     EventTypeDM,
				Channel:  ev.Channel,
				User:     ev.User,
				Text:     ev.Text,
				TS:       ev.TimeStamp,
				ThreadTS: ev.ThreadTimeStamp,
				Files:    convertMessageEventFiles(ev),
			})
			return
		}
		// Non-DM (C... public, G... private). Emit only when a mode allows it.
		if l.shouldFilterSelf(ev.User, ev.BotID) {
			return
		}
		if ev.SubType != "" && ev.SubType != messageSubtypeFileShare {
			return
		}
		isThread := ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp
		parentUserID := ""
		if ev.Message != nil {
			parentUserID = ev.Message.ParentUserId
		}
		switch {
		case l.allMessages:
			eventType := EventTypeChannelMessage
			if isThread {
				eventType = EventTypeThreadReply
			}
			l.emit(Event{
				Type:         eventType,
				Channel:      ev.Channel,
				User:         ev.User,
				Text:         ev.Text,
				TS:           ev.TimeStamp,
				ThreadTS:     ev.ThreadTimeStamp,
				ParentUserID: parentUserID,
				Files:        convertMessageEventFiles(ev),
			})
		case isThread && parentUserID == l.botUserID:
			// Replies in the bot's own threads are emitted by default — they're
			// the highest-signal slice of channel traffic and the natural place
			// users reply to bot-authored messages.
			l.emit(Event{
				Type:         EventTypeThreadReply,
				Channel:      ev.Channel,
				User:         ev.User,
				Text:         ev.Text,
				TS:           ev.TimeStamp,
				ThreadTS:     ev.ThreadTimeStamp,
				ParentUserID: parentUserID,
				Files:        convertMessageEventFiles(ev),
			})
		}

	case *slackevents.ReactionAddedEvent:
		if l.shouldFilterSelf(ev.User, "") {
			return // Self-filter
		}
		l.emit(Event{
			Type:    EventTypeReaction,
			Action:  ReactionActionAdded,
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})

	case *slackevents.ReactionRemovedEvent:
		if l.shouldFilterSelf(ev.User, "") {
			return
		}
		l.emit(Event{
			Type:    EventTypeReaction,
			Action:  ReactionActionRemoved,
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
	}
}

func (l *Listener) emit(e Event) {
	if l.types != nil && !l.types[e.Type] {
		return
	}
	// Strip thread_ts when empty or equals ts (top-level message, not a reply)
	if e.ThreadTS == "" || e.ThreadTS == e.TS {
		e.ThreadTS = ""
	}
	if l.outputFormat != OutputFormatJSON {
		_, _ = fmt.Fprint(l.out, formatEventText(e))
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return // Should never happen with simple structs
	}
	_, _ = fmt.Fprintln(l.out, string(data))
}

func formatEventText(e Event) string {
	var b strings.Builder
	if e.Type == EventTypeReaction {
		parts := []string{e.Type, e.Action, e.Channel, e.User}
		if e.ItemTS != "" {
			parts = append(parts, "item="+e.ItemTS)
		}
		if e.Emoji != "" {
			parts = append(parts, e.Emoji)
		}
		b.WriteString(strings.Join(nonEmpty(parts), " "))
		b.WriteByte('\n')
		return b.String()
	}

	parts := []string{e.Type, e.Channel, e.User, e.TS}
	if e.ThreadTS != "" {
		parts = append(parts, "thread="+e.ThreadTS)
	}
	if e.ParentUserID != "" {
		parts = append(parts, "parent="+e.ParentUserID)
	}
	b.WriteString(strings.Join(nonEmpty(parts), " "))
	if e.Text != "" {
		b.WriteByte(' ')
		b.WriteString(singleLine(e.Text))
	}
	b.WriteByte('\n')
	writeFileTextLines(&b, e.Files)
	return b.String()
}

func writeFileTextLines(b *strings.Builder, files []FileMeta) {
	for _, f := range files {
		parts := []string{"  file", f.ID, f.Name, strconv.Itoa(f.Size), f.Mimetype}
		b.WriteString(strings.Join(nonEmpty(parts), " "))
		if f.Title != "" {
			b.WriteByte(' ')
			b.WriteString(singleLine(f.Title))
		}
		b.WriteByte('\n')
	}
}

func nonEmpty(parts []string) []string {
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", `\n`)
}

// convertMessageEventFiles extracts FileMeta from a MessageEvent. Files are
// stored on the embedded Message field (populated by the custom unmarshaler).
func convertMessageEventFiles(ev *slackevents.MessageEvent) []FileMeta {
	if ev.Message == nil {
		return nil
	}
	return convertFiles(ev.Message.Files)
}

func convertFiles(in []goslack.File) []FileMeta {
	if len(in) == 0 {
		return nil
	}
	out := make([]FileMeta, len(in))
	for i, f := range in {
		out[i] = FileMeta{
			ID:       f.ID,
			Name:     f.Name,
			Mimetype: f.Mimetype,
			Size:     f.Size,
			Title:    f.Title,
		}
	}
	return out
}
