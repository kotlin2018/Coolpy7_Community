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

package broker

import (
	"errors"
	"github.com/Coolpy7/Coolpy7_Community/packet"
	"syscall"
	"time"
)

func (c *Client) send(pkt packet.Generic) error {
	if !c.conn.IsActive() {
		return errors.New("writer err")
	}
	wt := c.conn.Writer()
	alloc, err := wt.Malloc(pkt.Len())
	if err != nil {
		return err
	}
	n, _ := pkt.Encode(alloc)
	if n != len(alloc) {
		return errors.New("encode err")
	}
	err = wt.Flush()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) readPacket() (packet.Generic, error) {
	if !c.conn.IsActive() {
		return nil, errors.New("read err")
	}
	detectionLength := 2
	reader := c.conn.Reader()
	defer reader.Release()
	for {
		if detectionLength > 5 {
			return nil, errors.New("ErrDetectionOverflow")
		}
		header, err := reader.Peek(detectionLength)
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		packetLength, packetType := packet.DetectPacket(header)
		if packetLength <= 0 {
			detectionLength++
			continue
		}
		if reader.Len() < packetLength {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if uint64(packetLength) > packet.MaxVarint {
			return nil, errors.New("ErrOutOfPacketLen")
		}
		buf, err := reader.Next(packetLength)
		if err != nil {
			return nil, err
		}
		pkt, err := packetType.New()
		if err != nil {
			return nil, err
		}
		_, err = pkt.Decode(buf)
		if err != nil {
			return nil, err
		}
		return pkt, nil
	}

}
