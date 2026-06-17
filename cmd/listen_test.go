package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/prime-radiant-inc/slackline/errs"
)

func TestListenLongDocumentsEventSchema(t *testing.T) {
	// `listen --help` must document the event JSON field names (an agent working
	// from --help alone otherwise has to guess them) and the "ready" status.
	for _, want := range []string{"item_ts", "action", "channel", "ready", "file_share"} {
		if !strings.Contains(listenCmd.Long, want) {
			t.Errorf("listen Long help missing %q", want)
		}
	}
}

func TestParseListenTypes(t *testing.T) {
	t.Run("empty returns nil (emit all)", func(t *testing.T) {
		set, err := parseListenTypes("", false)
		if err != nil || set != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", set, err)
		}
	})

	t.Run("valid types", func(t *testing.T) {
		set, err := parseListenTypes("mention, reaction", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !set["mention"] || !set["reaction"] || len(set) != 2 {
			t.Fatalf("set = %v", set)
		}
	})

	t.Run("unknown type is a usage error", func(t *testing.T) {
		_, err := parseListenTypes("mentions", false)
		var se *errs.SlackError
		if !errors.As(err, &se) || se.Code != errs.Usage {
			t.Fatalf("err = %v, want Usage SlackError", err)
		}
	})

	t.Run("channel_message requires --all-messages", func(t *testing.T) {
		_, err := parseListenTypes("channel_message", false)
		var se *errs.SlackError
		if !errors.As(err, &se) || se.Code != errs.Usage {
			t.Fatalf("err = %v, want Usage SlackError", err)
		}
		if _, err := parseListenTypes("channel_message", true); err != nil {
			t.Fatalf("with --all-messages: unexpected error %v", err)
		}
	})
}

func TestParseOutputFormat(t *testing.T) {
	for _, raw := range []string{"", "text", "TEXT"} {
		got, err := parseOutputFormat(raw)
		if err != nil {
			t.Fatalf("parseOutputFormat(%q): %v", raw, err)
		}
		if got != outputFormatText {
			t.Fatalf("parseOutputFormat(%q) = %q, want %q", raw, got, outputFormatText)
		}
	}

	got, err := parseOutputFormat("json")
	if err != nil {
		t.Fatalf("parseOutputFormat(json): %v", err)
	}
	if got != outputFormatJSON {
		t.Fatalf("parseOutputFormat(json) = %q, want %q", got, outputFormatJSON)
	}

	t.Run("unknown format is usage error", func(t *testing.T) {
		_, err := parseOutputFormat("xml")
		var se *errs.SlackError
		if !errors.As(err, &se) || se.Code != errs.Usage {
			t.Fatalf("err = %v, want Usage SlackError", err)
		}
	})
}

func TestClassifyListenRunError(t *testing.T) {
	err := classifyListenRunError(errors.New("websocket: bad handshake"))

	var se *errs.SlackError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T, want SlackError", err)
	}
	if se.Code != errs.SlackAPI {
		t.Fatalf("code = %d, want %d", se.Code, errs.SlackAPI)
	}
	if se.Err != "socket_mode_failed" {
		t.Fatalf("error = %q, want socket_mode_failed", se.Err)
	}
	if !strings.Contains(se.Detail, "websocket: bad handshake") {
		t.Fatalf("detail = %q, want underlying error", se.Detail)
	}

	authErr := classifyListenRunError(errors.New("invalid_auth"))
	var authSE *errs.SlackError
	if !errors.As(authErr, &authSE) {
		t.Fatalf("auth err = %T, want SlackError", authErr)
	}
	if authSE.Code != errs.Auth {
		t.Fatalf("auth code = %d, want %d", authSE.Code, errs.Auth)
	}
}
