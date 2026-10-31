# Remote text and caret integration plan

## Outcome

When the user focuses an editable field on the controlled computer, Ottoman should show that field's current text and caret/selection in the trackpad preview. Text entered from either Ottoman keyboard should update the focused field, and edits made directly on the controlled computer should flow back to the phone.

This needs native accessibility APIs. Reconstructing the field from injected keystrokes alone cannot detect mouse-set caret positions, edits from another keyboard, autocomplete, or application-side formatting.

## User experience

- Keep the preview passive: it is not selectable and does not become a second editor.
- Show only the focused editable field, with its actual caret or selection.
- Clear the preview immediately when focus leaves an editable field, the connection closes, or the field is protected.
- Fade the preview after inactivity, then reveal it again on a text, caret, or selection change.
- Add a `Mirror focused text` keyboard setting. Explain and request the operating-system accessibility permission only when it is enabled.
- Fall back to the current keystroke-based indicator when an application does not expose an accessible text model.

## Protocol and state model

Extend the existing bidirectional `/api/trackpad` WebSocket schema in `api/openapi.yaml` rather than adding another connection.

Agent to browser:

```json
{
  "type": "textcontext",
  "fieldId": "opaque-per-focus-token",
  "revision": 42,
  "value": "draft text",
  "selectionStart": 5,
  "selectionEnd": 5,
  "editable": true,
  "multiline": false
}
```

Browser to agent:

```json
{
  "type": "textedit",
  "fieldId": "opaque-per-focus-token",
  "baseRevision": 42,
  "start": 5,
  "end": 5,
  "text": "ed"
}
```

- `fieldId` changes whenever focus changes, preventing a delayed edit from landing in a different field.
- `revision` increments for every accepted native text or selection change. Reject stale edits and immediately send the latest context.
- Offsets are Unicode code-point offsets on the wire. Each platform adapter owns conversion to its native unit (UTF-16 on Windows/macOS, character offsets in AT-SPI).
- Cap mirrored context to a configurable window around the caret (initially 4,096 characters) for performance and privacy. Include the window's absolute start offset when truncation is active.
- The browser keeps one authoritative context instead of attempting to merge it with the current local `draftRef` history.

The controller already transparently pipes both WebSocket directions in `internal/controller/controller.go`. The primary protocol work is the generated API types, `internal/agent/agent.go`, `web/src/useTrackpadWebSocket.ts`, and trackpad state.

## Native adapters

Define a small agent-side interface:

```go
type TextContextProvider interface {
    Watch(context.Context, chan<- TextContext) error
    ApplyEdit(context.Context, TextEdit) error
    Close() error
}
```

Keep platform code behind build tags and emit normalized snapshots from one goroutine. Never call native accessibility APIs from the WebSocket writer goroutine.

### Windows

Use Microsoft UI Automation from a Windows-specific provider:

1. Subscribe to focus, text-changed, and selection-changed automation events.
2. Read the focused element through `TextPattern`/`TextPattern2`; use its document range and selection range for content and caret.
3. Use `ValuePattern` only for simple controls that expose it. `TextPattern` itself does not provide general text insertion, so preserve selection and inject text for rich editors when no safe writable value pattern exists.
4. Never mirror protected/password content. UI Automation providers are explicitly expected not to expose it.

Reference: [Microsoft UI Automation TextPattern overview](https://learn.microsoft.com/en-us/dotnet/framework/ui-automation/ui-automation-textpattern-overview).

### Linux

Use AT-SPI2 over the user's accessibility D-Bus session:

1. Track `object:state-changed:focused`, `object:text-changed`, `object:text-caret-moved`, and selection events.
2. Require focused + editable state and the Text interface.
3. Read text with `GetText`, caret with `CaretOffset`, and selection with `GetSelection`.
4. Apply selection/caret through the Text interface and edits through EditableText where supported; otherwise fall back to injected input.

AT-SPI exposes character count, caret offset, text ranges, and selections directly: [AT-SPI Text interface](https://gnome.pages.gitlab.gnome.org/at-spi2-core/devel-docs/doc-org.a11y.atspi.Text.html). This works independently of Ottoman's X11/Wayland input backend, but only for applications/toolkits that publish a usable accessibility tree.

### macOS

Add the adapter when Ottoman has a supported macOS agent/input backend:

1. Request Accessibility trust using `AXIsProcessTrustedWithOptions`; do not repeatedly trigger the system prompt.
2. Query the system-wide `kAXFocusedUIElementAttribute`.
3. Read `kAXValueAttribute` and `kAXSelectedTextRangeAttribute`; the latter is defined in characters rather than bytes and is required for editable text accessibility elements.
4. Subscribe through `AXObserver` to focused-element, value, and selection notifications.
5. Check whether an attribute is settable before using `AXUIElementSetAttributeValue`; fall back to injected input for unsupported rich editors.

References: [Apple accessibility trust API](https://developer.apple.com/documentation/applicationservices/1459186-axisprocesstrustedwithoptions), [focused UI element](https://developer.apple.com/documentation/applicationservices/kaxfocuseduielementattribute), and [selected text range](https://developer.apple.com/documentation/applicationservices/kaxselectedtextrangeattribute).

## Safety and privacy

- Treat password/protected fields as `focused but unavailable`; send no value, selection, placeholder, app title, or field name.
- Keep mirrored values in memory only. Do not log messages or include text context in diagnostics, analytics, screenshots, or persisted settings.
- Disable mirroring on the lock screen, greeter, and before authentication.
- Stop every native observer and zero the last context on disconnect.
- Rate-limit snapshots (for example, coalesce within 16–32 ms), but never drop the final update.
- Preserve the existing authenticated WebSocket boundary; add message-size limits before accepting `textedit` payloads.

## Delivery phases

1. **Protocol and simulator:** add schemas, generated types, revision handling, a simulated focused field, and browser reducer tests.
2. **Read-only mirroring:** Windows and Linux adapters emit focused text/caret/selection. The current keyboard still injects edits. This delivers the most valuable visual feedback with low write risk.
3. **Synchronized editing:** add range edits, stale-revision recovery, selection replacement, and cursor movement. Fall back per control when native writes are unsupported.
4. **macOS adapter:** land alongside a supported macOS Ottoman agent and permission flow.
5. **IME and hardening:** composition/pre-edit state, emoji/grapheme tests, RTL text, very large documents, Electron/browser editors, terminal controls, office suites, and permission-loss recovery.

## Acceptance criteria

- Focusing a normal text field shows its text and caret on the phone within 100 ms on a local network.
- Mouse/keyboard caret moves and selections on the controlled computer appear without typing from the phone.
- Phone edits replace the current native selection and converge after concurrent local edits without affecting a newly focused field.
- Password content is never transmitted or logged.
- Unsupported controls retain today's keyboard behavior and do not leave a stale preview behind.
- Disconnect, focus loss, permission revocation, and agent shutdown all clear the mirrored value immediately.

## Implementation status

The protocol, simulator, browser synchronization, Linux AT-SPI adapter, and
Windows UI Automation helper are implemented. `textwatch` opts each authenticated
socket into mirroring; `textcontext` also carries `available`, `protected`,
`windowStart`, and edit acknowledgement fields. `textedit` supports selection-only
updates. One browser edit is sent at a time; a rejection or external revision
resets pending edits to the authoritative snapshot.

Native calls run on one worker per socket. Focus notifications invalidate focus
tokens; snapshots are sampled every 40 ms to include text/caret changes even in
applications with incomplete event notifications. Linux uses EditableText range
operations. Windows uses ValuePattern only when its text matches TextPattern,
then restores the caret through TextPattern. Other rich editors retain injected
input and read-only mirroring. Unobservable lock state fails closed on Linux.
The Windows helper uses only system assemblies and exchanges text over private
stdio, with no temporary scripts, clipboard writes, or text in command arguments.

Automated coverage includes private-bus AT-SPI reads/writes and password read
suppression, session focus/revision guards, opt-in and disable behavior, browser
edit queuing, Unicode range conversion, and selection replacement. Live Windows
and Linux application/permission testing is still required to validate the
100 ms target across toolkits. macOS and broader application/RTL/grapheme testing
remain deferred as described above.
