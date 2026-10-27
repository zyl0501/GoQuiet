# gqclient-src — gq-client 静态构建用源码副本

本目录是上游 [GoQuiet](https://github.com/cbeuw/GoQuiet)（`cmd/gq-client` +
`gqclient` 包）的**只读语义副本**，仅为产出纯静态 Android 插件二进制而存在。
上游仓库不受影响；本目录不随上游自动同步。

## 与上游的差异（仅这些，其余文件原样）

| 文件 | 说明 |
|---|---|
| `go.mod` | module 名改为 `github.com/cbeuw/GoQuiet`（匹配包内 import 路径），无外部依赖 |
| `cmd/gq-client/log_static.go` | 替换上游 `log_android.go`（cgo `__android_log_write`）→ 纯 Go `log.SetOutput(os.Stderr)`；sslocal 会把插件 stderr 转发进 logcat（tag `libsslocal`） |
| `cmd/gq-client/protector_static.go` | 替换上游 `protector_android.go`（cgo/libancillary）→ 纯 Go `syscall.UnixRights + SendmsgN`，`protect_path` socket 协议与上游完全一致 |
| （删除）`cmd/gq-client/log.go`、`protector.go` | 上游的 `!android` 桩文件；本副本所有目标平台都用统一实现 |
| （未拷贝）`cmd/gq-server`、`gqserver` | 插件只需要客户端 |

注意：文件名**不能**带 `_android` 后缀——Go 按文件名隐式施加 GOOS 约束，
而本副本用 `GOOS=linux` 构建（`GOOS=android` 会强制动态链接，见
`../docs/zhuoyitong-static-build.md` §2.4）。

## 构建

```bash
./build-static.sh    # 输出 ../app/src/main/jniLibs/arm64-v8a/libgq-client.so
```

等价手工命令：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags="-s -w" \
    -o ../app/src/main/jniLibs/arm64-v8a/libgq-client.so ./cmd/gq-client
```

验证静态性（两项都必须满足）：

```bash
file <so>                # 应含 "statically linked"
llvm-readelf -l <so>     # 应无 INTERP 段
```

## 从上游同步更新

1. 覆盖拷贝上游 `cmd/gq-client/*.go` 与 `gqclient/` 到本目录；
2. 删除拷进来的 `log.go`、`protector.go`、`log_android.go`、
   `protector_android.go` 四个文件（保留本目录的 `log_static.go`、
   `protector_static.go`）；
3. 若上游改了 protector 的 socket 协议或日志约定，对照
   `protector_static.go` 头部注释同步修改；
4. 重新执行 `./build-static.sh` 并验证静态性。
