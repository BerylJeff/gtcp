// Copyright (c) 2023 The Glibevent Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package glibevent

import (
	"sync"
)

type connMatrix struct {
	mtx     sync.RWMutex
	connMap map[int]*conn
}

func (cm *connMatrix) init() {
	cm.mtx = sync.RWMutex{}
	cm.connMap = make(map[int]*conn)
}

func (cm *connMatrix) iterate(f func(*conn) bool) {
	cm.mtx.RLock()
	defer cm.mtx.RUnlock()
	for k, c := range cm.connMap {
		if c != nil {
			if !f(c) {
				delete(cm.connMap, k)
			}
		}
	}
}

func (cm *connMatrix) loadCount() (n int) {
	cm.mtx.RLock()
	defer cm.mtx.RUnlock()
	return len(cm.connMap)
}

func (cm *connMatrix) addConn(fd int, c *conn) {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()
	cm.connMap[fd] = c
}

func (cm *connMatrix) delConn(fd int) {
	cm.mtx.Lock()
	defer cm.mtx.Unlock()
	delete(cm.connMap, fd)
}

func (cm *connMatrix) getConn(fd int) *conn {
	cm.mtx.RLock()
	defer cm.mtx.RUnlock()
	return cm.connMap[fd]
}
