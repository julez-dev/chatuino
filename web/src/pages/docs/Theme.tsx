import { Link, Meta, Title } from "@solidjs/meta";

export default function Theme() {
  return (
    <div>
      <Title>Theme - Chatuino</Title>
      <Meta
        name="description"
        content="Switch between bundled Chatuino themes or write your own theme file. Nord is the default."
      />
      <Link rel="canonical" href="https://chatuino.net/docs/theme" />
      <h1 class="mb-8 text-3xl font-bold text-nord4">
        <span class="text-nord3">[</span>
        <span class="text-nord8"> Theme </span>
        <span class="text-nord3">]</span>
      </h1>

      <p class="mb-6 text-nord4">
        Chatuino ships with a set of themes and can load your own. The active
        theme is selected in{" "}
        <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
          settings.yaml
        </code>
        :
      </p>

      <pre class="mb-6 overflow-x-auto rounded-lg border border-nord2 bg-nord1 p-4 text-sm">
        <code class="text-nord4">theme: dracula</code>
      </pre>

      <p class="mb-6 text-nord4">
        Press{" "}
        <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
          ctrl+alt+h
        </code>{" "}
        to open the theme picker: moving the cursor previews a theme live,{" "}
        <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">enter</code>{" "}
        saves it and{" "}
        <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">esc</code>{" "}
        returns to the theme you started with. Emote and badge colors from the
        emote providers are resolved at startup, so those apply after a restart.
      </p>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Bundled Themes</h2>
        <ul class="mb-4 list-disc space-y-1 pl-6 text-nord4">
          <li>
            <code class="text-nord8">nord</code> (default),{" "}
            <code class="text-nord8">dracula</code>,{" "}
            <code class="text-nord8">gruvbox-dark</code>,{" "}
            <code class="text-nord8">catppuccin-mocha</code>,{" "}
            <code class="text-nord8">tokyo-night</code>,{" "}
            <code class="text-nord8">solarized-dark</code>,{" "}
            <code class="text-nord8">one-dark</code>,{" "}
            <code class="text-nord8">ansi16-high-contrast</code>
          </li>
          <li>
            Light themes, which need a light terminal background:{" "}
            <code class="text-nord8">catppuccin-latte</code>,{" "}
            <code class="text-nord8">solarized-light</code>,{" "}
            <code class="text-nord8">gruvbox-light</code>,{" "}
            <code class="text-nord8">one-light</code>
          </li>
        </ul>
        <p class="mb-4 text-nord4">
          Start with{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            --theme &lt;id&gt;
          </code>{" "}
          to override the saved theme for one run without changing it.
        </p>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Default Theme</h2>
        <p class="mb-4 text-nord4">
          The default theme is inspired by the{" "}
          <a
            href="https://www.nordtheme.com/"
            target="_blank"
            rel="noopener noreferrer"
            class="text-nord8 hover:text-nord7"
          >
            Nord color scheme
          </a>
          :
        </p>

        <pre class="overflow-x-auto rounded-lg border border-nord2 bg-nord1 p-4 text-sm">
          <code class="text-nord4">{`# Emote provider colors
seven_tv_emote_color: "#88c0d0"
twitch_tv_emote_color: "#b48ead"
better_ttv_emote_color: "#bf616a"

# Input
input_prompt_color: "#88c0d0"

# Chat user colors
chat_streamer_color: "#d08770"
chat_vip_color: "#b48ead"
chat_sub_color: "#a3be8c"
chat_turbo_color: "#5e81ac"
chat_moderator_color: "#a3be8c"
chat_indicator_color: "#88c0d0"

# Alert colors
chat_sub_alert_color: "#b48ead"
chat_notice_alert_color: "#ebcb8b"
chat_clear_chat_color: "#d08770"
chat_error_color: "#bf616a"

# List/selection colors
list_selected_color: "#88c0d0"
list_label_color: "#81a1c1"
active_label_color: "#ebcb8b"

# Status
status_color: "#88c0d0"

# Splash screen
chatuino_splash_color: "#fd00eb"
splash_highlight_color: "#88c0d0"

# Tab headers
tab_header_background_color: "#3b4252"
tab_header_active_background_color: "#2e3440"

# Borders
inspect_border_color: "#5e81ac"

# List styling
list_background_color: "#2e3440"
list_font_color: "#d8dee9"

# UI chrome
dimmed_text_color: "#4c566a"`}</code>
        </pre>
      </section>

      <section class="mb-12">
        <h2 class="mb-4 text-xl font-semibold text-nord4">Color Reference</h2>
        <p class="mb-4 text-nord4">
          Here's a visual reference of the Nord color palette used by Chatuino:
        </p>

        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {/* Polar Night */}
          <div>
            <h3 class="mb-2 text-sm font-medium text-nord4">Polar Night</h3>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#2e3440" }}
                />
                <code class="text-xs text-nord4">#2e3440</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#3b4252" }}
                />
                <code class="text-xs text-nord4">#3b4252</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#434c5e" }}
                />
                <code class="text-xs text-nord4">#434c5e</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#4c566a" }}
                />
                <code class="text-xs text-nord4">#4c566a</code>
              </div>
            </div>
          </div>

          {/* Snow Storm */}
          <div>
            <h3 class="mb-2 text-sm font-medium text-nord4">Snow Storm</h3>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded border border-nord2"
                  style={{ "background-color": "#d8dee9" }}
                />
                <code class="text-xs text-nord4">#d8dee9</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded border border-nord2"
                  style={{ "background-color": "#e5e9f0" }}
                />
                <code class="text-xs text-nord4">#e5e9f0</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded border border-nord2"
                  style={{ "background-color": "#eceff4" }}
                />
                <code class="text-xs text-nord4">#eceff4</code>
              </div>
            </div>
          </div>

          {/* Frost */}
          <div>
            <h3 class="mb-2 text-sm font-medium text-nord4">Frost</h3>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#8fbcbb" }}
                />
                <code class="text-xs text-nord4">#8fbcbb</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#88c0d0" }}
                />
                <code class="text-xs text-nord4">#88c0d0</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#81a1c1" }}
                />
                <code class="text-xs text-nord4">#81a1c1</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#5e81ac" }}
                />
                <code class="text-xs text-nord4">#5e81ac</code>
              </div>
            </div>
          </div>

          {/* Aurora */}
          <div>
            <h3 class="mb-2 text-sm font-medium text-nord4">Aurora</h3>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#bf616a" }}
                />
                <code class="text-xs text-nord4">#bf616a</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#d08770" }}
                />
                <code class="text-xs text-nord4">#d08770</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#ebcb8b" }}
                />
                <code class="text-xs text-nord4">#ebcb8b</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#a3be8c" }}
                />
                <code class="text-xs text-nord4">#a3be8c</code>
              </div>
              <div class="flex items-center gap-2">
                <div
                  class="h-6 w-6 rounded"
                  style={{ "background-color": "#b48ead" }}
                />
                <code class="text-xs text-nord4">#b48ead</code>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section>
        <h2 class="mb-4 text-xl font-semibold text-nord4">
          Custom Theme Example
        </h2>
        <p class="mb-4 text-nord4">
          A theme is one YAML file. Copy it into{" "}
          <code class="rounded bg-nord1 px-1.5 py-0.5 text-nord8">
            ~/.config/chatuino/themes/
          </code>{" "}
          and it appears in the picker; the file name without the extension is
          the theme id. Only <code class="text-nord8">.yaml</code> and{" "}
          <code class="text-nord8">.yml</code> files directly inside that
          directory are read.
        </p>
        <p class="mb-4 text-nord4">
          Every key is optional: a theme file is merged over the default theme,
          and an unknown key is reported as an error instead of being ignored.
          For example, to use a warmer color scheme:
        </p>
        <pre class="overflow-x-auto rounded-lg border border-nord2 bg-nord1 p-4 text-sm">
          <code class="text-nord4">{`# Custom warm theme
input_prompt_color: "#d08770"
list_selected_color: "#d08770"
status_color: "#d08770"
chat_indicator_color: "#d08770"`}</code>
        </pre>
      </section>
    </div>
  );
}
