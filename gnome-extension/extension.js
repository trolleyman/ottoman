// Ottoman GNOME Shell extension: native Quick Settings controls (bottom-right
// panel) for the local Ottoman agent — per-monitor brightness/power, TV volume,
// and layout switching. It talks only to the agent's local REST API, so it
// works even when the Pi controller is down, and greys out when the agent is
// unreachable.
//
// GNOME 46 (GJS ESM). Targets 45–47 with small shims expected on major upgrades.

import GObject from 'gi://GObject';
import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import Soup from 'gi://Soup';

import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';
import {
    QuickSlider,
    QuickMenuToggle,
} from 'resource:///org/gnome/shell/ui/quickSettings.js';

Gio._promisify(Soup.Session.prototype, 'send_and_read_async');

const REFRESH_SECONDS = 15;

// How long to trust an optimistic power toggle before letting the poll's real
// state win again — a TV wakes over Wake-on-LAN slowly, a DDC panel less so.
// Mirrors the web UI's confirmation caps (see useMonitorPower.ts).
const POWER_GRACE_ON_US = 20 * 1000 * 1000;
const POWER_GRACE_OFF_US = 15 * 1000 * 1000;

function clamp01(v) { return Math.max(0, Math.min(1, v)); }

// powerOn reports a monitor's real power state, the same way the web card seeds
// its switch: a TV reports the panel state directly (tv_state.on), other
// backends fall back to whether they're active in the current layout.
function powerOn(monitor) {
    if (monitor.control_backend === 'tv')
        return !!monitor.tv_state?.on;
    return !!monitor.active;
}

// readConfig extracts the agent's port and auth token from the Ottoman config
// file, so the extension needs no separate setup after `mage deployAgent`.
function readConfig() {
    const path = GLib.build_filenamev([GLib.get_home_dir(), '.config', 'ottoman', 'config.toml']);
    let token = '';
    let port = '17294';
    try {
        const [ok, contents] = GLib.file_get_contents(path);
        if (ok) {
            const text = new TextDecoder().decode(contents);
            const tokM = text.match(/auth_token\s*=\s*['"]([^'"]+)['"]/);
            if (tokM)
                token = tokM[1];
            const laM = text.match(/listen_addr(?:ess)?\s*=\s*['"]([^'"]+)['"]/);
            if (laM) {
                const pm = laM[1].match(/:(\d+)\s*$/);
                if (pm)
                    port = pm[1];
            }
        }
    } catch (_e) {
        // Fall back to defaults.
    }
    return {baseUrl: `http://127.0.0.1:${port}`, token};
}

// Api is a thin async wrapper over the agent's REST API.
class Api {
    constructor({baseUrl, token}) {
        this._base = baseUrl;
        this._token = token;
        this._session = new Soup.Session();
        this._session.timeout = 5;
    }

    async _request(method, path, body) {
        const msg = Soup.Message.new(method, this._base + path);
        if (this._token)
            msg.request_headers.append('Authorization', `Bearer ${this._token}`);
        if (body !== undefined) {
            const bytes = new GLib.Bytes(new TextEncoder().encode(JSON.stringify(body)));
            msg.set_request_body_from_bytes('application/json', bytes);
        }
        const resp = await this._session.send_and_read_async(msg, GLib.PRIORITY_DEFAULT, null);
        if (msg.get_status() >= 300)
            throw new Error(`HTTP ${msg.get_status()} for ${path}`);
        const data = resp?.get_data();
        if (!data || data.length === 0)
            return null;
        return JSON.parse(new TextDecoder().decode(data));
    }

    getMonitors() { return this._request('GET', '/api/monitors'); }
    getLayouts() { return this._request('GET', '/api/layouts'); }
    setBrightness(edid, brightness) { return this._request('POST', '/api/monitors/brightness', {edid, brightness}); }
    setPower(edid, on) { return this._request('POST', '/api/monitors/power', {edid, on}); }
    switchLayout(layout) { return this._request('POST', '/api/layouts/switch', {layout}); }
    setVolume(edid, volume) { return this._request('POST', '/api/monitors/volume', {edid, volume}); }
}

// visible honours the registry's per-control visibility overrides.
function visible(monitor, control) {
    const v = monitor.visibility?.[control];
    return v === undefined ? true : v;
}

// A brightness/volume slider bound to a setter. It commits live on every drag
// step — coalescing to at most one in-flight write, always sending the latest
// value — so the monitor tracks the thumb exactly like the web slider, instead
// of only updating once the drag stops. This mirrors web/src/useCoalescedSlider.
const OttomanSlider = GObject.registerClass(
class OttomanSlider extends QuickSlider {
    _init(iconName, accessibleName, value, onChange) {
        super._init({iconName});
        this.slider.accessible_name = accessibleName;
        this.slider.value = clamp01(value);
        this._onChange = onChange; // (percent) => Promise
        this._pending = null;      // latest percent awaiting a commit
        this._inflight = false;    // a commit is currently in progress
        this._lastSent = null;     // last percent handed to the backend
        this._changedId = this.slider.connect('notify::value',
            () => this._set(Math.round(this.slider.value * 100)));
    }

    _set(percent) {
        this._pending = percent;
        this._flush();
    }

    async _flush() {
        if (this._inflight || this._pending === null)
            return;
        this._inflight = true;
        const v = this._pending;
        this._pending = null;
        this._lastSent = v;
        try {
            await this._onChange(v);
        } finally {
            this._inflight = false;
            this._flush(); // a newer value may have queued while we were writing
        }
    }

    // setValueQuiet adopts a server value without firing the setter, but only
    // when idle and the server has caught up to our last write — otherwise a
    // lagging poll would snap the thumb backward mid- or post-drag.
    setValueQuiet(value) {
        if (this._pending !== null || this._inflight)
            return; // a local edit is in flight; don't fight it
        const percent = Math.round(clamp01(value) * 100);
        if (this._lastSent !== null && percent !== this._lastSent)
            return; // stale echo of a value older than our last write
        this._lastSent = null;
        this.slider.block_signal_handler(this._changedId);
        this.slider.value = clamp01(value);
        this.slider.unblock_signal_handler(this._changedId);
    }
});

// OttomanControls owns the set of Quick Settings items and keeps them in sync
// with the agent. Items are added directly to the Quick Settings grid (rather
// than via a SystemIndicator) so they can be created after async data arrives.
class OttomanControls {
    constructor(api) {
        this._api = api;
        this._qs = Main.panel.statusArea.quickSettings;
        this._items = [];
        this._brightnessSliders = new Map(); // edid -> OttomanSlider
        this._volumeSliders = new Map(); // edid -> OttomanSlider (TV-backed monitors)
        this._powerToggles = new Map(); // edid -> QuickMenuToggle
        this._powerPending = new Map(); // edid -> monotonic deadline (µs) for an optimistic toggle
        this._layoutToggle = null;
        this._layoutItems = [];
        this._signature = '';
    }

    async refresh() {
        const [monitors, layouts] = await Promise.all([
            this._api.getMonitors().catch(() => null),
            this._api.getLayouts().catch(() => null),
        ]);

        // Agent unreachable: grey out existing controls, keep them in place.
        if (monitors === null && layouts === null) {
            this._setReactive(false);
            return;
        }
        this._setReactive(true);

        const sig = this._computeSignature(monitors, layouts);
        if (sig !== this._signature) {
            this._rebuild(monitors || [], layouts);
            this._signature = sig;
        } else {
            this._updateValues(monitors || [], layouts);
        }
    }

    _computeSignature(monitors, layouts) {
        const mparts = (monitors || [])
            .filter(m => m.capabilities)
            .map(m => `${m.edid}:${m.capabilities.brightness && visible(m, 'brightness')}:${m.capabilities.power && visible(m, 'power')}:${m.tv_state ? 'tv' : ''}`);
        const lparts = (layouts?.layouts || []).map(l => l.id);
        return [...mparts, '|', ...lparts].join(',');
    }

    _setReactive(reactive) {
        for (const item of this._items)
            item.reactive = reactive;
    }

    _addItem(item, colSpan = 2) {
        try {
            this._qs.menu.addItem(item, colSpan);
        } catch (e) {
            // An actor that was created but never parented must be destroyed,
            // or gnome-shell leaks it on every refresh.
            item.destroy();
            throw e;
        }
        this._items.push(item);
    }

    _clear() {
        for (const item of this._items)
            item.destroy();
        this._items = [];
        this._brightnessSliders.clear();
        this._volumeSliders.clear();
        this._powerToggles.clear();
        this._layoutToggle = null;
        this._layoutItems = [];
    }

    _rebuild(monitors, layouts) {
        this._clear();

        for (const m of monitors) {
            if (!m.capabilities)
                continue;
            const name = m.friendly_name || m.name || m.port || 'Monitor';

            // Lead each monitor with its named power toggle, then its sliders
            // beneath it, so the group reads top-down (name -> brightness ->
            // volume). The sliders are icon-only, so a slider above the name
            // would look like it belongs to the monitor before it.
            if (m.capabilities.power && visible(m, 'power'))
                this._addPowerToggle(m, name);

            if (m.capabilities.brightness && visible(m, 'brightness')) {
                const b = (typeof m.brightness === 'number' && m.brightness >= 0) ? m.brightness : 50;
                const slider = new OttomanSlider(
                    'display-brightness-symbolic', `${name} brightness`, b / 100,
                    val => this._api.setBrightness(m.edid, val).catch(logError));
                this._brightnessSliders.set(m.edid, slider);
                this._addItem(slider);
            }

            if (m.tv_state && m.capabilities.volume && visible(m, 'volume')) {
                const slider = new OttomanSlider(
                    'audio-volume-high-symbolic', `${name} volume`,
                    Math.max(0, m.tv_state.volume ?? 0) / 100,
                    val => this._api.setVolume(m.edid, val).catch(logError));
                this._volumeSliders.set(m.edid, slider);
                this._addItem(slider);
            }
        }

        if (layouts?.layouts?.length)
            this._addLayoutToggle(layouts);
    }

    _addPowerToggle(monitor, name) {
        const toggle = new QuickMenuToggle({
            title: name,
            subtitle: 'Display power',
            iconName: 'video-display-symbolic',
            toggleMode: true,
            checked: powerOn(monitor),
        });
        toggle.connect('clicked', () => {
            // `checked` has already flipped to the target here. Trust it
            // optimistically for a grace window so the confirmation poll (a TV
            // takes seconds to wake) doesn't snap the switch back meanwhile.
            const grace = toggle.checked ? POWER_GRACE_ON_US : POWER_GRACE_OFF_US;
            this._powerPending.set(monitor.edid, GLib.get_monotonic_time() + grace);
            this._api.setPower(monitor.edid, toggle.checked).catch(logError);
        });
        this._powerToggles.set(monitor.edid, toggle);
        this._addItem(toggle);
    }

    // _powerPendingActive reports whether an optimistic toggle for a monitor is
    // still within its grace window, expiring the entry once it lapses.
    _powerPendingActive(edid) {
        const deadline = this._powerPending.get(edid);
        if (deadline === undefined)
            return false;
        if (GLib.get_monotonic_time() >= deadline) {
            this._powerPending.delete(edid);
            return false;
        }
        return true;
    }

    _addLayoutToggle(layouts) {
        const toggle = new QuickMenuToggle({
            title: 'Layout',
            subtitle: layouts.current_layout || 'Switch layout',
            iconName: 'video-joined-displays-symbolic',
            toggleMode: false,
        });
        toggle.menu.setHeader('video-joined-displays-symbolic', 'Display Layouts');
        this._layoutItems = [];
        for (const layout of layouts.layouts) {
            const label = layout.emoji ? `${layout.emoji}  ${layout.name}` : layout.name;
            const item = new PopupMenu.PopupMenuItem(label);
            if (layout.id === layouts.current_layout)
                item.setOrnament(PopupMenu.Ornament.CHECK);
            item.connect('activate', () => {
                this._api.switchLayout(layout.id).catch(logError);
            });
            toggle.menu.addMenuItem(item);
            this._layoutItems.push({id: layout.id, item});
        }
        this._layoutToggle = toggle;
        this._addItem(toggle);
    }

    _updateValues(monitors, layouts) {
        for (const m of monitors) {
            const slider = this._brightnessSliders.get(m.edid);
            if (slider && typeof m.brightness === 'number' && m.brightness >= 0)
                slider.setValueQuiet(m.brightness / 100);
            const vol = this._volumeSliders.get(m.edid);
            if (vol && typeof m.tv_state?.volume === 'number')
                vol.setValueQuiet(Math.max(0, m.tv_state.volume) / 100);
            const power = this._powerToggles.get(m.edid);
            if (power && !this._powerPendingActive(m.edid))
                power.checked = powerOn(m);
        }
        if (this._layoutToggle && layouts) {
            this._layoutToggle.subtitle = layouts.current_layout || 'Switch layout';
            for (const {id, item} of this._layoutItems)
                item.setOrnament(id === layouts.current_layout ? PopupMenu.Ornament.CHECK : PopupMenu.Ornament.NONE);
        }
    }

    destroy() {
        this._clear();
    }
}

export default class OttomanExtension extends Extension {
    enable() {
        this._api = new Api(readConfig());
        this._controls = new OttomanControls(this._api);

        // Build the controls once so they exist before the menu is first
        // opened, then only poll while the Quick Settings menu is actually
        // open. Polling while closed kept the agent probing DDC and dialling
        // the (often off) TV around the clock for data nobody was looking at.
        this._controls.refresh().catch(logError);
        this._menu = Main.panel.statusArea.quickSettings.menu;
        this._openId = this._menu.connect('open-state-changed', (_menu, open) => {
            if (open) {
                this._controls.refresh().catch(logError);
                this._startTimer();
            } else {
                this._stopTimer();
            }
        });
    }

    _startTimer() {
        if (this._timer)
            return;
        this._timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, REFRESH_SECONDS, () => {
            this._controls.refresh().catch(logError);
            return GLib.SOURCE_CONTINUE;
        });
    }

    _stopTimer() {
        if (this._timer) {
            GLib.source_remove(this._timer);
            this._timer = null;
        }
    }

    disable() {
        this._stopTimer();
        if (this._openId) {
            this._menu.disconnect(this._openId);
            this._openId = null;
        }
        this._menu = null;
        this._controls?.destroy();
        this._controls = null;
        this._api = null;
    }
}
