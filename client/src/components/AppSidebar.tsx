/* Hallmark · component: app-shell · genre: dashboard · theme: Glass (locked by DESIGN.md)
 * states: default · hover · focus · active · current
 * contrast: pass (Glass tokens)
 */
import { Sidebar } from "@heroui-pro/react";
import { Icon } from "@iconify/react";
import { Link, useMatchRoute, useRouter } from "@tanstack/react-router";
import type * as React from "react";

const NAV = [
  { to: "/", label: "Home", icon: "gravity-ui:house" },
  { to: "/live", label: "Live", icon: "gravity-ui:square-activity" },
  { to: "/sessions", label: "Sessions", icon: "gravity-ui:layers-3" },
] as const;

interface AppShellProps {
  children: React.ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const router = useRouter();
  const matchRoute = useMatchRoute();

  return (
    <Sidebar.Provider
      navigate={(href) => router.navigate({ to: href })}
      variant="floating"
      collapsible="icon"
      defaultOpen
    >
      <Sidebar>
        <Sidebar.Header>
          <Link
            to="/"
            className="flex items-center gap-2 px-2 py-2 no-underline text-foreground"
          >
            <span
              aria-hidden
              className="grid size-7 place-items-center rounded-md bg-accent text-accent-foreground"
            >
              <Icon icon="gravity-ui:car-arrow-right" className="size-4" />
            </span>
            <span className="text-sm font-semibold tracking-tight">Forza Telemetry</span>
          </Link>
        </Sidebar.Header>

        <Sidebar.Content>
          <Sidebar.Menu aria-label="Primary navigation">
            {NAV.map((item) => {
              const exact = item.to === "/";
              const active = !!matchRoute({ to: item.to, fuzzy: !exact });
              return (
                <Sidebar.MenuItem
                  key={item.to}
                  href={item.to}
                  isCurrent={active}
                  tooltip={item.label}
                >
                  <Sidebar.MenuIcon>
                    <Icon icon={item.icon} />
                  </Sidebar.MenuIcon>
                  <Sidebar.MenuLabel>{item.label}</Sidebar.MenuLabel>
                </Sidebar.MenuItem>
              );
            })}
          </Sidebar.Menu>
        </Sidebar.Content>

        <Sidebar.Footer>
          <div className="flex items-center gap-2 px-2 py-2">
            <span
              aria-hidden
              className="grid size-7 place-items-center rounded-full bg-surface-secondary text-foreground/70 text-xs"
            >
              LO
            </span>
            <div className="flex min-w-0 flex-col">
              <span className="truncate text-xs font-medium text-foreground">Local</span>
              <span className="truncate text-xs text-muted">LAN single-user</span>
            </div>
          </div>
        </Sidebar.Footer>
      </Sidebar>

      <Sidebar.Main className="bg-background">
        <div className="mx-auto w-full max-w-6xl px-6 pt-8 pb-12 sm:px-8">
          {children}
        </div>
      </Sidebar.Main>
    </Sidebar.Provider>
  );
}
