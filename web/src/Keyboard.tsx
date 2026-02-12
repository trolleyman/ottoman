import { useState } from "react";
import type { TrackpadSendArgs } from "./types";

export function Keyboard({ send }: { send: (msg: TrackpadSendArgs) => void }) {
  const [text, setText] = useState("");

  return (
    <section>
      <h2 className="text-lg font-semibold text-zinc-200 mb-4">Keyboard</h2>
      <div className="flex gap-2">
        <input
          type="text"
          value={text}
          onChange={(e) => {
            const val = e.target.value;
            if (val) {
              send({ t: "k", text: val });
            }
            setText("");
          }}
          placeholder="Type to send..."
          className="w-full rounded-xl border border-zinc-700 bg-zinc-900 px-4 py-3 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />
      </div>
    </section>
  );
}