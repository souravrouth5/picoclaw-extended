# Missing Tools — Priority List

## High Priority

| # | Tool | Why |
|---|------|-----|
| 1 | `grep_file` / `search_in_files` | Search for text patterns across files. Without this, the LLM has to blindly `read_file` everything. Most impactful missing tool for coding/file tasks. |
| 2 | `move_file` / `copy_file` / `delete_file` | Basic file management. Currently only doable via `exec`, which is blocked on remote channels and restricted by deny patterns. |
| 3 | `http_request` | Make arbitrary HTTP calls (POST/PUT/DELETE with custom headers/body). `web_fetch` only does GET for HTML. Needed for calling APIs, webhooks, REST services. |
| 4 | `remember` / `recall` | Explicit memory tool. The memory system exists in `pkg/memory/` but there's no tool exposing it to the LLM for on-demand save/retrieve. |

## Medium Priority

| # | Tool | Why |
|---|------|-----|
| 5 | `image_generate` | Call an image generation API (DALL-E, Stable Diffusion, etc.). The `send_file` + `media` infrastructure is already in place to deliver the result. |
| 6 | `diff_file` | Show a unified diff between two files or old/new content. Avoids reading full files just to review changes. |
| 7 | `env_get` | Read environment variables. Useful for config inspection without needing shell access. |
| 8 | `notify` | Send a push notification (e.g. Pushover, ntfy.sh) on async task completion. Complements the existing `spawn`/async tools. |

## Lower Priority

| # | Tool | Why |
|---|------|-----|
| 9 | `json_query` | Run a JQ-style query on a JSON file or string. Avoids loading large JSON into context just to extract one field. |
| 10 | `git_status` / `git_diff` | Read-only git operations. `exec` deny list already allows these commands, but wrapping them as dedicated tools makes them available on remote channels too. |

---

## Implementation Notes

- All tools go in `pkg/tools/` — one file per tool
- Register each in `pkg/agent/instance.go` alongside existing `toolsRegistry.Register(...)` calls
- Use `SilentResult` for operations the user doesn't need to see, `UserResult` for output they should see, `ErrorResult` for failures
- See `pkg/tools/base.go` for the `Tool` interface
