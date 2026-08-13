# bethrou

**The Bethrou client.**

`bethrou` is a local SOCKS5 proxy that routes your traffic to an exit node in your own private [Bethrou](https://github.com/bethrou) mesh — attempting a direct connection first, falling back to an authenticated relay only when direct isn't possible. See [bethrou/bethroud](https://github.com/bethrou/bethroud) for the exit-node daemon it connects to.

A client is useless on its own: it must **enroll** with a control-plane instance before it will start (see `client enroll`). The control plane replaces hand-edited peer lists and manually distributed keys with accounts, one-time enrollment tokens, and liveness tracking.

## Building

```sh
go build -o bethrou ./cmd/bethrou
```

Requires Go 1.24+.

## Running

```sh
# One-time: enroll with your control plane, using a token issued from the
# dashboard/API.
./bethrou enroll --api-url https://your-control-plane --token <token>

# Start the local SOCKS5 proxy (defaults to 127.0.0.1:1080).
./bethrou connect --config client.yaml
```

Copy `client.yaml.example` to `client.yaml` and fill in real values — see it for every available field (SOCKS listen address/auth, routing strategy, target-node pinning, logging). Every field also has a `--flag`/`BETHROU_CLIENT_*` env var equivalent; run `./bethrou connect --help`.

`connect` also drives a terminal UI showing live connection status alongside the SOCKS5 proxy.

## Docker

```sh
docker build -t bethrou .
docker run -e BETHROU_CLIENT_API_URL=https://your-control-plane bethrou
```

## License

MIT — see [LICENSE](LICENSE).
