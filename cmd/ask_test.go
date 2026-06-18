package cmd

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/prime-radiant-inc/slackline/errs"
	slackpkg "github.com/prime-radiant-inc/slackline/slack"
	goslack "github.com/slack-go/slack"
)

func TestRunAskWithAPI_Timeout(t *testing.T) {
	api := &fakeSlackAPI{} // no replies queued
	// Clock that jumps past the deadline after the first (deadline-computing) call.
	calls := 0
	base := time.Unix(1_000_000, 0)
	now := func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(time.Hour)
	}
	err := runAskWithAPI(api, "C123", "UBOT", "hi", 300, 10, now, func(time.Duration) {}, &bytes.Buffer{})
	var se *errs.SlackError
	if !errors.As(err, &se) || se.Code != errs.Timeout {
		t.Fatalf("err = %v, want Timeout SlackError", err)
	}
}

func TestRunAskWithAPI_Reply(t *testing.T) {
	api := &fakeSlackAPI{
		repliesMessages: []goslack.Message{makeMessage("200.1", "U_other", "here you go")},
	}
	base := time.Unix(1_000_000, 0)
	now := func() time.Time { return base } // never advances; deadline never reached
	out := &bytes.Buffer{}
	err := runAskWithAPI(api, "C123", "UBOT", "hi", 300, 10, now, func(time.Duration) {}, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "200.1 U_other here you go\n"
	if out.String() != want {
		t.Fatalf("reply output = %q, want %q", out.String(), want)
	}
}

func TestRunAskWithAPI_JSONFormat(t *testing.T) {
	api := &fakeSlackAPI{
		repliesMessages: []goslack.Message{makeMessage("200.1", "U_other", "here you go")},
	}
	base := time.Unix(1_000_000, 0)
	out := &bytes.Buffer{}
	err := runAskWithAPIFormat(api, "C123", "UBOT", "hi", 300, 10, outputFormatJSON, func() time.Time { return base }, func(time.Duration) {}, out, nil, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"text":"here you go"`)) {
		t.Fatalf("JSON reply not written to out: %q", out.String())
	}
}

func TestRunAskWithAPIFormat_ResolvesReplyNames(t *testing.T) {
	api := &fakeSlackAPI{
		users:           drewUser(), // U1 -> drew
		repliesMessages: []goslack.Message{makeMessage("200.1", "U1", "ok <@U1>")},
	}
	base := time.Unix(1_000_000, 0)
	out := &bytes.Buffer{}
	dir := slackpkg.NewUserDirectory(api)
	err := runAskWithAPIFormat(api, "C123", "UBOT", "hi", 300, 10, outputFormatText, func() time.Time { return base }, func(time.Duration) {}, out, dir, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "200.1 U1|drew ok <@U1|drew>\n"
	if out.String() != want {
		t.Fatalf("reply output = %q, want %q (author + mention resolved)", out.String(), want)
	}
}
