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
	"github.com/cloudwego/netpoll"
	"github.com/gobwas/ws"
	"log"
	"strings"
	"sync"
	"time"
)

const SockAddr = "/tmp/cp9.sock"

type WsProxy struct {
	Clients *sync.Map
	srv     netpoll.EventLoop
}

func NewWsProxy() *WsProxy {
	return &WsProxy{
		Clients: &sync.Map{},
	}
}

func (w *WsProxy) AddClient(conn netpoll.Connection) (*WsBridge, error) {
	wp, err := NewWsbridge(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	w.Clients.Store(conn, wp)
	return wp, nil
}

func (w *WsProxy) GetClient(conn netpoll.Connection) (*WsBridge, bool) {
	c, ok := w.Clients.Load(conn)
	if ok {
		return c.(*WsBridge), ok
	}
	return nil, ok
}

func (w *WsProxy) DelClient(conn netpoll.Connection) {
	w.Clients.Delete(conn)
}

func (w *WsProxy) OnConnect(ctx context.Context, conn netpoll.Connection) context.Context {
	u := ws.Upgrader{
		ProtocolCustom: func(p []byte) (string, bool) {
			np := strings.ToLower(string(p))
			switch np {
			case "mqtt", "mqttv3.1":
				return np, true
			default:
				return "", false
			}
		},
	}
	// Zero-copy upgrade to WebSocket connection.
	_, err := u.Upgrade(conn)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		_ = conn.Close()
		return ctx
	}
	_ = conn.AddCloseCallback(w.OnDisconnect)
	wsb, err := w.AddClient(conn)
	if err != nil {
		wsb.Close()
		return ctx
	}
	return ctx
}

func (w *WsProxy) OnDisconnect(conn netpoll.Connection) error {
	wsb, ok := w.GetClient(conn)
	if ok {
		w.DelClient(conn)
		wsb.Close()
	}
	return nil
}

func (w *WsProxy) OnMessage(ctx context.Context, conn netpoll.Connection) (err error) {
	wsb, ok := w.GetClient(conn)
	if ok {
		err = wsb.Receive()
	}
	return err
}

func (w *WsProxy) Start(addr string) error {
	listener, err := netpoll.CreateListener("tcp", addr)
	if err != nil {
		panic("create websocket netpoll listener failed")
	}

	srv, err := netpoll.NewEventLoop(
		w.OnMessage,
		netpoll.WithOnConnect(w.OnConnect),
		netpoll.WithReadTimeout(2*time.Second),
		netpoll.WithIdleTimeout(30*time.Minute),
	)
	if err != nil {
		panic(err)
	}
	go func() {
		if err = srv.Serve(listener); err != nil {
			log.Println(err)
		}
	}()
	log.Println("Coolpy7 Community Websocket On Port", addr)

	return nil
}
