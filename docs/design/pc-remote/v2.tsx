// Design-only adapter around the real keyboard and scroll rail. No desktop API.
import { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { createPortal } from 'react-dom';
import { Menu, Monitor, ChevronRight, X, Clipboard, Settings, Volume2, Sun, SkipBack, SkipForward, Pause, Play, Power, TextCursorInput, ArrowLeft, Check, Music2, Maximize } from '../../../web/node_modules/lucide-react';
import { CustomKeyboard } from '../../../web/src/CustomKeyboard';
import { ScrollRail } from '../../../web/src/ScrollRail';
import { DEFAULT_SETTINGS } from '../../../web/src/trackpadSettings';
import { Modifier } from '../../../web/src/api';
import '../../../web/src/index.css';
import './v2.css';

const noop = () => {};
const sample = 'I like the current keyboard. Keep the scroller on the right, and let me select and edit this text normally.';

function Phone({ variant = 'overlay', menu = false, offline = false, emoji = false, clip = false, settings = false, dismissed = false }: {
  variant?: 'overlay' | 'split'; menu?: boolean; offline?: boolean; emoji?: boolean; clip?: boolean; settings?: boolean; dismissed?: boolean;
}) {
  const [text, setText] = useState(sample);
  const [previewVisible, setPreviewVisible] = useState(!dismissed);
  const [openMenu, setMenu] = useState(menu);
  const [showSettings, setSettings] = useState(settings);
  const [clipboardMode, setClipboardMode] = useState(clip);
  const [copyPreview, setCopyPreview] = useState(false);
  const [status, setStatus] = useState('');
  const [holdEnabled, setHoldEnabled] = useState(true);
  const [holdDelay, setHoldDelay] = useState(500);
  const [holdSlop, setHoldSlop] = useState(6);
  const hold = useRef<{ timer?: ReturnType<typeof setTimeout>; x: number; y: number }>({ x: 0, y: 0 });
  const cancelHold = () => { clearTimeout(hold.current.timer); hold.current.timer = undefined; };
  useEffect(() => cancelHold, []);
  const [modifiers, setModifiers] = useState<Modifier[]>([]);
  const [preferences, setPreferences] = useState({ ...DEFAULT_SETTINGS, tabKey: false, commaKey: false, periodKey: false, interactivePreview: true, haptics: false });
  const [controls, setControls] = useState<HTMLElement | null>(null);
  const [suggestions, setSuggestions] = useState<HTMLElement | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const field = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const [photo, setPhoto] = useState('');
  useEffect(() => {
    const el = root.current!;
    setControls(el.querySelector('[aria-label="Keyboard controls"]'));
    setSuggestions(el.querySelector('.keyboard-suggestion-bar'));
    const button = el.querySelector<HTMLButtonElement>('button[aria-label="Function keys"]');
    if (clip) button?.click();
    const emojiObserver = new MutationObserver(() => {
      const emojiKey = el.querySelector<HTMLButtonElement>('button[aria-label="Emoji"]');
      if (emoji && emojiKey) { emojiObserver.disconnect(); emojiKey.click(); }
    });
    if (emoji) {
      emojiObserver.observe(el, { childList: true, subtree: true });
      el.querySelector<HTMLButtonElement>('button[aria-label="Symbols"]')?.click();
    }
    const observer = new MutationObserver(() => {
      if (button?.getAttribute('aria-pressed') === 'false') setClipboardMode(false);
    });
    if (button) observer.observe(button, { attributes: true, attributeFilter: ['aria-pressed'] });
    return () => { observer.disconnect(); emojiObserver.disconnect(); };
  }, [clip, emoji, showSettings]);
  useEffect(() => () => { if (photo) URL.revokeObjectURL(photo); }, [photo]);
  function insert(value: string) {
    setPreviewVisible(true);
    const start = field.current?.selectionStart ?? text.length;
    const end = field.current?.selectionEnd ?? text.length;
    setText(text.slice(0, start) + value + text.slice(end));
    requestAnimationFrame(() => field.current?.setSelectionRange(start + value.length, start + value.length));
  }
  function clipboard() {
    const fn = root.current!.querySelector<HTMLButtonElement>('button[aria-label="Function keys"]');
    if (fn?.getAttribute('aria-pressed') !== 'true') fn?.click();
    setClipboardMode(value => !value);
  }
  const textarea = <textarea ref={field} aria-label="Remote text" inputMode="none" spellCheck={false} value={text} onChange={e => setText(e.target.value)} disabled={offline} />;
  return <div ref={root} className={`v2-phone ${variant} ${offline ? 'is-offline' : ''}`}>
    <div className="v2-status"><span>9:41</span><span>••• ▰</span></div>
    <header className="v2-header"><button onClick={() => setMenu(true)} aria-label="Open menu"><Menu /></button><div><b>PC remote</b><span><i className={offline ? 'offline-dot' : ''} />hades · {offline ? 'Disconnected' : 'Connected'}</span></div><button className="desktop-fullscreen" aria-label="Fullscreen"><Maximize /></button></header>
    {showSettings ? <InputSettings preferences={preferences} onChange={setPreferences} onBack={() => setSettings(false)} holdEnabled={holdEnabled} setHoldEnabled={setHoldEnabled} holdDelay={holdDelay} setHoldDelay={setHoldDelay} holdSlop={holdSlop} setHoldSlop={setHoldSlop} /> : <>
      <div className="v2-pad" onPointerDown={e => {
        cancelHold();
        if (!holdEnabled || offline || !e.isPrimary || e.button !== 0 || (e.target as Element).closest('textarea,button,[aria-label="One-finger scroll control"]')) return;
        hold.current.x = e.clientX; hold.current.y = e.clientY;
        hold.current.timer = setTimeout(() => { setStatus('Right-click · design preview'); }, holdDelay);
      }} onPointerMove={e => { if (Math.hypot(e.clientX - hold.current.x, e.clientY - hold.current.y) >= holdSlop) cancelHold(); }}
        onPointerUp={cancelHold} onPointerCancel={cancelHold} onPointerLeave={cancelHold}
        onContextMenu={e => { if (!(e.target as Element).closest('textarea')) e.preventDefault(); }}>
        {offline ? <span className="v2-disconnected">Disconnected</span> : <>
          {variant === 'overlay' && previewVisible && <div className="v2-text-wrap"><div className="v2-text-actions"><button onClick={() => setPreviewVisible(false)} aria-label="Dismiss preview"><X /></button></div>{textarea}</div>}
          <ScrollRail send={noop} sensitivity={preferences.scrollSensitivity} friction={preferences.scrollFriction} haptics={false} onInteract={noop} />
          {variant === 'split' && <button className="v2-field-toggle" aria-label="Show text field" onClick={() => field.current?.focus()}><TextCursorInput /></button>}
        </>}
        {copyPreview && <div className="v2-copy"><header><b>PC clipboard</b><button aria-label="Close clipboard" onClick={() => setCopyPreview(false)}><X /></button></header><textarea aria-label="Clipboard text" readOnly value="Booking reference: OTT-2048" /><button className="v2-primary" onClick={() => { setStatus('Copied · design preview'); setCopyPreview(false); }}>Copy to iPhone</button></div>}
        {photo && <div className="v2-image-progress"><Check /><span>Image selected · design preview</span><button aria-label="Dismiss image status" onClick={() => setPhoto('')}><X /></button></div>}
      </div>
      {variant === 'split' && previewVisible && <div className="v2-separate-text"><button aria-label="Dismiss preview" onClick={() => setPreviewVisible(false)}><X /></button>{textarea}</div>}
      <div className={`v2-keyboard ${clipboardMode ? 'clipboard-mode' : ''}`}>
        <CustomKeyboard open connected={!offline} docked textContext={1} remoteOS="linux" settings={preferences}
          textBeforeCursor={text} sensitiveInput={false} onText={insert} onReplaceText={(previous, next) => setText(value => value.endsWith(previous) ? value.slice(0, -previous.length) + next : value + next)}
          onBackspace={() => setText(value => value.slice(0, -1))} onEnter={() => insert('\n')} onTab={() => insert('\t')} onNavigate={noop} onCursorDrag={() => false}
          onClose={noop} onSettingsChange={patch => setPreferences(value => ({ ...value, ...patch }))} onUseCustomKeyboard={noop} onUseDeviceKeyboard={noop}
          usingCustomKeyboard modifiers={modifiers} lockedModifiers={[]} onModifierDown={modifier => setModifiers(value => [...value, modifier])}
          onModifierUp={modifier => setModifiers(value => value.filter(item => item !== modifier))} onSuperPress={noop} onPasteImage={noop} />
        {controls && createPortal(<button className={`v2-clipboard-button ${clipboardMode ? 'selected' : ''}`} onClick={clipboard} aria-label="Clipboard"><Clipboard /></button>, controls)}
        {suggestions && clipboardMode && createPortal(<div className="v2-clipboard-choices"><button disabled={offline} onClick={() => picker.current?.click()}>Paste image</button><button disabled={offline} onClick={() => { setStatus('Paste here'); field.current?.focus(); }}>Paste from iPhone</button><button disabled={offline} onClick={() => setCopyPreview(true)}>Copy from PC</button></div>, suggestions)}
      </div>
      {status && <div className="v2-toast" role="status" onClick={() => setStatus('')}>{status}</div>}
    </>}
    <input ref={picker} hidden type="file" accept="image/*" onChange={e => { const image = e.target.files?.[0]; if (image) setPhoto(URL.createObjectURL(image)); }} />
    <div className="v2-home" />
    {openMenu && <><button className="v2-scrim" aria-label="Close menu" onClick={() => setMenu(false)} /><Drawer onClose={() => setMenu(false)} onSettings={() => { setSettings(true); setMenu(false); }} /></>}
  </div>;
}

function Slider({ label, value, brightness = false, sensitivity = false }: { label: string; value: number; brightness?: boolean; sensitivity?: boolean }) {
  const [current, setCurrent] = useState(value);
  return <label className="v2-slider"><span>{sensitivity ? <TextCursorInput /> : brightness ? <Sun /> : <Volume2 />}<span>{label}</span><output>{sensitivity ? `${(current / 20).toFixed(1)}×` : `${current}%`}</output></span><input aria-label={label} type="range" min="0" max="100" value={current} onChange={e => setCurrent(Number(e.target.value))} /></label>;
}

function Drawer({ onClose, onSettings }: { onClose: () => void; onSettings: () => void }) {
  const [playing, setPlaying] = useState(true);
  return <aside className="v2-drawer"><header><b>ottoman<span>.</span></b><button aria-label="Close menu" onClick={onClose}><X /></button></header>
    <label className="v2-layout-selector"><span>Display layout</span><select aria-label="Display layout"><option>TV</option><option>Desk</option></select></label>
    <section><h2><Monitor />PC <span>hades</span></h2><Slider label="Volume" value={42} />
      <div className="v2-media"><div className="v2-art"><Music2 /></div><div className="v2-track"><b>Something About Us</b><span>Daft Punk</span></div><div className="v2-playback"><button aria-label="Previous track"><SkipBack /></button><button aria-label={playing ? 'Pause' : 'Play'} onClick={() => setPlaying(value => !value)}>{playing ? <Pause /> : <Play />}</button><button aria-label="Next track"><SkipForward /></button></div></div>
    </section>
    <section><h2><Monitor />TV <span>HDMI 1</span><button aria-label="TV power"><Power /></button></h2><Slider label="Volume" value={7} /><Slider label="Brightness" value={65} brightness /></section>
    <nav><button><Monitor /><span>Displays</span><ChevronRight /></button><button onClick={onSettings}><Settings /><span>Settings</span><ChevronRight /></button><button><Power /><span>Connection & power</span><ChevronRight /></button></nav>
  </aside>;
}

function InputSettings({ preferences, onChange, onBack, holdEnabled, setHoldEnabled, holdDelay, setHoldDelay, holdSlop, setHoldSlop }: {
  preferences: typeof DEFAULT_SETTINGS; onChange: (settings: typeof DEFAULT_SETTINGS) => void; onBack: () => void;
  holdEnabled: boolean; setHoldEnabled: (value: boolean) => void;
  holdDelay: number; setHoldDelay: (value: number) => void; holdSlop: number; setHoldSlop: (value: number) => void;
}) {
  const [tab, setTab] = useState(new URLSearchParams(location.search).get('view') === 'gestures' ? 'Trackpad' : 'Keyboard');
  const toggle = (label: string, name: 'tabKey' | 'commaKey' | 'periodKey' | 'numberRow' | 'tallerKeys' | 'emojiKey' | 'swipeTyping' | 'haptics') => <label className="v2-setting"><span>{label}</span><input type="checkbox" role="switch" checked={preferences[name]} onChange={e => onChange({ ...preferences, [name]: e.target.checked })} /></label>;
  return <div className="v2-settings"><header><button aria-label="Back to remote" onClick={onBack}><ArrowLeft /></button><b>Settings</b></header><div className="v2-tabs">{['Keyboard', 'Trackpad'].map(name => <button key={name} className={tab === name ? 'selected' : ''} onClick={() => setTab(name)}>{name}</button>)}</div>
    {tab === 'Keyboard' ? <><div className="v2-settings-group"><label className="v2-setting"><span>Keyboard</span><select><option>Ottoman</option><option>Device</option></select></label><label className="v2-setting"><span>Layout</span><select><option>British (UK)</option><option>English (US)</option></select></label><label className="v2-setting"><span>Appearance</span><select><option>Dark</option><option>Light</option><option>System</option></select></label></div>
    <h3>Keys</h3><div className="v2-settings-group">{toggle('Number row', 'numberRow')}{toggle('Taller keys', 'tallerKeys')}{toggle('Show emoji key on letters', 'emojiKey')}{toggle('Tab beside space', 'tabKey')}{toggle('Comma beside space', 'commaKey')}{toggle('Period beside space', 'periodKey')}</div>
    <div className="v2-settings-group">{toggle('Swipe typing', 'swipeTyping')}{toggle('Haptics', 'haptics')}</div><details><summary>Text & terminal</summary><label className="v2-setting">Terminal font<select><option>Iosevka Term</option><option>Fira Code</option></select></label></details><details><summary>Advanced</summary><span>Prediction · Gesture timing · Preview behavior</span></details></> : <><Slider label="Cursor sensitivity" value={30} sensitivity /><Slider label="Scroll sensitivity" value={30} sensitivity /><h3>Gestures</h3><div className="v2-settings-group"><label className="v2-setting"><span>Hold to right-click</span><input type="checkbox" role="switch" aria-label="Hold to right-click" checked={holdEnabled} onChange={e => setHoldEnabled(e.target.checked)} /></label><label className="v2-setting"><span>Hold duration</span><span className="v2-number"><input type="number" aria-label="Hold duration" min="250" max="1000" step="50" value={holdDelay} onChange={e => setHoldDelay(Math.max(250, Math.min(1000, Number(e.target.value))))} />ms</span></label><label className="v2-setting"><span>Movement tolerance</span><span className="v2-number"><input type="number" aria-label="Movement tolerance" min="2" max="20" step="1" value={holdSlop} onChange={e => setHoldSlop(Math.max(2, Math.min(20, Number(e.target.value))))} />px</span></label></div><details><summary>Advanced</summary><span>Cursor friction · Scroll friction</span></details></>}
  </div>;
}

function App() {
  const view = new URLSearchParams(location.search).get('view') || 'layouts';
  if (view === 'gestures') return <main className="v2-single"><Phone settings /></main>;
  if (view === 'phone') return <main className="v2-single"><Phone variant={new URLSearchParams(location.search).get('variant') === 'split' ? 'split' : 'overlay'} /></main>;
  return <main className="v2-board"><header className="v2-board-head"><span>OTTOMAN / REVISED</span><h1>{view === 'layouts' ? 'Keep the keyboard. Clear the clutter.' : 'Everything stays within reach.'}</h1></header><div className="v2-columns">
    {view === 'layouts' ? <><section><h2>Remote / Two-line field</h2><Phone /></section><section><h2>Preview dismissed</h2><Phone dismissed /></section><section><h2>Menu / Layout, PC, TV</h2><Phone menu /></section></> : <><section><h2>Emoji / Same width</h2><Phone emoji /></section><section><h2>Clipboard / Same row</h2><Phone clip /></section><section><h2>Settings / Optional keys</h2><Phone settings /></section></>}
  </div></main>;
}
createRoot(document.getElementById('root')!).render(<App />);
