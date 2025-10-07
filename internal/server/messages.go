package server

import (
	"encoding/json"

	"zombierush/internal/game"
)

// ClientMessage 定義 WebSocket 客戶端發送的通用訊息格式
type ClientMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// 大廳與房間管理請求
type CreateRoomPayload struct {
	Name string `json:"name"`
}

type JoinRoomPayload struct {
	RoomID string `json:"roomId"`
}

type LeaveRoomPayload struct{}

type BotCommandPayload struct {
	Seat *int   `json:"seat,omitempty"`
	Name string `json:"name,omitempty"`
}

// 對戰階段請求
type StartGamePayload struct{}

type ChallengePayload struct {
    TargetID int   `json:"targetId"`
    Cards    []int `json:"cards"`
}

type DefensePayload struct {
    Cards []int `json:"cards"`
}

// ServerMessage 是伺服器端對外推送的通用訊息格式
type ServerMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// 大廳資訊
type RoomSummary struct {
	RoomID   string `json:"roomId"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Players  int    `json:"players"`
	Capacity int    `json:"capacity"`
	Host     string `json:"host"`
}

type LobbyRoomsPayload struct {
	Rooms []RoomSummary `json:"rooms"`
}

// 房間內公開資訊
type PublicRoomStatePayload struct {
	RoomID     string               `json:"roomId"`
	RoomName   string               `json:"roomName"`
	Status     string               `json:"status"`
	Seats      []SeatPublicSnapshot `json:"seats"`
	HostSeat   int                  `json:"hostSeat"`
	PublicGame *PublicGamePayload   `json:"publicGame,omitempty"`
}

type SeatPublicSnapshot struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Filled bool   `json:"filled"`
	IsBot  bool   `json:"isBot"`
	IsHost bool   `json:"isHost"`
	Alive  *bool  `json:"alive,omitempty"`
	Hand   *int   `json:"hand,omitempty"`
}

type PublicGamePayload struct {
	Snapshot     game.PublicSnapshot `json:"snapshot"`
	CurrentTurn  int                 `json:"currentTurn"`
	CurrentRound int                 `json:"currentRound"`
	PendingType  string              `json:"pendingType,omitempty"`
}

// 私人資訊與提示
type PrivateStatePayload struct {
	Snapshot game.PrivatePlayerSnapshot `json:"snapshot"`
}

type TurnPromptPayload struct {
	PlayerID int    `json:"playerId"`
	Name     string `json:"name"`
}

type DefensePromptPayload struct {
    AttackerID    int             `json:"attackerId"`
    AttackerName  string          `json:"attackerName"`
    AttackCards   []game.CardView `json:"attackCards"`
    Suit          *game.Suit      `json:"suit,omitempty"`
    MaxSelectable int             `json:"maxSelectable"`
    Options       []game.CardView `json:"options"`
}

type InfectionPromptPayload struct {
	WinnerID int    `json:"winnerId"`
	LoserID  int    `json:"loserId"`
	Winner   string `json:"winner"`
	Loser    string `json:"loser"`
}

type LogPayload struct {
	Message string `json:"message"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type PrivateInfoPayload struct {
	Message string `json:"message"`
}
