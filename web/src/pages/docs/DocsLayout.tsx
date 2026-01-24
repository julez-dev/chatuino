import { A, useLocation } from "@solidjs/router";
import { createSignal, Show, type ParentProps } from "solid-js";
import { cn } from "@/lib/utils";

interface NavItem {
  href: string;
  label: string;
}

const navItems: NavItem[] = [
  { href: "/docs/features", label: "Features" },
  { href: "/docs/settings", label: "Settings" },
  { href: "/docs/theme", label: "Theme" },
  { href: "/docs/self-host", label: "Self-Host" },
];

export default function DocsLayout(props: ParentProps) {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = createSignal(false);

  const isActive = (href: string) => location.pathname === href;

  return (
    <div class="mx-auto flex max-w-6xl gap-8 px-4 py-8">
      {/* Mobile sidebar toggle */}
      <button
        class="fixed bottom-4 right-4 z-50 rounded-full bg-nord8 p-3 text-nord0 shadow-lg md:hidden"
        onClick={() => setSidebarOpen(!sidebarOpen())}
        aria-label="Toggle sidebar"
      >
        <Show
          when={sidebarOpen()}
          fallback={
            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          }
        >
          <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </Show>
      </button>

      {/* Sidebar overlay (mobile) */}
      <Show when={sidebarOpen()}>
        <div
          class="fixed inset-0 z-40 bg-nord0/80 md:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      </Show>

      {/* Sidebar */}
      <aside
        class={cn(
          "fixed inset-y-0 left-0 z-40 w-64 transform border-r border-nord2 bg-nord0 p-6 pt-20 transition-transform md:relative md:inset-auto md:translate-x-0 md:border-0 md:bg-transparent md:p-0 md:pt-0",
          sidebarOpen() ? "translate-x-0" : "-translate-x-full"
        )}
      >
        <nav class="sticky top-24">
          <h3 class="mb-4 text-sm font-semibold text-nord3">Documentation</h3>
          <ul class="space-y-2">
            {navItems.map((item) => (
              <li>
                <A
                  href={item.href}
                  class={cn(
                    "block rounded-md px-3 py-2 text-sm transition-colors",
                    isActive(item.href)
                      ? "bg-nord1 text-nord8"
                      : "text-nord4 hover:bg-nord1 hover:text-nord8"
                  )}
                  onClick={() => setSidebarOpen(false)}
                >
                  {item.label}
                </A>
              </li>
            ))}
          </ul>
        </nav>
      </aside>

      {/* Main content */}
      <main class="min-w-0 flex-1">
        <article class="prose prose-invert max-w-none">
          {props.children}
        </article>
      </main>
    </div>
  );
}
