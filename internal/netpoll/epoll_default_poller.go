// Copyright (c) 2019 Andy Pan
// Copyright (c) 2017 Joshua J Baker
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

//go:build linux && !poll_opt
// +build linux,!poll_opt

package netpoll

import (
	"log"
	"os"
	"runtime"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	readEvents      = unix.EPOLLPRI | unix.EPOLLIN
	writeEvents     = unix.EPOLLOUT
	readWriteEvents = readEvents | writeEvents
)

// Poller represents a poller which is in charge of monitoring file-descriptors.
// OpenPoller instantiates a poller.
func OpenPoller() (pollFd int, err error) {
	return unix.EpollCreate1(unix.EPOLL_CLOEXEC)
}

// Close closes the poller.
func ClosePoller(pollFd int) error {
	if err := os.NewSyscallError("close", unix.Close(pollFd)); err != nil {
		return err
	}
	return nil
}

// Polling blocks the current goroutine, waiting for network-events.
func Polling(pollFd int, callback func(fd int, ev uint32) error) error {
	el := newEventList(InitPollEventsCap)
	msec := -1
	for {
		n, err := unix.EpollWait(pollFd, el.events, msec)
		if n == 0 || (n < 0 && err == unix.EINTR) {
			msec = -1
			runtime.Gosched()
			//	log.Printf("error occurs in epoll: %v", os.NewSyscallError("epoll_wait", err))
			continue
		} else if err != nil {
			log.Printf("error occurs in epoll: %v", os.NewSyscallError("epoll_wait", err))
			return err
		}
		msec = 0
		wg := sync.WaitGroup{}
		for i := 0; i < n; i++ {
			ev := &el.events[i]
			fd := int(ev.Fd)
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := callback(fd, ev.Events)
				if err != nil {
					log.Printf("error occurs in event-loop: %v", err)
				}
			}()
		}
		wg.Wait()

		if n == el.size {
			el.expand()
		} else if n < el.size>>1 {
			el.shrink()
		}
	}
}

// AddReadWrite registers the given file-descriptor with readable and writable events to the poller.
func AddReadWrite(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl add",
		unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readWriteEvents}))
}

// AddRead registers the given file-descriptor with readable event to the poller.
func AddRead(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl add",
		unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readEvents}))
}

// AddWrite registers the given file-descriptor with writable event to the poller.
// func AddWrite(pa *PollAttachment) error {
// 	return os.NewSyscallError("epoll_ctl add",
// 		unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: writeEvents}))
// }

// ModRead renews the given file-descriptor with readable event in the poller.
func ModRead(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl mod",
		unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_MOD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readEvents}))
}

// ModReadWrite renews the given file-descriptor with readable and writable events in the poller.
func ModReadWrite(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl mod",
		unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_MOD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readWriteEvents}))
}

// Delete removes the given file-descriptor from the poller.
func Delete(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl del", unix.EpollCtl(pa.PollFd, unix.EPOLL_CTL_DEL, pa.FD, nil))
}
