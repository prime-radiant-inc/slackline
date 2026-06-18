package slack

import (
	"errors"
	"testing"

	goslack "github.com/slack-go/slack"
)

// countingUsersAPI counts GetUsers calls to assert caching / lazy-fetch behavior.
type countingUsersAPI struct {
	fakeAPI
	calls int
}

func (c *countingUsersAPI) GetUsers(opts ...goslack.GetUsersOption) ([]goslack.User, error) {
	c.calls++
	return c.fakeAPI.GetUsers(opts...)
}

func makeUser(id, handle, display, real string) goslack.User {
	u := goslack.User{}
	u.ID = id
	u.Name = handle
	u.RealName = real
	u.Profile.DisplayName = display
	u.Profile.RealName = real
	return u
}

func userFixtures() []goslack.User {
	return []goslack.User{
		makeUser("U1", "drew", "Drew", "Drew Smith"),
		makeUser("U2", "sam", "Sammy", "Sam Jones"),
		makeUser("U3", "pat.lee", "Pat", "Pat Lee"),
	}
}

func TestUserDirectory_ResolveHandle_ByHandle(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	id, ok, err := d.ResolveHandle("drew")
	if err != nil {
		t.Fatalf("ResolveHandle error: %v", err)
	}
	if !ok || id != "U1" {
		t.Fatalf("ResolveHandle(drew) = %q, %v; want U1, true", id, ok)
	}
}

func TestUserDirectory_ResolveHandle_CaseInsensitive(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	id, ok, _ := d.ResolveHandle("Drew")
	if !ok || id != "U1" {
		t.Fatalf("ResolveHandle(Drew) = %q, %v; want U1, true", id, ok)
	}
}

func TestUserDirectory_ResolveHandle_ByDisplayName(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	id, ok, _ := d.ResolveHandle("Sammy")
	if !ok || id != "U2" {
		t.Fatalf("ResolveHandle(Sammy) = %q, %v; want U2, true", id, ok)
	}
}

func TestUserDirectory_ResolveHandle_HandleBeatsName(t *testing.T) {
	// "sam" is U2's handle and also a substring-free exact display name elsewhere?
	// Here ensure the exact handle wins even if another user's real name collides.
	users := []goslack.User{
		makeUser("U2", "sam", "Sammy", "Sam Jones"),
		makeUser("U9", "robert", "Rob", "sam"), // real name collides with handle "sam"
	}
	d := NewUserDirectory(&fakeAPI{users: users})
	id, ok, _ := d.ResolveHandle("sam")
	if !ok || id != "U2" {
		t.Fatalf("ResolveHandle(sam) = %q, %v; want U2 (handle wins), true", id, ok)
	}
}

func TestUserDirectory_ResolveHandle_AmbiguousName(t *testing.T) {
	users := []goslack.User{
		makeUser("U1", "drew1", "Drew", "Drew One"),
		makeUser("U2", "drew2", "Drew", "Drew Two"),
	}
	d := NewUserDirectory(&fakeAPI{users: users})
	_, ok, err := d.ResolveHandle("Drew")
	if err != nil {
		t.Fatalf("ResolveHandle error: %v", err)
	}
	if ok {
		t.Fatal("ResolveHandle(Drew) should be unresolved (ambiguous display name)")
	}
}

func TestUserDirectory_ResolveHandle_Unknown(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	_, ok, _ := d.ResolveHandle("nobody")
	if ok {
		t.Fatal("ResolveHandle(nobody) should be unresolved")
	}
}

func TestUserDirectory_ResolveHandle_SkipsDeleted(t *testing.T) {
	deleted := makeUser("U8", "ghost", "Ghost", "Ghost User")
	deleted.Deleted = true
	d := NewUserDirectory(&fakeAPI{users: []goslack.User{deleted}})
	_, ok, _ := d.ResolveHandle("ghost")
	if ok {
		t.Fatal("ResolveHandle should not resolve a deactivated user")
	}
}

func TestUserDirectory_ResolveHandle_APIError(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{usersErr: errors.New("missing_scope")})
	_, _, err := d.ResolveHandle("drew")
	if err == nil {
		t.Fatal("expected API error to propagate")
	}
}

func TestUserDirectory_Name(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	h, ok, err := d.Name("U1")
	if err != nil {
		t.Fatalf("Name error: %v", err)
	}
	if !ok || h != "drew" {
		t.Fatalf("Name(U1) = %q, %v; want drew, true", h, ok)
	}
}

func TestUserDirectory_Name_Unknown(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	_, ok, _ := d.Name("U404")
	if ok {
		t.Fatal("Name(U404) should be not-ok")
	}
}

func TestUserDirectory_LinkifyMentions_Basic(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, unresolved, err := d.LinkifyMentions("hey @drew ping")
	if err != nil {
		t.Fatalf("LinkifyMentions error: %v", err)
	}
	if out != "hey <@U1> ping" {
		t.Fatalf("LinkifyMentions out = %q", out)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unexpected unresolved: %v", unresolved)
	}
}

func TestUserDirectory_LinkifyMentions_AtStart(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, _, _ := d.LinkifyMentions("@drew heads up")
	if out != "<@U1> heads up" {
		t.Fatalf("LinkifyMentions out = %q", out)
	}
}

func TestUserDirectory_LinkifyMentions_TrailingPunctuation(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, _, _ := d.LinkifyMentions("ping @drew.")
	if out != "ping <@U1>." {
		t.Fatalf("LinkifyMentions out = %q (want trailing period preserved)", out)
	}
}

func TestUserDirectory_LinkifyMentions_HandleWithDot(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, unresolved, _ := d.LinkifyMentions("hi @pat.lee")
	if out != "hi <@U3>" {
		t.Fatalf("LinkifyMentions out = %q", out)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unexpected unresolved: %v", unresolved)
	}
}

func TestUserDirectory_LinkifyMentions_EmailUntouched(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, unresolved, _ := d.LinkifyMentions("mail me at drew@example.com please")
	if out != "mail me at drew@example.com please" {
		t.Fatalf("email should be untouched, got %q", out)
	}
	if len(unresolved) != 0 {
		t.Fatalf("email should not register as unresolved mention: %v", unresolved)
	}
}

func TestUserDirectory_LinkifyMentions_Unresolved(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, unresolved, _ := d.LinkifyMentions("ping @nobody now")
	if out != "ping @nobody now" {
		t.Fatalf("unresolved mention should stay literal, got %q", out)
	}
	if len(unresolved) != 1 || unresolved[0] != "nobody" {
		t.Fatalf("unresolved = %v, want [nobody]", unresolved)
	}
}

func TestUserDirectory_LinkifyMentions_NoAPICallWithoutMentions(t *testing.T) {
	api := &countingUsersAPI{}
	api.users = userFixtures()
	d := NewUserDirectory(api)
	out, _, err := d.LinkifyMentions("no mentions here, just text")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if out != "no mentions here, just text" {
		t.Fatalf("out = %q", out)
	}
	if api.calls != 0 {
		t.Fatalf("GetUsers called %d times; want 0 when no @mention present", api.calls)
	}
}

func TestUserDirectory_EnrichMentions_BareToLabeled(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, err := d.EnrichMentions("hi <@U1> and <@U2>")
	if err != nil {
		t.Fatalf("EnrichMentions error: %v", err)
	}
	if out != "hi <@U1|drew> and <@U2|sam>" {
		t.Fatalf("EnrichMentions out = %q", out)
	}
}

func TestUserDirectory_EnrichMentions_AlreadyLabeled(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, _ := d.EnrichMentions("hi <@U1|olddrew>")
	if out != "hi <@U1|olddrew>" {
		t.Fatalf("already-labeled mention should be untouched, got %q", out)
	}
}

func TestUserDirectory_EnrichMentions_UnknownUntouched(t *testing.T) {
	d := NewUserDirectory(&fakeAPI{users: userFixtures()})
	out, _ := d.EnrichMentions("hi <@U404>")
	if out != "hi <@U404>" {
		t.Fatalf("unknown ID should be untouched, got %q", out)
	}
}

func TestUserDirectory_All_FetchesOnce(t *testing.T) {
	api := &countingUsersAPI{}
	api.users = userFixtures()
	d := NewUserDirectory(api)
	if _, err := d.All(); err != nil {
		t.Fatalf("All error: %v", err)
	}
	// A second populate-triggering call must not refetch.
	_, _, _ = d.ResolveHandle("drew")
	if api.calls != 1 {
		t.Fatalf("GetUsers called %d times; want 1 (cached)", api.calls)
	}
}
