// Copyright (c) 2020 The Glibevent Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux || freebsd || dragonfly || darwin
// +build linux freebsd dragonfly darwin

package netpoll

import (
	"errors"
	// "internal/syscall/unix"
	"io"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

// CloseFunc is used to hook the close call.
var CloseFunc func(int) error = syscall.Close

// AcceptFunc is used to hook the accept call.
var AcceptFunc func(int) (int, syscall.Sockaddr, error) = syscall.Accept

// errNetClosing is the type of the variable ErrNetClosing.
// This is used to implement the net.Error interface.
type errNetClosing struct{}

// Error returns the error message for ErrNetClosing.
// Keep this string consistent because of issue #4373:
// since historically programs have not been able to detect
// this error, they look for the string.
func (e errNetClosing) Error() string { return "use of closed network connection" }

func (e errNetClosing) Timeout() bool   { return false }
func (e errNetClosing) Temporary() bool { return false }

// ErrNetClosing is returned when a network descriptor is used after
// it has been closed.
var ErrNetClosing = errNetClosing{}

// ErrFileClosing is returned when a file descriptor is used after it
// has been closed.
var ErrFileClosing = errors.New("use of closed file")

// ErrNoDeadline is returned when a request is made to set a deadline
// on a file type that does not use the poller.
var ErrNoDeadline = errors.New("file type does not support deadline")

// Return the appropriate closing error based on isFile.
func errClosing(isFile bool) error {
	if isFile {
		return ErrFileClosing
	}
	return ErrNetClosing
}

func fcntl(fd int, cmd int, arg int) (int, error) {
	r, _, e := syscall.Syscall(unix.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if e != 0 {
		return int(r), syscall.Errno(e)
	}
	return int(r), nil
}

// ErrDeadlineExceeded is returned for an expired deadline.
// This is exported by the os package as os.ErrDeadlineExceeded.
var ErrDeadlineExceeded error = &DeadlineExceededError{}

// DeadlineExceededError is returned for an expired deadline.
type DeadlineExceededError struct{}

// Implement the net.Error interface.
// The string is "i/o timeout" because that is what was returned
// by earlier Go versions. Changing it may break programs that
// match on error strings.
func (e *DeadlineExceededError) Error() string   { return "i/o timeout" }
func (e *DeadlineExceededError) Timeout() bool   { return true }
func (e *DeadlineExceededError) Temporary() bool { return true }

// ErrNotPollable is returned when the file or socket is not suitable
// for event notification.
var ErrNotPollable = errors.New("not pollable")

// consume removes data from a slice of byte slices, for writev.
func consume(v *[][]byte, n int64) {
	for len(*v) > 0 {
		ln0 := int64(len((*v)[0]))
		if ln0 > n {
			(*v)[0] = (*v)[0][n:]
			return
		}
		n -= ln0
		(*v)[0] = nil
		*v = (*v)[1:]
	}
}

// tryDupCloexec indicates whether F_DUPFD_CLOEXEC should be used.
// If the kernel doesn't support it, this is set to 0.
var tryDupCloexec = int32(1)

// TestHookDidWritev is a hook for testing writev.
var TestHookDidWritev = func(wrote int) {}

// DupCloseOnExec dups fd and marks it close-on-exec.
func DupCloseOnExec(fd int) (int, string, error) {
	if syscall.F_DUPFD_CLOEXEC != 0 && atomic.LoadInt32(&tryDupCloexec) == 1 {
		r0, e1 := fcntl(fd, syscall.F_DUPFD_CLOEXEC, 0)
		if e1 == nil {
			return r0, "", nil
		}
		switch e1.(syscall.Errno) {
		case syscall.EINVAL, syscall.ENOSYS:
			// Old kernel, or js/wasm (which returns
			// ENOSYS). Fall back to the portable way from
			// now on.
			atomic.StoreInt32(&tryDupCloexec, 0)
		default:
			return -1, "fcntl", e1
		}
	}
	return dupCloseOnExecOld(fd)
}

// dupCloseOnExecOld is the traditional way to dup an fd and
// set its O_CLOEXEC bit, using two system calls.
func dupCloseOnExecOld(fd int) (int, string, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	newfd, err := syscall.Dup(fd)
	if err != nil {
		return -1, "dup", err
	}
	syscall.CloseOnExec(newfd)
	return newfd, "", nil
}

// NetFD is a file descriptor. The net and os packages use this type as a
// field of a larger type representing a network connection or OS file.
type NetFD struct {
	// System file descriptor. Immutable until Close.
	Sysfd int
}

func NewNetFD(sysfd int) *NetFD {
	return &NetFD{Sysfd: sysfd}
}

// Dup duplicates the file descriptor.
func (fd *NetFD) Dup() (int, string, error) {
	return DupCloseOnExec(fd.Sysfd)
}

// Destroy closes the file descriptor. This is called when there are
// no remaining references.
func (fd *NetFD) destroy() error {
	// We don't use ignoringEINTR here because POSIX does not define
	// whether the descriptor is closed if close returns EINTR.
	// If the descriptor is indeed closed, using a loop would race
	// with some other goroutine opening a new descriptor.
	// (The Linux kernel guarantees that it is closed on an EINTR error.)
	err := CloseFunc(fd.Sysfd)

	fd.Sysfd = -1
	return err
}

// Close closes the NetFD. The underlying file descriptor is closed by the
// destroy method when there are no remaining references.
func (fd *NetFD) Close() error {
	err := CloseFunc(fd.Sysfd)
	fd.Sysfd = -1
	return err
}

// SetBlocking puts the file into blocking mode.
func (fd *NetFD) SetBlocking() error {
	return syscall.SetNonblock(fd.Sysfd, false)
}

// Darwin and FreeBSD can't read or write 2GB+ files at a time,
// even on 64-bit systems.
// The same is true of socket implementations on many systems.
// See golang.org/issue/7812 and golang.org/issue/16266.
// Use 1GB instead of, say, 2GB-1, to keep subsequent reads aligned.
const maxRW = 1 << 30

// Read implements io.Reader.
func (fd *NetFD) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(p) > maxRW {
		p = p[:maxRW]
	}
	n, err := ignoringEINTRIO(syscall.Read, fd.Sysfd, p)
	if err != nil {
		n = 0
	}
	return n, err
}

// Pread wraps the pread system call.
func (fd *NetFD) Pread(p []byte, off int64) (int, error) {
	if len(p) > maxRW {
		p = p[:maxRW]
	}
	var (
		n   int
		err error
	)
	for {
		n, err = syscall.Pread(fd.Sysfd, p, off)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		n = 0
	}
	return n, err
}

// ReadFrom wraps the recvfrom network call.
func (fd *NetFD) ReadFrom(p []byte) (int, syscall.Sockaddr, error) {
	for {
		n, sa, err := syscall.Recvfrom(fd.Sysfd, p, 0)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
		}
		return n, sa, err
	}
}

// ReadFromInet4 wraps the recvfrom network call for IPv4.
// func (fd *NetFD) ReadFromInet4(p []byte, from *syscall.SockaddrInet4) (int, error) {
// 	for {
// 		n, err := unix.RecvfromInet4(fd.Sysfd, p, 0, from)
// 		if err != nil {
// 			if err == syscall.EINTR {
// 				continue
// 			}
// 			n = 0
// 		}
// 		return n, err
// 	}
// }

// // ReadFromInet6 wraps the recvfrom network call for IPv6.
// func (fd *NetFD) ReadFromInet6(p []byte, from *syscall.SockaddrInet6) (int, error) {
// 	for {
// 		n, err := unix.RecvfromInet6(fd.Sysfd, p, 0, from)
// 		if err != nil {
// 			if err == syscall.EINTR {
// 				continue
// 			}
// 			n = 0
// 		}
// 		return n, err
// 	}
// }

// ReadMsg wraps the recvmsg network call.
func (fd *NetFD) ReadMsg(p []byte, oob []byte, flags int) (int, int, int, syscall.Sockaddr, error) {
	for {
		n, oobn, sysflags, sa, err := syscall.Recvmsg(fd.Sysfd, p, oob, flags)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
		}
		return n, oobn, sysflags, sa, err
	}
}

// ReadMsgInet4 is ReadMsg, but specialized for syscall.SockaddrInet4.
// func (fd *NetFD) ReadMsgInet4(p []byte, oob []byte, flags int, sa4 *syscall.SockaddrInet4) (int, int, int, error) {
// 	for {
// 		n, oobn, sysflags, err := unix.RecvmsgInet4(fd.Sysfd, p, oob, flags, sa4)
// 		if err != nil {
// 			if err == syscall.EINTR {
// 				continue
// 			}
// 		}
// 		return n, oobn, sysflags, err
// 	}
// }

// // ReadMsgInet6 is ReadMsg, but specialized for syscall.SockaddrInet6.
// func (fd *NetFD) ReadMsgInet6(p []byte, oob []byte, flags int, sa6 *syscall.SockaddrInet6) (int, int, int, error) {
// 	for {
// 		n, oobn, sysflags, err := unix.RecvmsgInet6(fd.Sysfd, p, oob, flags, sa6)
// 		if err != nil {
// 			if err == syscall.EINTR {
// 				continue
// 			}
// 		}
// 		return n, oobn, sysflags, err
// 	}
// }

func (fd *NetFD) Writev(iov [][]byte) (int, error) {
	if len(iov) == 0 {
		return 0, nil
	}
	return unix.Writev(fd.Sysfd, iov)
}

// Readv calls readv() on Linux.
func (fd *NetFD) Readv(iov [][]byte) (int, error) {
	if len(iov) == 0 {
		return 0, nil
	}
	return unix.Readv(fd.Sysfd, iov)
}

// Write implements io.Writer.
func (fd *NetFD) Write(p []byte) (int, error) {
	var nn int
	for {
		max := len(p)
		if max-nn > maxRW {
			max = nn + maxRW
		}
		n, err := ignoringEINTRIO(syscall.Write, fd.Sysfd, p[nn:max])
		if n > 0 {
			nn += n
		}
		if nn == len(p) {
			return nn, err
		}
		if err != nil {
			return nn, err
		}
		if n == 0 {
			return nn, io.ErrUnexpectedEOF
		}
	}
}

// Pwrite wraps the pwrite system call.
func (fd *NetFD) Pwrite(p []byte, off int64) (int, error) {
	var nn int
	for {
		max := len(p)
		if max-nn > maxRW {
			max = nn + maxRW
		}
		n, err := syscall.Pwrite(fd.Sysfd, p[nn:max], off+int64(nn))
		if err == syscall.EINTR {
			continue
		}
		if n > 0 {
			nn += n
		}
		if nn == len(p) {
			return nn, err
		}
		if err != nil {
			return nn, err
		}
		if n == 0 {
			return nn, io.ErrUnexpectedEOF
		}
	}
}

// WriteToInet4 wraps the sendto network call for IPv4 addresses.
// func (fd *NetFD) WriteToInet4(p []byte, sa *syscall.SockaddrInet4) (int, error) {
// 	for {
// 		err := unix.SendtoInet4(fd.Sysfd, p, 0, sa)
// 		if err == syscall.EINTR {
// 			continue
// 		}
// 		if err != nil {
// 			return 0, err
// 		}
// 		return len(p), nil
// 	}
// }

// // WriteToInet6 wraps the sendto network call for IPv6 addresses.
// func (fd *NetFD) WriteToInet6(p []byte, sa *syscall.SockaddrInet6) (int, error) {
// 	for {
// 		err := unix.SendtoInet6(fd.Sysfd, p, 0, sa)
// 		if err == syscall.EINTR {
// 			continue
// 		}
// 		if err != nil {
// 			return 0, err
// 		}
// 		return len(p), nil
// 	}
// }

// WriteTo wraps the sendto network call.
func (fd *NetFD) WriteTo(p []byte, sa syscall.Sockaddr) (int, error) {
	for {
		err := syscall.Sendto(fd.Sysfd, p, 0, sa)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
}

// WriteMsg wraps the sendmsg network call.
func (fd *NetFD) WriteMsg(p []byte, oob []byte, sa syscall.Sockaddr) (int, int, error) {
	for {
		n, err := syscall.SendmsgN(fd.Sysfd, p, oob, sa, 0)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return n, 0, err
		}
		return n, len(oob), err
	}
}

// // WriteMsgInet4 is WriteMsg specialized for syscall.SockaddrInet4.
// func (fd *NetFD) WriteMsgInet4(p []byte, oob []byte, sa *syscall.SockaddrInet4) (int, int, error) {
// 	for {
// 		n, err := unix.SendmsgNInet4(fd.Sysfd, p, oob, sa, 0)
// 		if err == syscall.EINTR {
// 			continue
// 		}
// 		if err != nil {
// 			return n, 0, err
// 		}
// 		return n, len(oob), err
// 	}
// }

// // WriteMsgInet6 is WriteMsg specialized for syscall.SockaddrInet6.
// func (fd *NetFD) WriteMsgInet6(p []byte, oob []byte, sa *syscall.SockaddrInet6) (int, int, error) {
// 	for {
// 		n, err := unix.SendmsgNInet6(fd.Sysfd, p, oob, sa, 0)
// 		if err == syscall.EINTR {
// 			continue
// 		}
// 		if err != nil {
// 			return n, 0, err
// 		}
// 		return n, len(oob), err
// 	}
// }

// Seek wraps syscall.Seek.
func (fd *NetFD) Seek(offset int64, whence int) (int64, error) {
	return syscall.Seek(fd.Sysfd, offset, whence)
}

// ReadDirent wraps syscall.ReadDirent.
// We treat this like an ordinary system call rather than a call
// that tries to fill the buffer.
func (fd *NetFD) ReadDirent(buf []byte) (int, error) {
	for {
		n, err := ignoringEINTRIO(syscall.ReadDirent, fd.Sysfd, buf)
		if err != nil {
			n = 0
		}
		// Do not call eofError; caller does not expect to see io.EOF.
		return n, err
	}
}

// Fchmod wraps syscall.Fchmod.
func (fd *NetFD) Fchmod(mode uint32) error {
	return ignoringEINTR(func() error {
		return syscall.Fchmod(fd.Sysfd, mode)
	})
}

// Fchdir wraps syscall.Fchdir.
func (fd *NetFD) Fchdir() error {
	return syscall.Fchdir(fd.Sysfd)
}

// Fstat wraps syscall.Fstat
func (fd *NetFD) Fstat(s *syscall.Stat_t) error {
	return ignoringEINTR(func() error {
		return syscall.Fstat(fd.Sysfd, s)
	})
}

// On Unix variants only, expose the IO event for the net code.
// WriteOnce is for testing only. It makes a single write call.
func (fd *NetFD) WriteOnce(p []byte) (int, error) {
	return ignoringEINTRIO(syscall.Write, fd.Sysfd, p)
}

// RawRead invokes the user-defined function f for a read operation.
func (fd *NetFD) RawRead(f func(uintptr) bool) error {
	for {
		if f(uintptr(fd.Sysfd)) {
			return nil
		}
	}
}

// RawWrite invokes the user-defined function f for a write operation.
func (fd *NetFD) RawWrite(f func(uintptr) bool) error {
	for {
		if f(uintptr(fd.Sysfd)) {
			return nil
		}
	}
}

// eofError returns io.EOF when fd is available for reading end of
// file.
// func (fd *NetFD) eofError(n int, err error) error {
// 	if n == 0 && err == nil && fd.ZeroReadIsEOF {
// 		return io.EOF
// 	}
// 	return err
// }

// Shutdown wraps syscall.Shutdown.
func (fd *NetFD) Shutdown(how int) error {
	return syscall.Shutdown(fd.Sysfd, how)
}

// Fchown wraps syscall.Fchown.
func (fd *NetFD) Fchown(uid, gid int) error {
	return ignoringEINTR(func() error {
		return syscall.Fchown(fd.Sysfd, uid, gid)
	})
}

// Ftruncate wraps syscall.Ftruncate.
func (fd *NetFD) Ftruncate(size int64) error {
	return ignoringEINTR(func() error {
		return syscall.Ftruncate(fd.Sysfd, size)
	})
}

// RawControl invokes the user-defined function f for a non-IO
// operation.
func (fd *NetFD) RawControl(f func(uintptr)) error {
	f(uintptr(fd.Sysfd))
	return nil
}

// ignoringEINTR makes a function call and repeats it if it returns
// an EINTR error. This appears to be required even though we install all
// signal handlers with SA_RESTART: see #22838, #38033, #38836, #40846.
// Also #20400 and #36644 are issues in which a signal handler is
// installed without setting SA_RESTART. None of these are the common case,
// but there are enough of them that it seems that we can't avoid
// an EINTR loop.
func ignoringEINTR(fn func() error) error {
	for {
		err := fn()
		if err != syscall.EINTR {
			return err
		}
	}
}

// ignoringEINTRIO is like ignoringEINTR, but just for IO calls.
func ignoringEINTRIO(fn func(fd int, p []byte) (int, error), fd int, p []byte) (int, error) {
	for {
		n, err := fn(fd, p)
		if err != syscall.EINTR {
			return n, err
		}
	}
}
