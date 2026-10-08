import "server-only";

// Server-side fetch for public data used in server components. Requests go straight to the Go API
// on the internal network and are cached with short revalidation.

const base = process.env.API_INTERNAL_URL ?? process.env.BACKEND_URL ?? "http://localhost:8080";

export class ServerApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function serverApi<T>(path: string, revalidate = 60): Promise<T> {
  const res = await fetch(`${base}/api/v1${path}`, {
    next: { revalidate },
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new ServerApiError(res.status, `API ${path} returned ${res.status}`);
  return (await res.json()) as T;
}

// serverApiOr returns a fallback when the API is unavailable, so a page can still render its
// static structure (the storefront must remain usable if a dependency fails).
export async function serverApiOr<T>(path: string, fallback: T, revalidate = 60): Promise<T> {
  try {
    return await serverApi<T>(path, revalidate);
  } catch (e) {
    if (e instanceof ServerApiError && e.status === 404) return fallback;
    console.error(`[serverApi] ${path}:`, e instanceof Error ? e.message : e);
    return fallback;
  }
}
