package game

import (
	"fmt"
	"math/rand"
)

// Game 表示整場遊戲的狀態
type Game struct {
	Players       []*Player
	cardDeck      []Card
	cardDiscarded []Card
	Round         int
	MaxRounds     int
	rng           *rand.Rand
	logs          []string
}

func (g *Game) addLog(entry string) {
	g.logs = append(g.logs, entry)
}

// Logs 返回行動紀錄的副本
func (g *Game) Logs() []string {
	copySlice := make([]string, len(g.logs))
	copy(copySlice, g.logs)
	return copySlice
}

// AlivePlayers 回傳仍在場的玩家
func (g *Game) AlivePlayers() []*Player {
	result := make([]*Player, 0, len(g.Players))
	for _, p := range g.Players {
		if p.Alive {
			result = append(result, p)
		}
	}
	return result
}

// CountLivingIdentities 統計存活的人類與僵屍數量
func (g *Game) CountLivingIdentities() (humans int, zombies int) {
	for _, p := range g.Players {
		if !p.Alive {
			continue
		}
		if p.IsHuman() {
			humans++
		} else if p.IsZombie() {
			zombies++
		}
	}
	return
}

// AdvanceRound 進入下一回合
func (g *Game) AdvanceRound() {
	g.Round++
}

// RemainingCardCount 回傳牌堆剩餘數
func (g *Game) RemainingCardCount() int {
	return len(g.cardDeck)
}

// RemainingItemCount 回傳物品牌堆剩餘數
// IsComplete 判斷回合是否結束
func (g *Game) IsComplete() bool {
	return g.Round >= g.MaxRounds
}

// DetermineWinner 根據目前存活玩家判定勝負
func (g *Game) DetermineWinner() (humanWins bool, humans int, zombies int) {
	humans, zombies = g.CountLivingIdentities()
	humanWins = humans >= zombies
	return
}

// Player 根據編號取得玩家
func (g *Game) Player(id int) (*Player, error) {
	if id < 0 || id >= len(g.Players) {
		return nil, fmt.Errorf("找不到編號為 %d 的玩家", id)
	}
	player := g.Players[id]
	if !player.Alive {
		return nil, fmt.Errorf("玩家 %s 已被淘汰", player.Name)
	}
	return player, nil
}
