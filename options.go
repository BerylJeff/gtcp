// Copyright (c) 2019 The Glibevent Authors. All rights reserved.
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

package glibevent

import (
	"time"
)

// Option is a function that will set up option.
type Option func(opts *Options)

func loadOptions(options ...Option) *Options {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	return opts
}

// TCPSocketOpt is the type of TCP socket options.
type TCPSocketOpt int

// Available TCP socket options.
const (
	TCPNoDelay TCPSocketOpt = iota
	TCPDelay
)

// Options are configurations for the gnet application.
type Options struct {
	// ================================== Options for only server-side ==================================

	// Multicore indicates whether the engine will be effectively created with multi-cores, if so,
	// then you must take care with synchronizing memory between all event callbacks, otherwise,
	// it will run the engine with single thread. The number of threads in the engine will be automatically
	// assigned to the value of logical CPUs usable by the current process.
	Multicore bool

	// NumEventLoop is set up to start the given number of event-loop goroutine.
	// Note: Setting up NumEventLoop will override Multicore.
	NumEventLoop int

	// ReuseAddr indicates whether to set up the SO_REUSEADDR socket option.
	ReuseAddr bool

	// ReusePort indicates whether to set up the SO_REUSEPORT socket option.
	ReusePort bool

	// MulticastInterfaceIndex is the index of the interface name where the multicast UDP addresses will be bound to.
	MulticastInterfaceIndex int

	// ============================= Options for both server-side and client-side =============================

	// ReadBufferCap is the maximum number of bytes that can be read from the peer when the readable event comes.
	// The default value is 64KB, it can either be reduced to avoid starving the subsequent connections or increased
	// to read more data from a socket.
	//
	// Note that ReadBufferCap will always be converted to the least power of two integer value greater than
	// or equal to its real amount.
	ReadBufferCap int

	// WriteBufferCap is the maximum number of bytes that a static outbound buffer can hold,
	// if the data exceeds this value, the overflow will be stored in the elastic linked list buffer.
	// The default value is 64KB.
	//
	// Note that WriteBufferCap will always be converted to the least power of two integer value greater than
	// or equal to its real amount.
	WriteBufferCap int

	// LockOSThread is used to determine whether each I/O event-loop is associated to an OS thread, it is useful when you
	// need some kind of mechanisms like thread local storage, or invoke certain C libraries (such as graphics lib: GLib)
	// that require thread-level manipulation via cgo, or want all I/O event-loops to actually run in parallel for a
	// potential higher performance.
	LockOSThread bool

	// Ticker indicates whether the ticker has been set up.
	Ticker bool

	// TCPKeepAlive sets up a duration for (SO_KEEPALIVE) socket option.
	TCPKeepAlive time.Duration

	// TCPNoDelay controls whether the operating system should delay
	// packet transmission in hopes of sending fewer packets (Nagle's algorithm).
	//
	// The default is true (no delay), meaning that data is sent
	// as soon as possible after a write operation.
	TCPNoDelay TCPSocketOpt

	// SocketRecvBuffer sets the maximum socket receive buffer in bytes.
	SocketRecvBuffer int

	// SocketSendBuffer sets the maximum socket send buffer in bytes.
	SocketSendBuffer int
}

// WithOptions sets up all options.
func WithOptions(options Options) Option {
	return func(opts *Options) {
		*opts = options
	}
}

// WithMulticore sets up multi-cores in gnet engine.
func WithMulticore(multicore bool) Option {
	return func(opts *Options) {
		opts.Multicore = multicore
	}
}

// WithLockOSThread sets up LockOSThread mode for I/O event-loops.
func WithLockOSThread(lockOSThread bool) Option {
	return func(opts *Options) {
		opts.LockOSThread = lockOSThread
	}
}

// WithReadBufferCap sets up ReadBufferCap for reading bytes.
func WithReadBufferCap(readBufferCap int) Option {
	return func(opts *Options) {
		opts.ReadBufferCap = readBufferCap
	}
}

// WithWriteBufferCap sets up WriteBufferCap for pending bytes.
func WithWriteBufferCap(writeBufferCap int) Option {
	return func(opts *Options) {
		opts.WriteBufferCap = writeBufferCap
	}
}

// WithNumEventLoop sets up NumEventLoop in gnet engine.
func WithNumEventLoop(numEventLoop int) Option {
	return func(opts *Options) {
		opts.NumEventLoop = numEventLoop
	}
}

// WithReusePort sets up SO_REUSEPORT socket option.
func WithReusePort(reusePort bool) Option {
	return func(opts *Options) {
		opts.ReusePort = reusePort
	}
}

// WithReuseAddr sets up SO_REUSEADDR socket option.
func WithReuseAddr(reuseAddr bool) Option {
	return func(opts *Options) {
		opts.ReuseAddr = reuseAddr
	}
}

// WithTCPKeepAlive sets up the SO_KEEPALIVE socket option with duration.
func WithTCPKeepAlive(tcpKeepAlive time.Duration) Option {
	return func(opts *Options) {
		opts.TCPKeepAlive = tcpKeepAlive
	}
}

// WithTCPNoDelay enable/disable the TCP_NODELAY socket option.
func WithTCPNoDelay(tcpNoDelay TCPSocketOpt) Option {
	return func(opts *Options) {
		opts.TCPNoDelay = tcpNoDelay
	}
}

// WithSocketRecvBuffer sets the maximum socket receive buffer in bytes.
func WithSocketRecvBuffer(recvBuf int) Option {
	return func(opts *Options) {
		opts.SocketRecvBuffer = recvBuf
	}
}

// WithSocketSendBuffer sets the maximum socket send buffer in bytes.
func WithSocketSendBuffer(sendBuf int) Option {
	return func(opts *Options) {
		opts.SocketSendBuffer = sendBuf
	}
}

// WithTicker indicates that a ticker is set.
func WithTicker(ticker bool) Option {
	return func(opts *Options) {
		opts.Ticker = ticker
	}
}

// WithMulticastInterfaceIndex sets the interface name where UDP multicast sockets will be bound to.
func WithMulticastInterfaceIndex(idx int) Option {
	return func(opts *Options) {
		opts.MulticastInterfaceIndex = idx
	}
}
