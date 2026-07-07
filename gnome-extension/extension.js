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
const SLIDER_DEBOUNCE_MS = 250;

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
    getTVState() { return this._request('GET', '/api/tv/state'); }
    setBrightness(edid, brightness) { return this._request('POST', '/api/monitors/brightness', {edid, brightness}); }
    setPower(edid, on) { return this._request('POST', '/api/monitors/power', {edid, on}); }
    switchLayout(layout) { return this._request('POST', '/api/layouts/switch', {layout}); }
    setTVVolume(volume) { return this._request('POST', '/api/tv/volume', {volume}); }
}

// visible honours the registry's per-control visibility overrides.
function visible(monitor, control) {
    const v = monitor.visibility?.[control];
    return v === undefined ? true : v;
}

// A brightness/volume slider bound to a setter.
const OttomanSlider = GObject.registerClass(
class OttomanSlider extends QuickSlider {
    _init(iconName, accessibleName, value, onChange) {
        super._init({iconName});
        this.slider.accessible_name = accessibleName;
        this.slider.value = Math.max(0, Math.min(1, value));
        this._onChange = onChange;
        this._debounceId = 0;
        this._changedId = this.slider.connect('notify::value', () => this._debounce());
    }

    _debounce() {
        if (this._debounceId)
            GLib.source_remove(this._debounceId);
        this._debounceId = GLib.timeout_add(GLib.PRIORITY_DEFAULT, SLIDER_DEBOUNCE_MS, () => {
            this._debounceId = 0;
            this._onChange(Math.round(this.slider.value * 100));
            return GLib.SOURCE_REMOVE;
        });
    }

    // setValueQuiet updates the displayed value without triggering the setter.
    setValueQuiet(value) {
        if (this._debounceId)
            return; // a local edit is pending; don't fight it
        this.slider.block_signal_handler(this._changedId);
        this.slider.value = Math.max(0, Math.min(1, value));
        this.slider.unblock_signal_handler(this._changedId);
    }

    destroy() {
        if (this._debounceId) {
            GLib.source_remove(this._debounceId);
            this._debounceId = 0;
        }
        super.destroy();
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
        this._tvSlider = null;
        this._layoutToggle = null;
        this._layoutItems = [];
        this._signature = '';
    }

    async refresh() {
        const [monitors, layouts, tv] = await Promise.all([
            this._api.getMonitors().catch(() => null),
            this._api.getLayouts().catch(() => null),
            this._api.getTVState().catch(() => null),
        ]);

        // Agent unreachable: grey out existing controls, keep them in place.
        if (monitors === null && layouts === null) {
            this._setReactive(false);
            return;
        }
        this._setReactive(true);

        const sig = this._computeSignature(monitors, layouts, tv);
        if (sig !== this._signature) {
            this._rebuild(monitors || [], layouts, tv);
            this._signature = sig;
        } else {
            this._updateValues(monitors || [], layouts, tv);
        }
    }

    _computeSignature(monitors, layouts, tv) {
        const mparts = (monitors || [])
            .filter(m => m.capabilities)
            .map(m => `${m.edid}:${m.capabilities.brightness && visible(m, 'brightness')}:${m.capabilities.power && visible(m, 'power')}`);
        const lparts = (layouts?.layouts || []).map(l => l.id);
        const tvpart = tv?.configured ? 'tv' : '';
        return [...mparts, '|', ...lparts, '|', tvpart].join(',');
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
        this._tvSlider = null;
        this._layoutToggle = null;
        this._layoutItems = [];
    }

    _rebuild(monitors, layouts, tv) {
        this._clear();

        for (const m of monitors) {
            if (!m.capabilities)
                continue;
            const name = m.friendly_name || m.name || m.port || 'Monitor';

            if (m.capabilities.brightness && visible(m, 'brightness')) {
                const b = (typeof m.brightness === 'number' && m.brightness >= 0) ? m.brightness : 50;
                const slider = new OttomanSlider(
                    'display-brightness-symbolic', `${name} brightness`, b / 100,
                    val => this._api.setBrightness(m.edid, val).catch(logError));
                this._brightnessSliders.set(m.edid, slider);
                this._addItem(slider);
            }

            if (m.capabilities.power && visible(m, 'power'))
                this._addPowerToggle(m, name);
        }

        if (tv?.configured) {
            this._tvSlider = new OttomanSlider(
                'audio-volume-high-symbolic', 'TV volume', Math.max(0, tv.volume ?? 0) / 100,
                val => this._api.setTVVolume(val).catch(logError));
            this._addItem(this._tvSlider);
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
            checked: true,
        });
        toggle.connect('clicked', () => {
            this._api.setPower(monitor.edid, toggle.checked).catch(logError);
        });
        this._addItem(toggle);
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

    _updateValues(monitors, layouts, tv) {
        for (const m of monitors) {
            const slider = this._brightnessSliders.get(m.edid);
            if (slider && typeof m.brightness === 'number' && m.brightness >= 0)
                slider.setValueQuiet(m.brightness / 100);
        }
        if (this._tvSlider && tv && typeof tv.volume === 'number')
            this._tvSlider.setValueQuiet(Math.max(0, tv.volume) / 100);
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

        this._controls.refresh().catch(logError);
        this._timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, REFRESH_SECONDS, () => {
            this._controls.refresh().catch(logError);
            return GLib.SOURCE_CONTINUE;
        });
    }

    disable() {
        if (this._timer) {
            GLib.source_remove(this._timer);
            this._timer = null;
        }
        this._controls?.destroy();
        this._controls = null;
        this._api = null;
    }
}
