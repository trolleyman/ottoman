import { useState } from "react";
import type { TrackpadSendArgs } from "./types";

export function Keyboard({
  send,
  connected,
}: {
  send: (msg: TrackpadSendArgs) => void;
  connected: boolean;
}) {
  const [text, setText] = useState("");

  return (
    <section>
      <h2 className="text-lg font-semibold text-zinc-200 mb-4">Keyboard</h2>
      <input
        type="text"
        value={text}
        onChange={(e) => {
          const val = e.target.value;
          if (val) send({ t: "k", text: val });
          setText("");
        }}
        disabled={!connected}
        placeholder={connected ? "Type to send..." : "Connect to type..."}
        className={`w-full rounded-xl border px-4 py-3 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors ${
          connected ? "border-zinc-700 bg-zinc-900 focus:border-blue-500" : "border-red-900/50 bg-red-950/10 cursor-not-allowed opacity-75"
        }`}
        autoComplete="off"
      />
    </section>
  );
}