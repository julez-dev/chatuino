import { For } from "solid-js";

interface Feature {
  title: string;
  description: string;
  icon: string;
}

const features: Feature[] = [
  {
    title: "Multiple Accounts",
    description:
      "Switch between Twitch accounts easily. Support for anonymous lurking without login.",
    icon: "account",
  },
  {
    title: "Graphical Emotes",
    description:
      "Emotes rendered directly in your terminal. Works with Kitty and Ghostty.",
    icon: "emote",
  },
  {
    title: "7TV, BTTV & FFZ",
    description:
      "Full support for third-party emote providers including FrankerFaceZ. See all your favorite emotes.",
    icon: "provider",
  },
  {
    title: "User Inspect",
    description:
      "View chat history per user. See follow age, subscription status, and all their messages.",
    icon: "inspect",
  },
  {
    title: "Mention Notifications",
    description:
      "Dedicated tab for @mentions across all open channels. Never miss when someone talks to you.",
    icon: "mention",
  },
  {
    title: "Live Alerts",
    description:
      "Know when channels go online or offline. Dedicated notification tab.",
    icon: "live",
  },
  {
    title: "Message Search",
    description:
      "Search through chat history. Find messages and usernames quickly.",
    icon: "search",
  },
  {
    title: "Chat Logging",
    description:
      "SQLite-backed local message persistence. Keep a record of all chats you visit.",
    icon: "log",
  },
  {
    title: "Configurable",
    description: "Customize themes, keybinds, and behavior.",
    icon: "config",
  },
  {
    title: "Self-Hostable",
    description:
      "Don't want to use chatuino.net? Run your own server component. Full control over your data.",
    icon: "server",
  },
];

function FeatureIcon(props: { icon: string }) {
  const iconMap: Record<string, string> = {
    account:
      "M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z",
    emote:
      "M15.182 15.182a4.5 4.5 0 01-6.364 0M21 12a9 9 0 11-18 0 9 9 0 0118 0zM9.75 9.75c0 .414-.168.75-.375.75S9 10.164 9 9.75 9.168 9 9.375 9s.375.336.375.75zm-.375 0h.008v.015h-.008V9.75zm5.625 0c0 .414-.168.75-.375.75s-.375-.336-.375-.75.168-.75.375-.75.375.336.375.75zm-.375 0h.008v.015h-.008V9.75z",
    provider:
      "M9.568 3H5.25A2.25 2.25 0 003 5.25v4.318c0 .597.237 1.17.659 1.591l9.581 9.581c.699.699 1.78.872 2.607.33a18.095 18.095 0 005.223-5.223c.542-.827.369-1.908-.33-2.607L11.16 3.66A2.25 2.25 0 009.568 3z M6 6h.008v.008H6V6z",
    inspect:
      "M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z",
    mention:
      "M16.5 12a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0zm0 0c0 1.657 1.007 3 2.25 3S21 13.657 21 12a9 9 0 10-2.636 6.364M16.5 12V8.25",
    live: "M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.348a1.125 1.125 0 010 1.971l-11.54 6.347a1.125 1.125 0 01-1.667-.985V5.653z",
    search:
      "M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z",
    log: "M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z",
    config:
      "M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z M15 12a3 3 0 11-6 0 3 3 0 016 0z",
    server:
      "M21.75 17.25v-.228a4.5 4.5 0 00-.12-1.03l-2.268-9.64a3.375 3.375 0 00-3.285-2.602H7.923a3.375 3.375 0 00-3.285 2.602l-2.268 9.64a4.5 4.5 0 00-.12 1.03v.228m19.5 0a3 3 0 01-3 3H5.25a3 3 0 01-3-3m19.5 0a3 3 0 00-3-3H5.25a3 3 0 00-3 3m16.5 0h.008v.008h-.008v-.008zm-3 0h.008v.008h-.008v-.008z",
  };

  return (
    <svg
      class="h-6 w-6"
      fill="none"
      viewBox="0 0 24 24"
      stroke-width="1.5"
      stroke="currentColor"
      aria-hidden="true"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d={iconMap[props.icon] || iconMap.config}
      />
    </svg>
  );
}

export default function Features() {
  return (
    <section class="border-t border-nord2 bg-nord1/30 py-16 md:py-24">
      <div class="mx-auto max-w-6xl px-4">
        {/* Section header */}
        <div class="mb-12 text-center">
          <h2 class="mb-4 text-3xl font-bold">
            <span class="text-nord3">[</span>
            <span class="text-nord8"> Features </span>
            <span class="text-nord3">]</span>
          </h2>
          <p class="text-nord4">
            Everything you need for Twitch chat in your terminal
          </p>
        </div>

        {/* Feature grid */}
        <div class="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          <For each={features}>
            {(feature) => (
              <div class="rounded-lg border border-nord2 bg-nord1 p-6 transition-colors hover:border-nord8">
                <div class="mb-4 flex items-center gap-3">
                  <div class="text-nord8">
                    <FeatureIcon icon={feature.icon} />
                  </div>
                  <h3 class="font-semibold text-nord4">{feature.title}</h3>
                </div>
                <p class="text-sm text-nord4">{feature.description}</p>
              </div>
            )}
          </For>
        </div>
      </div>
    </section>
  );
}
