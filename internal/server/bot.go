package server

import (
	"sort"
	"time"

	"zombierush/internal/game"
)

// BotPlayer 代表 AI 玩家
type BotPlayer struct {
	SeatIndex    int
	Name         string
	KnownZombies map[int]struct{}
}

func (r *Room) executeBotTurn(bot *BotPlayer) {
	time.Sleep(1200 * time.Millisecond)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != RoomStatusRunning || r.currentTurn != bot.SeatIndex {
		return
	}

	seat := r.seats[bot.SeatIndex]
	if seat.Player == nil || !seat.Player.Alive {
		r.advanceTurnLocked()
		return
	}
	if bot.KnownZombies == nil {
		bot.KnownZombies = make(map[int]struct{})
	}

	targets := r.aliveTargetsLocked(bot.SeatIndex)
	if len(targets) == 0 {
		r.advanceTurnLocked()
		return
	}

	attacker := seat.Player
	var attackCards []int
	var attackKind game.CardKind
	var attackSuit game.Suit
	targetIndex := -1

	// 優先使用僵屍牌感染未知陣營的玩家
	if idx := findCardKind(attacker, game.CardKindZombie); idx >= 0 {
		candidates := make([]int, 0, len(targets))
		for _, tid := range targets {
			seat := r.seats[tid]
			if seat.Player == nil || !seat.Player.Alive {
				continue
			}
			if _, known := bot.KnownZombies[tid]; known {
				continue
			}
			candidates = append(candidates, tid)
		}
		if len(candidates) == 0 {
			candidates = append(candidates, targets...)
		}
		if len(candidates) > 0 {
			attackCards = []int{idx}
			attackKind = game.CardKindZombie
			targetIndex = candidates[r.rng.Intn(len(candidates))]
		}
	}

	// 其次使用獵槍攻擊已知僵屍
	if attackCards == nil {
		if idx := findCardKind(attacker, game.CardKindShotgun); idx >= 0 {
			zombieTargets := make([]int, 0)
			for _, tid := range targets {
				seat := r.seats[tid]
				if seat.Player == nil || !seat.Player.Alive {
					continue
				}
				if _, known := bot.KnownZombies[tid]; known {
					zombieTargets = append(zombieTargets, tid)
				}
			}
			if len(zombieTargets) > 0 {
				attackCards = []int{idx}
				attackKind = game.CardKindShotgun
				targetIndex = zombieTargets[r.rng.Intn(len(zombieTargets))]
			}
		}
	}

	// 否則出最高的數字牌
	if attackCards == nil {
		bestSuit, bestIndices := selectStrongestNumericSet(attacker)
		if len(bestIndices) == 0 {
			// 無牌可出，直接結束回合
			r.advanceTurnLocked()
			return
		}
		attackCards = bestIndices
		attackKind = game.CardKindNumber
		attackSuit = bestSuit
		targetIndex = targets[r.rng.Intn(len(targets))]
	}

	defenderSeat := r.seats[targetIndex]
	if defenderSeat.Player == nil || !defenderSeat.Player.Alive {
		r.advanceTurnLocked()
		return
	}

	if attackKind == game.CardKindNumber && defenderSeat.Player.HasSuit(attackSuit) {
		if defenderSeat.Bot != nil {
			defense := selectBotDefenseIndices(defenderSeat.Player, attackSuit, len(attackCards))
			_ = r.resolveChallengeLocked(seat.Index, defenderSeat.Index, attackCards, defense)
		} else {
			// 交由真人防守
			r.pendingChallenge = &pendingChallenge{
				AttackerSeat:  seat.Index,
				DefenderSeat:  defenderSeat.Index,
				AttackerCards: attackCards,
				AttackKind:    game.CardKindNumber,
				AttackSuit:    attackSuit,
			}
			r.sendDefensePromptLocked(
				defenderSeat,
				collectSuitOptions(defenderSeat.Player, attackSuit),
				buildCardViewsFromIndices(attacker, attackCards),
				&attackSuit,
			)
			r.broadcastPublicStateLocked()
		}
		return
	}

	_ = r.resolveChallengeLocked(seat.Index, defenderSeat.Index, attackCards, nil)
}

func findCardKind(player *game.Player, kind game.CardKind) int {
	for idx, card := range player.Hand {
		if card.Kind == kind {
			return idx
		}
	}
	return -1
}

func selectStrongestNumericSet(player *game.Player) (game.Suit, []int) {
	suitMap := make(map[game.Suit][]int)
	for idx, card := range player.Hand {
		if card.Kind != game.CardKindNumber {
			continue
		}
		suitMap[card.Suit] = append(suitMap[card.Suit], idx)
	}
	var bestSuit game.Suit
	bestScore := -1
	bestIndices := []int{}
	for suit, indices := range suitMap {
		sort.SliceStable(indices, func(i, j int) bool {
			return player.Hand[indices[i]].Value > player.Hand[indices[j]].Value
		})
		limit := len(indices)
		if limit > 5 {
			limit = 5
		}
		selected := indices[:limit]
		score := 0
		for _, idx := range selected {
			score += player.Hand[idx].Value
		}
		if score > bestScore {
			bestScore = score
			bestSuit = suit
			bestIndices = append([]int(nil), selected...)
		}
	}
	sort.Ints(bestIndices)
	return bestSuit, bestIndices
}
