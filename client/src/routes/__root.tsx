import { Toast } from "@heroui/react";
import { addCollection } from "@iconify/react";
import { icons as lucideIcons } from "@iconify-json/lucide";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { HeadContent, Outlet, Scripts, createRootRouteWithContext } from "@tanstack/react-router";
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools";
import { AppShell } from "~/components/AppSidebar";
import { DefaultCatchBoundary } from "~/components/DefaultCatchBoundary";
import { NotFound } from "~/components/NotFound";
import appCss from "~/styles/app.css?url";
import { seo } from "~/utils/seo";
import type { QueryClient } from "@tanstack/react-query";
import type * as React from "react";

// Bundle the full lucide set so icons render offline — @iconify/react
// otherwise fetches icon data from api.iconify.design at runtime, and on an
// offline LAN every icon silently became an empty <span>. ~540KB raw
// (~100KB gz) inside an embedded binary buys "any lucide name always works"
// with no per-icon bookkeeping. Runs at module scope, before any Icon mounts.
addCollection(lucideIcons);

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient;
}>()({
  component: RootComponent,
  errorComponent: (props) => (
    <RootDocument>
      <DefaultCatchBoundary {...props} />
    </RootDocument>
  ),
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      ...seo({
        title: "Forza Telemetry",
        description: "Live + historical telemetry from Forza Horizon Data Out.",
      }),
    ],
    links: [
      // Fonts are self-hosted via @fontsource imports in app.css — no external
      // hosts; this SPA must render identically on an offline LAN.
      { rel: "stylesheet", href: appCss },
      { rel: "apple-touch-icon", sizes: "180x180", href: "/apple-touch-icon.png" },
      { rel: "icon", type: "image/png", sizes: "32x32", href: "/favicon-32x32.png" },
      { rel: "icon", type: "image/png", sizes: "16x16", href: "/favicon-16x16.png" },
      { rel: "manifest", href: "/site.webmanifest", color: "#fffff" },
      { rel: "icon", href: "/favicon.ico" },
    ],
  }),
  notFoundComponent: () => (
    <RootDocument>
      <AppShell>
        <NotFound />
      </AppShell>
    </RootDocument>
  ),
});

function RootComponent() {
  return (
    <RootDocument>
      <AppShell>
        <Outlet />
      </AppShell>
    </RootDocument>
  );
}

function RootDocument({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" data-theme="glass-dark" className="glass-dark">
      <head>
        <HeadContent />
      </head>
      <body className="bg-background font-sans text-foreground antialiased">
        {children}
        <Toast.Provider placement="bottom end" />
        <TanStackRouterDevtools position="bottom-right" />
        <ReactQueryDevtools buttonPosition="bottom-left" />
        <Scripts />
      </body>
    </html>
  );
}
