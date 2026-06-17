# Project Dictionary

Normative: docs, code identifiers, commit messages, and UI strings use these
terms as defined. Divergences live in Exceptions, nowhere else. Maintained by
the superpowers-docs documentation skill; `docmaint scan` enforces the
`Use instead of:` lines mechanically (this file is excluded from its own sweep).

<!--
Entry format (parsed by docmaint — keep the grammar exact):
  ### term                 — the heading IS the canonical term/replacement
  1–2 sentence definition: what kind of thing + what distinguishes it.
  Distinct from: *neighbor* (one clause on the difference).      [optional]
  Use instead of: syn1, homograph [manual] (reason).             [optional]
    - comma-separated plain terms, no markup
    - [manual] = homograph; agents check it in audits, scan skips it.
      Applies per-term — tag each homograph individually.
    - one trailing (parenthetical) reason allowed; scan strips it
      before parsing the synonyms
Inclusion bar: project-specific, ambiguous, or non-standard usage only.
The dictionary defines; it never explains — link to the owning doc instead.
-->

## Terms

These entries pin the two clusters readers actually conflate: the four Slack
token types, and the three lifecycle commands (`provision`, `bootstrap`,
`init`) with their two config files. Definitions only — the owning docs
(`README.md`, the `using-slack` skill) carry the how.

### App Configuration Token

The admin credential pair (access token `xoxe.xoxp-…` plus refresh token
`xoxe-…`) for Slack's app-configuration API; stored in `provision.json` and
used only by `slackline provision` to create apps and rotate tokens.
Distinct from: *bot token* and *app token* (those are a single bot's runtime
identity, not an admin credential).

### app token

A bot's Socket Mode connection token (`xapp-…`); required by `slackline listen`
to open the event websocket.
Distinct from: *bot token* (the `xoxb-…` REST identity); *App Configuration
Token* (the admin credential).

### bootstrap

The `slackline provision bootstrap` step: a one-time-per-machine action that
seeds `provision.json` with App Configuration Tokens. It does not contact Slack.
Distinct from: *provision* (`provision NAME`, which calls Slack to create an
app); *init* (which writes a per-developer `config.json`).

### bot token

A bot's runtime identity (`xoxb-…`), used for every REST call (`send`, `read`,
`react`, `download`, `channels`, `auth`).
Distinct from: *app token* (Socket Mode, `xapp-…`); *App Configuration Token*
(admin credential, `xoxe…`).

### config.json

The runtime config file (`~/.config/slackline/config.json`): one bot's identity
(bot token, app token, workspace info). Read by every runtime command.
Distinct from: *provision.json* (admin tokens, read only by `slackline
provision`).

### init

The `slackline init` command: writes `config.json` from already-provisioned
`xoxb-`/`xapp-` tokens (env vars or interactive prompt). It validates
the bot token via `auth.test` and never touches `provision.json`.
Distinct from: *provision* (which creates the Slack app); *bootstrap* (which
seeds the admin tokens).

### provision

The admin `slackline provision NAME` command: creates a Slack app through the
app-configuration API and emits install and OAuth URLs as JSON. Requires a
seeded `provision.json`.
Distinct from: *init* (which consumes already-issued bot/app tokens);
*bootstrap* (which seeds the admin tokens `provision` depends on).

### provision.json

The admin-only config file (`~/.config/slackline/provision.json`): the App
Configuration Token and its refresh token. Read only by `slackline provision`;
never distributed to non-admin machines.
Distinct from: *config.json* (a bot's runtime identity).

## Names

<!--
Names also state exact spelling/capitalization and a location (path, command,
or upstream URL). scan flags case-variants of the canonical spelling
automatically; list spacing/hyphenation variants in Use instead of:.
-->

<!-- No Name entries: the project name `slackline` is spelled consistently
(lowercase in prose and as the command) and is not conflated or misspelled, so
per the dictionary's inclusion bar it earns no entry. Capitalized "Slackline"
in dated specs/plans is sentence/title case and those docs are point-in-time. -->

## Exceptions

<!--
Format (parsed by docmaint):
  - `term` — `glob`[, `glob`…]; reason, tracking pointer. [temporary|permanent]
Scopes are path globs only — never prose predicates.
[temporary] needs a tracking pointer; scan reports it as a removal candidate
when the term has zero matches inside its glob-matched files (confirm via
git log -S before removing).
[permanent] is never flagged; zero current matches doesn't expire it.
-->

<!-- No exceptions: no entry above carries a `Use instead of:` line, so scan
has nothing to enforce and nothing to except. -->

## Voice

Resolved per the superpowers-docs voice reference. One artifact, one voice;
sections may change audience, never speaker.

- **Engineering docs** (`README.md`, `CLAUDE.md`, `AGENTS.md`, the `using-slack`
  skill): **the engineer who built it** (preset 1). Flat declaratives,
  mechanism first, the fact carries the weight. Dry humor at most once, only
  when load-bearing.
- **Adopter docs** (`docs/BROCHURE.md`): **the publication writer** (preset 5).
  Lead with what the reader can do; plain English; no contrastive negation
  ("not X, but Y"); no em dashes in copy. Every benefit traces to a cited
  capability.

Exemplars (from this repo's own prose — the enforcement instrument):

- "Give AI agents a Slack identity. Send messages, read channels, stream
  real-time events."
- "The bot can only read, ask, and listen in channels it has joined."
- "Status is on stderr; stdout is only events."
- "App tokens can't be validated via the REST API."

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `a8c5108` · verified against code (1 claim deferred to review)._
