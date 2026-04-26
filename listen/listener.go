package listen

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Listener connects to Slack via Socket Mode and emits events as JSONL.
type Listener struct {
	api            *goslack.Client
	sm             *socketmode.Client
	botUserID      string
	includeBotSelf bool
	out            io.Writer
	status         io.Writer
}

// NewListener creates a Socket Mode listener.
// botToken is the xoxb- token; appToken is the xapp- token.
// botUserID is used to filter self-messages.
// includeBotSelf disables the self-filter when true.
func NewListener(botToken, appToken, botUserID string, includeBotSelf bool, out, status io.Writer) *Listener {
	api := goslack.New(botToken, goslack.OptionAppLevelToken(appToken))
	sm := socketmode.New(api)
	return &Listener{
		api:            api,
		sm:             sm,
		botUserID:      botUserID,
		includeBotSelf: includeBotSelf,
		out:            out,
		status:         status,
	}
}

// shouldFilterSelf reports whether an event from the given user should be
// suppressed because it was authored by the bot itself.
func (l *Listener) shouldFilterSelf(user string) bool {
	if l.includeBotSelf {
		return false
	}
	return user == l.botUserID
}

// Run starts the Socket Mode connection and blocks until interrupted.
// Events are written as JSONL to l.out. Status messages go to l.status.
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
	}
}

func (l *Listener) handleEventsAPI(evt slackevents.EventsAPIEvent) {
	switch ev := evt.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		if l.shouldFilterSelf(ev.User) {
			return // Self-filter
		}
		l.emit(Event{
			Type:     "mention",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
		})

	case *slackevents.MessageEvent:
		// Only handle DMs (im channel type starts with D)
		if len(ev.Channel) == 0 || ev.Channel[0] != 'D' {
			return
		}
		if l.shouldFilterSelf(ev.User) {
			return // Self-filter: drop our own messages
		}
		// Skip message subtypes (edits, deletes, etc.) — only new messages in v1
		if ev.SubType != "" {
			return
		}
		l.emit(Event{
			Type:     "dm",
			Channel:  ev.Channel,
			User:     ev.User,
			Text:     ev.Text,
			TS:       ev.TimeStamp,
			ThreadTS: ev.ThreadTimeStamp,
		})

	case *slackevents.ReactionAddedEvent:
		if l.shouldFilterSelf(ev.User) {
			return // Self-filter
		}
		l.emit(Event{
			Type:    "reaction_added",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})

	case *slackevents.ReactionRemovedEvent:
		if l.shouldFilterSelf(ev.User) {
			return
		}
		l.emit(Event{
			Type:    "reaction_removed",
			Channel: ev.Item.Channel,
			User:    ev.User,
			Emoji:   ev.Reaction,
			ItemTS:  ev.Item.Timestamp,
		})
	}
}

func (l *Listener) emit(e Event) {
	// Strip thread_ts when empty or equals ts (top-level message, not a reply)
	if e.ThreadTS == "" || e.ThreadTS == e.TS {
		e.ThreadTS = ""
	}
	data, err := json.Marshal(e)
	if err != nil {
		return // Should never happen with simple structs
	}
	_, _ = fmt.Fprintln(l.out, string(data))
}
