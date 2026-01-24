import { A } from "@solidjs/router";

export default function NotFound() {
  return (
    <div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
      <h1 class="mb-4 text-6xl font-bold">
        <span class="text-nord3">[</span>
        <span class="text-nord8"> 404 </span>
        <span class="text-nord3">]</span>
      </h1>

      <p class="mb-2 text-xl text-nord4">Page not found</p>
      <p class="mb-8 text-nord4">
        The page you're looking for doesn't exist or has been moved.
      </p>

      <div class="flex gap-4">
        <A
          href="/"
          class="rounded-md bg-nord8 px-6 py-3 font-medium text-nord0 transition-colors hover:bg-nord7"
        >
          Go Home
        </A>
        <A
          href="/docs/features"
          class="rounded-md border border-nord2 bg-nord1 px-6 py-3 font-medium text-nord4 transition-colors hover:border-nord8 hover:text-nord8"
        >
          View Docs
        </A>
      </div>
    </div>
  );
}
