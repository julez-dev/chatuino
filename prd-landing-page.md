# PRD: Chatuino Landing Page & Documentation Site

**Date:** 2026-01-24

---

## Problem Statement

### What problem are we solving?
Chatuino has no web presence beyond GitHub. Users must navigate raw markdown files to learn about features, installation, and configuration. No visual showcase, no OS-specific install guidance, no branded landing page.

### Why now?
Project has matured with stable feature set. A proper landing page:
- Increases discoverability and trust
- Reduces friction for new users (OS-specific install)
- Showcases the terminal aesthetic that differentiates Chatuino
- Centralizes documentation in navigable format

### Who is affected?
- **Primary users:** Potential new users discovering Chatuino
- **Secondary users:** Existing users referencing documentation

---

## Proposed Solution

### Overview
Build a SolidJS landing page and documentation site with TUI-inspired Nord aesthetics, bundled into the Go server binary via `go:embed`. Landing page showcases demo GIF, key features, and provides OS-detected installation instructions. Documentation pages mirror existing markdown content with improved navigation.

### User Experience

#### User Flow: First Visit
1. User visits chatuino.net
2. Landing page loads with hero section, GIF demo, terminal-styled heading
3. Key features grid shows 6-8 highlights with icons
4. Install section auto-detects OS, shows primary method prominently
5. User expands "Other platforms" to see all options
6. User clicks "Documentation" in nav to explore detailed docs

#### User Flow: Documentation
1. User navigates to /docs (or clicks nav link)
2. Redirects to /docs/features (default)
3. Sidebar shows all doc sections: Features, Settings, Theme, Self-Host
4. User clicks section to navigate; content renders with syntax highlighting
5. Screenshots display inline at appropriate sizes

### Design Considerations

**Visual Language (from TUI screenshot):**
- Dark background: `#2e3440` (Polar Night) - matches TUI background
- Primary accent: `#88c0d0` (Frost cyan) - used for borders, highlights, usernames
- Panel borders with bracket notation: `[ Section ]` style headers
- Status bar aesthetic for footer/navigation
- Colored usernames/badges mimicking chat display
- Monospace throughout for authentic terminal feel

**Key TUI Elements to Replicate:**
- Bordered panels with `[ Title ]` headers
- Cyan (`#88c0d0`) as primary accent color
- Dark gray panel backgrounds (`#3b4252` for tabs area)
- Status line at bottom (`-- View --`, counters)
- Timestamp styling (dimmed `#4c566a`)
- Color-coded badges (mod green, VIP purple, sub badges)

**Typography:**
- Primary: IBM Plex Mono (headers, code, all terminal elements)
- Consistent monospace throughout for TUI authenticity
- Readable weights (400-500) for body text

**Accessibility:**
- WCAG AA color contrast (Nord palette is accessible)
- Keyboard navigation for all interactive elements
- Semantic HTML structure
- Screen reader friendly navigation

---

## End State

When this PRD is complete, the following will be true:

- [x] Landing page exists at `/` with hero, demo GIF, features, install section
- [x] Documentation site exists at `/docs/*` with multi-page routing
- [x] OS detection shows relevant install instructions first
- [x] Site is bundled via go:embed into server binary
- [x] Server serves static files at root when running
- [x] Existing API routes (`/auth/*`, `/ttv/*`, etc.) continue working unchanged
- [x] Site renders correctly on mobile and desktop
- [x] All existing doc content (FEATURES, SETTINGS, THEME, SELF_HOST) is accessible
- [x] Screenshots from `doc/screenshot/` display correctly
- [x] Visual style evokes TUI aesthetic while remaining modern and readable

---

## Success Metrics

### Quantitative
| Metric | Current | Target | Measurement Method |
|--------|---------|--------|-------------------|
| Landing page exists | No | Yes | Visual verification |
| Lighthouse performance | N/A | >90 | Lighthouse audit |
| Mobile responsive | N/A | Yes | Device testing |
| Build size (JS + assets) | N/A | <500KB gzipped | Build output |

### Qualitative
- Site feels cohesive with TUI aesthetic
- Install experience is frictionless
- Documentation is easily navigable

---

## Acceptance Criteria

### Landing Page
- [ ] Hero section with "Chatuino" heading in terminal/bracket style (e.g., `[ Chatuino ]`)
- [ ] Animated GIF demo (`doc/demo.gif`) prominently displayed
- [ ] Tagline: "A Twitch chat client that runs in your terminal"
- [ ] 6-8 key features displayed in responsive grid with terminal-style borders
- [ ] Install section with OS auto-detection (Linux/macOS/Windows)
- [ ] Primary install method shown for detected OS
- [ ] Expandable/collapsible section showing all install methods
- [ ] Navigation to documentation
- [ ] Footer with GitHub link, license info, styled like TUI status bar

### Documentation Site
- [ ] Multi-page routing: `/docs/features`, `/docs/settings`, `/docs/theme`, `/docs/self-host`
- [ ] Sidebar navigation visible on desktop, hamburger menu on mobile
- [ ] Content from existing markdown files rendered with proper formatting
- [ ] Code blocks with syntax highlighting (YAML, shell) using Nord colors
- [ ] Screenshots displayed inline with appropriate sizing
- [ ] Internal links between doc pages work correctly
- [ ] `/docs` redirects to `/docs/features`

### Technical Implementation
- [ ] Frontend source in `web/` directory
- [ ] Stack: Vite + SolidJS + TypeScript + Tailwind CSS v4
- [ ] shadcn-solid components for UI primitives
- [ ] Build outputs to `web/dist/`
- [ ] Go server embeds `web/dist/` via `//go:embed`
- [ ] Static file handler serves embedded files at `/`
- [ ] API routes take precedence over static file serving
- [ ] npm scripts: `dev`, `build`, `preview`

### OS Detection Logic
- [ ] Detect via `navigator.userAgent` and `navigator.platform`
- [ ] Linux: Show curl install script first
- [ ] macOS: Show curl install script first
- [ ] Windows: Show "Pre-built binaries" with releases link first
- [ ] Unknown/other: Show all options equally
- [ ] Always show "Other platforms" expandable section below primary

---

## Technical Context

### Existing Patterns
- `server/router.go` - Chi router setup; add static file handler here
- `save/theme.go` - Nord color palette to replicate in CSS variables
- `doc/*.md` - Source content for documentation pages
- `doc/screenshot/*.png` - Images to embed in docs
- `doc/demo.gif` - Hero demo animation
- `doc/emote-demo.gif` - Emote rendering demo (optional inclusion)

### Key Files to Modify
- `server/router.go` - Add static file serving route
- New `server/static.go` - go:embed directive and handler

### System Dependencies (build-time only)
- Node.js 18+
- Vite 5.x
- SolidJS 1.x
- Tailwind CSS 4.x
- shadcn-solid (solid-ui)

### Directory Structure
```
web/
├── src/
│   ├── components/
│   │   ├── ui/              # shadcn-solid components
│   │   ├── Header.tsx
│   │   ├── Hero.tsx
│   │   ├── Features.tsx
│   │   ├── Install.tsx
│   │   ├── Footer.tsx
│   │   └── DocSidebar.tsx
│   ├── pages/
│   │   ├── Landing.tsx
│   │   └── docs/
│   │       ├── Features.tsx
│   │       ├── Settings.tsx
│   │       ├── Theme.tsx
│   │       └── SelfHost.tsx
│   ├── lib/
│   │   ├── os-detect.ts
│   │   └── theme.ts         # Nord colors as CSS vars
│   ├── App.tsx
│   ├── index.tsx
│   └── index.css            # Tailwind imports + custom styles
├── public/
│   ├── demo.gif             # Copied from doc/
│   └── screenshots/         # Copied from doc/screenshot/
├── index.html
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
└── package.json
```

### Go Embedding Pattern
```go
// server/static.go
package server

import "embed"

//go:embed all:../web/dist
var staticFS embed.FS

// Handler serves static files, returns index.html for SPA routes
```

### Route Priority
```
GET /auth/*              → Existing auth endpoints (matched first)
GET /ttv/*               → Existing Helix proxy (matched first)
GET /internal/*          → Existing health endpoints (matched first)
GET /proxy/*             → Existing proxy endpoints (matched first)
GET /install             → Existing install script redirect (matched first)
GET /*                   → Static files / SPA fallback (catchall)
```

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Build complexity increases | Med | Med | Clear npm scripts, documented build process |
| Binary size increases significantly | Med | Low | Tree-shake, compress assets, <500KB gzipped budget |
| Route conflicts (static vs API) | Low | High | API routes registered before static catchall |
| SPA routing breaks on page refresh | Med | Med | Server returns index.html for unknown routes (except API prefixes) |
| Font loading causes layout shift | Low | Low | Preload IBM Plex Mono, use system monospace fallback |

---

## Alternatives Considered

### Alternative 1: Hugo/Jekyll Static Site Generator
- **Description:** Use a static site generator instead of SolidJS SPA
- **Pros:** Simpler build, no JS framework, faster initial load
- **Cons:** Less interactive, harder to implement smooth OS detection UX, doesn't match "modern" requirement
- **Decision:** Rejected. SolidJS provides better UX for dynamic elements and matches requested tech stack.

### Alternative 2: Separate Deployment (GitHub Pages)
- **Description:** Deploy site separately from Go server
- **Pros:** CDN benefits, no binary size increase, independent deploys
- **Cons:** Two deployment targets, can't share context, more infrastructure
- **Decision:** Rejected per user preference. Single binary deployment preferred.

### Alternative 3: Server-Side Rendering (SolidStart)
- **Description:** Use SolidStart for SSR
- **Pros:** Better SEO, faster first contentful paint
- **Cons:** Significantly more complex, requires Node runtime or edge functions
- **Decision:** Rejected. Landing page SEO needs are minimal; static SPA sufficient.

---

## Non-Goals (v1)

Explicitly out of scope for this PRD:
- **Search functionality in docs** - Docs are small enough to navigate manually; future enhancement
- **Dark/light theme toggle** - TUI aesthetic is inherently dark; light mode deferred
- **Internationalization (i18n)** - English only for v1
- **Analytics/tracking** - Privacy-focused; no tracking in v1
- **Blog/changelog section** - GitHub releases sufficient
- **Interactive terminal demo (asciinema)** - GIF sufficient; deferred
- **PWA/offline support** - Not needed for documentation site

---

## Interface Specifications

### CLI (existing, behavior extended)
```
chatuino server [options]

Server now also serves landing page and documentation at /
No new flags required.
```

### URL Routes
```
GET /                    → Landing page (index.html)
GET /docs                → Redirect to /docs/features
GET /docs/features       → Features documentation
GET /docs/settings       → Settings documentation
GET /docs/theme          → Theme documentation
GET /docs/self-host      → Self-hosting documentation
GET /assets/*            → Static assets (JS, CSS, fonts, images)
GET /demo.gif            → Demo animation
GET /screenshots/*       → Documentation screenshots
```

### Build Commands
```bash
# Development (hot reload)
cd web && npm run dev    # Vite dev server on :5173

# Production build
cd web && npm run build  # Outputs to web/dist/

# Preview production build
cd web && npm run preview

# Full Go build (assumes web/dist exists)
go build .
```

---

## Documentation Requirements

- [ ] README.md updated with web development instructions
- [ ] `web/README.md` with frontend-specific setup and contribution guide
- [ ] Build process documented (npm → go:embed flow)

---

## Open Questions

| Question | Owner | Due Date | Status |
|----------|-------|----------|--------|
| Optimize demo.gif for web? | julez | - | Resolved: Use current, may replace with mp4 later |
| shadcn-solid vs custom components? | julez | - | Resolved: shadcn-solid |
| Include emote-demo.gif on features page? | julez | - | Resolved: Yes |
| Build hook: Makefile, goreleaser pre-hook, or manual? | julez | - | Resolved: GitHub Action builds web before Go compilation |
| Font hosting: Google Fonts CDN or self-host? | julez | - | Resolved: Self-host |

---

## Appendix

### Nord Color Palette (from theme.go / TUI)
```css
/* Polar Night - backgrounds */
--nord0: #2e3440;  /* Main background */
--nord1: #3b4252;  /* Panel/tab header background */
--nord2: #434c5e;  /* Elevated surfaces */
--nord3: #4c566a;  /* Dimmed text, timestamps */

/* Snow Storm - text */
--nord4: #d8dee9;  /* Primary text */
--nord5: #e5e9f0;  /* Secondary text */
--nord6: #eceff4;  /* Bright text */

/* Frost - accents (primary UI colors) */
--nord7: #8fbcbb;  /* Splash/teal accent */
--nord8: #88c0d0;  /* PRIMARY ACCENT - borders, highlights, links */
--nord9: #81a1c1;  /* Secondary accent */
--nord10: #5e81ac; /* Muted accent, borders */

/* Aurora - semantic colors */
--nord11: #bf616a; /* Error, BTTV emotes */
--nord12: #d08770; /* Warning, streamer color */
--nord13: #ebcb8b; /* Active/selected, notices */
--nord14: #a3be8c; /* Success, mod/sub color */
--nord15: #b48ead; /* Purple, VIP, Twitch emotes */
```

### Key Features for Landing Page Grid
1. **Multiple Accounts** - Easy switching between Twitch accounts
2. **Graphical Emotes** - Rendered in-terminal (Kitty, Ghostty)
3. **7TV & BTTV Support** - Third-party emote providers
4. **User Inspect Mode** - View chat history per user
5. **Mention Notifications** - Dedicated tab for @mentions
6. **Live Alerts** - Know when channels go online/offline
7. **Message Search** - Fuzzy search through chat history
8. **Local Chat Logging** - SQLite-backed message persistence
9. **Configurable Themes** - Full Nord color customization
10. **Self-Hostable** - Run your own server component

### Screenshots Available
- `doc/demo.gif` - Main animated demo
- `doc/emote-demo.gif` - Graphical emote rendering
- `doc/screenshot/account-ui.png` - Account management
- `doc/screenshot/auto-completions.png` - Tab completion
- `doc/screenshot/auto-completions-emotes.png` - Emote completion
- `doc/screenshot/auto-completions_user.png` - User completion
- `doc/screenshot/cache-stats.png` - Cache statistics
- `doc/screenshot/chat-view.png` - Main chat view
- `doc/screenshot/message-log.png` - User inspection / message log
- `doc/screenshot/message-search.png` - Search interface
- `doc/screenshot/new-window-prompt.png` - New tab creation
- `doc/screenshot/vertical-mode.png` - Vertical tab layout

### References
- [SolidJS](https://solidjs.com)
- [shadcn-solid](https://shadcn-solid.com)
- [Tailwind CSS v4](https://tailwindcss.com)
- [Nord Theme](https://nordtheme.com)
- [IBM Plex](https://www.ibm.com/plex)
- [Vite](https://vitejs.dev)
- [go:embed](https://pkg.go.dev/embed)
