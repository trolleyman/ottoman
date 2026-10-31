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

The browser regression script builds the UI and a temporary Go binary, starts
the simulated controller on an available port, and checks keyboard interactions
while capturing phone and desktop states:

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
