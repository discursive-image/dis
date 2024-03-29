// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.

package ws

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hypebeast/go-osc/osc"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	Addr string
	hub  *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan *DI

	osc *osc.Client
}

type ClientEvent struct {
	Type     string `json:"type"`
	FileName string `json:"file_name"`
}

func wsError(ws *websocket.Conn, err error) {
	logf("websocket error: %v", err)
	ws.WriteMessage(websocket.TextMessage, []byte(err.Error()))
}

func (c *Client) readMessages() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		var event ClientEvent
		if err := c.conn.ReadJSON(&event); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				errorf(err.Error())
				break
			}
			wsError(c.conn, err)
			break
		}

		logf("client event received from %v: %v -> %v", c.Addr, event.Type, event.FileName)

		var msg *osc.Message
		switch event.Type {
		case "on-screen":
			msg = osc.NewMessage("max/play")
			msg.Append(event.FileName)
		case "off-screen":
			msg = osc.NewMessage("max/stop")
		default:
			wsError(c.conn, fmt.Errorf("undefined event type %v", event.Type))
			continue
		}

		if err := c.osc.Send(msg); err != nil {
			wsError(c.conn, err)
		}
	}
}

func (c *Client) forwardMessages() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				errorf("unable to broadcast DI: %w", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
