package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"zombierush/internal/game"
)

const (
	RoomStatusLobby    = "lobby"
	RoomStatusRunning  = "running"
	RoomStatusFinished = "finished"
)

// Room 負責管理單一遊戲房間的生命週期
type Room struct {
	id       string
	name     string
	hub      *Hub
	mu       sync.Mutex
	status   string
	seats    []*Seat
	capacity int
	hostSeat int
	game     *game.Game

	currentTurn  int
	currentRound int

	pendingChallenge *pendingChallenge

	rng *rand.Rand
}

// Seat 表示一個座位資訊
type Seat struct {
	Index  int
	Name   string
	Token  string
	Client *Client
	Bot    *BotPlayer
	Player *game.Player
}

func (s *Seat) isFilled() bool {
	return s.Client != nil || s.Bot != nil
}

func (s *Seat) displayBaseName() string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("座位%d", s.Index)
}

func (s *Seat) displayName() string {
	if s.Client != nil {
		return s.Client.name
	}
	if s.Bot != nil {
		return s.Bot.Name
	}
	return s.displayBaseName()
}

func (r *Room) firstEmptySeatLocked() *Seat {
	for _, seat := range r.seats {
		if !seat.isFilled() {
			return seat
		}
	}
	return nil
}

func (r *Room) assignHostLocked() {
	r.hostSeat = -1
	for _, seat := range r.seats {
		if seat.Client != nil {
			r.hostSeat = seat.Index
			return
		}
	}
	for _, seat := range r.seats {
		if seat.Bot != nil {
			r.hostSeat = seat.Index
			return
		}
	}
}

func (r *Room) isHost(c *Client) bool {
	if c == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return c.seatIndex == r.hostSeat
}

func (r *Room) addBot(name string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != RoomStatusLobby {
		return -1, fmt.Errorf("僅能在待機狀態新增機器人")
	}
	seat := r.firstEmptySeatLocked()
	if seat == nil {
		return -1, fmt.Errorf("房間已滿")
	}
	if name == "" {
		name = fmt.Sprintf("機器人%d", seat.Index+1)
	}
	seat.Bot = &BotPlayer{SeatIndex: seat.Index, Name: name, KnownZombies: make(map[int]struct{})}
	seat.Name = name
	seat.Token = fmt.Sprintf("bot-%d-%d", seat.Index, r.rng.Int63())
	seat.Client = nil
	if r.hostSeat == -1 {
		r.hostSeat = seat.Index
	}
	r.broadcastLobbyLocked()
	r.broadcastPublicStateLocked()
	return seat.Index, nil
}

func (r *Room) removeBot(seatIdx int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if seatIdx < 0 || seatIdx >= len(r.seats) {
		return fmt.Errorf("無效座位")
	}
	seat := r.seats[seatIdx]
	if seat.Bot == nil {
		return fmt.Errorf("座位 %d 沒有機器人", seatIdx)
	}
	seat.Bot = nil
	if seat.Client == nil {
		seat.Name = ""
		seat.Token = ""
	}
	if r.hostSeat == seatIdx {
		r.assignHostLocked()
	}
	r.broadcastLobbyLocked()
	r.broadcastPublicStateLocked()
	return nil
}

func (r *Room) summary() RoomSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hostSeat < 0 || r.hostSeat >= len(r.seats) {
		r.assignHostLocked()
	}
	players := 0
	hostName := ""
	if r.hostSeat >= 0 && r.hostSeat < len(r.seats) {
		hostName = r.seats[r.hostSeat].displayName()
	}
	for _, seat := range r.seats {
		if seat.isFilled() {
			players++
		}
	}
	return RoomSummary{
		RoomID:   r.id,
		Name:     r.name,
		Status:   r.status,
		Players:  players,
		Capacity: r.capacity,
		Host:     hostName,
	}
}

func (r *Room) isEmpty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, seat := range r.seats {
		if seat.isFilled() {
			return false
		}
	}
	return true
}

// pendingChallenge 暫存待防守者回應的挑戰
type pendingChallenge struct {
	AttackerSeat  int
	DefenderSeat  int
	AttackerCards []int
	AttackKind    game.CardKind
	AttackSuit    game.Suit
}

// NewRoom 建立房間
func NewRoom(id, name string, capacity int, hub *Hub) *Room {
	if capacity <= 0 {
		capacity = 8
	}
	seats := make([]*Seat, capacity)
	for i := 0; i < capacity; i++ {
		seats[i] = &Seat{Index: i}
	}
	return &Room{
		id:       id,
		name:     name,
		hub:      hub,
		status:   RoomStatusLobby,
		seats:    seats,
		capacity: capacity,
		hostSeat: -1,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Join 將玩家加入座位
func (r *Room) Join(c *Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 嘗試以 token 找到原座位進行重連
	if c.token != "" {
		for _, seat := range r.seats {
			if seat.Token == c.token {
				if seat.Client != nil {
					return fmt.Errorf("該座位已有連線")
				}
				seat.Client = c
				seat.Bot = nil
				if seat.Name != "" {
					c.name = seat.Name
				} else {
					seat.Name = c.name
				}
				if seat.Token == "" {
					seat.Token = fmt.Sprintf("seat-%d-%d", seat.Index, r.rng.Int63())
				}
				c.token = seat.Token
				c.room = r
				c.seatIndex = seat.Index
				c.inLobby = false
				r.assignHostLocked()
				r.sendWelcomeLocked(c)
				r.broadcastLobbyLocked()
				r.broadcastPublicStateLocked()
				if r.game != nil {
					r.sendPrivateStateLocked(seat.Index)
				}
				return nil
			}
		}
	}

	if r.status != RoomStatusLobby {
		return fmt.Errorf("遊戲已開始或結束，無法加入")
	}

	for _, seat := range r.seats {
		if !seat.isFilled() {
			seat.Client = c
			seat.Bot = nil
			seat.Name = c.name
			token := c.token
			if token == "" {
				token = fmt.Sprintf("seat-%d-%d", seat.Index, r.rng.Int63())
			}
			seat.Token = token
			c.token = token
			c.room = r
			c.seatIndex = seat.Index
			c.inLobby = false
			r.assignHostLocked()
			r.sendWelcomeLocked(c)
			r.broadcastLobbyLocked()
			r.broadcastPublicStateLocked()
			return nil
		}
	}

	return fmt.Errorf("房間已滿")
}

func (r *Room) onClientLeft(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c.seatIndex >= 0 && c.seatIndex < len(r.seats) {
		seat := r.seats[c.seatIndex]
		if seat.Client == c {
			seat.Client = nil
			if seat.Name == "" {
				seat.Name = c.name
			}
			if r.status == RoomStatusRunning {
				if seat.Bot == nil {
					seat.Bot = &BotPlayer{SeatIndex: seat.Index, Name: fmt.Sprintf("%s (AI)", seat.displayBaseName()), KnownZombies: make(map[int]struct{})}
				}
				if r.pendingChallenge == nil && r.currentTurn == seat.Index {
					go r.executeBotTurn(seat.Bot)
				}
			} else {
				seat.Bot = nil
				seat.Name = ""
				seat.Token = ""
			}
			if seat.Index == r.hostSeat {
				r.assignHostLocked()
			}
		}
	}

	r.broadcastLobbyLocked()
	if r.game != nil {
		r.broadcastPublicStateLocked()
	}
}

func (r *Room) sendWelcomeLocked(c *Client) {
	var token string
	if c.seatIndex >= 0 && c.seatIndex < len(r.seats) {
		if seat := r.seats[c.seatIndex]; seat != nil {
			token = seat.Token
		}
	}
	payload := ServerMessage{Type: "welcome", Payload: map[string]interface{}{
		"roomId":      r.id,
		"roomName":    r.name,
		"seatIndex":   c.seatIndex,
		"status":      r.status,
		"token":       token,
		"capacity":    r.capacity,
		"displayName": c.name,
		"account":     c.account,
		"userId":      c.userID,
	}}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		go c.close()
	}
}

func (r *Room) broadcastLobbyLocked() {
	msg := ServerMessage{
		Type: "lobby_state",
		Payload: PublicRoomStatePayload{
			RoomID:   r.id,
			RoomName: r.name,
			Status:   r.status,
			Seats:    r.buildSeatSnapshotsLocked(),
			HostSeat: r.hostSeat,
		},
	}
	r.broadcastLocked(msg)
}

func (r *Room) buildSeatSnapshotsLocked() []SeatPublicSnapshot {
	seats := make([]SeatPublicSnapshot, 0, len(r.seats))
	for _, seat := range r.seats {
		snapshot := SeatPublicSnapshot{
			Index:  seat.Index,
			Name:   seat.displayName(),
			Filled: seat.isFilled(),
			IsBot:  seat.Bot != nil,
			IsHost: seat.Index == r.hostSeat,
		}
		if r.game != nil && seat.Player != nil {
			alive := seat.Player.Alive
			snapshot.Alive = &alive
			if !alive {
				hand := 0
				snapshot.Hand = &hand
			}
		}
		seats = append(seats, snapshot)
	}
	return seats
}

func (r *Room) broadcastLocked(msg ServerMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	recipients := r.collectClientsLocked()
	for _, c := range recipients {
		select {
		case c.send <- payload:
		default:
			go c.close()
		}
	}
}

func (r *Room) collectClientsLocked() []*Client {
	clients := make([]*Client, 0, len(r.seats))
	for _, seat := range r.seats {
		if seat.Client != nil {
			clients = append(clients, seat.Client)
		}
	}
	return clients
}

func (r *Room) broadcastPublicStateLocked() {
	payload := PublicRoomStatePayload{
		RoomID:   r.id,
		RoomName: r.name,
		Status:   r.status,
		Seats:    r.buildSeatSnapshotsLocked(),
	}
	if r.game != nil {
		payload.PublicGame = &PublicGamePayload{
			Snapshot:     r.game.BuildPublicSnapshot(),
			CurrentTurn:  r.currentTurn,
			CurrentRound: r.currentRound,
		}
		if r.pendingChallenge != nil {
			payload.PublicGame.PendingType = "challenge"
		}
	}
	payload.HostSeat = r.hostSeat
	r.broadcastLocked(ServerMessage{Type: "public_state", Payload: payload})
}

func (r *Room) sendPrivateStateLocked(seatIdx int) {
	if seatIdx < 0 || seatIdx >= len(r.seats) {
		return
	}
	seat := r.seats[seatIdx]
	if seat.Client == nil || r.game == nil {
		return
	}
	snapshot, err := r.game.BuildPrivateSnapshot(seatIdx)
	if err != nil {
		r.sendErrorLocked(seat.Client, err)
		return
	}
	payload, err := json.Marshal(ServerMessage{Type: "private_state", Payload: PrivateStatePayload{Snapshot: snapshot}})
	if err != nil {
		return
	}
	select {
	case seat.Client.send <- payload:
	default:
		go seat.Client.close()
	}
}

func (r *Room) sendErrorLocked(c *Client, err error) {
	payload, encodeErr := json.Marshal(ServerMessage{Type: "error", Payload: ErrorPayload{Message: err.Error()}})
	if encodeErr != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
		go c.close()
	}
}

func (r *Room) sendPrivateInfoLocked(c *Client, message string) {
	payload, encodeErr := json.Marshal(ServerMessage{Type: "private_info", Payload: PrivateInfoPayload{Message: message}})
	if encodeErr != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
		go c.close()
	}
}

func (r *Room) sendPrivateLogLocked(c *Client, message string) {
	payload, encodeErr := json.Marshal(ServerMessage{Type: "log", Payload: LogPayload{Message: message}})
	if encodeErr != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
		go c.close()
	}
}

// StartGame 由主持端觸發正式開局
func (r *Room) StartGame() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status != RoomStatusLobby {
		return fmt.Errorf("遊戲已在進行或結束")
	}

	names := make([]string, len(r.seats))
	for i, seat := range r.seats {
		if seat.Client != nil {
			names[i] = seat.Client.name
		} else {
			botName := fmt.Sprintf("機器人%d", i+1)
			seat.Bot = &BotPlayer{SeatIndex: i, Name: botName, KnownZombies: make(map[int]struct{})}
			seat.Name = botName
			names[i] = botName
		}
	}

	newGame, err := game.NewGame(names, r.rng.Int63())
	if err != nil {
		return err
	}
	r.game = newGame
	for i, seat := range r.seats {
		seat.Player = r.game.Players[i]
	}

	r.status = RoomStatusRunning
	r.currentRound = 1
	r.game.Round = 1
	r.currentTurn = r.findNextAlive(-1)

	r.broadcastLobbyLocked()
	r.broadcastPublicStateLocked()
	for _, seat := range r.seats {
		if seat.Client != nil {
			r.sendPrivateStateLocked(seat.Index)
		}
	}

	r.notifyTurnLocked()

	return nil
}

func (r *Room) notifyTurnLocked() {
	if r.currentTurn < 0 || r.currentTurn >= len(r.seats) {
		return
	}
	seat := r.seats[r.currentTurn]
	msg := ServerMessage{Type: "turn_start", Payload: TurnPromptPayload{PlayerID: seat.Index, Name: seat.displayName()}}
	r.broadcastLocked(msg)

	if seat.Client != nil {
		r.sendPrivateStateLocked(seat.Index)
	}

	if seat.Bot != nil {
		go r.executeBotTurn(seat.Bot)
	}
}

func (r *Room) findNextAlive(start int) int {
	total := len(r.seats)
	for offset := 1; offset <= total; offset++ {
		idx := (start + offset) % total
		if r.game != nil && r.seats[idx].Player != nil && r.seats[idx].Player.Alive {
			return idx
		}
	}
	return -1
}

// handleChallenge 由當前玩家提出挑戰
func (r *Room) handleChallenge(attacker *Client, payload ChallengePayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensurePlayerTurnLocked(attacker); err != nil {
		return err
	}
	if r.pendingChallenge != nil {
		return fmt.Errorf("目前有待處理的挑戰")
	}

	attackerSeat := r.seats[attacker.seatIndex]
	if attackerSeat.Player == nil || !attackerSeat.Player.Alive {
		return fmt.Errorf("攻擊者狀態異常")
	}

	defenderSeat := r.getSeatLocked(payload.TargetID)
	if defenderSeat == nil || defenderSeat.Player == nil || !defenderSeat.Player.Alive {
		return fmt.Errorf("目標不可挑戰")
	}
	if defenderSeat.Index == attackerSeat.Index {
		return fmt.Errorf("不可挑戰自己")
	}

	if len(payload.Cards) == 0 {
		return fmt.Errorf("必須選擇至少一張牌")
	}

	attackerCards, err := normalizeCardSelection(payload.Cards, attackerSeat.Player.HandSize())
	if err != nil {
		return err
	}

	firstCard := attackerSeat.Player.Hand[attackerCards[0]]
	switch firstCard.Kind {
	case game.CardKindNumber:
		suit, err := ensureNumericSelection(attackerSeat.Player, attackerCards)
		if err != nil {
			return err
		}
		if len(attackerCards) > 5 {
			return fmt.Errorf("一次最多只能出五張數字牌")
		}

		if defenderSeat.Player.HasSuit(suit) && defenderSeat.Player.Alive {
			if defenderSeat.Bot != nil {
				defense := selectBotDefenseIndices(defenderSeat.Player, suit, len(attackerCards))
				return r.resolveChallengeLocked(attackerSeat.Index, defenderSeat.Index, attackerCards, defense)
			}

			r.pendingChallenge = &pendingChallenge{
				AttackerSeat:  attackerSeat.Index,
				DefenderSeat:  defenderSeat.Index,
				AttackerCards: attackerCards,
				AttackKind:    game.CardKindNumber,
				AttackSuit:    suit,
			}

			r.sendDefensePromptLocked(
				defenderSeat,
				collectSuitOptions(defenderSeat.Player, suit),
				buildCardViewsFromIndices(attackerSeat.Player, attackerCards),
				&suit,
			)
			r.broadcastPublicStateLocked()
			return nil
		}

		return r.resolveChallengeLocked(attackerSeat.Index, defenderSeat.Index, attackerCards, nil)

	case game.CardKindZombie:
		if !attackerSeat.Player.IsZombie() {
			return fmt.Errorf("僵屍牌僅能由僵屍使用")
		}
		return r.resolveChallengeLocked(attackerSeat.Index, defenderSeat.Index, attackerCards, nil)

	case game.CardKindShotgun:
		return r.resolveChallengeLocked(attackerSeat.Index, defenderSeat.Index, attackerCards, nil)

	case game.CardKindVaccine:
		return fmt.Errorf("疫苗僅能在防守時使用")

	default:
		return fmt.Errorf("未知牌型")
	}
}

func (r *Room) sendDefensePromptLocked(seat *Seat, options []game.CardView, attackViews []game.CardView, suit *game.Suit) {
	if seat.Client == nil {
		return
	}
	payload := DefensePromptPayload{
		AttackerID:    r.pendingChallenge.AttackerSeat,
		AttackerName:  r.seats[r.pendingChallenge.AttackerSeat].displayName(),
		AttackCards:   attackViews,
		MaxSelectable: 5,
		Options:       options,
	}
	if suit != nil {
		payload.Suit = suit
	}
	msg := ServerMessage{Type: "defense_prompt", Payload: payload}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case seat.Client.send <- data:
	default:
		go seat.Client.close()
	}
}

func (r *Room) resolveChallengeLocked(attackerSeat, defenderSeat int, attackerCards, defenderCards []int) error {
	outcome, err := r.game.Challenge(game.ChallengeOptions{
		AttackerID:    attackerSeat,
		DefenderID:    defenderSeat,
		AttackerCards: attackerCards,
		DefenderCards: defenderCards,
	})
	if err != nil {
		return err
	}

	r.pendingChallenge = nil

	r.broadcastPublicStateLocked()
	r.sendPrivateStateLocked(attackerSeat)
	r.sendPrivateStateLocked(defenderSeat)

	if outcome.Infection {
		r.registerInfectionKnowledge(attackerSeat, defenderSeat)
	}

	publicNotes := make([]string, 0, len(outcome.Notes))
	privateNotes := make([]string, 0)
	for _, note := range outcome.Notes {
		if outcome.Infection && (strings.Contains(note, "僵屍牌") || strings.Contains(note, "成為僵屍")) {
			privateNotes = append(privateNotes, note)
			continue
		}
		publicNotes = append(publicNotes, note)
	}
	for _, note := range publicNotes {
		r.broadcastLocked(ServerMessage{Type: "log", Payload: LogPayload{Message: note}})
	}
	if len(privateNotes) > 0 {
		if attackerSeatObj := r.getSeatLocked(attackerSeat); attackerSeatObj != nil && attackerSeatObj.Client != nil {
			for _, note := range privateNotes {
				r.sendPrivateLogLocked(attackerSeatObj.Client, note)
			}
		}
		if defenderSeatObj := r.getSeatLocked(defenderSeat); defenderSeatObj != nil && defenderSeatObj.Client != nil {
			for _, note := range privateNotes {
				r.sendPrivateLogLocked(defenderSeatObj.Client, note)
			}
		}
	}
	if outcome.StolenCard != nil {
		r.broadcastLocked(ServerMessage{Type: "log", Payload: LogPayload{Message: fmt.Sprintf("奪得手牌：%s", outcome.StolenCard)}})
	}
	if len(outcome.ConvertedToHuman) > 0 {
		for _, idx := range outcome.ConvertedToHuman {
			r.clearZombieKnowledge(idx)
			if seat := r.getSeatLocked(idx); seat != nil {
				r.sendPrivateStateLocked(idx)
			}
		}
	}
	if len(outcome.Eliminated) > 0 {
		for _, idx := range outcome.Eliminated {
			r.clearZombieKnowledge(idx)
		}
		r.handleEliminationLocked(outcome.Eliminated)
	}

	r.advanceTurnLocked()
	return nil
}

func (r *Room) handleDefenseResponse(defender *Client, payload DefensePayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pendingChallenge == nil {
		return fmt.Errorf("目前沒有待防禦的挑戰")
	}
	if defender.seatIndex != r.pendingChallenge.DefenderSeat {
		return fmt.Errorf("非指定防守者")
	}

	defense, err := normalizeDefenseSelection(payload.Cards, r.seats[defender.seatIndex].Player.HandSize())
	if err != nil {
		return err
	}

	if r.pendingChallenge.AttackKind == game.CardKindNumber && len(defense) > 0 {
		suit := r.pendingChallenge.AttackSuit
		if err := ensureDefenseMatchesSuit(r.seats[defender.seatIndex].Player, defense, suit); err != nil {
			return err
		}
	}

	return r.resolveChallengeLocked(r.pendingChallenge.AttackerSeat, r.pendingChallenge.DefenderSeat, r.pendingChallenge.AttackerCards, defense)
}

func normalizeCardSelection(indices []int, handSize int) ([]int, error) {
	return uniqueSortedIndices(indices, handSize)
}

func normalizeDefenseSelection(indices []int, handSize int) ([]int, error) {
	if len(indices) == 0 {
		return nil, nil
	}
	return uniqueSortedIndices(indices, handSize)
}

func ensureNumericSelection(player *game.Player, indices []int) (game.Suit, error) {
	if len(indices) == 0 {
		return "", fmt.Errorf("必須選擇至少一張數字牌")
	}
	suit := player.Hand[indices[0]].Suit
	for _, idx := range indices {
		card := player.Hand[idx]
		if card.Kind != game.CardKindNumber {
			return "", fmt.Errorf("數字牌不能與特殊牌混出")
		}
		if card.Suit != suit {
			return "", fmt.Errorf("多張數字牌必須為相同花色")
		}
	}
	return suit, nil
}

func ensureDefenseMatchesSuit(player *game.Player, indices []int, suit game.Suit) error {
	if len(indices) > 5 {
		return fmt.Errorf("一次最多只能防禦五張牌")
	}
	for _, idx := range indices {
		card := player.Hand[idx]
		if card.Kind != game.CardKindNumber || card.Suit != suit {
			return fmt.Errorf("防守牌需同花色 %s", suit)
		}
	}
	return nil
}

func uniqueSortedIndices(indices []int, handSize int) ([]int, error) {
	sorted := append([]int(nil), indices...)
	sort.Ints(sorted)
	for i, idx := range sorted {
		if idx < 0 || idx >= handSize {
			return nil, fmt.Errorf("手牌索引 %d 無效", idx)
		}
		if i > 0 && sorted[i-1] == idx {
			return nil, fmt.Errorf("手牌索引 %d 重複", idx)
		}
	}
	return sorted, nil
}

func buildCardViewsFromIndices(player *game.Player, indices []int) []game.CardView {
	views := make([]game.CardView, 0, len(indices))
	for _, idx := range indices {
		card := player.Hand[idx]
		views = append(views, game.CardView{Index: idx, Kind: card.Kind, Suit: card.Suit, Value: card.Value, Label: card.String()})
	}
	return views
}

func collectSuitOptions(player *game.Player, suit game.Suit) []game.CardView {
	options := make([]game.CardView, 0)
	for idx, card := range player.Hand {
		if card.Kind == game.CardKindNumber && card.Suit == suit {
			options = append(options, game.CardView{Index: idx, Kind: card.Kind, Suit: card.Suit, Value: card.Value, Label: card.String()})
		}
	}
	return options
}

func (r *Room) registerInfectionKnowledge(attackerSeat, targetSeat int) {
	seat := r.getSeatLocked(attackerSeat)
	if seat == nil || seat.Bot == nil {
		return
	}
	if seat.Bot.KnownZombies == nil {
		seat.Bot.KnownZombies = make(map[int]struct{})
	}
	seat.Bot.KnownZombies[targetSeat] = struct{}{}
}

func (r *Room) clearZombieKnowledge(targetSeat int) {
	for _, seat := range r.seats {
		if seat.Bot != nil && seat.Bot.KnownZombies != nil {
			delete(seat.Bot.KnownZombies, targetSeat)
		}
	}
}

func selectBotDefenseIndices(player *game.Player, suit game.Suit, limit int) []int {
	candidates := make([]int, 0)
	for idx, card := range player.Hand {
		if card.Kind == game.CardKindNumber && card.Suit == suit {
			candidates = append(candidates, idx)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return player.Hand[candidates[i]].Value > player.Hand[candidates[j]].Value
	})
	if limit > len(candidates) {
		limit = len(candidates)
	}
	return uniquePrefix(candidates, limit)
}

func uniquePrefix(indices []int, n int) []int {
	selected := make([]int, 0, n)
	seen := make(map[int]struct{})
	for _, idx := range indices {
		if len(selected) >= n {
			break
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		selected = append(selected, idx)
	}
	sort.Ints(selected)
	return selected
}

func (r *Room) aliveTargetsLocked(exclude int) []int {
	targets := make([]int, 0)
	for _, seat := range r.seats {
		if seat.Index == exclude {
			continue
		}
		if seat.Player != nil && seat.Player.Alive {
			targets = append(targets, seat.Index)
		}
	}
	return targets
}

func (r *Room) handleEliminationLocked(ids []int) {
	for _, id := range ids {
		seat := r.getSeatLocked(id)
		if seat != nil {
			note := fmt.Sprintf("玩家 %s 被淘汰", seat.displayName())
			r.broadcastLocked(ServerMessage{Type: "log", Payload: LogPayload{Message: note}})
		}
	}
}

func (r *Room) ensurePlayerTurnLocked(c *Client) error {
	if r.status != RoomStatusRunning {
		return fmt.Errorf("遊戲尚未開始")
	}
	if c.seatIndex != r.currentTurn {
		return fmt.Errorf("尚未輪到你行動")
	}
	return nil
}

func (r *Room) getSeatLocked(id int) *Seat {
	if id < 0 || id >= len(r.seats) {
		return nil
	}
	return r.seats[id]
}

func (r *Room) advanceTurnLocked() {
	if r.game == nil {
		return
	}

	humans, zombies := r.game.CountLivingIdentities()
	if humans == 0 || zombies == 0 {
		r.finishGameLocked()
		return
	}

	next := r.findNextAlive(r.currentTurn)
	if next == -1 {
		r.currentRound++
		if r.currentRound > r.game.MaxRounds {
			r.finishGameLocked()
			return
		}
		r.game.Round = r.currentRound
		next = r.findNextAlive(-1)
		if next == -1 {
			r.finishGameLocked()
			return
		}
	} else if next <= r.currentTurn {
		r.currentRound++
		if r.currentRound > r.game.MaxRounds {
			r.finishGameLocked()
			return
		}
		r.game.Round = r.currentRound
	}

	r.currentTurn = next
	r.broadcastPublicStateLocked()
	r.notifyTurnLocked()
}

func (r *Room) finishGameLocked() {
	if r.game == nil {
		return
	}

	r.status = RoomStatusFinished
	humanWins, _, _ := r.game.DetermineWinner()
	winner := "僵屍陣營"
	if humanWins {
		winner = "人類陣營"
	}
	r.broadcastLocked(ServerMessage{Type: "log", Payload: LogPayload{Message: fmt.Sprintf("對局結束，%s取得勝利", winner)}})
	r.broadcastPublicStateLocked()

	go func() {
		time.Sleep(5 * time.Second)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.status != RoomStatusFinished {
			return
		}
		r.resetToLobbyLocked()
	}()
}

func (r *Room) resetToLobbyLocked() {
	emptyPayload, err := json.Marshal(ServerMessage{Type: "private_state", Payload: PrivateStatePayload{}})
	if err != nil {
		emptyPayload = nil
	}

	for _, seat := range r.seats {
		seat.Player = nil
		if emptyPayload != nil && seat.Client != nil {
			select {
			case seat.Client.send <- emptyPayload:
			default:
				go seat.Client.close()
			}
		}
		if seat.Bot != nil {
			seat.Bot.KnownZombies = make(map[int]struct{})
		}
	}

	r.game = nil
	r.currentRound = 0
	r.currentTurn = -1
	r.pendingChallenge = nil
	r.status = RoomStatusLobby

	r.broadcastPublicStateLocked()
	r.broadcastLobbyLocked()
}
