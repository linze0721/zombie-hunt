package game

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

var (
	// ErrInvalidPlayerCount 表示玩家人數不正確
	ErrInvalidPlayerCount = errors.New("玩家人數必須為 8 人")
)

const (
	initialHandSize            = 15
	maxGameRounds              = 12
	numericDeckCopies          = 3
	totalVaccineCards          = 2
	initialZombieCardPerPlayer = 1
	initialShotgunPerPlayer    = 1
)

// NewGame 建立並初始化一場遊戲
func NewGame(names []string, seed int64) (*Game, error) {
	if len(names) != 8 {
		return nil, ErrInvalidPlayerCount
	}

	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	rng := rand.New(rand.NewSource(seed))

	g := &Game{
		Players:   make([]*Player, len(names)),
		MaxRounds: maxGameRounds,
		rng:       rng,
	}
	identities := buildIdentityDeck()
	shuffleIdentities(rng, identities)

	for i, name := range names {
		identity := identities[i]
		player := &Player{
			ID:               i,
			Name:             name,
			originalIdentity: identity,
			currentIdentity:  identity,
			Alive:            true,
			Hand:             make([]Card, 0, initialHandSize+8),
		}
		g.Players[i] = player
	}

	g.cardDeck = buildNumberDeck(numericDeckCopies)
	shuffleCards(rng, g.cardDeck)

	// 發送數字牌
	for i := 0; i < initialHandSize; i++ {
		for _, p := range g.Players {
			card, ok := g.drawCard()
			if !ok {
				return nil, fmt.Errorf("初始牌庫不足: 回合 %d 玩家 %s", i, p.Name)
			}
			p.AddCard(card)
		}
	}

	distributeSpecialCards(g)
	equipInitialZombieCards(g)

	return g, nil
}

func buildIdentityDeck() []Identity {
	deck := make([]Identity, 0, 8)
	for i := 0; i < 6; i++ {
		deck = append(deck, IdentityHuman)
	}
	for i := 0; i < 2; i++ {
		deck = append(deck, IdentityZombie)
	}
	return deck
}

func shuffleIdentities(rng *rand.Rand, identities []Identity) {
	rng.Shuffle(len(identities), func(i, j int) {
		identities[i], identities[j] = identities[j], identities[i]
	})
}

func buildNumberDeck(copies int) []Card {
	deck := make([]Card, 0, len(suits)*13*copies)
	for copyIndex := 0; copyIndex < copies; copyIndex++ {
		for _, suit := range suits {
			for value := 1; value <= 13; value++ {
				deck = append(deck, Card{Kind: CardKindNumber, Suit: suit, Value: value})
			}
		}
	}
	return deck
}

func shuffleCards(rng *rand.Rand, deck []Card) {
	rng.Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})
}

func (g *Game) drawCard() (Card, bool) {
	if len(g.cardDeck) == 0 {
		return Card{}, false
	}
	card := g.cardDeck[0]
	g.cardDeck = g.cardDeck[1:]
	return card, true
}

func (g *Game) discardCard(card Card) {
	g.cardDiscarded = append(g.cardDiscarded, card)
}

func distributeSpecialCards(g *Game) {
	for _, p := range g.Players {
		need := initialShotgunPerPlayer - p.CountKind(CardKindShotgun)
		for i := 0; i < need; i++ {
			p.AddCard(Card{Kind: CardKindShotgun})
		}
	}

	indices := make([]int, len(g.Players))
	for i := range indices {
		indices[i] = i
	}
	g.rng.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	// 分發疫苗
	for i := 0; i < totalVaccineCards && i < len(indices); i++ {
		g.Players[indices[i]].AddCard(Card{Kind: CardKindVaccine})
	}
}

func equipInitialZombieCards(g *Game) {
	for _, p := range g.Players {
		if p.IsZombie() {
			for i := 0; i < initialZombieCardPerPlayer; i++ {
				p.AddCard(Card{Kind: CardKindZombie})
			}
		}
	}
}
