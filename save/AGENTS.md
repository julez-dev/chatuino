# SAVE - PERSISTENCE LAYER

Handles app state, settings, accounts, themes, keymaps, and SQLite message logging. Uses JSON for state, YAML for configs, system keyring for tokens, SQLite for logs.

## STORAGE TYPES

| Type | File | Format | Notes |
|------|------|--------|-------|
| **App state** | `state.json` | JSON | Tab states, focus, channels (app.go:16) |
| **Settings** | `settings.yaml` | YAML | Moderation, chat, custom commands, blocklists (settings.go:15) |
| **Accounts** | System keyring | JSON | Tokens, display names, main account flag (account_provider.go:14) |
| **Accounts fallback** | `accounts.json` | JSON | Plaintext when keyring unavailable (plain_keyring.go:14) |
| **Theme** | `theme.yaml` | YAML | Colors (40 fields), defaults to Catppuccin-style (theme.go:11) |
| **Keymaps** | `keymap.yaml` | YAML | Custom key bindings (key.go:14) |
| **Message logs** | `*.db` | SQLite | JSONB payload, 3 indexes, WAL mode (messagelog/logger.go:26-39) |

## FILE LOCATIONS

**All files**: `$XDG_CONFIG_DIR/chatuino/` (or `os.UserConfigDir()`)  
**DB**: Separate path (not in config dir)

## PERSISTENCE PATTERNS

### Read/Write Flow
1. `openCreateConfigFile()` ensures `chatuino/` dir exists (app.go:120)
2. Open file with `O_RDWR|O_CREATE`, perms `0600` (app.go:112)
3. Parse JSON/YAML, merge with defaults (settings.go:123, theme.go:108)
4. `Truncate(0)` before write (app.go:53, plain_keyring.go:74)

### Defaults Merge
- **Settings**: Start with `BuildDefaultSettings()`, unmarshal user YAML on top (settings.go:123)
- **Theme**: Same pattern (theme.go:108)
- **Keymap**: Defaults written to disk if empty (key.go:222)

### Keyring Strategy
1. Try system keyring (`KeyringWrapper` with mutex, keyring_wrapper.go:12)
2. On `ErrNotFound` or init failure → `PlainKeyringFallback` (plain_keyring.go:14)
3. Anonymous account always injected at runtime (account_provider.go:20-27, :223)

### Message Logging
- **Batch insert**: Max 20 items OR 5s timeout (messagelog/logger.go:47-48)
- **WAL mode**: `journal_mode=WAL`, `synchronous=normal` (messagelog/logger.go:72)
- **Payload**: easyjson-marshaled `PrivateMessage` stored as JSONB (logger.go:230)
- **Filter channels**: Include/exclude lists (logger.go:254-272)

## CONVENTIONS

- **afero.Fs injection**: All file ops testable (app.go:32, plain_keyring.go:18)
- **Mutex-protected keyring**: Prevent concurrent system keyring calls (keyring_wrapper.go:18)
- **Ignore JSON syntax errors**: Return empty state/defaults (app.go:82-86, account_provider.go:214-218)
- **Validation**: Settings validate on load (no include+exclude, min 3 chars for commands, settings.go:64-91)
- **Anonymous account**: Hardcoded `justinfan123123`, never saved (account_provider.go:20, :231)
- **Main account logic**: Only one `IsMain=true`, reassign on remove (account_provider.go:98-105)

## ANTI-PATTERNS

- **NEVER** delete anonymous account from runtime list (account_provider.go:223)
- **NEVER** allow both `logs_channel_include` and `logs_channel_exclude` (settings.go:65)
- **NEVER** write keymaps on every read—only if empty (key.go:222)
- **NEVER** skip `Truncate(0)` before rewrite (causes appending, app.go:53)
- **NEVER** commit SQLite transaction without checking channel filter first (messagelog/logger.go:120)
- **NEVER** assume keyring available—have plaintext fallback (plain_keyring.go)
