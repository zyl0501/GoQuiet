# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

A Shadowsocks **native plugin** for Android that ships the [GoQuiet](https://github.com/cbeuw/GoQuiet) Go client binary. It is not a standalone app — it is discovered and launched by the Shadowsocks Android host app (5.x, shadowsocks-rust core `libsslocal.so`) via the shadowsocks plugin protocol (plugin id `gq-client`).

This fork is patched for the **HarmonyOS NEXT 卓易通 (DroiTong) container**: the plugin binary is fully static pure Go (`CGO_ENABLED=0 GOOS=linux`), because the classic dynamic `GOOS=android` build fails there with `libsslocal exits too fast (exit code: 70)`. Full diagnosis and rationale: `docs/zhuoyitong-static-build.md` (中文).

## Build

Two stages, both run from the repo root:

1. **Native binary** — `./gqclient-src/build-static.sh` → `app/src/main/jniLibs/arm64-v8a/libgq-client.so` (static ELF, no PT_INTERP, arm64 only; Go cannot build cgo-free android/32). `jniLibs/` is gitignored — an APK built without this step ships no plugin binary. Requires only the Go toolchain (stdlib-only module).
2. **APK** — `JAVA_HOME=<jdk17> ./gradlew assembleDebug` → `app/build/outputs/apk/debug/app-debug.apk` (debug-signed). Toolchain: Gradle 8.11.1 wrapper + AGP 8.7.3 + Kotlin 2.0.20 + compileSdk 34, minSdk 21, targetSdk 29. **JDK 17 required** (e.g. `/Users/admin/developer/jdk-17.0.2` on this machine; older JDKs won't run Gradle 8.11). The legacy `app/src/make.sh` (NDK standalone toolchain, dynamic CGO binary) is obsolete, kept for reference.

Tests: only a placeholder instrumented test (`./gradlew connectedAndroidTest`, needs a device). No unit tests, no lint config.

## Architecture

**Kotlin side** — three classes in `com.github.shadowsocks.plugin.gq_client`, subclassing types from the jitpack dependency `com.github.shadowsocks:plugin:1.3.4`:

- **BinaryProvider** (`NativePluginProvider`) — registered with `ACTION_NATIVE_PLUGIN`; `getExecutable()` returns `nativeLibraryDir/libgq-client.so`; `openFile()` serves it to Shadowsocks for URI path `/gq-client`. The string `gq-client` must stay identical across `populateFiles()`, `openFile()`, manifest `pathPrefix`/plugin-id metadata.
- **ConfigActivity** (`ConfigurationActivity`) — launched via `ACTION_CONFIGURE` on `plugin://com.github.shadowsocks/gq-client`; "apply" calls `saveChanges(options)`.
- **ConfigFragment** — legacy `android.preference.PreferenceFragment` binding four options: `Key`, `ServerName` (default `bing.com`), `TicketTimeHint` (`3600`), `Browser` (`chrome`). Option keys are duplicated in three places that must change together: `ConfigFragment.onInitializePluginOptions` (hardcoded `ary`), `res/xml/config.xml`, and the Go client's config parsing. Options serialize via `PluginOptions` as semicolon-separated `key=value`.

**Go side** — `gqclient-src/` is a source copy of upstream GoQuiet (`cmd/gq-client` + `gqclient` package) with the two cgo files replaced by pure-Go equivalents (`log_static.go` → stderr, `protector_static.go` → SCM_RIGHTS over the `protect_path` unix socket). Never rename those files with a `_android` suffix (implicit GOOS constraint breaks the `GOOS=linux` build). `gqclient-src/README.md` documents the diff and upstream-sync procedure.

## Build-config invariants (break silently if changed)

- `app/build.gradle`: `packaging.jniLibs.useLegacyPackaging = true` — **required**. AGP 8's default (`extractNativeLibs=false`) keeps the `.so` inside the APK; the plugin protocol needs a real extracted file at `nativeLibraryDir/libgq-client.so`.
- `gradle.properties`: `android.nonTransitiveRClass=false` — `R.id.toolbar` comes from the plugin AAR's layout; AGP 8 default non-transitive R breaks compilation.
- Runtime constraint (documented, not enforced): the shadowsocks server address must be an **IP**, not a domain — the static binary uses Go's pure resolver and the container has no resolv.conf/bionic DNS.

## Debugging plugin failures

Plugin logs land in logcat under tag **`libsslocal`** (sslocal pipes plugin stderr; there is no more `goquiet` tag). `adb logcat -s libsslocal` — the line before "exits too fast" is the real error. Exit code 70 = shadowsocks-rust `sysexits::Software` (plugin spawn failure, or plugin died instantly → `ping_balancer.rs` panics). Decision tree in `docs/zhuoyitong-static-build.md` §5.
