---
id: TUI
status: implemented
version: V1
depends_on: [CFG, META, STRAT, ROUTE, EXEC, HIST, REFRESH]
modules: [cmd/agent-factory, internal/cli]
---
# TUI

Goal: an interactive terminal screen (built with Bubble Tea, D-016) and the app's entry point. Presentation and wiring only; all logic lives in application (ARCH-3).

## Rules

### Layout and dispatch

- TUI-1 `agent-factory` MUST open an interactive screen with a scrollable center output area and a bottom input box.
- TUI-2 MUST classify each submitted line by its first word only: a tool command (ROUTE-7) goes to its application handler with the rest of the line passed through unparsed; everything else goes to ROUTE unchanged, including text with no routing command (refused per ROUTE-3/ROUTE-4). The TUI MUST NOT validate arguments or select models (ARCH-3).
- TUI-3 MUST render tool and model output in the center area. Model output is labeled with the model name. After a successful run MUST render the usage line (EXEC-5).
- TUI-4 MUST render streamed chunks incrementally (EXEC-2).
- TUI-6 MUST prefix every warning line with `warning: ` and every error line with `error: `. Color MAY be added but MUST NOT be the only difference.
- TUI-7 MUST NOT display API keys anywhere (SEC-1).
- TUI-8 `/help` MUST list all tool commands and routing commands.
- TUI-9 `/models` MUST list every configured model with its id, its provider, and the routing commands assigned to it in the active configuration. It MUST NOT show keys (TUI-7).

### Startup

- TUI-10 The screen MUST open before configuration loading and the metadata fetch start, and MUST show a loading line while they run.
- TUI-11 When loading finishes, MUST render its results in the output area in this order: missing-file warnings (CFG-2), configuration errors, metadata warnings (META-8), then the summary (CFG-7).
- TUI-12 A line submitted before loading finishes MUST be refused with a message saying startup is in progress. Nothing is dispatched. If loading fails, the screen MUST show the failure, keep refusing lines with a message saying startup failed, and still exit on Ctrl+C (TUI-17).

### While a request runs

- TUI-13 The input box MUST stay editable while a request runs. `/help`, `/models`, `/suggest` and `/history` (list) MUST run normally and print their output while the stream continues.
- TUI-14 A routing request submitted while a request runs MUST be refused with `error: a request is already running`. Nothing is executed.
- TUI-15 `/history delete` and `/history clear` submitted while a request runs MUST be refused with a message. `/refresh` is refused per REFRESH-9.

### Cancellation and exit

- TUI-5 Ctrl+C while a request runs MUST trigger cancellation (EXEC-3).
- TUI-16 After cancellation, MUST render the EXEC-3 outcome, including the "provider could not be cancelled" notice when it applies, then return to an idle input box.
- TUI-17 Ctrl+C while no request is running MUST exit the app, including during startup loading.
- TUI-18 Ctrl+D on an empty input box MUST exit the app. With text in the input box it MUST do nothing.

### Confirmation prompt

- TUI-19 When the application asks for confirmation (`/history clear`, HIST-6), MUST show its prompt and treat the next submitted line as the answer: `y` or `yes` confirms, anything else cancels. That line MUST NOT be dispatched as a command. Ctrl+C during the prompt cancels the prompt and does not exit the app.

### Scrolling and wrapping

- TUI-20 While the view is at the bottom, new output MUST scroll it to show the newest line. When the user has scrolled up, new output MUST NOT move the view until the user returns to the bottom.
- TUI-21 PgUp and PgDn MUST scroll the output area by one page.
- TUI-22 Long lines MUST soft-wrap to the window width and reflow when the window is resized. Wrapping MUST prefer breaking at spaces; only a word wider than the window is split.
- TUI-23 The `/suggest` JSON block MUST be rendered without any added prefix, label, border or indentation, so that text copied from the terminal at a width of 80 columns or more parses as valid JSON (STRAT-6, STRAT-11). Because TUI-22 breaks at spaces, a long line breaks between JSON tokens. A single string value longer than the window width cannot be kept intact; this is accepted (D-021).

## Out of scope

Mouse UI, themes, multi-pane layouts, session persistence, multiline input, input history (up-arrow recall), configurable keybindings, a `/quit` command (D-005 fixes the tool command list), non-TTY or piped mode.

## Acceptance criteria

Tests render the screen as plain text at a fixed size (80x24 unless stated otherwise), use fake application services and a fake provider, and use no network (TEST-2).

- AC1 (TUI-1): after startup with valid fixtures, the rendered screen has an output area above a one-line input box.
- AC2 (TUI-3,4): a fake provider streams `Hel`, `lo` and then usage 3/2/5. The output shows the model label, then `Hel`, then `Hello`, then the usage line naming the model and 3/2/5.
- AC3 (TUI-5,16): Ctrl+C after the first chunk cancels the fake stream, shows the interrupted outcome, and the input box is idle again. With a non-cancellable fake adapter, the notice is shown.
- AC4 (TUI-9,7): with a fixture key `sk-test-SECRET`, `/models` shows each model's id, provider and assigned commands, and the screen never contains `sk-test-SECRET`.
- AC5 (TUI-7): with a fixture key `sk-test-SECRET`, the startup summary, a fake `auth` error and the `/help` output never contain `sk-test-SECRET`.
- AC6 (TUI-6): a CFG-2 missing-file warning renders with the prefix `warning: `; a fake `provider_failure` error renders with the prefix `error: `. Both hold with `NO_COLOR=1`.
- AC7 (TUI-2): `hello` goes to ROUTE unchanged and its refusal is shown; `/models extra` goes to the `/models` handler with `extra` passed through; neither reaches the provider.
- AC8 (TUI-8): `/help` lists `/refresh`, `/suggest`, `/models`, `/history`, `/help`, `/fast`, `/code`, `/reason`, `/creative`.
- AC9 (TUI-10,11): a fake loader that blocks shows the loading line. Once it is released with one missing file and one metadata warning, the output shows the missing-file warning, then the metadata warning, then the summary.
- AC10 (TUI-12): `/models` submitted while the fake loader blocks is refused, and the `/models` handler is not called. If the fake loader fails instead, the failure is shown, `/models` is refused with a startup-failed message, and Ctrl+C exits.
- AC11 (TUI-13,14): during a fake stream, `/models` prints its list and the stream keeps going; `/code hi` shows `error: a request is already running` and the provider is called only once.
- AC12 (TUI-15): during a fake stream, `/history clear` and `/refresh` are both refused and the history and configuration fakes are not called.
- AC13 (TUI-17,18): Ctrl+C while idle exits; Ctrl+D on an empty input exits; Ctrl+D with `abc` typed does nothing.
- AC14 (TUI-19): after `/history clear`, answering `n` leaves history intact; answering `yes` clears it; answering `/models` cancels the prompt and does not run `/models`; Ctrl+C during the prompt cancels it and the app keeps running.
- AC15 (TUI-20,21): with 100 lines of output, PgUp moves the view up. A new line arriving while scrolled up does not move the view. After scrolling back to the bottom, the next new line is visible.
- AC16 (TUI-22): a 200-character line wraps at 80 columns and reflows to 120 columns after a resize to 120x24. A line of words wraps at a space, not inside a word.
- AC17 (TUI-23): `/suggest capability` output copied from the rendered 80-column screen (the JSON block's lines joined) parses as JSON and passes the `strategies.json` schema, also when a model id makes a JSON line wider than 80 columns (the line breaks between tokens).

## Open questions

None.

## V2 amendments (draft, not implemented; D-041)

Apply only when SDD is built, after `v1.0.0` (D-022). V1 rules above stay unchanged for V1. New IDs never reuse old ones.

- TUI-24 `/help` MUST also list `/sdd` and its subcommands (extends TUI-8).
- TUI-25 When SDD asks for approval (SDD-49), MUST show the proposed files with their diffs, then the valid answers `yes`, `no`, `redo [feedback]`, `edit`. The next submitted line is the answer and MUST NOT be dispatched as a command. Ctrl+C at this prompt answers `no` and does not exit the app (like TUI-19).
- TUI-26 For `edit` (SDD-50), MUST suspend the screen, run the editor in the terminal, then restore the screen and show the result. The editor launch sits behind an interface (SDD-60).
- TUI-27 While an SDD subcommand runs or waits for approval, a routing request or another `/sdd` subcommand MUST be refused with an error naming the active subcommand (extends TUI-14). `/help`, `/models`, `/suggest` and `/history` (list) still run (TUI-13). `/history delete`, `/history clear` and `/refresh` are refused (TUI-15, REFRESH-10).
- TUI-28 `/sdd init` confirmation (SDD-36) uses the TUI-19 prompt.

- AC18 (TUI-24): `/help` lists `/sdd`.
- AC19 (TUI-25,26): a fake proposal shows its diff and the four answers; `maybe` re-shows them; `edit` suspends and restores the screen with a fake editor; Ctrl+C answers `no` and the app keeps running.
- AC20 (TUI-27): during a fake SDD stream, `/code hi`, `/sdd review`, `/history clear` and `/refresh` are refused; `/models` runs.
