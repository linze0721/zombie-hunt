package game

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Suit 表示數字牌的花色
type Suit string

const (
	SuitSpade   Suit = "♠"
	SuitHeart   Suit = "♥"
	SuitClub    Suit = "♣"
	SuitDiamond Suit = "♦"
)

var suits = []Suit{SuitSpade, SuitHeart, SuitClub, SuitDiamond}

// Identity 表示玩家身份
type Identity int

const (
	IdentityHuman Identity = iota
	IdentityZombie
)

func (i Identity) String() string {
	switch i {
	case IdentityHuman:
		return "人類"
	case IdentityZombie:
		return "僵屍"
	default:
		return "未知"
	}
}

func (i Identity) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// CardKind 描述牌面類型
type CardKind int

const (
	CardKindNumber CardKind = iota
	CardKindZombie
	CardKindShotgun
	CardKindVaccine
)

func (k CardKind) String() string {
	switch k {
	case CardKindNumber:
		return "數字牌"
	case CardKindZombie:
		return "僵屍牌"
	case CardKindShotgun:
		return "獵槍牌"
	case CardKindVaccine:
		return "疫苗牌"
	default:
		return "未知牌"
	}
}

// Card 表示手牌/特殊牌
type Card struct {
	Kind  CardKind `json:"kind"`
	Suit  Suit     `json:"suit,omitempty"`
	Value int      `json:"value,omitempty"`
}

func (c Card) String() string {
	switch c.Kind {
	case CardKindNumber:
		return fmt.Sprintf("%s%d", c.Suit, c.Value)
	case CardKindZombie:
		return "僵屍牌"
	case CardKindShotgun:
		return "獵槍牌"
	case CardKindVaccine:
		return "疫苗牌"
	default:
		return "未知牌"
	}
}

func (c Card) IsNumeric() bool {
	return c.Kind == CardKindNumber
}

// Player 表示一名玩家
type Player struct {
	ID               int
	Name             string
	originalIdentity Identity
	currentIdentity  Identity
	Alive            bool
	Hand             []Card
}

func (p *Player) Identity() Identity {
	return p.currentIdentity
}

func (p *Player) OriginalIdentity() Identity {
	return p.originalIdentity
}

func (p *Player) SetIdentity(identity Identity) {
	p.currentIdentity = identity
}

func (p *Player) IsHuman() bool {
	return p.currentIdentity == IdentityHuman
}

func (p *Player) IsZombie() bool {
	return p.currentIdentity == IdentityZombie
}

// AddCard 將牌加入手牌末端
func (p *Player) AddCard(card Card) {
	p.Hand = append(p.Hand, card)
}

// RemoveCardAt 移除指定索引的牌
func (p *Player) RemoveCardAt(index int) (Card, error) {
	if index < 0 || index >= len(p.Hand) {
		return Card{}, fmt.Errorf("玩家 %s 的手牌索引超出範圍", p.Name)
	}
	card := p.Hand[index]
	p.Hand = append(p.Hand[:index], p.Hand[index+1:]...)
	return card, nil
}

// RemoveCards 依索引陣列移除多張牌（索引將自動由大到小排序）
func (p *Player) RemoveCards(indices []int) ([]Card, error) {
	if len(indices) == 0 {
		return nil, nil
	}
	sorted := append([]int(nil), indices...)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))

	removed := make([]Card, 0, len(sorted))
	seen := make(map[int]struct{})
	for _, idx := range sorted {
		if _, duplicated := seen[idx]; duplicated {
			return nil, fmt.Errorf("重複的牌索引 %d", idx)
		}
		seen[idx] = struct{}{}
		card, err := p.RemoveCardAt(idx)
		if err != nil {
			return nil, err
		}
		removed = append(removed, card)
	}

	// 移除後順序與索引由大到小，為了回傳順序與被選擇順序一致，反轉一次
	for i, j := 0, len(removed)-1; i < j; i, j = i+1, j-1 {
		removed[i], removed[j] = removed[j], removed[i]
	}
	return removed, nil
}

// HandSize 回傳手牌數量
func (p *Player) HandSize() int {
	return len(p.Hand)
}

// HasSuit 檢查是否持有指定花色數字牌
func (p *Player) HasSuit(suit Suit) bool {
	for _, c := range p.Hand {
		if c.Kind == CardKindNumber && c.Suit == suit {
			return true
		}
	}
	return false
}

// CountKind 回傳特定牌型數量
func (p *Player) CountKind(kind CardKind) int {
	count := 0
	for _, c := range p.Hand {
		if c.Kind == kind {
			count++
		}
	}
	return count
}
