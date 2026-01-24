# Self-Hosting

Chatuino requires a server component to handle the user authentication flow with Twitch.

Additionally, the server component serves responses from Twitch API endpoints that normally require an authenticated user or app when the anonymous account is used to connect to chat.

The server component is included in the Chatuino binary.

## Prerequisites

To self-host Chatuino, you need to register an app with Twitch and obtain a `client_id` and `client_secret`.

Register your application in the [Twitch Dev Console](https://dev.twitch.tv/console/apps) under "Register Your Application".

Twitch allows localhost as a valid non-HTTPS redirect URL. Use the port that the server will use.

![Creating an app in the Twitch Dev Console](twitch_dev.png)

Pass the `client_id` and `client_secret` to the server via environment variables `CHATUINO_CLIENT_ID` and `CHATUINO_CLIENT_SECRET`.

Configure the `CHATUINO_API_HOST` environment variable (e.g., `http://localhost:8080`) for the Chatuino main application.

## Running the Server

### Running the Binary Directly

Run the server by executing the Chatuino binary with the `server` subcommand:

```sh
chatuino --log --human-readable server --redirect-url=http://localhost:8080/auth/redirect
```

### Running with Docker

Run the server in a Docker container:

```sh
docker run -d \
 -e CHATUINO_CLIENT_SECRET \
 -e CHATUINO_CLIENT_ID \
 -e CHATUINO_REDIRECT_URL=http://localhost:8080/auth/redirect \
 -p 8080:8080 \
 ghcr.io/julez-dev/chatuino:latest --log server
```

> **Note**: Don't forget the http:// prefix in the `CHATUINO_REDIRECT_URL` environment variable.

## Launching Chatuino

Once the server is running and `CHATUINO_API_HOST` is configured, start the Chatuino application:

```sh
CHATUINO_API_HOST=http://localhost:8080 chatuino
```
