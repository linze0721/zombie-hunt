package server

import (
	"fmt"
	"sync"
	"time"
)

// Hub 管理大廳與房間
type Hub struct {
	mu           sync.Mutex
	rooms        map[string]*Room
	lobbyClients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		rooms:        make(map[string]*Room),
		lobbyClients: make(map[*Client]struct{}),
	}
}

func (h *Hub) RegisterLobbyClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.inLobby = true
	h.lobbyClients[c] = struct{}{}
	h.sendRoomListLocked(c)
}

func (h *Hub) unregisterLobbyClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.lobbyClients, c)
}

func (h *Hub) sendRoomListLocked(c *Client) {
	summaries := h.buildRoomSummariesLocked()
	c.sendMessage(ServerMessage{Type: "lobby_rooms", Payload: LobbyRoomsPayload{Rooms: summaries}})
}

func (h *Hub) broadcastLobbyLocked() {
	summaries := h.buildRoomSummariesLocked()
	payload := ServerMessage{Type: "lobby_rooms", Payload: LobbyRoomsPayload{Rooms: summaries}}
	for client := range h.lobbyClients {
		client.sendMessage(payload)
	}
}

func (h *Hub) buildRoomSummariesLocked() []RoomSummary {
	rooms := make([]RoomSummary, 0, len(h.rooms))
	for _, room := range h.rooms {
		rooms = append(rooms, room.summary())
	}
	return rooms
}

func (h *Hub) CreateRoom(name string, host *Client) (*Room, error) {
	if host == nil {
		return nil, fmt.Errorf("缺少房主資訊")
	}
	if name == "" {
		name = "未命名房間"
	}
	roomID := fmt.Sprintf("room-%d", time.Now().UnixNano())
	room := NewRoom(roomID, name, 8, h)

	h.mu.Lock()
	h.rooms[roomID] = room
	delete(h.lobbyClients, host)
	h.mu.Unlock()

	host.token = ""
	if err := room.Join(host); err != nil {
		h.mu.Lock()
		delete(h.rooms, roomID)
		h.lobbyClients[host] = struct{}{}
		h.sendRoomListLocked(host)
		h.mu.Unlock()
		return nil, err
	}

	h.mu.Lock()
	h.broadcastLobbyLocked()
	h.mu.Unlock()
	return room, nil
}

func (h *Hub) JoinRoom(roomID string, client *Client) error {
	h.mu.Lock()
	room, ok := h.rooms[roomID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("房間不存在")
	}
	delete(h.lobbyClients, client)
	h.mu.Unlock()

	if err := room.Join(client); err != nil {
		h.mu.Lock()
		h.lobbyClients[client] = struct{}{}
		h.sendRoomListLocked(client)
		h.broadcastLobbyLocked()
		h.mu.Unlock()
		return err
	}

	h.mu.Lock()
	h.broadcastLobbyLocked()
	h.mu.Unlock()
	return nil
}

func (h *Hub) LeaveRoom(client *Client) {
	if client == nil || client.room == nil {
		return
	}
	room := client.room
	room.onClientLeft(client)

	client.room = nil
	client.seatIndex = -1
	client.inLobby = true

	h.mu.Lock()
	h.lobbyClients[client] = struct{}{}
	h.sendRoomListLocked(client)
	if room.isEmpty() {
		delete(h.rooms, room.id)
	}
	h.broadcastLobbyLocked()
	h.mu.Unlock()
}

func (h *Hub) RemoveClient(c *Client) {
	if c == nil {
		return
	}

	if c.room != nil {
		room := c.room
		room.onClientLeft(c)
		c.room = nil
		c.seatIndex = -1
	}

	h.mu.Lock()
	delete(h.lobbyClients, c)
	for id, room := range h.rooms {
		if room.isEmpty() {
			delete(h.rooms, id)
		}
	}
	h.broadcastLobbyLocked()
	h.mu.Unlock()
}

func (h *Hub) RoomByID(id string) (*Room, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, ok := h.rooms[id]
	return room, ok
}
