package game

import "fmt"

// PublicPlayerSnapshot 用於前端展示公共資訊
type PublicPlayerSnapshot struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Alive    bool   `json:"alive"`
	HandSize int    `json:"handSize"`
}

// PublicSnapshot 表示外部可見的遊戲狀態摘要
type PublicSnapshot struct {
	Round     int                    `json:"round"`
	MaxRounds int                    `json:"maxRounds"`
	Players   []PublicPlayerSnapshot `json:"players"`
}

// CardView 提供手牌的前端展示結構
type CardView struct {
	Index int      `json:"index"`
	Kind  CardKind `json:"kind"`
	Suit  Suit     `json:"suit,omitempty"`
	Value int      `json:"value,omitempty"`
	Label string   `json:"label"`
}

// PrivatePlayerSnapshot 給指定玩家查看自身詳細資訊
type PrivatePlayerSnapshot struct {
	PlayerID         int        `json:"playerId"`
	Name             string     `json:"name"`
	Identity         Identity   `json:"identity"`
	OriginalIdentity Identity   `json:"originalIdentity"`
	Hand             []CardView `json:"hand"`
}

// BuildPublicSnapshot 建立對外可觀察的玩家資訊
func (g *Game) BuildPublicSnapshot() PublicSnapshot {
	players := make([]PublicPlayerSnapshot, 0, len(g.Players))
	for _, p := range g.Players {
		handSize := len(p.Hand)
		if p.Alive {
			handSize = -1
		}
		players = append(players, PublicPlayerSnapshot{
			ID:       p.ID,
			Name:     p.Name,
			Alive:    p.Alive,
			HandSize: handSize,
		})
	}
	return PublicSnapshot{
		Round:     g.Round,
		MaxRounds: g.MaxRounds,
		Players:   players,
	}
}

// BuildPrivateSnapshot 為指定玩家製作詳細資訊
func (g *Game) BuildPrivateSnapshot(playerID int) (PrivatePlayerSnapshot, error) {
	if playerID < 0 || playerID >= len(g.Players) {
		return PrivatePlayerSnapshot{}, fmt.Errorf("無法建立玩家 %d 的視角", playerID)
	}
	p := g.Players[playerID]
	if p == nil {
		return PrivatePlayerSnapshot{}, fmt.Errorf("玩家 %d 尚未初始化", playerID)
	}
	handViews := make([]CardView, len(p.Hand))
	for i, c := range p.Hand {
		handViews[i] = CardView{
			Index: i,
			Kind:  c.Kind,
			Suit:  c.Suit,
			Value: c.Value,
			Label: c.String(),
		}
	}

	return PrivatePlayerSnapshot{
		PlayerID:         p.ID,
		Name:             p.Name,
		Identity:         p.Identity(),
		OriginalIdentity: p.OriginalIdentity(),
		Hand:             handViews,
	}, nil
}
