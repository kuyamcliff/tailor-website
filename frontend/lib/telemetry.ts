// Error and performance reporting integration point. Without NEXT_PUBLIC_TELEMETRY_URL nothing is sent.
// Reports never include personal data: only the error message, route and non-identifying context.

const endpoint = process.env.NEXT_PUBLIC_TELEMETRY_URL;

function send(kind: string, payload: Record<string, unknown>) {
  if (!endpoint || typeof window === "undefined") return;
  const body = JSON.stringify({ kind, path: window.location.pathname, ts: Date.now(), ...payload });
  try {
    if (!navigator.sendBeacon?.(endpoint, body)) {
      void fetch(endpoint, { method: "POST", body, keepalive: true, headers: { "Content-Type": "application/json" } });
    }
  } catch {
    // reporting must never break the page
  }
}

export function reportError(error: Error, context: Record<string, unknown> = {}) {
  if (process.env.NODE_ENV !== "production") console.error(error);
  send("error", { message: error.message.slice(0, 300), name: error.name, ...context });
}

export function reportMetric(name: string, value: number, extra: Record<string, unknown> = {}) {
  send("metric", { name, value: Math.round(value * 100) / 100, ...extra });
}
