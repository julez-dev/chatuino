# PRD: Server Proxy Refactor

**Date:** 2026-01-15

---

## Problem Statement

### What problem are we solving?

The chatuino server (`server/`) proxies Twitch Helix API endpoints for anonymous users. Currently:

1. **Custom URL schemes diverge from Twitch Helix**: Proxy uses `/ttv/channel/{id}/emotes` while Twitch uses `/helix/chat/emotes?broadcaster_id=`. This creates cognitive overhead and maintenance burden.

2. **Server is tightly coupled to Twitch data models**: Handlers import `twitchapi.API`, call its methods, and marshal its response types. Adding a new endpoint requires understanding both Twitch API and internal abstractions.

3. **No rate limiting**: The server is open to abuse. Any party can flood the proxy, exhausting Twitch API quota or degrading service for legitimate chatuino users.

4. **Double handling**: The server calls `twitchapi.API` methods which parse responses, then re-marshals them. Wasteful and adds failure points.

### Why now?

The divergent API design makes adding new endpoints error-prone. Rate limiting is a prerequisite before any public exposure.

### Who is affected?

- **Primary users:** Anonymous chatuino users (no Twitch account linked)
- **Secondary:** Maintainers adding new proxied endpoints

---

## Proposed Solution

### Overview

Transform the `/ttv` route group into a transparent reverse proxy that mirrors Twitch Helix paths exactly. The server becomes a dumb pipe that:

1. Accepts requests matching Twitch Helix URL patterns
2. Validates the request against an allowlist of supported endpoints
3. Applies rate limiting per IP
4. Forwards to `api.twitch.tv/helix/*` with app access token
5. Returns response unchanged

The server no longer imports or understands Twitch data models.

### Architecture

```
Client Request                     Server                           Twitch
     |                               |                                |
     |  GET /ttv/chat/emotes/global  |                                |
     |------------------------------>|                                |
     |                               |  1. Check allowlist            |
     |                               |  2. Check rate limit (IP)      |
     |                               |  3. Attach app token           |
     |                               |  GET /helix/chat/emotes/global |
     |                               |------------------------------->|
     |                               |         200 OK + JSON          |
     |                               |<-------------------------------|
     |         200 OK + JSON         |                                |
     |<------------------------------|                                |
```

---

## End State

When this PRD is complete, the following will be true:

- [ ] `/ttv/*` routes replaced with reverse proxy (maps to `/helix/*` on Twitch)
- [ ] Proxy only allows pre-defined Twitch Helix endpoints (allowlist)
- [ ] Rate limiting per IP: burst accommodates startup spike, sustained ~25 req/min
- [ ] `server.Client` updated to call `/helix/*` paths (mirrors `twitchapi.API`)
- [ ] Server package has no imports from `twitch/twitchapi` for data models
- [ ] All existing functionality preserved (emotes, badges, users, streams, chat settings)
- [ ] Tests cover rate limiting behavior and endpoint allowlist

---

## Success Metrics

### Quantitative

| Metric | Current | Target | Measurement Method |
|--------|---------|--------|-------------------|
| Lines of handler code | ~200 (handler.go:248-452) | ~50 | `wc -l` on proxy handler |
| Twitch model imports in server | 1 (`twitchapi.API`) | 0 | `grep` imports |
| Time to add new endpoint | ~30 min (handler + client) | ~2 min (add to allowlist) | Manual estimate |

### Qualitative

- Adding new Helix endpoints requires only allowlist update
- Client code mirrors Twitch SDK patterns exactly

---

## Acceptance Criteria

### Reverse Proxy

- [ ] `GET /ttv/*` forwards to `https://api.twitch.tv/helix/*` (strips `/ttv`, prepends `/helix`)
- [ ] Request query params passed through unchanged
- [ ] App access token injected via `Authorization: Bearer <token>` header
- [ ] `Client-Id` header injected
- [ ] Response status, headers, body returned unchanged (including `Ratelimit-Reset` header on 429)
- [ ] Non-allowlisted paths return `403 Forbidden`
- [ ] On 401 from Twitch: refresh app access token and retry once
- [ ] On 429 from Twitch: pass through to client (don't wait server-side)

### Endpoint Allowlist

Supported endpoints (read-only, app token compatible):

| Proxy Path | Twitch Helix Path |
|------------|-------------------|
| `GET /ttv/chat/emotes/global` | `/helix/chat/emotes/global` |
| `GET /ttv/chat/emotes` | `/helix/chat/emotes` (query: `broadcaster_id`) |
| `GET /ttv/streams` | `/helix/streams` (query: `user_id`) |
| `GET /ttv/users` | `/helix/users` (query: `login`, `id`) |
| `GET /ttv/chat/settings` | `/helix/chat/settings` (query: `broadcaster_id`) |
| `GET /ttv/chat/badges/global` | `/helix/chat/badges/global` |
| `GET /ttv/chat/badges` | `/helix/chat/badges` (query: `broadcaster_id`) |

### Rate Limiting

- [ ] Per-IP rate limiting using token bucket or similar
- [ ] Burst capacity: accommodate startup spike (~100 requests)
- [ ] Sustained rate: ~25 requests/minute
- [ ] Rate limit exceeded returns `429 Too Many Requests`
- [ ] `Retry-After` header included in 429 responses
- [ ] Redis backend for distributed rate limiting
- [ ] Hardcoded rate limit values (not runtime configurable)

### App Access Token Management

- [ ] On 401 from Twitch: refresh app access token
- [ ] Retry failed request once after token refresh
- [ ] Token refresh uses client credentials flow

### Client Updates (`server/client.go`)

- [ ] `GetGlobalEmotes` calls `/ttv/chat/emotes/global`
- [ ] `GetChannelEmotes` calls `/ttv/chat/emotes?broadcaster_id=`
- [ ] `GetStreamInfo` calls `/ttv/streams?user_id=`
- [ ] `GetUsers` calls `/ttv/users?login=&id=`
- [ ] `GetChatSettings` calls `/ttv/chat/settings?broadcaster_id=`
- [ ] `GetGlobalChatBadges` calls `/ttv/chat/badges/global`
- [ ] `GetChannelChatBadges` calls `/ttv/chat/badges?broadcaster_id=`
- [ ] Response types unchanged (still use `twitchapi.*` types for deserialization)
- [ ] Handle 429 responses: wait until `Ratelimit-Reset` and retry

### Shared Rate Limit Handling

- [ ] Extract 429/retry logic from `twitchapi.API` into shared helper
- [ ] Both `twitchapi.API` and `server.Client` use shared helper
- [ ] Helper parses `Ratelimit-Reset` header, waits, retries request

### Non-Goals for Client

- Client still needs `twitchapi` types for response deserialization (that's fine)
- Client logic (batching, validation) remains unchanged

---

## Technical Context

### Existing Patterns

- Chi router with middleware chain: `server/router.go:9-17`
- `httputil.ReverseProxy` commented out but shows intent: `server/handler.go:248-268`

### Key Files

- `server/router.go` - Route definitions, middleware
- `server/handler.go:248-452` - Handlers to replace with reverse proxy
- `server/client.go` - Client to update with new paths
- `server/api.go` - Server struct, holds `ttvAPI` to remove

### System Dependencies

- Redis for distributed rate limiting
- `github.com/redis/go-redis/v9` or similar client
- Twitch app access token (already available via `ttvAPI`)

### Data Model Changes

None. Response shapes unchanged (Twitch responses pass through).

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Twitch rate limits hit | Med | High | Server-side rate limiting prevents abuse; monitor Twitch 429s |
| App token expiry during request | Low | Med | Refresh on 401 and retry once |
| Allowlist bypass | Low | High | Strict path matching, explicit allowlist |
| IP spoofing for rate limit evasion | Low | Med | Trust `X-Forwarded-For` only from known proxies |
| Redis unavailable | Low | Med | Fail open - allow requests through (acceptable risk for simple architecture) |

---

## Alternatives Considered

### Alternative 1: Keep Custom URL Scheme

- **Description:** Maintain `/ttv/*` paths but add rate limiting
- **Pros:** No client changes, backward compatible
- **Cons:** Perpetuates divergence, double handling remains
- **Decision:** Rejected. Technical debt compounds; now is the time to align.

### Alternative 2: Full Twitch Proxy (All Endpoints)

- **Description:** Proxy all `/helix/*` without allowlist
- **Pros:** Maximum flexibility
- **Cons:** App token limits functionality; abuse vector; supports endpoints we don't need
- **Decision:** Rejected. Allowlist provides security and clarity.

### Alternative 3: Use httputil.ReverseProxy Directly

- **Description:** Use stdlib reverse proxy without custom logic
- **Pros:** Less code
- **Cons:** No allowlist, no rate limiting, no token injection
- **Decision:** Partial adoption. Use ReverseProxy as base, wrap with middleware.

---

## Non-Goals (v1)

Explicitly out of scope for this PRD:

- **Auth endpoints** (`/auth/*`) - Different concern, already working
- **Link check proxy** (`/proxy/link_check`) - Unrelated to Twitch API
- **Write operations** - App token doesn't support them; read-only proxy
- **Response caching** - Future optimization
- **Metrics/observability** - Future enhancement

---

## Interface Specifications

### Proxy Endpoint

```
GET /ttv/{path}

Server processing:
  1. Extract path after /ttv/ prefix
  2. Check path against server-side allowlist
  3. Check rate limit (Redis, per-IP)
  4. Rewrite path: /ttv/* -> /helix/*
  5. Inject headers:
       Authorization: Bearer <app_access_token>
       Client-Id: <client_id>
  6. Forward to api.twitch.tv
  7. On 401: refresh token, retry once

Response: Proxied from api.twitch.tv/helix/{path}

Errors:
  403 Forbidden - Path not in server allowlist
  429 Too Many Requests - Rate limit exceeded (includes Retry-After header)
  502 Bad Gateway - Twitch API unreachable
```

### Server-Side Allowlist

Validated by server before proxying (hardcoded, not runtime configurable):

```go
// server/allowlist.go
var allowedPaths = []string{
    "chat/emotes/global",
    "chat/emotes",
    "streams",
    "users",
    "chat/settings",
    "chat/badges/global",
    "chat/badges",
}
```

Request `/ttv/chat/emotes?broadcaster_id=123`:
1. Server extracts path: `chat/emotes`
2. Server checks: `chat/emotes` in allowlist? Yes
3. Server rewrites to: `/helix/chat/emotes?broadcaster_id=123`
4. Server forwards to Twitch

---

## Open Questions

None - all resolved.

---

## Appendix

### Current vs New URL Mapping

| Current (Custom) | New (Helix-aligned) | Twitch Helix |
|------------------|---------------------|--------------|
| `/ttv/emotes/global` | `/ttv/chat/emotes/global` | `/helix/chat/emotes/global` |
| `/ttv/channel/{id}/emotes` | `/ttv/chat/emotes?broadcaster_id={id}` | `/helix/chat/emotes?broadcaster_id={id}` |
| `/ttv/channel/{id}/info` | `/ttv/streams?user_id={id}` | `/helix/streams?user_id={id}` |
| `/ttv/channels/info?user_id=` | `/ttv/streams?user_id=` | `/helix/streams?user_id=` |
| `/ttv/channel/{login}/user` | `/ttv/users?login={login}` | `/helix/users?login={login}` |
| `/ttv/channels?logins=&ids=` | `/ttv/users?login=&id=` | `/helix/users?login=&id=` |
| `/ttv/channel/{id}/chat/settings` | `/ttv/chat/settings?broadcaster_id={id}` | `/helix/chat/settings?broadcaster_id={id}` |
| `/ttv/chat/badges/global` | `/ttv/chat/badges/global` | `/helix/chat/badges/global` |
| `/ttv/channel/{id}/chat/badges` | `/ttv/chat/badges?broadcaster_id={id}` | `/helix/chat/badges?broadcaster_id={id}` |

### References

- [Twitch Helix API Reference](https://dev.twitch.tv/docs/api/reference)
- Existing reverse proxy pattern: `server/handler.go:248-268` (commented)
