// Copyright 2022 LiDonghai Email:5241871@qq.com Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wsProxy

import (
	"context"
	"errors"
	"github.com/Coolpy7/Coolpy7_Community/packet"
	"github.com/cloudwego/netpoll"
	"github.com/gobwas/ws/wsutil"
	"syscall"
	"time"
)

type WsBridge struct {
	wsConn netpoll.Connection
	cpConn netpoll.Connection
}

func NewWsbridge(conn netpoll.Connection) (*WsBridge, error) {
	wp := &WsBridge{
		wsConn: conn,
	}
	cp, err := netpoll.DialConnection("unix", SockAddr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	wp.cpConn = cp
	_ = wp.cpConn.AddCloseCallback(wp.OnDisconnect)
	_ = wp.cpConn.SetOnRequest(wp.OnRequest)
	return wp, nil
}

func (u *WsBridge) OnDisconnect(conn netpoll.Connection) error {
	_ = u.wsConn.Close()
	return nil
}

func (u *WsBridge) OnRequest(ctx context.Context, conn netpoll.Connection) (err error) {
	if !u.cpConn.IsActive() {
		return errors.New("not active")
	}
	packetLength := 0
	detectionLength := 2
	reader := u.cpConn.Reader()
	defer reader.Release()
	for {
		if detectionLength > 5 {
			u.Close()
			return err
		}
		header, err := reader.Peek(detectionLength)
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if err != nil {
			u.Close()
			return err
		}
		packetLength, _ = packet.DetectPacket(header)
		if packetLength <= 0 {
			detectionLength++
			continue
		}
		if reader.Len() < packetLength {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if uint64(packetLength) > packet.MaxVarint {
			u.Close()
			return errors.New("MaxVarint")
		}
		break
	}
	buf, err := reader.Next(packetLength)
	if err != nil {
		u.Close()
		return err
	}
	if err = wsutil.WriteServerBinary(u.wsConn, buf); err != nil {
		u.Close()
		return err
	}
	return err
}

func (u *WsBridge) Receive() error {
	if !u.wsConn.IsActive() {
		return errors.New("read err")
	}
	msg, op, err := wsutil.ReadClientData(u.wsConn)
	if err != nil {
		return err
	}
	if op.IsData() {
		if !u.cpConn.IsActive() {
			return errors.New("not active")
		}
		if _, err = u.cpConn.Write(msg); err != nil {
			return err
		}
	}
	return nil
}

func (u *WsBridge) Close() {
	if u.cpConn != nil {
		_ = u.cpConn.Close()
	}
	_ = u.wsConn.Close()
}
