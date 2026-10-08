"use client";

export default function GlobalError({ reset }: { error: Error; reset: () => void }) {
  return (
    <html lang="en">
      <body style={{ background: "#0b0b0c", color: "#ece2d0", fontFamily: "system-ui, sans-serif", padding: 32 }}>
        <h1 style={{ fontWeight: 500 }}>Something went wrong.</h1>
        <p>Please reload the page.</p>
        <button onClick={() => reset()} style={{ marginTop: 16, padding: "12px 20px" }}>
          Reload
        </button>
      </body>
    </html>
  );
}
