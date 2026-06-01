# slackline: cut jq friction + consolidate Slack skills (PRI-2017)

## Problem

Writing the `using-slack` skill exposed where slackline forces agents to chain `jq`/`tail` or parse stderr to do ordinary things:

- **Watching for events.** The common task — "react to mentions" — requires `slackline listen | while read line; do [ "$(jq -r .type <<<"$line")" = mention ] && …; done`. There's no way to ask for only the events you want.
- **Two reaction event types.** `reaction_added` and `reaction_removed` are separate `type` values, so "any reaction" means matching two strings.
- **`ask` reply vs. timeout.** Timeout and API errors both exit `1`, so callers must parse the stderr `"error"` field to tell them apart.

(The `read … | tail -n1` smell was self-inflicted in the skill — `--limit 1` already returns the newest line. No tool change needed; the skill rewrite fixes it.)

## Scope

Three binary changes (`v0.3.0`, minor — two are breaking) plus a skill restructure.

### 1. `listen --type <types>`

A comma-separated allowlist filter applied at the single `emit()` choke point in `listen/listener.go`. With no `--type`, behavior is unchanged.

- Valid types: `mention`, `dm`, `thread_reply`, `channel_message`, `reaction`.
- Unknown type → usage error (exit 4) naming the valid set. Kills silent typos (`--type mentions`).
- `channel_message` is only produced under the firehose, so `--type channel_message` **without** `--all-messages` → usage error: "channel_message events require --all-messages". (Explicit over magic — chosen over auto-enabling.)
- Implemented as `ListenerOptions.Types` (a `map[string]bool`, empty = emit all). `emit()` drops events whose `Type` isn't in the set. Type validation + the `channel_message`/`--all-messages` check live in `cmd/listen.go` and **must run before `loadConfig()`/`AuthTest()`** (currently `runListen:33,:46`), not merely "before constructing the listener." `runListen` does a live `auth.test` near the top; if validation lands after it, the usage-error tests can't be exercised without a configured bot + network, breaking the "no live Slack" test claim. Put the `--type` parse/validate first thing in `runListen`.
- **`--type` is an emit-time filter, not a subscription widener.** `thread_reply` is emitted for bot-parent threads by default and for all threads only under `--all-messages` (`listener.go:177,190`). So `--type thread_reply` honors whichever subscription is active — it does NOT widen coverage to all threads. Document this in the flag help and skill so it isn't a silent under-delivery surprise. (`channel_message` is the one type with no default subscription, hence its explicit `--all-messages` guard above.)

### 2. Unified `reaction` event

Collapse `reaction_added` / `reaction_removed` into one event type `reaction` with an `action` field (`"added"` | `"removed"`), mirroring the `react` command's existing `action` output.

- `events.go`: replace `EventTypeReactionAdded`/`EventTypeReactionRemoved` with `EventTypeReaction = "reaction"`; add `Action string \`json:"action,omitempty"\`` to `Event`.
- `listener.go`: both reaction branches (`:211,:223`) emit `Type: EventTypeReaction` with `Action: "added"`/`"removed"`.
- The Slack-side manifest still subscribes to both `reaction_added` and `reaction_removed` Slack events (unchanged — `provision/manifest_test.go` golden is unaffected; those are Slack event names, distinct constants in `provision/manifest.go`, not slackline output types).

Breaking change to `listen` output schema. **Note this reverses a prior deliberate split:** v0.2.x renamed `reaction` → `reaction_added` and added `reaction_removed` (see README Migration). The `action` field preserves the add/remove distinction that split was for, so this is a net simplification — but the README Migration log and reaction event-reference sections must be reconciled, not just appended to (see Versioning & docs).

### 3. `ask` timeout exit code

Add `Timeout = 5` to the `errs` code taxonomy. `ask`'s timeout path returns `Code: Timeout` instead of `SlackAPI`. The stderr JSON (`{"error":"timeout","detail":…}`) is unchanged. Callers branch cleanly:

```bash
if out=$(slackline ask --channel '#ops' --message 'q?' --timeout 120); then
  echo "reply: $out"
elif [ $? -eq 5 ]; then
  echo "timed out"
else
  echo "error"
fi
```

Breaking change to the exit code for timeout (was 1).

**Testability refactor (required):** unlike `react`/`send`/`download`, `runAsk` (`cmd/ask.go:40`) builds its client internally (`slackpkg.NewClient`, `:68`), runs `AuthTest` + `resolver.Resolve` itself (`:68,:71,:80`), and calls real `time.Sleep(pollInterval)` (`:102`) — there is no injection seam and no `cmd/ask_test.go`. Extract a `runAskWithAPI(api slackpkg.SlackAPI, …)` seam: move `AuthTest` (for bot user id) and channel resolution into the `RunE` wrapper (react's seam takes an already-resolved channel and does no AuthTest, so this is a looser "mirror" — the wrapper must do more), and pass the resolved channel + `api` in.

**The timeout is wall-clock, not poll-derived — get this right or the test hangs.** The loop computes `deadline := now + askTimeout` (`:98`) and breaks on `time.Now().After(deadline)` (`:103`), independent of `pollInterval`. So zeroing the poll sleep alone does NOT fire the timeout — with a positive `askTimeout` and a fake returning no replies you get a busy infinite loop. To test the timeout→exit-5 path deterministically: **inject a `now func() time.Time` clock** (preferred — also lets the reply-before-deadline path be tested), or as a cruder alternative drive `--timeout 0` (deadline already past) *and* inject a no-op poll sleep so iteration 1 times out instantly.

### 4. Skill restructure

- Fold `skills/slackline-provision-bot/` into `using-slack` as progressive disclosure:
  - `skills/using-slack/SKILL.md` — main reference (gains a short "Provisioning a new bot" pointer).
  - `skills/using-slack/provisioning.md` — the admin/browser recipe (moved from the provision skill body).
  - `skills/using-slack/copy-buttons.md` — selector reference (moved from the provision skill's companion).
  - Delete `skills/slackline-provision-bot/`.
- Rewrite `using-slack/SKILL.md` against the cleaner UX. This is a body rewrite, not just the `:14` pointer — the current body has several spots that the v0.3.0 behavior makes wrong: the "ask reply vs. timeout" section (exit-1 guidance + stderr-parsing workaround), the `listen` default-types line and event examples naming `reaction_added`/`reaction_removed`, the exit-code table row "1 | Slack API error (includes ask timeout)", and the `jq .type` mention/file loops. Replace with `--limit 1` for newest, `listen --type`, `ask` exit-code branching, unified `reaction` event.
- **Description must keep provisioning triggers.** The standalone skill triggered on "create / deploy / provision / set up a bot"; `using-slack`'s current description has none of that vocabulary. "Shorten to triggers" must still *merge in* the provisioning triggers, or "provision a new bot" requests match no skill — a discovery regression. (Provisioning detail stays progressively disclosed in `provisioning.md`; only the trigger words go in the description.)
- Repoint **every** live reference to the deleted skill. Full set (verified via grep — `docs/specs/*` and `docs/plans/*` are historical and left as-is):
  - `README.md:200,228` — "Provisioning a new bot" section.
  - `CLAUDE.md:55` and `AGENTS.md:55` — both describe the recipe as living in `skills/slackline-provision-bot/SKILL.md`; update the path to `skills/using-slack/provisioning.md`.
  - `CLAUDE.md:35` and `AGENTS.md:35` — the `listen/` event-type list names `reaction_added`, `reaction_removed`; update to the unified `reaction` (with `action`). These are doc-of-record for agents working in the repo.
  - `skills/using-slack/SKILL.md:14` — its own prerequisites pointer ("use the `slackline-provision-bot` skill"). The target is now a **section/file of this skill**, not an invocable skill name — reword to "see the Provisioning section below" / `provisioning.md`.
  - `.claude-plugin/marketplace.json:12` — plugin description says "Bundles the using-slack and slackline-provision-bot skills"; drop the dead name.
  - `cc-plugin-primeradiant-ops` → `skills/using-slack-at-prime-radiant/SKILL.md:43` — cross-repo, ships/versions independently. Reword to point at the using-slack provisioning section. Coordinate: there's a window where the slackline plugin no longer has `slackline-provision-bot` but a not-yet-updated downstream skill still names it — land both before announcing.

## Testing

- `cmd/listen_test.go` (or helpers): `--type` parsing/validation (valid set, unknown→error exit 4, `channel_message` without `--all-messages`→error), and that the emit filter drops non-matching types.
- **Removing `EventTypeReactionAdded`/`EventTypeReactionRemoved` breaks compilation of two existing test files — both must be updated or the `listen` package test build fails (not just an assertion):**
  - `listen/events_test.go:57,63,64,78,84,85` — `TestReactionAddedEvent_JSON` / `TestReactionRemovedEvent_JSON` → fold into a single `reaction`+`action` test.
  - `listen/listener_test.go:282,328` — reaction-output assertions → update to `reaction`/`action`.
- `listen/listener_test.go`: `emit()` filters by `Types` (add coverage).
- `cmd/ask` test (NEW file): timeout path returns exit code 5 (Timeout), reply path returns 0. Requires the `runAskWithAPI` + injectable-poll seam from §3 — there is no `cmd/ask_test.go` today and no existing fake seam for `ask`.
- `errs` test: `Timeout` maps to 5; confirm 5 was previously unused.
- No live Slack — but note this needs the new `ask` seam, not just "existing fake-based tests."

## Versioning & docs

`v0.3.0`, then `make release VERSION=0.3.0`. Specific edits (the reviewers found the generic "update README" was hiding several concrete touch-points):

- **CHANGELOG**: `Added` `listen --type`; `Changed` unified `reaction` event + `ask` timeout exit code.
- **`plugin.json`** 0.2.3 → 0.3.0. (`marketplace.json` slackline entry has no version field — uses commit SHA — so no bump there, but its description string is edited per §4.)
- **README event reference** (`:266-276`): collapse the `### reaction_added` and `### reaction_removed` sections into one `### reaction` with the `action` field shown.
- **README Migration** (`:286-290`): the existing `### reaction → reaction_added` entry is now historically inverted by this change. Add a new entry `### reaction_added / reaction_removed → reaction` (match `"type":"reaction"` + read `action`); reconcile the old entry so the log reads coherently rather than contradicting itself.
- **README `ask`** (`:100`): "exits 1 on timeout" → exit 5.
- **Exit-code tables — BOTH** need a row 5: the **README Exit Codes table** (`:180-188`) and the **`using-slack/SKILL.md` exit-code table** (`:88-95`), which both currently stop at 4. Add `| 5 | Timeout (ask received no reply) |` to each. (§4's "fix the row-1 wording" is not enough — the skill table also needs the new row.)
- **README `listen` flags table** (`~:110`): add `--type`; and the **`(none)` row** (`:112`) names `reaction_added`/`reaction_removed` — update to `reaction`.
- **README `listen` usage synopsis** (`:105`): `slackline listen [--threads] [--all-messages] [--include-bot-self]` omits `--type`.
- **In-binary help text** (so `--help` matches behavior): `cmd/ask.go:27` `Long` ("exits 1 on timeout") and `cmd/listen.go:28` `Long`/flag help (mention `--type`).
- **CHANGELOG/Migration:** match the existing dated-header format (`## [0.3.0] - <date>`, no `[Unreleased]` block — the repo doesn't use one). Use the actual release date; if that's still 2026-06-01 it collides with the existing `[0.2.3]` header — fine, but make it a deliberate choice rather than an accident.

## Out of scope (YAGNI)

- `read --replies-only` — `--limit 1` covers "newest reply"; full replies-without-parent isn't a demonstrated need.
- Auto-enabling the firehose from `--type` — explicit `--all-messages` preferred.
