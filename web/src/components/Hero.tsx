import { A } from "@solidjs/router";
import { createSignal, onMount } from "solid-js";

export default function Hero() {
  const [videoFailed, setVideoFailed] = createSignal(false);

  let videoRef: HTMLVideoElement | undefined;

  onMount(() => {
    if (!videoRef) return;
    // Detect if video fails to play (Safari/iOS issues)
    videoRef.play().catch(() => setVideoFailed(true));
  });

  return (
    <section class="relative overflow-hidden py-16 md:py-24">
      {/* Background gradient */}
      <div class="absolute inset-0 bg-linear-to-b from-nord1/50 to-transparent" />

      <div class="relative mx-auto max-w-6xl px-4">
        <div class="flex flex-col items-center text-center">
          {/* Logo */}
          <h1 class="mb-4">
            <img
              src="/chatuino_splash_brackets_cropped.png"
              alt="Chatuino"
              class="h-12 md:h-16"
              width={400}
              height={103}
            />
          </h1>

          {/* Tagline */}
          <p class="mb-8 max-w-2xl text-lg text-nord4 md:text-xl">
            A Twitch chat client that runs in your terminal
          </p>

          {/* CTA buttons */}
          <div class="mb-12 flex flex-wrap items-center justify-center gap-4">
            <a
              href="#install"
              class="rounded-md bg-nord8 px-6 py-3 font-medium transition-colors hover:bg-nord7"
              style={{ color: "#000b1e" }}
            >
              Install Now
            </a>
            <A
              href="/docs/features"
              class="rounded-md border border-nord2 bg-nord1 px-6 py-3 font-medium text-nord4 transition-colors hover:border-nord8 hover:text-nord8"
            >
              View Docs
            </A>
          </div>

          {/* Demo Video */}
          <div class="w-full max-w-4xl overflow-hidden rounded-lg border border-nord2 bg-nord1 shadow-2xl">
            {/* Terminal header bar */}
            <div class="flex items-center gap-2 border-b border-nord2 bg-nord1 px-4 py-2">
              <div class="h-3 w-3 rounded-full bg-nord11" />
              <div class="h-3 w-3 rounded-full bg-nord13" />
              <div class="h-3 w-3 rounded-full bg-nord14" />
              <span class="ml-4 text-sm text-nord4">chatuino</span>
            </div>
            {videoFailed() ? (
              <img
                src="/demo.gif"
                alt="Chatuino demo"
                class="w-full"
                width={896}
                height={504}
              />
            ) : (
              <video
                ref={videoRef}
                autoplay
                loop
                muted
                playsinline
                class="w-full"
                width={896}
                height={504}
                onError={() => setVideoFailed(true)}
              >
                <source src="/demo_optimized.mp4" type="video/mp4" />
              </video>
            )}
          </div>

          {/* Feature highlights */}
          <ul class="mt-12 grid list-none grid-cols-1 gap-6 text-sm text-nord4 sm:grid-cols-3">
            <li class="flex items-center gap-2">
              <span class="text-nord14" aria-hidden="true">
                *
              </span>
              <span>Multiple accounts</span>
            </li>
            <li class="flex items-center gap-2">
              <span class="text-nord14" aria-hidden="true">
                *
              </span>
              <span>Graphical emotes</span>
            </li>
            <li class="flex items-center gap-2">
              <span class="text-nord14" aria-hidden="true">
                *
              </span>
              <span>7TV, BTTV & FFZ support</span>
            </li>
          </ul>
        </div>
      </div>
    </section>
  );
}
