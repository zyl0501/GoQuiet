# 鸿蒙 NEXT「卓易通」环境下 GoQuiet 插件的修复记录（静态化重编）

> 2026-10 实测于：华为鸿蒙 NEXT + 卓易通容器 + shadowsocks-android 5.x（Rust 内核）
> + GoQuiet 插件。修复后插件正常工作。本文完整记录症状、诊断链路、根因、
> 修复方案和已知限制，供后续维护或移植到其他受限环境时参考。

## 1. 症状

在卓易通里给 shadowsocks 配置 `gq-client` 插件后，启动即报：

```
后台服务启动失败：libsslocal exits too fast(exit code:70)
```

同样的 APK 和配置在普通 Android 系统上完全正常。

## 2. 诊断链路（每一环都有源码/日志证据）

### 2.1 exit code 70 是什么

新版 shadowsocks-android（5.x）的内核是 **shadowsocks-rust**，二进制名
`libsslocal.so`（报错里的 `libsslocal` 即它）。70 是 sysexits 的
`EX_SOFTWARE`：

```rust
// shadowsocks-rust src/error.rs
Self::ServerExitUnexpectedly(_) | Self::ServerAborted(_)
    => sysexits::ExitCode::Software,   // = 70
```

即"sslocal 运行时启动失败"的统一退出码。退出前真实错误会打印到 stderr，
shadowsocks-android 的 `GuardedProcessPool` 会把 stderr 转发进 logcat
（**tag = `libsslocal`**），`exits too fast` 是它对"1 秒内退出"进程的封装报错
（`GuardedProcessPool.kt`），中文前缀来自官方 `vpn_error` 字符串资源。

### 2.2 插件失败为什么会导致 70

- sslocal 通过 `Plugin::start()` → `cmd.spawn()` 启动插件
  （`crates/shadowsocks/src/plugin/ss_plugin.rs`）。spawn 失败时
  `PingBalancerContext::new` 里的 `Plugin::start(p, ...)?` 直接把错误上抛
  → `ServerAborted` → exit 70。
- 即使 spawn 成功，`ping_balancer.rs` 的插件监控任务在插件进程退出后会
  `panic!("all plugins are exited...")` → 服务中止 → 同样 exit 70。

所以 **70 覆盖两种死法**：(a) sslocal 无法 exec 插件二进制；(b) 插件
二进制启动后立刻死亡。

### 2.3 宿主 hilog 的时间线证据

用 `hdc` 抓的宿主日志显示（启用插件时）：

```
19:51:49.580  ANCO 收到容器内 VpnService 连接（NotifyAncoVpnDetails pkgname: com.github.shadowsocks）
              宿主建 vpn-tun + 默认路由 → VPN 层是通的
19:51:49.723  shadowsocks 通知被取消（服务 stopRunner 自杀）
19:51:49.932  VPN 拆除
…4 秒内共 5 轮"连上→~300ms 死亡→拆除"循环
```

**禁用插件后**：一次连接、持续存活、零拆除（且 `anco NetworkVpn` 的
`connect error: 13` 在成功连接时同样出现，属良性噪音，可排除嫌疑）。
二分测试实锤：故障 100% 在插件执行链路。

### 2.4 根因

原构建产物（`app/src/make.sh`，NDK standalone toolchain + `CGO_ENABLED=1` +
`GOOS=android`）是**动态链接**的 Go 二进制，`PT_INTERP=/system/bin/linker64`，
依赖容器内的 bionic 动态链接器与 libc/liblog。卓易通是鸿蒙内核上的
iSulad 容器（宿主日志中的 `rgm_hmos` / `anco_service_broker` /
`rgm_engine_plugin`），其安全/兼容层对这种跨应用 exec 的动态二进制
加载失败，插件进程秒死 → sslocal exit 70。

> 验证方法：`GOOS=android` 即使 `CGO_ENABLED=0`，Go 也强制外部链接，
> 产物仍带 `interpreter /system/bin/linker64`（`file` 可见）。
> 换成 `GOOS=linux` 才得到 `statically linked`、无 INTERP 段的纯静态 ELF。
> Android 内核可直接运行静态 Linux ELF。

## 3. 修复方案

### 3.1 gq-client 去 cgo、纯静态化（`gqclient-src/`）

从上游 GoQuiet 源码拷贝的副本（见 `gqclient-src/README.md`），改动仅两处，
均为"去 cgo 等价重写"，线上协议不变：

| 文件 | 原实现（cgo） | 新实现（纯 Go） |
|---|---|---|
| `log_static.go` | `__android_log_write`（`-landroid -llog`），logcat tag `goquiet` | `log.SetOutput(os.Stderr)`；sslocal 会把插件 stderr 转发进 logcat（tag `libsslocal`） |
| `protector_static.go` | libancillary 的 `ancil_send_fd` + `set_timeout`（C 内嵌） | `syscall.Socket/Connect` + `SetsockoptTimeval`(3s) + `syscall.UnixRights` + `SendmsgN`，协议一致：连 CWD 下 `protect_path` unix socket，发 `!` + SCM_RIGHTS fd，读 1 字节回执 |

另删除 `log.go` / `protector.go` 两个 `!android` 桩文件，且文件名不能带
`_android` 后缀（Go 按文件名隐式限定 GOOS，`GOOS=linux` 时会整文件排除）。

构建命令（`gqclient-src/build-static.sh`）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
    -o ../app/src/main/jniLibs/arm64-v8a/libgq-client.so ./cmd/gq-client
```

产物验证：`file` 应显示 `statically linked`；
`llvm-readelf -l` 应无 `INTERP` 段。

### 3.2 APK 工程侧的关键改动

| 位置 | 改动 | 为什么 |
|---|---|---|
| `app/build.gradle` | `packaging.jniLibs.useLegacyPackaging = true` | **关键**。AGP 8 默认 `extractNativeLibs=false`（.so 不解压、留在 APK 内），而插件协议要求 `BinaryProvider.getExecutable()` 返回 nativeLibraryDir 下的真实文件，必须强制解压 |
| `app/build.gradle` | AGP 8.7.3 / Kotlin 2.0.20 / compileSdk 34 / versionCode 3 / versionName 1.1.0 | 原 Gradle 4.10.1+AGP 3.3.0 无法在 JDK 17 上运行 |
| `app/build.gradle` | 移除 `kotlin-android-extensions` | Kotlin 1.8+ 已删除该插件（代码未用 synthetics，无影响） |
| `settings.gradle` | 删除 `:preference-v7` `:plugin` 幽灵模块；仓库 jcenter → google+mavenCentral+jitpack | 目录不存在；jcenter 已停服 |
| `AndroidManifest.xml` | 删除 `package` 属性 | AGP 8 要求移入 `namespace` DSL |
| `gradle.properties` | `android.nonTransitiveRClass=false` | `R.id.toolbar` 来自 plugin AAR 的布局，AGP 8 默认非传递 R 类会编译失败 |
| `BinaryProvider.kt` | `context` → `context!!` | 新编译目标下 `getContext()` 为可空类型 |
| `gradle-wrapper.properties` | Gradle 8.11.1 | 与 AGP 8.7 匹配 |

### 3.3 复现构建

```bash
./gqclient-src/build-static.sh                        # 产出静态 .so
JAVA_HOME=<jdk17> ./gradlew assembleDebug             # 产出 app-debug.apk
```

## 4. 安装与使用注意

1. **先卸载**卓易通内旧的 GoQuiet 插件（debug 签名与原签名不同，无法覆盖安装），
   新 APK 必须装进卓易通容器内（与 shadowsocks 同环境）。
2. 重装后在 shadowsocks 配置里**重新选择插件** `gq-client`。
3. **服务器地址必须是 IP**：静态纯 Go 二进制用的是 Go 自带 DNS 解析器，
   容器内没有 `/etc/resolv.conf` 也没有 bionic DNS，域名解析必失败
   （日志表现为 `lookup xxx: no such host`）。
4. 仅 `arm64-v8a`：Go 的 android/32 位目标无法脱离 cgo；卓易通在麒麟
   设备上必为 arm64 容器。如目标环境确为 32 位，需另行评估。
5. 日志位置变化：插件日志现在出现在 logcat 的 **`libsslocal`** tag 下
   （stderr 转发），不再有 `goquiet` tag。

## 5. 如果将来在别的环境又见 exit 70

按本文 §2 的方法二分定位：

```bash
adb logcat -s libsslocal        # exits too fast 前一行就是真实错误
```

- `failed to start plugin ... Permission denied / No such file or directory`
  → sslocal 无法 exec 插件（沙箱拦截跨应用执行，静态化也救不了）→ 出路：
  fork shadowsocks-android 把 `libgq-client.so` 内嵌进它自己的 jniLibs，
  在 `PluginManager.init()` 特判 `gq-client` 直接返回本应用
  nativeLibraryDir 路径；
- 有插件日志输出但连接失败 → 检查服务器是否用了域名（§4.3）、
  `protect_path` 保护是否生效（VPN 回环）。

## 6. 参考

- shadowsocks-rust：`src/error.rs`（exit 70 语义）、
  `crates/shadowsocks/src/plugin/ss_plugin.rs`（插件 spawn）、
  `crates/shadowsocks-service/src/local/loadbalancing/ping_balancer.rs`（插件监控 panic）
- shadowsocks-android：`core/src/main/java/com/github/shadowsocks/bg/GuardedProcessPool.kt`
  （"exits too fast" 与 stderr→logcat 转发）、
  `core/src/main/java/com/github/shadowsocks/plugin/PluginManager.kt`（三级插件路径解析）
- SIP003 插件规范：https://shadowsocks.org/doc/sip003.html
