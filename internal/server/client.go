package server

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

// Client 封裝一位連線玩家
type Client struct {
	conn      *websocket.Conn
	hub       *Hub
	room      *Room
	seatIndex int
	name      string
	account   string
	userID    int64
	token     string
	inLobby   bool
	send      chan []byte
	closeOnce sync.Once
}

// NewWebClient 建立客戶端
func NewWebClient(conn *websocket.Conn, hub *Hub, userID int64, account, displayName, seatToken string) *Client {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = strings.TrimSpace(account)
		if name == "" {
			name = "玩家"
		}
	}
	if len(name) > 24 {
		name = name[:24]
	}

	return &Client{
		conn:      conn,
		hub:       hub,
		seatIndex: -1,
		name:      name,
		account:   account,
		userID:    userID,
		token:     seatToken,
		send:      make(chan []byte, 256),
		inLobby:   true,
	}
}

func (c *Client) ReadPump() {
	defer c.close()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("讀取訊息異常: %v", err)
			}
			break
		}
		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError(err)
			continue
		}
		c.handleMessage(msg)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(msg ClientMessage) {
	switch msg.Type {
	case "lobby_list":
		c.hub.RegisterLobbyClient(c)
	case "room_create":
		var payload CreateRoomPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError(err)
			return
		}
		if payload.Name == "" {
			payload.Name = "未命名房間"
		}
		if c.room != nil {
			c.sendErrorErr("請先離開目前房間")
			return
		}
		if _, err := c.hub.CreateRoom(payload.Name, c); err != nil {
			c.sendError(err)
		}
	case "room_join":
		var payload JoinRoomPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError(err)
			return
		}
		if payload.RoomID == "" {
			c.sendErrorErr("缺少房間 ID")
			return
		}
		if c.room != nil {
			c.sendErrorErr("請先離開目前房間")
			return
		}
		if err := c.hub.JoinRoom(payload.RoomID, c); err != nil {
			c.sendError(err)
		}
	case "room_leave":
		c.hub.LeaveRoom(c)
		c.hub.RegisterLobbyClient(c)
	case "room_add_bot":
		if c.room == nil {
			c.sendErrorErr("尚未加入房間")
			return
		}
		if !c.room.isHost(c) {
			c.sendErrorErr("僅房主可新增機器人")
			return
		}
		var payload BotCommandPayload
		_ = json.Unmarshal(msg.Payload, &payload)
		if _, err := c.room.addBot(payload.Name); err != nil {
			c.sendError(err)
		}
	case "room_remove_bot":
		if c.room == nil {
			c.sendErrorErr("尚未加入房間")
			return
		}
		if !c.room.isHost(c) {
			c.sendErrorErr("僅房主可移除機器人")
			return
		}
		var payload BotCommandPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError(err)
			return
		}
		if payload.Seat == nil {
			c.sendErrorErr("請指定座位")
			return
		}
		if err := c.room.removeBot(*payload.Seat); err != nil {
			c.sendError(err)
		}
	case "start_game":
		if c.room == nil {
			c.sendErrorErr("尚未加入房間")
			return
		}
		if !c.room.isHost(c) {
			c.sendErrorErr("僅房主可開始對戰")
			return
		}
		if err := c.room.StartGame(); err != nil {
			c.sendError(err)
		}
	case "action_challenge":
		if c.room == nil {
			c.sendErrorErr("尚未加入房間")
			return
		}
		var payload ChallengePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError(err)
			return
		}
		if err := c.room.handleChallenge(c, payload); err != nil {
			c.sendError(err)
		}
	case "action_defense":
		if c.room == nil {
			c.sendErrorErr("尚未加入房間")
			return
		}
		var payload DefensePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			c.sendError(err)
			return
		}
		if err := c.room.handleDefenseResponse(c, payload); err != nil {
			c.sendError(err)
		}
	default:
		c.sendErrorErr("未知指令")
	}
}

func (c *Client) sendError(err error) {
	if err == nil {
		return
	}
	c.sendMessage(ServerMessage{Type: "error", Payload: ErrorPayload{Message: err.Error()}})
}

func (c *Client) sendErrorErr(msg string) {
	c.sendMessage(ServerMessage{Type: "error", Payload: ErrorPayload{Message: msg}})
}

func (c *Client) sendMessage(msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		go c.close()
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.send)
		_ = c.conn.Close()
		if c.hub != nil {
			c.hub.RemoveClient(c)
		}
	})
}
