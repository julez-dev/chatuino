# <img src="doc/chatuino_logo_trans.png" alt="Chatuino logo" height="32"> Chatuino

[![chatuino-bin](https://img.shields.io/aur/version/chatuino-bin?color=1793d1&label=chatuino-bin&logo=arch-linux&style=for-the-badge)](https://aur.archlinux.org/packages/chatuino-bin/)

A Twitch chat client that runs in your terminal.

![Demo of chatuino.](doc/demo.gif)

![vertical chatuino.](doc/screenshot/vertical-mode.png)

[More screenshots](doc/SCREENSHOTS.md)

## Table of Contents

- [Intro](#intro)
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
- [License](#license)

## Intro

If you spend time in Twitch chat and prefer working in a terminal, Chatuino gives you a native experience without the browser overhead. It handles multiple accounts, displays emotes directly in supported terminals, and stays out of your way.

The project draws inspiration from [Chatterino](https://github.com/Chatterino/chatterino2) and [twitch-tui](https://github.com/Xithrius/twitch-tui).

## Features

- Multiple accounts with easy switching
- Join any number of chats simultaneously
- Anonymous lurking without an account
- Emotes rendered in-terminal (Kitty, Ghostty)
- 7TV and BTTV emote support
- Tab completion for emotes and usernames
- User inspect mode for viewing chat history per user
- Mention notifications and live alerts in dedicated tabs
- Message search and local chat logging
- Moderator tools with quick timeout shortcuts
- Custom commands with templating support
- Theming and configurable keybinds
- Self-hostable server component

For the full list, see [Features](doc/FEATURES.md).

## Installation

**Arch Linux:** Install `chatuino-bin` from the AUR.

**Install script (Linux/macOS):**
```sh
curl -sSfL https://chatuino.net/install | sh
```

Options:
```sh
# Install to a specific directory
curl -sSfL https://chatuino.net/install | sh -s -- -b /usr/local/bin

# Install a specific version
curl -sSfL https://chatuino.net/install | sh -s -- -v v0.6.2
```

**Pre-built binaries:** Available on the [releases page](https://github.com/julez-dev/chatuino/releases).

**Install from source:**
```
go install github.com/julez-dev/chatuino@latest
```

## Usage

Run `chatuino --help` to see available commands.

### Adding an account

```
chatuino account
```

This opens the account manager. To link a Twitch account, you'll need to authenticate through `https://chatuino.net/auth/start` (or your own server) and paste the resulting token.

### Configuration

See [Settings](doc/SETTINGS.md) for keybinds, emote display options, chat logging, and other configuration.

### Self-hosting

Chatuino connects to `chatuino.net` by default for authentication and API proxying. If you prefer to run your own server, follow the [self-host guide](doc/SELF_HOST.md).

## License

MIT. See [LICENSE](LICENSE).
