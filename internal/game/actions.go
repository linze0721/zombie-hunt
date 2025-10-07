package game

import (
	"fmt"
	"sort"
)

// ChallengeOptions 用於描述一次挑戰行動的輸入
type ChallengeOptions struct {
	AttackerID    int
	DefenderID    int
	AttackerCards []int // 進攻方選擇的手牌索引
	DefenderCards []int // 防守方選擇的手牌索引；若無可出牌可為空
}

// ChallengeOutcome 描述挑戰結果
type ChallengeOutcome struct {
	AttackerID int
	DefenderID int

	AttackerCards []Card
	DefenderCards []Card

	WinnerID *int
	LoserID  *int

	ForcedReveal bool
	Infection    bool

	Eliminated       []int
	ConvertedToHuman []int
	StolenCard       *Card

	Notes []string
}

type playedSet struct {
	cards     []Card
	isNumeric bool
	suit      Suit
	total     int
	kind      CardKind
}

func (g *Game) playerByID(id int) (*Player, error) {
	if id < 0 || id >= len(g.Players) {
		return nil, fmt.Errorf("找不到編號為 %d 的玩家", id)
	}
	player := g.Players[id]
	if !player.Alive {
		return nil, fmt.Errorf("玩家 %s 已被淘汰", player.Name)
	}
	return player, nil
}

func (g *Game) eliminatePlayer(p *Player, reason string, outcomeNotes *[]string, eliminated *[]int) {
	if !p.Alive {
		return
	}
	p.Alive = false
	*eliminated = append(*eliminated, p.ID)
	note := fmt.Sprintf("玩家 %s 被淘汰（%s）", p.Name, reason)
	g.addLog(note)
	if outcomeNotes != nil {
		*outcomeNotes = append(*outcomeNotes, note)
	}
}

// Challenge 解析並結算一次挑戰
func (g *Game) Challenge(opts ChallengeOptions) (*ChallengeOutcome, error) {
	if opts.AttackerID == opts.DefenderID {
		return nil, fmt.Errorf("挑戰目標不可為自己")
	}

	attacker, err := g.playerByID(opts.AttackerID)
	if err != nil {
		return nil, err
	}
	defender, err := g.playerByID(opts.DefenderID)
	if err != nil {
		return nil, err
	}

	if len(opts.AttackerCards) == 0 {
		return nil, fmt.Errorf("攻擊者必須選擇要出的牌")
	}

	outcome := &ChallengeOutcome{AttackerID: opts.AttackerID, DefenderID: opts.DefenderID}

	attackerCards, err := removeCardsByIndices(attacker, opts.AttackerCards)
	if err != nil {
		return nil, err
	}
	outcome.AttackerCards = append(outcome.AttackerCards, attackerCards...)

	attackSet, err := analyzePlayedSet(attackerCards, true)
	if err != nil {
		// 將牌還給攻擊者後回報錯誤
		attacker.Hand = append(attacker.Hand, attackerCards...)
		sortHand(attacker)
		return nil, err
	}

	defenderCards, err := removeCardsByIndices(defender, opts.DefenderCards)
	if err != nil {
		attacker.Hand = append(attacker.Hand, attackerCards...)
		sortHand(attacker)
		return nil, err
	}
	outcome.DefenderCards = append(outcome.DefenderCards, defenderCards...)

	defenseSet, err := analyzeDefenseSet(defenderCards)
	if err != nil {
		// 還原雙方手牌
		attacker.Hand = append(attacker.Hand, attackerCards...)
		defender.Hand = append(defender.Hand, defenderCards...)
		sortHand(attacker)
		sortHand(defender)
		return nil, err
	}

	result := g.resolveChallenge(attacker, defender, attackSet, defenseSet, outcome)

	if result.err != nil {
		// 發生錯誤時還原手牌
		attacker.Hand = append(attacker.Hand, attackerCards...)
		defender.Hand = append(defender.Hand, defenderCards...)
		sortHand(attacker)
		sortHand(defender)
		return nil, result.err
	}

	winner := result.winner
	loser := result.loser

	// 勝者取回本次所有出牌
	if winner != nil {
		for _, c := range outcome.AttackerCards {
			winner.AddCard(c)
		}
		for _, c := range outcome.DefenderCards {
			winner.AddCard(c)
		}
		sortHand(winner)
	}

	// 失敗方失去本次出牌，不做處理（已在 remove cards 時移除）

	if winner != nil && loser != nil {
		gid := winner.ID
		lid := loser.ID
		outcome.WinnerID = &gid
		outcome.LoserID = &lid
		g.addLog(fmt.Sprintf("挑戰結果：%s 勝，%s 負", winner.Name, loser.Name))

		// 勝者抽取一張對手的數字牌
		if stolen := stealRandomNumericCard(g, winner, loser); stolen != nil {
			outcome.StolenCard = stolen
			note := fmt.Sprintf("%s 從 %s 抽走 %s", winner.Name, loser.Name, stolen.String())
			outcome.Notes = append(outcome.Notes, note)
			g.addLog(note)
		}

		// 如果失敗者手牌耗盡則淘汰
		if loser.HandSize() == 0 && loser.Alive {
			g.eliminatePlayer(loser, "手牌耗盡", &outcome.Notes, &outcome.Eliminated)
		}
	}

	return outcome, nil
}

// resolveChallenge 根據出牌決定勝負
func (g *Game) resolveChallenge(attacker, defender *Player, attackSet, defenseSet playedSet, outcome *ChallengeOutcome) challengeResult {
	// 僵屍牌攻擊處理
	if attackSet.kind == CardKindZombie {
		if defenseSet.kind == CardKindVaccine {
			// 疫苗反制
			convertToHuman(g, attacker, outcome)
			declareWinner(defender, attacker, outcome)
			note := fmt.Sprintf("%s 使用疫苗反制，%s 被強制轉為人類", defender.Name, attacker.Name)
			outcome.Notes = append(outcome.Notes, note)
			g.addLog(note)
			return challengeResult{winner: defender, loser: attacker}
		}

		// 僵屍必勝
		declareWinner(attacker, defender, outcome)
		outcome.Infection = true
		defender.SetIdentity(IdentityZombie)
		infectionNote := fmt.Sprintf("僵屍牌出擊！%s 將 %s 感染為僵屍", attacker.Name, defender.Name)
		outcome.Notes = append(outcome.Notes, infectionNote)
		g.addLog(infectionNote)

		if defender.CountKind(CardKindZombie) == 0 {
			defender.AddCard(Card{Kind: CardKindZombie})
			sortHand(defender)
			grantNote := fmt.Sprintf("%s 成為僵屍並獲得一張僵屍牌", defender.Name)
			outcome.Notes = append(outcome.Notes, grantNote)
			g.addLog(grantNote)
		}
		return challengeResult{winner: attacker, loser: defender}
	}

	// 獵槍攻擊
	if attackSet.kind == CardKindShotgun {
		if !defender.IsZombie() {
			// 對人類無效，攻擊失敗
			failNote := fmt.Sprintf("獵槍失效！%s 並非僵屍，%s 攻擊落空", defender.Name, attacker.Name)
			outcome.Notes = append(outcome.Notes, failNote)
			g.addLog(failNote)
			declareWinner(defender, attacker, outcome)
			return challengeResult{winner: defender, loser: attacker}
		}
		// 命中僵屍，直接淘汰
		hitNote := fmt.Sprintf("獵槍命中！%s 射殺僵屍 %s", attacker.Name, defender.Name)
		outcome.Notes = append(outcome.Notes, hitNote)
		g.addLog(hitNote)
		g.eliminatePlayer(defender, "遭獵槍射擊", &outcome.Notes, &outcome.Eliminated)
		declareWinner(attacker, defender, outcome)
		return challengeResult{winner: attacker, loser: defender}
	}

	// 攻方為數字牌，處理防守
	if defenseSet.kind == CardKindVaccine {
		convertToHuman(g, attacker, outcome)
		note := fmt.Sprintf("%s 使用疫苗逆轉，%s 被迫轉回人類", defender.Name, attacker.Name)
		outcome.Notes = append(outcome.Notes, note)
		g.addLog(note)
		declareWinner(defender, attacker, outcome)
		return challengeResult{winner: defender, loser: attacker}
	}

	// 防守方無牌可出 -> 強制揭露
	if len(defenseSet.cards) == 0 {
		outcome.ForcedReveal = true
		note := fmt.Sprintf("防守方 %s 無同花色牌可出", defender.Name)
		outcome.Notes = append(outcome.Notes, note)
		g.addLog(note)
		declareWinner(attacker, defender, outcome)
		return challengeResult{winner: attacker, loser: defender}
	}

	if !defenseSet.isNumeric {
		return challengeResult{err: fmt.Errorf("防守方的出牌無效")}
	}

	if attackSet.suit != defenseSet.suit {
		return challengeResult{err: fmt.Errorf("防守方必須出相同花色")}
	}

	switch {
	case attackSet.total > defenseSet.total:
		note := fmt.Sprintf("%s 以 %d 點擊敗 %s 的 %d 點", attacker.Name, attackSet.total, defender.Name, defenseSet.total)
		outcome.Notes = append(outcome.Notes, note)
		g.addLog(note)
		declareWinner(attacker, defender, outcome)
		return challengeResult{winner: attacker, loser: defender}
	case defenseSet.total > attackSet.total:
		note := fmt.Sprintf("%s 以 %d 點守下 %s 的 %d 點", defender.Name, defenseSet.total, attacker.Name, attackSet.total)
		outcome.Notes = append(outcome.Notes, note)
		g.addLog(note)
		declareWinner(defender, attacker, outcome)
		return challengeResult{winner: defender, loser: attacker}
	default:
		note := fmt.Sprintf("雙方 %s 花色打成 %d 點平手", attackSet.suit, attackSet.total)
		outcome.Notes = append(outcome.Notes, note)
		g.addLog(note)
		// 平手：兩邊收回各自牌
		for _, c := range outcome.AttackerCards {
			attacker.AddCard(c)
		}
		for _, c := range outcome.DefenderCards {
			defender.AddCard(c)
		}
		sortHand(attacker)
		sortHand(defender)
		outcome.AttackerCards = nil
		outcome.DefenderCards = nil
		return challengeResult{}
	}
}

type challengeResult struct {
	winner *Player
	loser  *Player
	err    error
}

func convertToHuman(g *Game, player *Player, outcome *ChallengeOutcome) {
	if player.IsZombie() {
		player.SetIdentity(IdentityHuman)
		outcome.ConvertedToHuman = append(outcome.ConvertedToHuman, player.ID)
	}
}

func declareWinner(winner, loser *Player, outcome *ChallengeOutcome) {
	if winner == nil || loser == nil {
		return
	}
	wid := winner.ID
	lid := loser.ID
	outcome.WinnerID = &wid
	outcome.LoserID = &lid
}

func sortHand(p *Player) {
	sort.SliceStable(p.Hand, func(i, j int) bool {
		if p.Hand[i].Kind != p.Hand[j].Kind {
			return p.Hand[i].Kind < p.Hand[j].Kind
		}
		if p.Hand[i].Suit != p.Hand[j].Suit {
			return p.Hand[i].Suit < p.Hand[j].Suit
		}
		return p.Hand[i].Value < p.Hand[j].Value
	})
}

func removeCardsByIndices(p *Player, indices []int) ([]Card, error) {
	if len(indices) == 0 {
		return nil, nil
	}
	return p.RemoveCards(indices)
}

func analyzePlayedSet(cards []Card, attacker bool) (playedSet, error) {
	if len(cards) == 0 {
		return playedSet{}, fmt.Errorf("必須選擇至少一張牌")
	}

	first := cards[0]
	if first.Kind == CardKindNumber {
		if len(cards) > 5 {
			return playedSet{}, fmt.Errorf("一次最多只能打出五張數字牌")
		}
		suit := first.Suit
		total := 0
		for _, c := range cards {
			if c.Kind != CardKindNumber {
				return playedSet{}, fmt.Errorf("數字牌不可與特殊牌混出")
			}
			if c.Suit != suit {
				return playedSet{}, fmt.Errorf("多張數字牌必須為同一花色")
			}
			total += c.Value
		}
		return playedSet{cards: cards, isNumeric: true, suit: suit, total: total, kind: CardKindNumber}, nil
	}

	if len(cards) != 1 {
		return playedSet{}, fmt.Errorf("特殊牌一次只能出一張")
	}

	if !attacker && first.Kind == CardKindShotgun {
		return playedSet{}, fmt.Errorf("防守方不可使用獵槍")
	}

	if !attacker && first.Kind == CardKindZombie {
		return playedSet{}, fmt.Errorf("防守方不可使用僵屍牌")
	}

	if attacker && first.Kind == CardKindVaccine {
		return playedSet{}, fmt.Errorf("攻擊方不可使用疫苗")
	}

	return playedSet{cards: cards, kind: first.Kind}, nil
}

func analyzeDefenseSet(cards []Card) (playedSet, error) {
	if len(cards) == 0 {
		return playedSet{}, nil
	}
	return analyzePlayedSet(cards, false)
}

func stealRandomNumericCard(g *Game, winner, loser *Player) *Card {
	numericIndices := make([]int, 0)
	for idx, c := range loser.Hand {
		if c.Kind == CardKindNumber {
			numericIndices = append(numericIndices, idx)
		}
	}
	if len(numericIndices) == 0 {
		return nil
	}
	choice := numericIndices[g.rng.Intn(len(numericIndices))]
	card, err := loser.RemoveCardAt(choice)
	if err != nil {
		return nil
	}
	winner.AddCard(card)
	sortHand(winner)
	return &card
}
