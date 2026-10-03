/** Preserve server diagnostics instead of the generated client's generic 500 text. */
export function describeApiError(error: unknown, fallback: string): string {
  if (typeof error === "object" && error !== null) {
    const apiError = error as { status?: unknown; body?: unknown };
    const body = apiError.body;
    let detail: unknown;
    if (typeof body === "string") detail = body;
    else if (typeof body === "object" && body !== null) {
      const response = body as { error?: unknown; message?: unknown };
      detail = response.error || response.message;
    }
    const message = typeof detail === "string" && detail.trim()
      ? detail.trim()
      : error instanceof Error ? error.message : fallback;
    return typeof apiError.status === "number"
      ? `HTTP ${apiError.status}: ${message}`
      : message;
  }
  return fallback;
}
