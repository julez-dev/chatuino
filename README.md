# Chatuino

Cha*tui*no is a cross-platform TUI utilizing bubbletea to bring a feature rich Twitch Chat Client to your terminal.

> **Note**: This project is still in early development (colors may be weird, terminals may not work, panics may happen and resizing may cause issues/become glitchy etc.)

![Demo of chatuino.](doc/demo.gif)

## Table of Contents

- [Introduction](#introduction)
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
- [License](#license)

## Introduction

Chatuino aims to be a feature rich and portable Twitch Chat Client for your terminal.

The name and idea to create a Twitch Chat Client is inspired by [Chatterino](https://github.com/Chatterino/chatterino2).

## Features

- Multi Account support
- Multi Tab support/Can join multiple channels and switch between them
- Join Chat as Anonymous User
- Auto resize when window size changes
- You can host the server component for authentication/anonymous usage yourself
- Emote and User Suggestions
- Local save state to persist sessions
- Simple emote support for Twitch(purple), BTTV(red) and SevenTV(blue) emotes (Emotes are colored), graphic display planned
- Advanced moderation tools planned
- Manage ban requests
- Quick ban users with ctrl+shift+t
- Copy and Paste messages with alt+c
- Simple Twitch duplicate message bypass
- More twitch integration planned (Announcements etc.)

## Installation

You can use go install to install the program (`go install github.com/julez-dev/chatuino`) or grab a binary from the [releases](https://github.com/julez-dev/chatuino/releases) page.

## Usage

The binary comes with the account management, the TUI itself and the server component if you want to self host the server.

You can use the --help flag to get help.

### account sub-command

You can use the account sub-command to link your twitch account to Chatuino. The command will launch a TUI to help you manage your accounts.

If want to link a new account you need to provide a user token to Chatuino which you can generate with the server component. You can start the auth flow here: `https://chatuino.net/auth/start` or your own server if you want to.

### key-binds

Key-binds are configurable in the ~/.config/chatuino/keymap.yaml file (config directory may be different depending on OS).

An overview of the keybinds can be opened with `?` inside Chatuino.

### server sub-command and hosting you own server

The server is responsible for dealing with authentication flows and proxying requests to the Twitch API that require an App Access Token when using an anonymous user. My hosted version is available under `https://chatuino.net` but you can just run your own version as well. This server is running on the lowest tier possible on hetzner cloud, so don't expect too much performance.

For a guide on how to host your own server see the [self-host guide](doc/SELF_HOST.md).

## License

This project is licensed under the MIT license. See the [LICENSE](LICENSE) file for details.
