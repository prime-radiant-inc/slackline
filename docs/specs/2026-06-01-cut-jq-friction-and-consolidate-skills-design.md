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
- Implemented as `ListenerOptions.Types` (a `map[string]bool`, empty = emit all). `emit()` drops events whose `Type` isn't in the set. Type validation + the `channel_message`/`--all-messages` check live in `cmd/listen.go` before constructing the listener.

### 2. Unified `reaction` event

Collapse `reaction_added` / `reaction_removed` into one event type `reaction` with an `action` field (`"added"` | `"removed"`), mirroring the `react` command's existing `action` output.

- `events.go`: replace `EventTypeReactionAdded`/`EventTypeReactionRemoved` with `EventTypeReaction = "reaction"`; add `Action string \`json:"action,omitempty"\`` to `Event`.
- `listener.go`: both reaction branches emit `Type: EventTypeReaction` with `Action: "added"`/`"removed"`.
- The Slack-side manifest still subscribes to both `reaction_added` and `reaction_removed` Slack events (unchanged — `provision/manifest_test.go` golden is unaffected; those are Slack event names, not slackline output types).

Breaking change to `listen` output schema.

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

### 4. Skill restructure

- Fold `skills/slackline-provision-bot/` into `using-slack` as progressive disclosure:
  - `skills/using-slack/SKILL.md` — main reference (gains a short "Provisioning a new bot" pointer).
  - `skills/using-slack/provisioning.md` — the admin/browser recipe (moved from the provision skill body).
  - `skills/using-slack/copy-buttons.md` — selector reference (moved from the provision skill's companion).
  - Delete `skills/slackline-provision-bot/`.
- Rewrite `using-slack/SKILL.md` against the cleaner UX: `--limit 1` for newest, `listen --type` instead of `jq .type` loops, `ask` exit-code branching, unified `reaction` event. Shorten the description to triggers only.
- Repoint references: README "Provisioning a new bot" section, and the `using-slack-at-prime-radiant` skill (primeradiant-ops) which references the provision skill by name.

## Testing

- `cmd/listen_test.go` (or helpers): `--type` parsing/validation (valid set, unknown→error, `channel_message` without `--all-messages`→error). Existing listener tests assert reaction output → update to the unified `reaction`/`action` shape.
- `listen/listener_test.go`: `emit()` filters by `Types`; reaction branches emit `reaction` + `action`.
- `cmd/ask` test: timeout path returns exit code 5 (Timeout). 
- `errs` test: `Timeout` maps to 5.
- All via the existing fake-based command tests; no live Slack.

## Versioning & docs

- `v0.3.0`: CHANGELOG (`Added`: `listen --type`; `Changed`: unified `reaction` event, `ask` timeout exit code), `plugin.json` 0.2.3→0.3.0, README (`listen` flags + event reference + exit-code table), then `make release VERSION=0.3.0`.

## Out of scope (YAGNI)

- `read --replies-only` — `--limit 1` covers "newest reply"; full replies-without-parent isn't a demonstrated need.
- Auto-enabling the firehose from `--type` — explicit `--all-messages` preferred.
