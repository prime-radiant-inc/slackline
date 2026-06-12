# Slack Admin UI — selector reference

These selectors were verified 2026-04-25 against api.slack.com. Slack updates their admin UI periodically; if a step fails, re-grep the rendered DOM for an `aria-label` or `data-qa` close to the broken selector — those tend to be the most stable.

## Configure App Configuration Tokens
URL: `https://api.slack.com/apps`

| Action | Selector |
|---|---|
| "Generate Token" button (under Your App Configuration Tokens) | `button.c-button.c-button--outline.c-button--medium` containing text "Generate Token" — there is one such button on the page |
| Workspace dropdown in the modal | `[role="combobox"][aria-label="Select a team"]` |
| Workspace option | `#team-picker_option_0` (or `[data-qa="team_picker_option_0"]`) |
| "Generate" button (modal) | `button.c-button.c-button--primary.c-button--medium` |
| Copy access token (post-generate row) | `button[aria-label="Copy access token"]` |
| Copy refresh token | `button[aria-label="Copy refresh token"]` |

## OAuth install URL
URL: `https://slack.com/oauth/v2/authorize?client_id=…&team=…&scope=…` (returned in `provision`'s `oauth_authorize_url` field)

| Action | Selector |
|---|---|
| Allow button | `button[data-qa="oauth_submit_button"]` |

## Bot token page
URL: `https://api.slack.com/apps/<app_id>/oauth`

| Action | Selector |
|---|---|
| Copy bot token (xoxb-) | `button.c-button.c-button--outline.c-button--small` (single Copy button on this page) |

## App-level token generation page
URL: `https://api.slack.com/apps/<app_id>/general`

| Action | Selector |
|---|---|
| "Generate Token and Scopes" | `button.c-button.c-button--outline.c-button--medium.margin_top_150` |
| Token name input | `input[name="app_level_tokens_generate_modal_description"]` |
| "Add Scope" button | `button[data-qa="app_scopes_list_add_oauth_scope"]` |
| Scope search combobox | `input[role="combobox"][aria-label="Select Scopes"]` |
| Scope option (after typing) | `[data-qa="app_scopes_picker_option_0"]` — but DO NOT click; press Enter to commit selection |
| "Generate" button (modal) | `button.c-button.c-button--primary.c-button--medium` |
| Copy app token (xapp-) (post-generate modal) | `button.p-app_level_tokens_info__input_button` |
| Dismiss "Got it!" tutorial popup if it appears | the button containing literal text "Got it!" |

---
<!-- doc-audit:last-reviewed -->
_Last reviewed: 2026-06-11 · commit `a8c5108` · verified against code (16 claims deferred to review)._
