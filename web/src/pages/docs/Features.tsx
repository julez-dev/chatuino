import { Link, Meta, Title } from "@solidjs/meta";
import { PreviewImage } from "@/components/ImagePreview";

export default function Features() {
  return (
    <div>
      <Title>Features - Chatuino</Title>
      <Meta
        name="description"
        content="Explore Chatuino features: account management, graphical emotes, 7TV/BTTV/FFZ support, user inspection, message logging, and more."
      />
      <Link rel="canonical" href="https://chatuino.net/docs/features" />
      <h1 class="mb-8 text-3xl font-bold text-nord4">
        <span class="text-nord3">[</span>
        <span class="text-nord8"> Features </span>
        <span class="text-nord3">]</span>
      </h1>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">
          Account Management
        </h2>
        <p class="mb-4 text-nord4">
          Chatuino allows you to manage multiple accounts in addition to an
          anonymous account, which lets you view chats without logging in.
        </p>
        <div class="overflow-hidden rounded-lg border border-nord2">
          <PreviewImage
            src="/screenshots/account-ui.png"
            alt="Account UI showing account management interface"
            class="w-full"
            loading="lazy"
          />
        </div>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">State Persistence</h2>
        <p class="mb-4 text-nord4">
          Chatuino saves your open tabs when you exit the application. When you
          restart, it attempts to restore your last session with all open tabs.
        </p>
        <p class="text-nord4">
          Chatuino is designed for users who monitor multiple channels
          simultaneously over extended periods.
        </p>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Chat</h2>
        <p class="mb-4 text-nord4">
          Chatuino displays various Twitch events including messages, sub-gifts,
          timeouts, announcements, and polls in your own chat.
        </p>
        <p class="mb-4 text-nord4">
          Use local commands like{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            /localsubscribers
          </code>{" "}
          and{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            /uniqueonly
          </code>{" "}
          to filter chat locally.
        </p>

        <h3 class="mb-2 mt-6 text-lg font-medium text-nord4">Navigation</h3>
        <ul class="mb-4 list-inside list-disc space-y-2 text-nord4">
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">/</code> to
            start a search (see syntax below)
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">t</code> to
            jump to the top of the buffer
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">b</code> to
            jump to the bottom
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">?</code> to
            view all key bindings
          </li>
        </ul>

        <h3 class="mb-2 mt-6 text-lg font-medium text-nord4">Search Syntax</h3>
        <p class="mb-4 text-nord4">
          Press <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">/</code>{" "}
          to open the search bar. By default, typing a term searches both
          message content and usernames. Use prefixes to target specific fields
          or apply advanced filters. Multiple filters are combined with AND.
        </p>
        <div class="mb-4 overflow-x-auto rounded-lg border border-nord2">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-nord2 bg-nord1">
              <tr>
                <th class="px-4 py-2 font-semibold text-nord8">Filter</th>
                <th class="px-4 py-2 font-semibold text-nord8">Description</th>
              </tr>
            </thead>
            <tbody class="text-nord4">
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">hello</code>
                </td>
                <td class="px-4 py-2">Message or username contains "hello"</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">content:term</code>
                </td>
                <td class="px-4 py-2">Message content contains term</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">user:term</code>
                </td>
                <td class="px-4 py-2">Username contains term</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">badge:name</code>
                </td>
                <td class="px-4 py-2">
                  User has badge (e.g.{" "}
                  <code class="text-nord8">badge:moderator</code>)
                </td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">is:mod|sub|vip|first</code>
                </td>
                <td class="px-4 py-2">Filter by user property</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">/pattern/</code>
                </td>
                <td class="px-4 py-2">Regex on content and username</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">user:/pattern/</code>
                </td>
                <td class="px-4 py-2">Regex scoped to username only</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">content:/pattern/</code>
                </td>
                <td class="px-4 py-2">Regex scoped to content only</td>
              </tr>
              <tr class="border-b border-nord2">
                <td class="px-4 py-2">
                  <code class="text-nord8">-filter</code>
                </td>
                <td class="px-4 py-2">
                  Negate any filter (e.g.{" "}
                  <code class="text-nord8">-user:nightbot</code>)
                </td>
              </tr>
              <tr>
                <td class="px-4 py-2">
                  <code class="text-nord8">"quoted value"</code>
                </td>
                <td class="px-4 py-2">Match a phrase with spaces</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="mb-4 text-sm text-nord4">
          <strong class="text-nord8">Aliases:</strong>{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">msg:</code> for{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">content:</code>,{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">from:</code> for{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">user:</code>,{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">regex:</code>{" "}
          for{" "}
          <code class="rounded bg-nord1 px-1 py-0.5 text-nord8">/pattern/</code>
        </p>
        <p class="text-sm text-nord4">
          <strong class="text-nord8">Example:</strong>{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            user:julez content:GG is:sub
          </code>{" "}
          matches messages from "julez" containing "GG" that are from a
          subscriber.
        </p>

        <h3 class="mb-2 mt-6 text-lg font-medium text-nord4">
          Writing Messages
        </h3>
        <ul class="mb-4 list-inside list-disc space-y-2 text-nord4">
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">i</code> to
            enter insert mode
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
              Escape
            </code>{" "}
            to exit insert mode
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">Enter</code>{" "}
            to send a message
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
              Alt+Enter
            </code>{" "}
            to send while keeping text in input
          </li>
          <li>
            Press{" "}
            <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">Alt+C</code>{" "}
            on a message to copy it to your input
          </li>
        </ul>

        <div class="mt-6 grid gap-4 md:grid-cols-2">
          <div class="overflow-hidden rounded-lg border border-nord2">
            <PreviewImage
              src="/screenshots/chat-view.png"
              alt="Chat view showing Twitch chat"
              class="w-full"
              loading="lazy"
            />
            <p class="border-t border-nord2 bg-nord1 px-3 py-2 text-sm text-nord4">
              Chat View
            </p>
          </div>
          <div class="overflow-hidden rounded-lg border border-nord2">
            <PreviewImage
              src="/screenshots/message-search.png"
              alt="Message search interface"
              class="w-full"
              loading="lazy"
            />
            <p class="border-t border-nord2 bg-nord1 px-3 py-2 text-sm text-nord4">
              Message Search
            </p>
          </div>
        </div>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Auto-Completion</h2>
        <p class="mb-4 text-nord4">
          Chatuino provides auto-completion for channel names when joining new
          chats, usernames in chat, and emotes.
        </p>
        <p class="mb-4 text-nord4">
          Commands like{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">/ban</code>,{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">/unban</code>,
          and{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            /timeout
          </code>{" "}
          are also suggested.
        </p>
        <div class="grid gap-4 md:grid-cols-2">
          <div class="overflow-hidden rounded-lg border border-nord2">
            <PreviewImage
              src="/screenshots/auto-completions-emotes.png"
              alt="Emote auto-completion"
              class="w-full"
              loading="lazy"
            />
            <p class="border-t border-nord2 bg-nord1 px-3 py-2 text-sm text-nord4">
              Emote Completion
            </p>
          </div>
          <div class="overflow-hidden rounded-lg border border-nord2">
            <PreviewImage
              src="/screenshots/auto-completions_user.png"
              alt="Username auto-completion"
              class="w-full"
              loading="lazy"
            />
            <p class="border-t border-nord2 bg-nord1 px-3 py-2 text-sm text-nord4">
              Username Completion
            </p>
          </div>
        </div>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">User Inspection</h2>
        <p class="mb-4 text-nord4">
          Inspect individual chatters to view all their messages (that you've
          seen), follow age, and subscription status.
        </p>
        <p class="mb-4 text-nord4">
          Search is supported. Start user inspection with{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">Ctrl+L</code>{" "}
          or the{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            /inspect username
          </code>{" "}
          command. Chatuino also displays all messages that mention the user.
        </p>
        <p class="mb-4 text-nord4">
          Chatuino only shows messages you've seen, but every message can be
          persisted locally when configured in settings, allowing you to
          maintain a local log of all chats you visit.
        </p>
        <div class="overflow-hidden rounded-lg border border-nord2">
          <PreviewImage
            src="/screenshots/message-log.png"
            alt="User inspection showing message log"
            class="w-full"
            loading="lazy"
          />
        </div>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Tab Types</h2>
        <p class="mb-4 text-nord4">
          Chatuino offers three tab types when creating a new tab with{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">Ctrl+T</code>:
        </p>
        <ul class="list-inside list-disc space-y-4 text-nord4">
          <li>
            <strong class="text-nord8">Channel:</strong> The default tab type.
            Join a specific channel/broadcaster, similar to the normal web chat.
          </li>
          <li>
            <strong class="text-nord8">Mention:</strong> Displays all messages
            from open Channel tabs that mention one of your configured users. A
            bell icon in the tab name indicates new mentions.
          </li>
          <li>
            <strong class="text-nord8">Live Notification:</strong> Notifies you
            when channels in open tabs go online or offline. A bell icon appears
            next to the tab when a channel goes offline.
          </li>
        </ul>
        <div class="mt-4 overflow-hidden rounded-lg border border-nord2">
          <PreviewImage
            src="/screenshots/new-window-prompt.png"
            alt="New tab creation prompt"
            class="w-full"
            loading="lazy"
          />
        </div>
      </section>

      <section>
        <h2 class="mb-4 text-xl font-semibold text-nord4">Vertical Mode</h2>
        <p class="mb-4 text-nord4">
          Chatuino supports a vertical tab layout for users who prefer this
          arrangement.
        </p>
        <div class="overflow-hidden rounded-lg border border-nord2">
          <PreviewImage
            src="/screenshots/vertical-mode.png"
            alt="Vertical tab layout"
            class="w-full"
            loading="lazy"
          />
        </div>
      </section>
    </div>
  );
}
