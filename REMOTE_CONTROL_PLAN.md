# Remaining remote-control work

The implemented keyboard and `/remote` page are no longer listed here. This plan
contains only unfinished work. Native focused-field synchronization remains in
`REMOTE_TEXT_INTEGRATION_PLAN.md`.

## 1. Prediction and input follow-ups

- Add contextual next-word predictions after a word boundary. Current predictions
  are local, frequency-ranked word completions, plus swipe alternatives; the word
  list alone is not a contextual language model. Evaluate a compact offline model.
- Add an optional prediction preference and assess completion quality with a
  broader British-English vocabulary, contractions, punctuation, and names.
- Extend Linux text injection beyond the supported standard British/US keymaps.
  GNOME's most recently selected source is detected remotely; per-window layout
  switching, other keymaps, arbitrary Unicode under uinput, and IME composition
  need a reliable compositor text provider rather than more assumed scancodes.
- Verify native-keyboard viewport behavior on physical iOS/Android devices, and
  add home-screen launch guidance where browser fullscreen is unavailable.
- Test a trackpad drag through keyboard opening on physical touch hardware.

## 2. Add optional layout-linked audio routing on Linux

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

## 3. Add media transport controls

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

## Remaining delivery order and checks

1. Evaluate contextual prediction and the native text-provider proposal.
2. Add Linux media transport/player state with capability-aware controls.
3. Add layout/audio associations with deterministic routing and manual override.

Before audio routing, inspect PipeWire/WirePlumber versions, stable sink names,
HDMI availability, and behavior of already-playing streams. Keep this read-only
until a concrete routing change is requested.

Routing tests must cover simultaneous displays, missing HDMI, stale requests,
manual override, rollback, queued wake/session handover, and existing playback.
Media tests must cover two players, unavailable metadata, unsupported skip/seek,
player exit, a disconnected agent, and command failures without stale playback
state. Do not present invented now-playing metadata or enabled unavailable controls.
