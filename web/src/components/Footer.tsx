export default function Footer() {
  return (
    <footer class="border-t border-nord2 bg-nord1">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-4 py-6 sm:flex-row">
        {/* Left side - status bar style */}
        <div class="flex items-center gap-4 text-sm text-nord4">
          <span>-- MIT License --</span>
        </div>

        {/* Right side - links */}
        <nav class="flex items-center gap-2 text-sm" aria-label="Footer links">
          <a
            href="https://github.com/julez-dev/chatuino"
            target="_blank"
            rel="noopener noreferrer"
            class="px-2 py-2 text-nord4 transition-colors hover:text-nord8"
          >
            GitHub
          </a>
          <a
            href="https://github.com/julez-dev/chatuino/releases"
            target="_blank"
            rel="noopener noreferrer"
            class="px-2 py-2 text-nord4 transition-colors hover:text-nord8"
          >
            Releases
          </a>
          <a
            href="https://github.com/julez-dev/chatuino/issues"
            target="_blank"
            rel="noopener noreferrer"
            class="px-2 py-2 text-nord4 transition-colors hover:text-nord8"
          >
            Issues
          </a>
        </nav>
      </div>
    </footer>
  );
}
