# Ottoman web client

React, TypeScript, and Vite frontend embedded in the Go binary via `web/embed.go`.
Run these commands from `web/`:

```sh
bun install --frozen-lockfile
bun run test
bun run lint
bun run build
```

`bun run dev -- --host 127.0.0.1 --port 43217` starts the frontend dev
server (choose an unused port). The production build writes `dist/`; build it before building the Go
binary so it embeds the current UI.

The trackpad keeps gesture handling in `Trackpad.tsx` and text reconciliation in
`trackpadText.ts`. `trackpadSettings.ts` defines defaults and shared UI/validation
bounds; `preferences.ts` handles browser storage. The custom keyboard composes
keys, data, haptics, and its settings panel from separate modules. WebSocket
ownership and reconnect cleanup live in `trackpadConnection.ts`, with React state
managed by `useTrackpadWebSocket.ts`.

`src/api/` is generated; change `api/openapi.yaml` and use the repository's
`mage generate:all` task rather than editing generated files.

The screenshot artifact script builds the UI and a temporary Go binary, starts
the simulated controller on an available port, and captures phone, desktop, menu, settings, clipboard and disconnected states:

```sh
PLAYWRIGHT_BROWSERS_PATH="$PWD/.playwright" bun x playwright install chromium --only-shell
PLAYWRIGHT_BROWSERS_PATH="$PWD/.playwright" bun scripts/screenshots/take-screenshots.ts
```

Set `HYDRA_ARTIFACT_OUTPUT` to override the default output directory,
`scripts/screenshots/out/`. The script stops its simulated server when done.

Keyboard settings default to British symbols and dark appearance. Layout,
appearance, swipe typing, the period key, number row, and hold/repeat timings are
configurable. Device-locale layout selection is optional; modifier labels always
follow the remote agent's reported OS. Symbol mode uses the same key positions
as the letter-key hold hints.

Swipe suggestions run locally using a frequency-ranked English dictionary. The
dictionary and grouped Unicode emoji data load on demand. Dataset notices ship
in `public/keyboard-data-licenses.txt`. Native key vibration is used only where
the browser supports it; iPhone Safari gets visual press/hold/scroll feedback.
For a launch without Safari's address bar, add Ottoman to the home screen.

Run the focused keyboard browser regressions with `bun run test:keyboard`. This
uses Vite on an OS-assigned localhost port and a simulated in-browser text field;
it checks preview navigation, spacing and period undo, held modifiers, adaptive
keys, autocorrect, generic tap dragging and menu volume/brightness controls. Set
`PLAYWRIGHT_EXECUTABLE_PATH` to use an existing bundled Chromium executable, or
install the project-local browser as shown above. The fixture closes its server
and browser after the checks. Preview/settings screenshots are saved under
`/tmp/keyboard-preview@2x.png` and `/tmp/keyboard-settings@2x.png`.

PC remote’s Trackpad settings include **Volume keys control selected output**
(on by default). When enabled, the keyboard’s Fn volume up/down keys and volume
key events delivered to the trackpad adjust the selected TV or PC slider in 1%
steps. Tap the sidebar’s TV/PC label to change the target. Volume keys keep
adjusting audio independently of brightness; mute retains its OS behavior. The
choice is saved in this browser and adds no controls to the sidebar.

`bun run test:remote` exercises the responsive shell, native field, default key
visibility, menu focus, mocked clipboard/media actions, long-press cancellation,
protected fields, emoji containment and disconnected keyboard. `bun run
test:native-text` builds and exercises the real simulated controller and
WebSocket text path. Tests never access the host clipboard or media players.

The authenticated application starts at `RemotePage`. It owns shared settings
and keeps `Trackpad` mounted while management panels are open. Phone menus
suspend input; desktop menus become a collapsible sidebar. `ClipboardActions`
uses on-demand text APIs and native/manual clipboard fallbacks. `NowPlaying`
polls only while its menu is visible.
