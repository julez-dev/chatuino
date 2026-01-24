import { A, useLocation } from "@solidjs/router";
import { createSignal, Show } from "solid-js";
import { cn } from "@/lib/utils";

export default function Header() {
  const location = useLocation();
  const [mobileMenuOpen, setMobileMenuOpen] = createSignal(false);

  const isActive = (path: string) => {
    if (path === "/") return location.pathname === "/";
    return location.pathname.startsWith(path);
  };

  return (
    <header class="sticky top-0 z-50 border-b border-nord2 bg-nord0/95 backdrop-blur-sm">
      <nav class="mx-auto flex max-w-6xl items-center justify-between px-4 py-3">
        {/* Logo */}
        <A href="/" class="flex items-center gap-2 text-lg font-semibold">
          <span class="text-nord3">[</span>
          <span class="text-nord8">Chatuino</span>
          <span class="text-nord3">]</span>
        </A>

        {/* Desktop nav */}
        <div class="hidden items-center gap-6 md:flex">
          <A
            href="/"
            class={cn(
              "transition-colors hover:text-nord8",
              isActive("/") && location.pathname === "/"
                ? "text-nord8"
                : "text-nord4",
            )}
          >
            Home
          </A>
          <A
            href="/docs/features"
            class={cn(
              "transition-colors hover:text-nord8",
              isActive("/docs") ? "text-nord8" : "text-nord4",
            )}
          >
            Documentation
          </A>
          <a
            href="https://github.com/julez-dev/chatuino"
            target="_blank"
            rel="noopener noreferrer"
            class="text-nord4 transition-colors hover:text-nord8"
          >
            GitHub
          </a>
        </div>

        {/* Mobile menu button */}
        <button
          type="button"
          class="text-nord4 hover:text-nord8 md:hidden"
          onClick={() => setMobileMenuOpen(!mobileMenuOpen())}
          aria-label="Toggle menu"
        >
          <Show
            when={mobileMenuOpen()}
            fallback={
              <svg
                class="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 12h16M4 18h16"
                />
              </svg>
            }
          >
            <svg
              class="h-6 w-6"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </Show>
        </button>
      </nav>

      {/* Mobile menu */}
      <Show when={mobileMenuOpen()}>
        <div class="border-t border-nord2 bg-nord1 px-4 py-4 md:hidden">
          <div class="flex flex-col gap-4">
            <A
              href="/"
              class={cn(
                "transition-colors hover:text-nord8",
                isActive("/") && location.pathname === "/"
                  ? "text-nord8"
                  : "text-nord4",
              )}
              onClick={() => setMobileMenuOpen(false)}
            >
              Home
            </A>
            <A
              href="/docs/features"
              class={cn(
                "transition-colors hover:text-nord8",
                isActive("/docs") ? "text-nord8" : "text-nord4",
              )}
              onClick={() => setMobileMenuOpen(false)}
            >
              Documentation
            </A>
            <a
              href="https://github.com/julez-dev/chatuino"
              target="_blank"
              rel="noopener noreferrer"
              class="text-nord4 transition-colors hover:text-nord8"
            >
              GitHub
            </a>
          </div>
        </div>
      </Show>
    </header>
  );
}
