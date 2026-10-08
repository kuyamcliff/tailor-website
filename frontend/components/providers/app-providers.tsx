"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { ConfigProvider } from "./config";
import { SessionProvider } from "./session";
import { ToastProvider } from "./toast";
import type { ContentBlocks, PublicConfig } from "@/lib/types";
import { ApiError } from "@/lib/api";

export function AppProviders({ config, content, children }: { config: PublicConfig; content: ContentBlocks; children: React.ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 15_000,
            refetchOnWindowFocus: true,
            retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
          },
        },
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <ConfigProvider config={config} content={content}>
        <SessionProvider>
          <ToastProvider>{children}</ToastProvider>
        </SessionProvider>
      </ConfigProvider>
    </QueryClientProvider>
  );
}
