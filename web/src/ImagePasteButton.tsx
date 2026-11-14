import { useEffect, useRef, useState } from "react";
import { ImagePlus, LoaderCircle, X } from "lucide-react";
import { ActionKey } from "./KeyboardKeys";

const MAX_UPLOAD = 20 * 1024 * 1024;

async function prepareImage(file: File): Promise<Blob> {
  if (file.size > 40 * 1024 * 1024) throw Error("Choose a photo smaller than 40 MB.");
  const url = URL.createObjectURL(file);
  const image = new Image();
  try {
    image.src = url;
    await image.decode();
    const scale = Math.min(1, 4096 / Math.max(image.naturalWidth, image.naturalHeight));
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) throw Error("Could not prepare this photo.");
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const png = await new Promise<Blob>((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(Error("Could not prepare this photo.")), "image/png"));
    if (png.size > MAX_UPLOAD) throw Error("This photo is too large to paste. Choose a smaller image.");
    return png;
  } catch (error) {
    if (error instanceof DOMException) throw Error("This phone cannot open that image format. Try a JPEG or PNG photo.", { cause: error });
    throw error;
  } finally {
    URL.revokeObjectURL(url);
  }
}

export function ImagePasteButton({ connected, onPaste }: { connected: boolean; onPaste: () => void }) {
  const input = useRef<HTMLInputElement>(null);
  const online = useRef(connected);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const dialog = useRef<HTMLDivElement>(null);
  useEffect(() => { online.current = connected; }, [connected]);
  const request = useRef<AbortController | null>(null);
  const [preview, setPreview] = useState<{ blob: Blob; url: string; name: string } | null>(null);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => () => { request.current?.abort(); }, []);
  useEffect(() => () => { if (preview) URL.revokeObjectURL(preview.url); }, [preview]);
  const close = () => {
    request.current?.abort();
    request.current = null;
    setOpen(false); setPreview(null); setError(""); setBusy(false);
  };
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement;
    cancelButton.current?.focus();
    return () => { if (previous instanceof HTMLElement) previous.focus(); };
  }, [open]);
  const select = async (file: File) => {
    const operation = new AbortController();
    request.current?.abort(); request.current = operation;
    setOpen(true); setBusy(true); setError(""); setPreview(null);
    try {
      const blob = await prepareImage(file);
      if (operation.signal.aborted) return;
      setPreview({ blob, url: URL.createObjectURL(blob), name: file.name });
    } catch (error) {
      if (!operation.signal.aborted) setError(error instanceof Error ? error.message : "Could not open this photo.");
    } finally { if (!operation.signal.aborted) setBusy(false); }
  };
  const paste = async () => {
    if (!preview || busy || !connected) return;
    const operation = new AbortController();
    request.current = operation; setBusy(true); setError("");
    const timeout = window.setTimeout(() => operation.abort(), 30000);
    try {
      const response = await fetch("/api/clipboard/image", { method: "POST", headers: { "Content-Type": "image/png" }, body: preview.blob, signal: operation.signal });
      if (!response.ok) {
        const message = response.status === 401 ? "Your session expired. Sign in again to paste images." : await response.text();
        throw Error(message || "Could not copy the image to the desktop.");
      }
      // Only send the paste shortcut after the desktop owns the clipboard.
      if (operation.signal.aborted) return;
      if (!online.current) throw Error("Desktop disconnected. Reconnect and try again.");
      onPaste(); close();
    } catch (error) {
      if (request.current === operation) setError(operation.signal.aborted ? "Upload timed out. Please try again." : error instanceof Error ? error.message : "Could not paste this image.");
    } finally { clearTimeout(timeout); if (request.current === operation) setBusy(false); }
  };
  return <>
    <input ref={input} type="file" accept="image/*" className="hidden" aria-label="Choose an image" onChange={event => {
      const file = event.currentTarget.files?.[0];
      event.currentTarget.value = "";
      if (file) void select(file);
    }} />
    <ActionKey type="button" aria-label="Paste image" title="Paste image" disabled={!connected}
      onPress={() => input.current?.click()}
      className="custom-keyboard-utility flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-zinc-800 bg-zinc-900 text-zinc-400 disabled:opacity-40">
      <ImagePlus className="h-4 w-4" />
    </ActionKey>
    {open && <div ref={dialog} onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); close(); }
      if (event.key === "Tab") {
        const buttons = Array.from(dialog.current!.querySelectorAll<HTMLButtonElement>("button:not(:disabled)"));
        const first = buttons[0], last = buttons[buttons.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
      }
    }} role="dialog" aria-modal="true" aria-label="Paste image" className="absolute inset-0 z-[60] flex flex-col gap-2 rounded-[inherit] bg-zinc-950 p-3 text-zinc-100">
      <div className="flex shrink-0 items-center justify-between">
        <span className="text-sm font-medium">Paste image</span>
        <button ref={cancelButton} type="button" aria-label="Cancel image paste" onClick={close} onPointerUp={event => { event.preventDefault(); close(); }} className="flex h-9 w-9 items-center justify-center rounded-md bg-zinc-800"><X className="h-4 w-4" /></button>
      </div>
      {preview && <img src={preview.url} alt={preview.name} className="min-h-0 flex-1 object-contain" />}
      {busy && <p role="status" className="flex items-center justify-center gap-2 text-sm"><LoaderCircle className="h-4 w-4 animate-spin" />{preview ? "Sending image…" : "Preparing image…"}</p>}
      {error && <p role="alert" className="text-sm text-red-300">{error}</p>}
      <p className="shrink-0 text-xs text-zinc-400">Pastes into the focused desktop app and replaces its clipboard.</p>
      <ActionKey type="button" disabled={!preview || busy || !connected} onPress={() => { void paste(); }}
        className="min-h-10 shrink-0 rounded-md bg-blue-500 px-3 text-sm font-medium text-white disabled:opacity-40">Paste</ActionKey>
    </div>}
  </>;
}
