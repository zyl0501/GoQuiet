
package main

// Pure-Go port of the former cgo implementation (which was stolen from
// https://github.com/shadowsocks/overture/blob/shadowsocks/core/utils/utils_android.go).
//
// In Android, once an app starts the VpnService, all outgoing traffic is
// routed by the system to the VPN app. In our case the VPN app is sslocal.
// Our outgoing traffic to gq-server would be routed back to sslocal which
// creates an infinite loop.
//
// The Android system provides an API VpnService.protect(int socketFD).
// shadowsocks-android exposes it to child processes through a unix socket
// named "protect_path" in its working directory: the client connects, sends
// the fd via SCM_RIGHTS together with a single '!' byte, and waits for a
// one-byte acknowledgement.

import (
	"log"
	"syscall"
)

// Relative to the working directory, which is inherited from sslocal
// (shadowsocks' noBackupFilesDir, where VpnService listens on the socket).
const protectSocketPath = "protect_path"

// protectFd asks the VPN service to bypass fd around the VPN tunnel.
func protectFd(fd int) bool {
	socket, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		log.Println(err)
		return false
	}
	defer syscall.Close(socket)

	tv := syscall.Timeval{Sec: 3}
	if err := syscall.SetsockoptTimeval(socket, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		log.Println(err)
	}
	if err := syscall.SetsockoptTimeval(socket, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv); err != nil {
		log.Println(err)
	}

	if err := syscall.Connect(socket, &syscall.SockaddrUnix{Name: protectSocketPath}); err != nil {
		log.Println(err)
		return false
	}

	// SCM_RIGHTS ancillary data carrying the fd, plus the '!' payload byte,
	// matching what the old libancillary-based code sent on the wire.
	oob := syscall.UnixRights(fd)
	if _, err := syscall.SendmsgN(socket, []byte("!"), oob, nil, 0); err != nil {
		log.Println(err)
		return false
	}

	dummy := make([]byte, 1)
	n, err := syscall.Read(socket, dummy)
	if err != nil {
		log.Println(err)
		return false
	}
	if n != 1 {
		log.Println("Failed to protect fd: ", fd)
		return false
	}
	return true
}

func protector(network string, address string, c syscall.RawConn) error {
	log.Println("Using Android VPN mode.")
	fn := func(s uintptr) {
		protectFd(int(s))
	}

	if err := c.Control(fn); err != nil {
		return err
	}

	return nil
}
