# GoQuiet-android

Shadowsocks plugin GoQuiet for Android.

This fork packages a **fully static, pure-Go** `gq-client` binary
(`CGO_ENABLED=0`, no ELF interpreter), so the plugin also works inside
restricted Android environments — notably the **HarmonyOS NEXT "卓易通"
(DroiTong) compatibility container**, where the classic dynamically-linked
build makes shadowsocks fail with:

```
后台服务启动失败：libsslocal exits too fast (exit code: 70)
```

Full background, diagnosis and troubleshooting (中文):
[docs/zhuoyitong-static-build.md](docs/zhuoyitong-static-build.md)

## Requirements

- Go (recent; the client only uses the standard library)
- JDK 17
- Android SDK: platform 34, build-tools 34+
- Gradle 8.11.1 (wrapper; downloads automatically on first use)

## Build

```sh
# 1. Build the static plugin binary
#    -> app/src/main/jniLibs/arm64-v8a/libgq-client.so
./gqclient-src/build-static.sh

# 2. Build the APK
#    -> app/build/outputs/apk/debug/app-debug.apk
./gradlew assembleDebug
```

The legacy NDK-toolchain flow (`app/src/make.sh`, dynamic CGO binary) is
obsolete and kept for reference only.

## Install notes (HarmonyOS 卓易通)

1. Uninstall any previously installed GoQuiet plugin first (signature differs).
2. Install the new APK **inside 卓易通** (same container as shadowsocks).
3. Re-select the `gq-client` plugin in your shadowsocks profile.
4. **The server address must be an IP, not a domain name** — the static
   binary has no access to Android's bionic DNS resolver, and there is no
   `/etc/resolv.conf` inside the container.
5. Only `arm64-v8a` is shipped (Go cannot build a cgo-free android/32-bit
   binary; 卓易通 containers on Kirin devices are arm64).

## How it works

- `BinaryProvider` hands shadowsocks the path
  `nativeLibraryDir/libgq-client.so` (the Go executable renamed to a `.so`
  so Android extracts it from the APK — `useLegacyPackaging = true` in
  `app/build.gradle` is **required** for this).
- `sslocal` (shadowsocks-rust) spawns it with `SS_*` environment variables;
  the plugin logs to stderr, which shadowsocks-android forwards to logcat
  under the tag `libsslocal`.
- Outbound sockets are protected from VPN routing loops through the
  `protect_path` unix socket (`protector_static.go`, pure-Go SCM_RIGHTS).
