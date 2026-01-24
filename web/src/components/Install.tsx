import { createSignal, For, onMount, Show } from "solid-js";
import { detectOS, getOSDisplayName, type OS } from "@/lib/os-detect";
import { cn } from "@/lib/utils";

interface InstallMethod {
  id: string;
  name: string;
  os: OS | "all";
  primary?: boolean;
  code?: string;
  description?: string;
  link?: string;
  linkText?: string;
}

const installMethods: InstallMethod[] = [
  {
    id: "curl-linux",
    name: "Install Script",
    os: "linux",
    primary: true,
    code: "curl -sSfL https://chatuino.net/install | sh",
    description: "Recommended for Linux. Downloads the latest release.",
  },
  {
    id: "curl-macos",
    name: "Install Script",
    os: "macos",
    primary: true,
    code: "curl -sSfL https://chatuino.net/install | sh",
    description: "Recommended for macOS. Downloads the latest release.",
  },
  {
    id: "windows-binary",
    name: "Pre-built Binary",
    os: "windows",
    primary: true,
    description: "Download the Windows executable from the releases page.",
    link: "https://github.com/julez-dev/chatuino/releases",
    linkText: "Download from Releases",
  },
  {
    id: "aur",
    name: "AUR (Arch Linux)",
    os: "linux",
    code: "yay -S chatuino-bin",
    description: "Available in the Arch User Repository.",
  },
  {
    id: "go-install",
    name: "Go Install",
    os: "all",
    code: "go install github.com/julez-dev/chatuino@latest",
    description: "Requires Go 1.21+.",
  },
  {
    id: "releases",
    name: "Pre-built Binaries",
    os: "all",
    description: "Download binaries for Linux, macOS, and Windows.",
    link: "https://github.com/julez-dev/chatuino/releases",
    linkText: "View Releases",
  },
  {
    id: "docker",
    name: "Docker",
    os: "all",
    code: "docker pull ghcr.io/julez-dev/chatuino:latest",
    description: "For running the server component.",
  },
];

function CodeBlock(props: { code: string }) {
  const [copied, setCopied] = createSignal(false);

  const copyToClipboard = async () => {
    await navigator.clipboard.writeText(props.code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div class="group relative">
      <pre class="overflow-x-auto rounded-md bg-nord0 p-4 text-sm text-nord4">
        <code>{props.code}</code>
      </pre>
      <button
        type="button"
        onClick={copyToClipboard}
        class="absolute right-2 top-2 rounded bg-nord2 px-2 py-1 text-xs text-nord4 opacity-0 transition-opacity hover:bg-nord3 focus:opacity-100 group-hover:opacity-100"
        aria-label="Copy to clipboard"
      >
        {copied() ? "Copied!" : "Copy"}
      </button>
    </div>
  );
}

function InstallCard(props: { method: InstallMethod }) {
  return (
    <div class="rounded-lg border border-nord2 bg-nord1 p-4">
      <h4 class="mb-2 font-medium text-nord4">{props.method.name}</h4>
      <Show when={props.method.description}>
        <p class="mb-3 text-sm text-nord4">{props.method.description}</p>
      </Show>
      <Show when={props.method.code} keyed>
        {(code) => <CodeBlock code={code} />}
      </Show>
      <Show when={props.method.link}>
        <a
          href={props.method.link}
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-2 rounded-md bg-nord8 px-4 py-2 text-sm font-medium transition-colors hover:bg-nord7"
          style={{ color: "#000b1e" }}
        >
          {props.method.linkText}
          <svg
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
            />
          </svg>
        </a>
      </Show>
    </div>
  );
}

export default function Install() {
  const [detectedOS, setDetectedOS] = createSignal<OS>("unknown");
  const [showAll, setShowAll] = createSignal(false);

  onMount(() => {
    setDetectedOS(detectOS());
  });

  const primaryMethod = () => {
    const os = detectedOS();
    return installMethods.find((m) => m.os === os && m.primary);
  };

  const otherMethods = () => {
    const primary = primaryMethod();
    return installMethods.filter((m) => m.id !== primary?.id);
  };

  return (
    <section id="install" class="border-t border-nord2 py-16 md:py-24">
      <div class="mx-auto max-w-4xl px-4">
        {/* Section header */}
        <div class="mb-12 text-center">
          <h2 class="mb-4 text-3xl font-bold">
            <span class="text-nord3">[</span>
            <span class="text-nord8"> Install </span>
            <span class="text-nord3">]</span>
          </h2>
          <p class="text-nord4">
            Get started with Chatuino on {getOSDisplayName(detectedOS())}
          </p>
        </div>

        {/* Primary install method */}
        <Show when={primaryMethod()}>
          {(method) => (
            <div class="mb-8">
              <div class="mb-4 flex items-center gap-2 text-sm text-nord4">
                <span class="text-nord14">*</span>
                <span>Recommended for {getOSDisplayName(detectedOS())}</span>
              </div>
              <InstallCard method={method()} />
            </div>
          )}
        </Show>

        {/* Toggle for other methods */}
        <div class="mb-6">
          <button
            type="button"
            onClick={() => setShowAll(!showAll())}
            class={cn(
              "flex items-center gap-2 py-2 text-sm transition-colors",
              showAll() ? "text-nord8" : "text-nord4 hover:text-nord8",
            )}
            aria-expanded={showAll()}
            aria-controls="other-methods"
          >
            <svg
              class={cn(
                "h-4 w-4 transition-transform",
                showAll() && "rotate-90",
              )}
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9 5l7 7-7 7"
              />
            </svg>
            <span>Other installation methods</span>
          </button>
        </div>

        {/* Other methods */}
        <Show when={showAll()}>
          <div id="other-methods" class="grid gap-4 sm:grid-cols-2">
            <For each={otherMethods()}>
              {(method) => <InstallCard method={method} />}
            </For>
          </div>
        </Show>

        {/* Post-install note */}
        <div class="mt-12 rounded-lg border border-nord2 bg-nord1 p-6">
          <h3 class="mb-2 font-medium text-nord4">After Installation</h3>
          <p class="mb-4 text-sm text-nord4">
            Run{" "}
            <code class="rounded bg-nord0 px-1.5 py-0.5 text-nord8">
              chatuino
            </code>{" "}
            to start the application. Use{" "}
            <code class="rounded bg-nord0 px-1.5 py-0.5 text-nord8">
              chatuino account
            </code>{" "}
            to manage your Twitch accounts.
          </p>
          <p class="text-sm text-nord4">
            Press{" "}
            <code class="rounded bg-nord0 px-1.5 py-0.5 text-nord8">?</code>{" "}
            inside Chatuino to view all keybindings.
          </p>
        </div>
      </div>
    </section>
  );
}
