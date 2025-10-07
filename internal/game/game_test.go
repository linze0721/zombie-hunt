package game

import "testing"

func TestNewGameSetup(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	g, err := NewGame(names, 42)
	if err != nil {
		t.Fatalf("NewGame 應該成功，卻得到錯誤：%v", err)
	}
	if len(g.Players) != 8 {
		t.Fatalf("預期 8 名玩家，實際 %d", len(g.Players))
	}

	numericTotal := 0
	zombieCount := 0
	shotgunTotal := 0
	for _, p := range g.Players {
		if !p.Alive {
			t.Fatalf("初始化時玩家不應被淘汰")
		}
		if p.HandSize() < 16 {
			t.Fatalf("玩家 %s 初始牌數至少 16（含獵槍），實際 %d", p.Name, p.HandSize())
		}
		countNumeric := 0
		playerShotguns := 0
		for _, c := range p.Hand {
			if c.Kind == CardKindNumber {
				countNumeric++
			} else if c.Kind == CardKindZombie {
				zombieCount++
			} else if c.Kind == CardKindShotgun {
				playerShotguns++
			}
		}
		if countNumeric != 15 {
			t.Fatalf("玩家 %s 的數字牌應為 15，實際 %d", p.Name, countNumeric)
		}
		numericTotal += countNumeric
		if playerShotguns < initialShotgunPerPlayer {
			t.Fatalf("玩家 %s 應持有至少 %d 張獵槍牌，實際 %d", p.Name, initialShotgunPerPlayer, playerShotguns)
		}
		shotgunTotal += playerShotguns
	}
	if numericTotal != 120 {
		t.Fatalf("總數字牌應為 120，實際 %d", numericTotal)
	}
	if zombieCount != 2 { // 初始僵屍各拿一張僵屍牌
		t.Fatalf("僵屍牌應為 2 張，實際 %d", zombieCount)
	}
	if shotgunTotal != len(g.Players)*initialShotgunPerPlayer {
		t.Fatalf("獵槍總數應為 %d，實際 %d", len(g.Players)*initialShotgunPerPlayer, shotgunTotal)
	}
	if g.RemainingCardCount() != 36 {
		t.Fatalf("數字牌牌庫剩餘 36，實際 %d", g.RemainingCardCount())
	}
}

func TestZombieCardGuaranteesInfection(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	g, _ := NewGame(names, 1)
	attacker := g.Players[0]
	defender := g.Players[1]
	attacker.SetIdentity(IdentityZombie)
	defender.SetIdentity(IdentityHuman)
	attacker.Hand = []Card{{Kind: CardKindZombie}}
	defender.Hand = []Card{{Kind: CardKindNumber, Suit: SuitSpade, Value: 5}}

	outcome, err := g.Challenge(ChallengeOptions{
		AttackerID:    attacker.ID,
		DefenderID:    defender.ID,
		AttackerCards: []int{0},
		DefenderCards: []int{0},
	})
	if err != nil {
		t.Fatalf("挑戰錯誤: %v", err)
	}
	if outcome.WinnerID == nil || *outcome.WinnerID != attacker.ID {
		t.Fatalf("僵屍牌應確保勝利")
	}
	if !outcome.Infection {
		t.Fatalf("僵屍牌應觸發感染")
	}
	if !defender.IsZombie() {
		t.Fatalf("防守方應被感染為僵屍")
	}
	if defender.CountKind(CardKindZombie) == 0 {
		t.Fatalf("新僵屍應獲得僵屍牌")
	}
}

func TestVaccineCountersZombieCard(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	g, _ := NewGame(names, 2)
	attacker := g.Players[0]
	defender := g.Players[1]
	attacker.SetIdentity(IdentityZombie)
	defender.SetIdentity(IdentityHuman)
	attacker.Hand = []Card{{Kind: CardKindZombie}}
	defender.Hand = []Card{{Kind: CardKindVaccine}}

	outcome, err := g.Challenge(ChallengeOptions{
		AttackerID:    attacker.ID,
		DefenderID:    defender.ID,
		AttackerCards: []int{0},
		DefenderCards: []int{0},
	})
	if err != nil {
		t.Fatalf("挑戰錯誤: %v", err)
	}
	if outcome.WinnerID == nil || *outcome.WinnerID != defender.ID {
		t.Fatalf("疫苗應逆轉勝利")
	}
	if attacker.IsZombie() {
		t.Fatalf("攻擊者應被轉回人類")
	}
}

func TestMultiCardComparison(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	g, _ := NewGame(names, 3)

	attacker := g.Players[0]
	defender := g.Players[1]
	attacker.Hand = []Card{{Kind: CardKindNumber, Suit: SuitSpade, Value: 9}, {Kind: CardKindNumber, Suit: SuitSpade, Value: 8}}
	defender.Hand = []Card{{Kind: CardKindNumber, Suit: SuitSpade, Value: 5}, {Kind: CardKindNumber, Suit: SuitSpade, Value: 4}}

	outcome, err := g.Challenge(ChallengeOptions{
		AttackerID:    attacker.ID,
		DefenderID:    defender.ID,
		AttackerCards: []int{0, 1},
		DefenderCards: []int{0, 1},
	})
	if err != nil {
		t.Fatalf("挑戰錯誤: %v", err)
	}
	if outcome.WinnerID == nil || *outcome.WinnerID != attacker.ID {
		t.Fatalf("攻擊者應以 17 點擊敗 9 點")
	}
}

func TestShotgunOnlyAffectsZombies(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	g, _ := NewGame(names, 4)
	attacker := g.Players[0]
	human := g.Players[1]
	zombie := g.Players[2]
	attacker.SetIdentity(IdentityHuman)
	human.SetIdentity(IdentityHuman)
	zombie.SetIdentity(IdentityZombie)

	attacker.Hand = []Card{{Kind: CardKindShotgun}, {Kind: CardKindShotgun}}
	human.Hand = []Card{{Kind: CardKindNumber, Suit: SuitClub, Value: 3}}
	zombie.Hand = []Card{{Kind: CardKindNumber, Suit: SuitClub, Value: 4}}

	// 對人類 -> 失敗
	outcome1, err := g.Challenge(ChallengeOptions{
		AttackerID:    attacker.ID,
		DefenderID:    human.ID,
		AttackerCards: []int{0},
		DefenderCards: []int{0},
	})
	if err != nil {
		t.Fatalf("挑戰錯誤: %v", err)
	}
	if outcome1.WinnerID == nil || *outcome1.WinnerID != human.ID {
		t.Fatalf("獵槍攻擊人類應失敗")
	}

	// 對僵屍 -> 命中淘汰
	outcome2, err := g.Challenge(ChallengeOptions{
		AttackerID:    attacker.ID,
		DefenderID:    zombie.ID,
		AttackerCards: []int{0},
		DefenderCards: []int{0},
	})
	if err != nil {
		t.Fatalf("挑戰錯誤: %v", err)
	}
	if outcome2.WinnerID == nil || *outcome2.WinnerID != attacker.ID {
		t.Fatalf("攻擊僵屍應成功")
	}
	if zombie.Alive {
		t.Fatalf("僵屍應被獵槍淘汰")
	}
}
