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
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/Coolpy7/Coolpy7_Community/broker"
	"github.com/Coolpy7/Coolpy7_Community/wsProxy"
	"github.com/cloudwego/netpoll"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var eng *broker.Engine
var srv netpoll.EventLoop

func init() {
	_ = netpoll.SetNumLoops(1) //非1惊群效应
	_ = netpoll.SetLoadBalance(netpoll.RoundRobin)
	_ = netpoll.DisableGopool()
}

func main() {
	setLimit()
	var (
		addr         = flag.String("l", ":1883", "host port (default 1883)")
		wsAddr       = flag.String("w", ":8083", "host ws port (default 8083)")
		jwtSecretKey = flag.String("j", "", "jwt secret key(multiple split by ,")
	)
	flag.Parse()

	eng = broker.NewEngine()
	if *jwtSecretKey != "" {
		jsks := strings.Split(*jwtSecretKey, ",")
		if len(jsks) > 0 {
			for _, jsk := range jsks {
				eng.JwtSecretKey = append(eng.JwtSecretKey, []byte(jsk))
			}
		}
	}

	if err := os.RemoveAll(wsProxy.SockAddr); err != nil {
		log.Fatal(err)
	}
	listenerUnix, err := netpoll.CreateListener("unix", wsProxy.SockAddr)
	if err != nil {
		panic("create netpoll listener failed")
	}

	listener, err := netpoll.CreateListener("tcp", *addr)
	if err != nil {
		panic("create netpoll listener failed")
	}

	srv, err = netpoll.NewEventLoop(
		OnMessage,
		netpoll.WithOnPrepare(Prepare),
		netpoll.WithOnConnect(OnConnect),
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
	go func() {
		if err = srv.Serve(listenerUnix); err != nil {
			log.Println(err)
		}
	}()
	log.Println("Coolpy7 Community On Port", *addr)

	wsp := wsProxy.NewWsProxy()
	err = wsp.Start(*wsAddr)
	if err != nil {
		log.Printf("Coolpy7 ws host error %s", err)
	}

	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	cleanup := make(chan bool)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range signalChan {
			log.Println("Waiting For Coolpy7 Community Close...")
			ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			go func() {
				_ = srv.Shutdown(ctx)
				cleanup <- true
			}()
			<-cleanup
			eng.Close()
			fmt.Println("safe exit")
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

func Prepare(conn netpoll.Connection) context.Context {
	return context.Background()
}

func OnConnect(ctx context.Context, conn netpoll.Connection) context.Context {
	_ = conn.AddCloseCallback(OnDisconnect)
	eng.AddClient(conn)
	return ctx
}

func OnDisconnect(conn netpoll.Connection) error {
	c, ok := eng.GetClient(conn)
	if ok {
		c.Clear()
		eng.DelClient(conn)
		go c.Wills()
	}
	return nil
}

func OnMessage(ctx context.Context, conn netpoll.Connection) (err error) {
	c, ok := eng.GetClient(conn)
	if ok {
		err = c.Receive()
		if err != nil && err.Error() == "disCon" {
			c.ClearWill()
		}
	}
	return err
}

func setLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatal(err)
	}
}
