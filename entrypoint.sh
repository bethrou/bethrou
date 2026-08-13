#!/bin/sh
# Fixes the "connect" subcommand and this container's standard
# config/key paths; trailing args (e.g. --identity-key overrides) are
# appended, not a replacement.
set -e
exec /usr/bin/bethrou connect --config /etc/bethrou/client.yaml --key /etc/bethrou/network.key "$@"
