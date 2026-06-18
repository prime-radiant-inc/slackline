package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

const fixtureChannelName = "ops"

func TestWriteChannelsOutput_Text(t *testing.T) {
	ch := goslack.Channel{}
	ch.ID = fixtureChannelID
	ch.Name = fixtureChannelName
	ch.Purpose.Value = "operations"
	var out bytes.Buffer

	if err := writeChannelsOutput(&out, outputFormatText, []goslack.Channel{ch}); err != nil {
		t.Fatalf("writeChannelsOutput failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "#ops") {
		t.Fatalf("text output missing table fields: %q", got)
	}
}

func TestWriteChannelsOutput_JSON(t *testing.T) {
	ch := goslack.Channel{}
	ch.ID = fixtureChannelID
	ch.Name = fixtureChannelName
	ch.Topic.Value = "deploys"
	ch.Purpose.Value = "operations"
	var out bytes.Buffer

	if err := writeChannelsOutput(&out, outputFormatJSON, []goslack.Channel{ch}); err != nil {
		t.Fatalf("writeChannelsOutput failed: %v", err)
	}

	var decoded []map[string]string
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 1 || decoded[0]["id"] != fixtureChannelID || decoded[0]["name"] != fixtureChannelName {
		t.Fatalf("decoded output = %#v", decoded)
	}
}
