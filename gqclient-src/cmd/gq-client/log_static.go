
package main

import (
	"log"
	"os"
)

// Pure-Go logging for the static (CGO_ENABLED=0) Android build.
//
// When run as a shadowsocks plugin, the process is spawned by sslocal with
// stdout/stderr piped; shadowsocks-android forwards those pipes to logcat
// (tag "libsslocal"). Writing to stderr is therefore equivalent to the old
// __android_log_write path and keeps the binary free of libc/liblog, so the
// resulting ELF is fully static and has no PT_INTERP dependency.
func log_init() {
	log.SetOutput(os.Stderr)
}
