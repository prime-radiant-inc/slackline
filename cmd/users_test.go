package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	goslack "github.com/slack-go/slack"
)

func usersFixture() []goslack.User {
	return []goslack.User{
		cmdMakeUser("U1", fixtureHandle, fixtureDisplay, fixtureReal),
		cmdMakeUser("U2", "sam", "Sammy", "Sam Jones"),
	}
}

func TestFilterUsers_NoMatchReturnsAllActive(t *testing.T) {
	deleted := cmdMakeUser("U3", "ghost", "Ghost", "Ghost User")
	deleted.Deleted = true
	users := append(usersFixture(), deleted)

	got := filterUsers(users, "")
	if len(got) != 2 {
		t.Fatalf("filterUsers returned %d users, want 2 (deleted excluded)", len(got))
	}
}

func TestFilterUsers_MatchHandle(t *testing.T) {
	got := filterUsers(usersFixture(), "drew")
	if len(got) != 1 || got[0].ID != "U1" {
		t.Fatalf("filterUsers(drew) = %+v, want [U1]", got)
	}
}

func TestFilterUsers_MatchDisplayCaseInsensitive(t *testing.T) {
	got := filterUsers(usersFixture(), "sammy")
	if len(got) != 1 || got[0].ID != "U2" {
		t.Fatalf("filterUsers(sammy) = %+v, want [U2]", got)
	}
}

func TestFilterUsers_MatchRealName(t *testing.T) {
	got := filterUsers(usersFixture(), "jones")
	if len(got) != 1 || got[0].ID != "U2" {
		t.Fatalf("filterUsers(jones) = %+v, want [U2]", got)
	}
}

func TestFilterUsers_MatchByID(t *testing.T) {
	got := filterUsers(usersFixture(), "U1")
	if len(got) != 1 || got[0].ID != "U1" {
		t.Fatalf("filterUsers(U1) = %+v, want [U1]", got)
	}
}

func TestWriteUsersOutput_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeUsersOutput(&buf, outputFormatJSON, usersFixture()); err != nil {
		t.Fatalf("writeUsersOutput failed: %v", err)
	}
	var got []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d users, want 2", len(got))
	}
	if got[0]["id"] != "U1" || got[0]["handle"] != fixtureHandle || got[0]["display_name"] != fixtureDisplay || got[0]["real_name"] != fixtureReal {
		t.Fatalf("user[0] = %+v", got[0])
	}
}

func TestWriteUsersOutput_Text(t *testing.T) {
	var buf bytes.Buffer
	if err := writeUsersOutput(&buf, outputFormatText, usersFixture()); err != nil {
		t.Fatalf("writeUsersOutput failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "U1") || !strings.Contains(out, fixtureHandle) || !strings.Contains(out, fixtureReal) {
		t.Fatalf("text output missing expected fields:\n%s", out)
	}
}
