# PC remote design review

Design-only proposal. No application UI, routes, desktop settings, TV inputs, or
clipboard behavior have been changed. Open `index.html` locally to review.

Views:

- `?view=options`: A (quiet canvas, recommended) and B (inset workspace).
- `?view=states`: menu, disconnected remote, and consolidated input settings.
- `?view=desktop`: A with a collapsible desktop sidebar.
- `?view=clipboard`: revised Fn clipboard row and an on-demand PC text preview.
- `?view=prototype&variant=a`: responsive local interaction demo; use `b` for B.
  Add `&offline` for the disconnected mobile state.

The prototype supports menu opening/closing, sliders, input settings tabs,
switches, Fn, the clipboard row and its preview. Navigation destinations and
desktop actions are illustrative. It never reads or writes a real clipboard,
sends a desktop command, or calls an API.

## Proposed decisions for approval

- Make the PC remote the default home page. Keep display, layout, audio and
  connection management accessible from a top-left menu. On desktop, that menu
  can stay open as a collapsible sidebar.
- Keep the keyboard below the trackpad at every width, with bounded keyboard
  width on desktop. Maintain the same remote surface while disconnected;
  disable remote input and reconnect automatically. TV controls belong in the
  menu's desktop sections, rather than replacing the trackpad.
- Move volume and brightness into the menu, with explicit target selection.
  Hide unavailable capabilities. Fullscreen appears only where supported;
  remove the unsupported-browser fullscreen button and its help message.
- Move the top-right trackpad preferences into the existing keyboard settings
  destination, called Input settings, with Keyboard and Trackpad tabs. Replace
  paragraphs with short contextual hints. Collapse key layout, prediction and
  timing controls into groups. Keep existing preferences, except Click and Drag.
- Remove Click and Drag. It currently toggles a held left mouse button between
  clicks while pointer-locked; ordinary press/release dragging and touch
  gestures can remain.
- Latest clipboard direction supersedes the initial separate clipboard button
  and bottom sheet: replace the existing Fn image-paste button with Clipboard.
  Clipboard swaps the F1–F12 row for Paste image, Paste from iPhone and Copy from
  PC, without increasing keyboard height. Tapping it again restores F keys.
  Copy from PC fetches a preview on demand, then Copy to iPhone performs the
  clipboard write from a fresh user tap. Brief success feedback replaces the
  text preview temporarily. Close/reset clipboard mode when leaving Fn.

## Clipboard feasibility

The current app uploads a selected image to the host clipboard, then sends the
paste shortcut. It does not yet expose bidirectional text clipboard endpoints.
Those need to be added to the authenticated agent/controller API for a shared
clipboard implementation. Begin with text both ways and preserve existing image
paste; do not imply arbitrary files or host-to-phone images are supported.

On iPhone, browser clipboard APIs require a secure context and user interaction;
Safari can prompt for Paste. Manual native paste into a small text field is the
fallback when programmatic reads are unavailable or refused. If a write is
unavailable, show selectable text and native Copy. HTTPS setup must be checked
before promising one-tap transfer on a deployed phone. Avoid a background sync
toggle, clipboard polling, or automatic replacement of the phone clipboard.

References:

- https://webkit.org/blog/10855/async-clipboard-api/
- https://developer.mozilla.org/en-US/docs/Web/API/Clipboard_API

## TV/HDMI 500 investigation

Read-only code inspection found a concrete failure path:

1. `internal/display/mutter_linux.go` resolves each requested layout monitor
   against the compositor's connected monitor list. A missing monitor returns
   `layout monitor ... is not connected` before applying the layout.
2. `internal/agent/agent.go` returns every layout-application error as HTTP 500.
3. The controller preserves that 500 and its error body when proxying a layout
   switch. TV input switching can independently return 500 if webOS rejects it
   or the TV cannot be reached.

The sandbox has no journal files. Two approved read-only host journal checks of
`ottoman-agent` / `ottoman-controller` over the preceding two days found routine
successful polling but no matching layout failure or POST HTTP 500 entry. This
does not establish the incident's cause; the Pi/controller logs or the exact
failed request and response are still needed. No service or TV state was changed.

Proposed behavior: report an unavailable layout/display as an actionable
operational failure, preserving the current layout and showing the affected
display/input with a retry action. Do not silently apply a partial layout, claim
success, or switch the TV's input automatically based on this unconfirmed report.

## Render previews

Install existing web dependencies with `cd web && bun install --frozen-lockfile`.
Then from the repository root:

```sh
node docs/design/pc-remote/render.mjs /tmp/ottoman-remote-design
```

Set `PLAYWRIGHT_EXECUTABLE_PATH` to a bundled sandbox browser if needed. The
renderer uses a local file URL, starts no server, closes its browser, and captures
all four boards at 2x scale. Previews are generated outside the repository.
