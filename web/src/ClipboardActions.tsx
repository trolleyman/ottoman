import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import { ActionKey } from "./KeyboardKeys";
import { ImagePasteButton } from "./ImagePasteButton";

export function ClipboardActions({ connected, onPaste }: { connected: boolean; onPaste: () => void }) {
  const [mode, setMode] = useState<"paste" | "copy" | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const request = useRef<AbortController | null>(null);
  const online = useRef(connected);
  const input = useRef<HTMLTextAreaElement>(null);
  const dialog = useRef<HTMLDivElement>(null);
  useEffect(() => { online.current = connected; if (!connected) request.current?.abort(); }, [connected]);
  useEffect(() => () => request.current?.abort(), []);
  useEffect(() => {
    if (!mode) return;
    const previous = document.activeElement;
    input.current?.focus();
    return () => { if (previous instanceof HTMLElement && previous.isConnected) previous.focus(); };
  }, [mode]);
  const close = () => { request.current?.abort(); setMode(null); setText(""); setMessage(""); setBusy(false); };
  const transfer = async (direction: "copy" | "paste", value = "") => {
    if (busy || !online.current) return;
    if (new TextEncoder().encode(value).length > 1048576) { setMessage("Clipboard text is too large."); return; }
    const operation = new AbortController(); request.current?.abort(); request.current = operation;
    setBusy(true); setMessage("");
    const timeout = window.setTimeout(() => operation.abort(), 12000);
    try {
      const response = await fetch("/api/clipboard/text", { method: direction === "copy" ? "GET" : "POST", headers: { "Content-Type": "text/plain; charset=utf-8" }, body: direction === "paste" ? value : undefined, signal: operation.signal, cache: "no-store" });
      const result = await response.text();
      if (!response.ok) throw Error(result || "Clipboard unavailable.");
      if (operation.signal.aborted || !online.current) return;
      if (direction === "paste") { onPaste(); close(); }
      else { setText(result); setMode("copy"); if (!result) setMessage("PC clipboard is empty."); }
    } catch (error) {
      if (!operation.signal.aborted && online.current) setMessage(error instanceof Error ? error.message : "Clipboard unavailable.");
      else if (online.current) setMessage("Clipboard request timed out.");
    } finally { clearTimeout(timeout); if (request.current === operation) setBusy(false); }
  };
  const paste = async () => {
    setText(""); setMessage(""); setMode("paste");
    try { const value = await navigator.clipboard.readText(); if (online.current) setText(value); } catch { input.current?.focus(); }
  };
  return <>
    <ImagePasteButton connected={connected} onPaste={onPaste} />
    <ActionKey type="button" disabled={!connected || busy} className="clipboard-choice" onPress={() => void paste()}>Paste from phone</ActionKey>
    <ActionKey type="button" disabled={!connected || busy} className="clipboard-choice" onPress={() => void transfer("copy")}>Copy from PC</ActionKey>
    {!mode && message && <span className="clipboard-feedback" role="alert">{message}</span>}
    {mode && <div ref={dialog} role="dialog" aria-modal="true" aria-label={mode === "copy" ? "PC clipboard" : "Paste from phone"} className="keyboard-clipboard-dialog" onKeyDown={e => {
      if (e.key === "Escape") { e.preventDefault(); close(); }
      if (e.key === "Tab") {
        const items = Array.from(dialog.current!.querySelectorAll<HTMLElement>('button:not(:disabled),textarea'));
        if (e.shiftKey && document.activeElement === items[0]) { e.preventDefault(); items.at(-1)?.focus(); }
        else if (!e.shiftKey && document.activeElement === items.at(-1)) { e.preventDefault(); items[0]?.focus(); }
      }
    }}>
      <header><span>{mode === "copy" ? "PC clipboard" : "Paste from phone"}</span><button aria-label="Close clipboard" onClick={close}><X /></button></header>
      <textarea ref={input} aria-label="Clipboard text" value={text} readOnly={mode === "copy"} onChange={e => setText(e.target.value)} placeholder="Touch and hold to paste" />
      {message && <p role="status">{message}</p>}
      <button className="clipboard-primary" disabled={busy || !connected || !text} onClick={() => {
        if (mode === "paste") void transfer("paste", text);
        else { void navigator.clipboard?.writeText(text).then(() => { setMessage("Copied"); }).catch(() => { input.current?.select(); setMessage("Touch and hold to copy."); }); if (!navigator.clipboard) { input.current?.select(); setMessage("Touch and hold to copy."); } }
      }}>{busy ? "Sending…" : mode === "copy" ? "Copy to phone" : "Paste on PC"}</button>
    </div>}
  </>;
}
