# Ottoman GNOME Shell extension

Adds Ottoman controls to the GNOME **Quick Settings** panel (bottom-right, next
to volume/network): per-monitor brightness sliders and power toggles, a TV
volume slider, and a layout switcher — all driven by the local Ottoman agent's
REST API.

It talks only to `http://127.0.0.1:<agent-port>`, reading the port and auth
token from `~/.config/ottoman/config.toml`, so it works even when the Pi
controller is offline and greys out when the agent is unreachable.

## Install

`mage deployAgent` copies it into place and prints the enable command. Manually:

```bash
cp -r gnome-extension ~/.local/share/gnome-shell/extensions/ottoman@trolleyman
# log out and back in (Wayland), then:
gnome-extensions enable ottoman@trolleyman
```

## Which controls appear

Controls are driven entirely by the agent's monitor registry + capabilities
(`GET /api/monitors`), so this extension has no device knowledge of its own:

- a brightness **QuickSlider** for each monitor whose backend reports
  `brightness` (DDC monitors, the TV via OLED backlight);
- a **QuickMenuToggle** for each monitor that reports `power`;
- a TV volume slider when a `[agent.tv]` is configured;
- a layout toggle whose submenu lists saved layouts (the active one is checked).

Controls hidden via the registry's per-monitor visibility overrides simply don't
render.

## Compatibility

GNOME Shell 45–47 (GJS ESM, GNOME 46 Quick Settings API). Extension code is
shell-version-coupled and restarts with the shell; expect small shims on a major
GNOME upgrade.
