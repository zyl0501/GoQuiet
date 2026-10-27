#!/usr/bin/env bash
# Rebuild the fully static gq-client binary for the Android plugin.
#
# Why CGO_ENABLED=0 + GOOS=linux:
#   - GOOS=android forces external/dynamic linking (PT_INTERP=/system/bin/linker64),
#     and android/arm even refuses CGO_ENABLED=0 outright.
#   - GOOS=linux + CGO_ENABLED=0 yields a fully static ELF with no interpreter,
#     which maximizes compatibility inside restricted Android environments
#     (e.g. the HarmonyOS "卓易通" container). It runs fine on the Android kernel.
#
# Notes:
#   - log_static.go writes to stderr; sslocal pipes plugin stderr into logcat
#     (tag "libsslocal").
#   - protector_static.go talks to the "protect_path" unix socket in the CWD
#     (shadowsocks' noBackupFilesDir) to protect outbound fds around the VPN.
#     With GOOS=linux the pure-Go DNS resolver is used and there is no
#     /etc/resolv.conf inside Android: if SS_REMOTE_HOST is a domain name,
#     resolution will fail. Use an IP for the server address.
set -e
cd "$(dirname "$0")"
OUT=../app/src/main/jniLibs
mkdir -p "$OUT/arm64-v8a"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
    -o "$OUT/arm64-v8a/libgq-client.so" ./cmd/gq-client
file "$OUT/arm64-v8a/libgq-client.so"
