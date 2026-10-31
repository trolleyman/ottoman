# Fullscreen PC remote and typing improvements

Status: proposed only. This change adds a plan and a disconnected design demo;
it does not modify the running app, send input, switch audio, or change the host.

## What already exists

- The trackpad, custom keyboard, scroll momentum, PC audio volume/mute, and TV
  hardware volume/mute already exist, but are distributed across dashboard sections.
- Linux audio-output switching is implemented: `web/src/Audio.tsx` exposes **Make
  default**, `internal/agent/audio.go` accepts `default: true`, and
  `internal/audio/pipewire_linux.go` calls `wpctl set-default` using a sink's stable
  name. This is a default-output switch, not a promise to move every playing app.
- The keyboard settings contain a browser fullscreen toggle. There is no dedicated
  fullscreen remote layout and no live desktop-video stream in the current API.
- Swipe-word alternatives exist; general suggestions for tapped words and next-word
  predictions do not. The idle strip currently contains the typing instructions.
- No display-layout-to-audio association was found in the saved layout model or
  layout-application path.

The older `IMPROVEMENT_PLAN.md` records a Linux/PipeWire desktop, NVIDIA HDMI TV
output, and Logi Z407 speakers. Those are historical observations, not a fresh
inspection of this machine. No host escape or host configuration change is needed
for this planning step.

## 1. Make a dedicated remote screen

Add **PC remote** beside the ordinary dashboard, with a direct route that can be
bookmarked. It should use the available screen for controls rather than enlarging
the whole dashboard. The demo in `docs/mockups/fullscreen-pc-remote.html` illustrates
this arrangement with sample values only.

- Compact header: computer/connection, display-layout selector,
  audio-output selector, and a visible way to leave the remote.
- Main area: large trackpad, one-finger scroll rail, left/right click and drag
  controls. Keep cursor movement and scrolling separate and show momentum feedback.
- Audio: distinct **PC volume** and **TV volume** sliders with independent mute
  controls. Always identify the PC's selected output; do not conflate software
  attenuation with the TV's hardware volume. Hide or disable unavailable controls
  with a clear reason, including when the desktop is offline but TV control works.
- Media: a compact now-playing row with previous, play/pause, next, and a player
  picker when multiple players exist. Keep transport controls reachable with the
  keyboard open. Show unavailable/stale playback state honestly.
- Keyboard: selecting/tapping or intentionally focusing the trackpad focuses the
  input surface and opens the preferred keyboard (custom or native). Remove the
  separate Show keyboard button. Keep an explicit Hide keyboard/close action.
  Dock below the trackpad when open; reclaim its space when closed. Opening must
  preserve an in-progress drag and avoid pointer jumps, unwanted remote clicks, or
  a focus loop that immediately reopens a keyboard the user just hid.
  Keep the trackpad, rail, and volume controls reachable in portrait and landscape.
- Fit dynamic viewport and safe-area insets without document scrolling or page
  selection during gestures. Preserve browser zoom and editable settings fields.

A dedicated screen and browser fullscreen are separate states. Offer an explicit
**Enter fullscreen / Exit fullscreen** action where supported and track
`fullscreenchange`; handle rejection visibly. On iPhone, offer home-screen launch
instructions rather than claiming the webpage can move or hide Safari's address
bar. This phase is a fullscreen control surface, not screen streaming; live desktop
video would be a separate feature.

## 2. Add Clear preview and neutral Enter

- Put a labeled **Clear preview** action beside the local text preview, reachable
  with the keyboard open or closed. Clearing resets the local draft, caret offset,
  current-word state, and stale suggestion/replacement state together.
- Clearing must send **no Backspace, Delete, text, or shortcut events**. It forgets
  the local mirror; it must not erase text on the PC. Keep native textarea state
  synchronized so the next edit cannot reconcile against the discarded buffer.
- Keep the existing dismiss-on-other-interaction behavior. Do not restore a timer.
- Style Enter like the other utility keys, retaining a clear return icon and
  accessible name. Use the accent only for actual pressed/selected states.
- Replace the invented diamond on Linux with a clearly labeled **Super** key.
  Use Windows and Command symbols only for those remote operating systems.
- Navigation row order: **Esc, Home, End, Left, Up, Down, Right, Delete**.
  Keep the first three together at the left.

## 3. Replace instructions with a compact word bar

Remove the permanent “Tap or swipe to type” and hold-for-symbols captions. Preserve
optional explanatory copy in settings rather than consuming a keyboard row.

Stage the word bar:

1. Show the current local word plus two or three locally generated completions.
   Preserve literal entry as a choice; do not automatically correct ordinary taps.
2. Reuse the strip for swipe alternatives after a swipe. Selecting an alternative
   replaces only the last known local word, preserving case and punctuation.
3. Add next-word suggestions only after a word boundary and only once completion
   quality is satisfactory. Assess an offline compact model separately; the
   frequency-ranked word list alone is not a contextual prediction model.

Use the local composition buffer, not remote field contents. Invalidate replacement
ranges on remote navigation/clicks, selection changes, shortcuts, context resets,
Clear preview, disconnection, or a native input edit that no longer matches the
known range. Keep emoji/symbol search separate from word prediction. Predictions
must remain local, optional, and unlogged; do not infer that the remote field is
known or synchronized. Native focused-field synchronization remains the separate
proposal in `REMOTE_TEXT_INTEGRATION_PLAN.md`.

## 4. Add optional layout-linked audio routing on Linux

Build on existing manual output switching. Give each saved display layout an
optional audio-output association, selected from discovered sinks and stored by
stable `node.name`, never a transient PipeWire node number.

Recommended policy, enabled explicitly:

| Situation | Proposed behavior |
| --- | --- |
| User applies a TV layout with an audio mapping | After verified display success, select the mapped TV/HDMI sink. |
| User applies a desktop layout with a mapping | Select the mapped desktop-speaker sink. |
| A layout has both TV and desktop displays | Use its explicit mapping; otherwise retain the current output. |
| TV is merely powered on or reachable | Do not steal PC audio. |
| Mapped output is missing or still appearing | Keep current audio, show a routing warning, and use a bounded retry tied to that layout request. |
| User manually picks an output | Pause automatic routing until the next deliberate layout selection; make the override visible. |
| Display switch fails, rolls back, or becomes ambiguous | Do not silently reroute audio as though the requested display layout succeeded. |

Treat display success and audio success as separate reported outcomes. Route through
one agent-side operation so CLI, web UI, queued wake layouts, and external callers
agree. Skip audio in greeter mode and apply it when the user-session agent becomes
available. A newer layout selection or manual audio choice must cancel stale work.

Verify existing playback as well as new playback. WirePlumber documents
`wpctl set-default` as choosing the target for **new** auto-connected streams:
https://pipewire.pages.freedesktop.org/wireplumber/man/wpctl.html
Whether current app streams follow depends on session policy and per-app targets;
inspect and test the installed version before choosing a stream-migration strategy.
Do not promise that the existing default-switch operation moves every application.

## 5. Add media transport controls

On Linux, evaluate MPRIS over the user's session D-Bus as the first backend. It
can expose the active player, playback state, title/artist, and transport
capabilities. See the [MPRIS player specification](https://specifications.freedesktop.org/mpris/latest/Player_Interface.html). Offer previous, play/pause, and next; add seek only for players
that report seeking support and provide a reliable track position.

- Follow the active/playing player by default; let the user choose a player when
  more than one is present. Keep an explicit choice until it exits or the user
  returns to automatic selection.
- Disable unsupported previous/next/seek operations. Update the pause/play icon
  from actual playback state, reconcile failed commands, and handle players
  appearing or disappearing without stale controls.
- Keep PC output volume and TV hardware volume separate from any player-specific
  volume. The default media row should use the existing PC volume control rather
  than silently introducing a third master slider.
- Existing input maps contain Pause/Break, but no media transport mappings were
  found; it must not be treated as Play/Pause. If adding a media-key fallback,
  account for platform-specific media key codes. Transport without metadata must
  be labeled as such rather than displaying invented now-playing information.
- Proxy the agent's media API through the controller with existing authentication.
  No media control in the login-screen agent. Do not change the host merely to
  create metadata for the demo.

## Delivery order

1. Clear preview, neutral Enter, remove instructions, and current-word strip.
2. Dedicated remote screen using existing controls; fullscreen/home-screen behavior.
3. Local completions and explicit swipe replacement behavior; next-word prediction later.
4. Linux media transport and player state, with capability-aware controls.
5. Linux layout/audio associations and deterministic routing/override behavior.

Before phase 5, perform a bounded read-only host inventory if authorized at that
point: PipeWire/WirePlumber versions, sink names/descriptions, current default,
HDMI availability across layouts, and existing-stream behavior. The user has offered
read-only host inspection; this plan does not itself run commands or alter settings.

## Acceptance checks

- Clear preview makes no remote-input calls, and the next tap/native input/swipe
  does not delete previously typed remote text or use stale suggestions.
- Enter is neutral in both themes; the instruction caption is absent; suggestions
  work with case, punctuation, composition, emoji, and both configured layouts.
- Trackpad tap/focus opens the preferred keyboard without a separate show button;
  hiding it stays hidden until a new intentional activation. Dragging across the
  opening transition keeps cursor movement stable.
- Portrait/landscape and short viewports keep essential controls accessible; no
  accidental selection, stuck pointer capture, stuck modifiers, or hidden exit.
- Fullscreen works only when supported; unsupported/rejected requests and standalone
  launch are represented honestly. The demo must never control the real PC.
- TV and PC volume remain independent, retain coalesced writes, and reconcile errors.
- Routing tests cover simultaneous displays, absent HDMI audio, stale requests,
  manual override, rollback, queued wake/session handover, and currently playing apps.
- Media tests cover two simultaneous players, unavailable metadata, unsupported
  skip/seek, player exit, disconnected agent, and command failure without stale
  play/pause state.
