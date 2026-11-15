import { useEffect, useRef, useState } from "react";
import { LoaderCircle, X } from "lucide-react";
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
  const request = useRef<AbortController | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { online.current = connected; if (!connected) request.current?.abort(); }, [connected]);
  useEffect(() => () => request.current?.abort(), []);
  const select = async (file: File) => {
    if (busy || !online.current) return;
    const operation = new AbortController(); request.current = operation;
    setBusy(true); setError("");
    const timeout = window.setTimeout(() => operation.abort(), 30000);
    try {
      const blob = await prepareImage(file);
      if (operation.signal.aborted) return;
      const response = await fetch("/api/clipboard/image", { method: "POST", headers: { "Content-Type": "image/png" }, body: blob, signal: operation.signal });
      if (!response.ok) throw Error(response.status === 401 ? "Sign in again to paste images." : await response.text() || "Could not paste image.");
      if (!operation.signal.aborted && online.current) onPaste();
    } catch (error) {
      if (online.current) setError(operation.signal.aborted ? "Image paste timed out." : error instanceof Error ? error.message : "Could not paste image.");
    } finally { clearTimeout(timeout); setBusy(false); }
  };
  return <>
    <input ref={input} type="file" accept="image/*" hidden aria-label="Choose an image" onChange={event => { const file = event.target.files?.[0]; event.target.value = ""; if (file) void select(file); }} />
    <ActionKey type="button" aria-label="Paste image" disabled={!connected || busy} onPress={() => input.current?.click()} className="clipboard-choice">{busy ? <LoaderCircle className="h-3 w-3 animate-spin" /> : null}Paste image</ActionKey>
    {busy && <span role="status" className="clipboard-feedback">Sending image…</span>}
    {error && <span role="alert" className="clipboard-feedback"><span>{error}</span><button aria-label="Dismiss image error" onClick={() => setError("")}><X className="h-4 w-4" /></button></span>}
  </>;
}
