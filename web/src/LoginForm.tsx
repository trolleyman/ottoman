import { useState } from "react";
import { OttomanWithLogo } from "./OttomanWithLogo";
import { useStore } from "./store";

export function LoginForm() {
  const login = useStore((s) => s.login);
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim() || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const result = await login(token);
      if (!result.success) {
        setError(result.message || "Authentication failed");
      }
    } catch {
      setError("Connection failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
      <form onSubmit={submit} className="w-full max-w-sm px-6">
        <OttomanWithLogo className="mb-4">
          <p className="text-zinc-500 text-sm">
            Enter your auth token to continue.
          </p>
        </OttomanWithLogo>

        {error && (
          <div className="mb-4 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm px-4 py-3">
            {error}
          </div>
        )}

        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Auth token"
          autoFocus
          className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-4 py-2.5 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />

        <button
          type="submit"
          disabled={submitting || !token.trim()}
          className="mt-4 w-full rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
        >
          {submitting ? "Authenticating..." : "Log in"}
        </button>
      </form>
    </div>
  );
}
