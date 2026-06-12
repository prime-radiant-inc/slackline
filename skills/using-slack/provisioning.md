# Provisioning a new slackline bot

Use this recipe when you need to deploy a new Slackline bot in a Slack workspace. The flow assumes:

- `slackline` binary is installed and on PATH.
- A workspace admin's browser session is established on this host (one-time per machine — Chrome/Playwright/etc. is signed into Slack as an admin).
- `slackline provision bootstrap` has been run on this host once to seed `~/.config/slackline/provision.json` with App Configuration Tokens. (If not, run it first; see `## Bootstrap` below.)
- You have access to a browser-driving tool (`use_browser`, Playwright MCP, or equivalent).

The full agentic recipe is below.

## End-to-end recipe

```bash
# 1. Provision the app via API.
slackline provision my-bot-name > /tmp/prov.json
# stdout: {"ok":true,"app_id":"A...","team_id":"T...","team_domain":"...","effective_name":"...","install_url":"...","oauth_authorize_url":"...","oauth_page_url":"...","general_page_url":"..."}
# Also check stderr: a `warning:` line appears if Slack mutated the app name
# (e.g. stripped dashes — see PRI-1618). When that happens, the bot was
# registered under `effective_name`, not the name you passed.

INSTALL_URL=$(jq -r .install_url /tmp/prov.json)
OAUTH_PAGE=$(jq -r .oauth_page_url /tmp/prov.json)
GENERAL_PAGE=$(jq -r .general_page_url /tmp/prov.json)
```

```text
# 2. Drive browser:
#    - navigate to $INSTALL_URL   (admin /install-on-team page — canonical entry)
#    - click the "Install to Workspace" button on that page
#    - click button[data-qa="oauth_submit_button"]   ("Allow")
#    - navigate to $OAUTH_PAGE
#    - click button.c-button.c-button--outline.c-button--small  (the "Copy" button — one per page on /oauth)
#    - read clipboard via `pbpaste` → save bot token (xoxb-)
#    - navigate to $GENERAL_PAGE
#    - click button.c-button.c-button--outline.c-button--medium.margin_top_150  ("Generate Token and Scopes")
#    - dismiss any "Got it!" tutorial popup
#    - type "socket-mode" into input[name="app_level_tokens_generate_modal_description"]
#    - click button[data-qa="app_scopes_list_add_oauth_scope"]  ("Add Scope")
#    - type "connections:write" into input[role="combobox"][aria-label="Select Scopes"]
#    - press Enter (commits the option — direct .click() on the option doesn't fire React handler)
#    - click button.c-button.c-button--primary.c-button--medium  ("Generate")
#    - click button.p-app_level_tokens_info__input_button  (the "Copy" button on the new app-level token modal)
#    - read clipboard via `pbpaste` → save app token (xapp-)
```

```bash
# 3. Configure slackline with the captured tokens.
SLACKLINE_BOT_TOKEN="$BOT_TOKEN" \
SLACKLINE_APP_TOKEN="$APP_TOKEN" \
SLACKLINE_WORKSPACE_URL="https://${WORKSPACE_DOMAIN}.slack.com" \
slackline init
# Writes ~/.config/slackline/config.json. Validates via auth.test.

# 4. Smoke test.
slackline auth status
slackline channels --all
```

The bot is now provisioned and ready. To actually post in a channel, the bot needs to be invited (`/invite @bot-name` from any channel member). Driving that through the browser is similar.

## Critical browser-automation gotchas

1. **Use real CDP clicks, not JS-driven `element.click()`.** Slack's React handlers don't fire on synthetic JS click events for the Copy buttons. With `use_browser`, the `click` action does the right thing; the `eval` action with `el.click()` does not.

2. **The clipboard read pattern only works after a real click.** First click → check clipboard with `pbpaste`. If the clipboard is empty, the click didn't trigger Slack's copy handler.

3. **Combobox selection requires Enter, not click.** The "Select Scopes" combobox in the generate-app-level-token modal: type the scope name, then press Enter. Clicking the option in the listbox does NOT add it to the form.

4. **Slack's `/messages/<channel-id>` page sometimes shows "couldn't load" briefly while the web app boots.** Use `await_element` for the composer (`div[role="textbox"][aria-label^="Message"]`) before typing.

5. **`/invite @bot-name` slash command in a channel:** type `/invite @<name>`, press Enter — that triggers Slack's autocomplete which resolves the @mention to a real user. Press Enter AGAIN to actually send the slash command. Two Enters total.

For the full selector reference, see `copy-buttons.md` next to this file.

## Bootstrap (one-time per machine)

Run before any per-bot provisioning:

```bash
# Option A: env vars (CI/scripts).
SLACKLINE_CONFIG_TOKEN=xoxe.xoxp-... \
SLACKLINE_REFRESH_TOKEN=xoxe-... \
slackline provision bootstrap

# Option B: interactive (paste from https://api.slack.com/apps "Your App Configuration Tokens").
slackline provision bootstrap
# Prompts for both tokens via stdin.
```

Bootstrap writes `~/.config/slackline/provision.json` with mode 0600.

## Migration note

The old `slackline create` command has been removed. It returns a discoverable migration error pointing here. Update any scripts referencing `slackline create --init` / `slackline create --name X` to use `slackline provision bootstrap` and `slackline provision <name>` respectively, plus `slackline init` (with env vars) for the per-bot config.json write.

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `a8c5108` · verified against code (2 claims deferred to review)._
