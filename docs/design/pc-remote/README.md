# PC remote design review

## Latest refinement

Use the in-trackpad field (A), capped at two visible lines with native scrolling.
It stays absolutely positioned so showing, typing or dismissing it never resizes
or moves the trackpad/keyboard. Use an × to dismiss without deleting text; typing
restores it. Avoid a permanent gradient on the editable field so selection and
the caret stay legible; the fixed-height field supplies native scrolling.

Move the selected display layout out of the right edge of the navigation row.
Give it a labeled full-width selector immediately below the menu header, above
PC and TV. The updated layout board shows the field, dismissed state and menu.

Unlike the broader redesign, tap-chain dragging, the × dismiss control, the
#+= label and optional emoji key on letters have now been implemented in the
application. The main-page emoji key is hidden by default but remains available
from symbols; its visibility switch can restore it on letters. Dragging reuses
one held native press across the existing 300 ms retouch window, preserving long holds
and immediate movement while preventing accidental titlebar/word double-clicks.
A single tap has immediate button-down feedback but delays release up to 300 ms;
each short stationary lift advances the native click count and reserves the
next press. Tap chains have no fixed length limit: two/three/four contacts
ending in a drag produce single/double/triple click-drags. The previous
100 ms retouch and 600 ms movement-buffer approaches are deliberately avoided.

## Revision 2

`v2.html` / `v2.tsx` supersede the earlier mockups. They import the actual
`CustomKeyboard` and `ScrollRail` components, with a design-only adapter for
proposed layout changes. `v2.css` is isolated from the application.

- **A, recommended:** a real multiline text field inside the existing trackpad,
  alongside the existing right-hand scroll rail. Empty space remains available
  for pointing. Text interaction stays inside the field; the rail scrolls the
  desktop. An implementation must isolate these gestures from trackpad events.
- **B:** move the same field below the trackpad. Clearer separation, at the cost
  of a smaller pointing area. Both designs keep native text selection and local
  scrolling, without instructional labels or a "Text preview" badge.
- Keep the existing keyboard, symbol hints, modifiers, navigation, suggestions,
  emoji categories/search/skin tones, long presses, swipe typing, Fn/media keys,
  space cursor movement and configurable keys. This preview reuses that code.
- Rename `?123` to `#+=`. Hide Tab, comma and period beside space by default,
  while retaining their switches. "commnd" was interpreted as comma, not the
  Command modifier; platform modifiers remain intact.
- Put Settings in the main menu. Remove the keyboard gear; place Clipboard
  immediately left of Fn at the right end of the existing control row.
  Clipboard reuses the actual F-key/suggestion row for its three actions.
- Menu: PC volume, then TV volume and brightness. No Quick Controls heading.
  Keep layout, displays, settings and connection/power below. Media metadata
  and playback controls appear under PC only if a player is available.
- Image selection should send/paste immediately; remove the second confirmation
  screen. Keep short progress/error feedback and disable duplicate submissions.
  The review adapter only opens a local picker and reports selection; it never
  uploads or pastes anything.
- Add one-finger hold-to-right-click on the pointing surface. Proposed defaults:
  500 ms and 6 CSS px maximum movement, matching the current pointer movement
  threshold. Trackpad → Gestures exposes an enable switch, hold duration
  (250–1,000 ms in 50 ms steps), and movement tolerance (2–20 CSS px).
  The preview implements a local timer and feedback only, with no host command.
  The app implementation must fire once, suppress the release's left click,
  and cancel on movement, another contact, cancellation, disconnect, unmount,
  or a settings change. Do not re-arm after a finger crosses the tolerance.
  The active double-tap-drag gesture takes priority; preserve two-finger tapping
  and scrolling. Native text selection, the scroll rail and keyboard must not
  trigger this desktop gesture. If tolerance differs from pointer movement
  slop, coordinate recognition so cursor movement cancels a pending hold.

The textarea is a real browser control with `inputMode="none"`, intended to keep
the custom keyboard while allowing native selection/copy/paste. Actual iPhone
selection handles, touch callouts and native keyboard suppression still require
device testing. Desktop screenshots do not establish iOS behavior. Remote
selection/edit reconciliation must preserve protected fields and the existing
text-context protections. Do not treat a local preview as proof an edit reached
the host; readonly terminal output must remain selectable without implying it is
editable. An empty field should stay hidden until needed, preserving trackpad
space. Fullscreen remains capability-gated, and the disconnected state continues
to use the remote surface, as agreed in the previous revision.

### Reported defects

- **Emoji increases page width:** the preview adds explicit shrinking and width
  constraints to the keyboard/picker flex chain. The actual component stays at
  the same width when switching to emoji at 390px in Chromium. This is a proposed
  containment approach, not an application fix or iOS validation.
- **Caret at a wrapped endpoint:** `CaretText` uses the end marker's horizontal
  coordinate but the final grapheme's vertical coordinate. If wrapping puts the
  endpoint on a later line, these can disagree. This is a likely explanation
  from code inspection; it has not yet been reproduced or fixed in the app.
  A native textarea supplies its own caret for editable text. Retained terminal
  rendering needs a regression case and a consistent endpoint rectangle.

### Media feasibility

The keyboard already sends media keys, but the app currently has no now-playing
metadata endpoint. On Linux, compatible players expose metadata, playback state
and control methods through MPRIS. Add an authenticated agent/controller source
for that information, expose capabilities, and hide unavailable actions/card.
The sample track in this design is illustrative, not read from the host.

Reference: https://specifications.freedesktop.org/mpris/latest/Player_Interface.html

### Render revision 2

```sh
node docs/design/pc-remote/render-v2.mjs /tmp/ottoman-remote-design-v2
```

Uses the existing web dependencies, a sandbox browser, an ephemeral localhost
port, and closes both browser and preview server. `PLAYWRIGHT_EXECUTABLE_PATH`
can select the bundled browser. Generates two 2x boards plus emoji and gesture-settings phone captures;
checks that opening emoji does not change keyboard/viewport width at 390px.

## Original proposal (superseded where revision 2 differs)

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
