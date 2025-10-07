const TOKEN_KEY = 'zombiehunt-token';
const SESSION_KEY = 'zombiehunt-session';
const DISPLAY_KEY = 'zombiehunt-display';

const CARD_KIND = Object.freeze({
  NUMBER: 0,
  ZOMBIE: 1,
  SHOTGUN: 2,
  VACCINE: 3,
});

function createToken() {
  return `token-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

function ensureToken() {
  try {
    const stored = localStorage.getItem(TOKEN_KEY);
    if (stored) {
      return stored;
    }
    const token = createToken();
    localStorage.setItem(TOKEN_KEY, token);
    return token;
  } catch (err) {
    console.warn('無法使用 localStorage，改用臨時識別', err);
    return createToken();
  }
}

const state = {
  ws: null,
  token: ensureToken(),
  sessionToken: typeof window !== 'undefined' ? localStorage.getItem(SESSION_KEY) : null,
  accountName: '',
  playerName: typeof window !== 'undefined' ? localStorage.getItem(DISPLAY_KEY) || '' : '',
  lobbyRooms: [],
  roomId: null,
  roomStatus: 'lobby',
  roomName: '',
  seatIndex: -1,
  hostSeat: -1,
  roomState: null,
  publicGame: null,
  privateSnapshot: null,
  logs: [],
  challengeTarget: null,
  selectedCards: new Set(),
  pendingDefense: null,
  defenseSelection: new Set(),
  reconnectTimer: null,
  authMode: 'login',
  postGameMessage: '',
};

const elements = {
  loginOverlay: document.getElementById('login-overlay'),
  authForm: document.getElementById('auth-form'),
  authUsername: document.getElementById('auth-username'),
  authPassword: document.getElementById('auth-password'),
  authDisplay: document.getElementById('auth-display'),
  btnAuthSubmit: document.getElementById('btn-auth-submit'),
  btnAuthToggle: document.getElementById('btn-auth-toggle'),
  authHint: document.getElementById('auth-hint'),

  lobbyView: document.getElementById('lobby-view'),
  roomView: document.getElementById('room-view'),
  gameView: document.getElementById('game-view'),

  currentPlayerName: document.getElementById('current-player-name'),
  btnRefreshLobby: document.getElementById('btn-refresh-lobby'),
  roomList: document.getElementById('room-list'),
  roomCount: document.getElementById('room-count'),

  createRoomForm: document.getElementById('create-room-form'),
  createRoomName: document.getElementById('create-room-name'),

  roomTitle: document.getElementById('room-title'),
  roomStatusBadge: document.getElementById('room-status-badge'),
  hostIndicator: document.getElementById('host-indicator'),
  seatGrid: document.getElementById('seat-grid'),
  addBotForm: document.getElementById('add-bot-form'),
  botName: document.getElementById('bot-name'),
  botSeatSelect: document.getElementById('bot-seat-select'),
  btnRemoveBot: document.getElementById('btn-remove-bot'),
  btnStartGame: document.getElementById('btn-start-game'),
  btnCopyInvite: document.getElementById('btn-copy-invite'),
  btnLeaveRoom: document.getElementById('btn-leave-room'),

  gameRoomName: document.getElementById('game-room-name'),
  gameSeatIndex: document.getElementById('game-seat-index'),
  infoRound: document.getElementById('info-round'),
  maxRounds: document.getElementById('max-rounds'),
  infoTurn: document.getElementById('info-turn'),
  btnLeaveGame: document.getElementById('btn-leave-game'),
  boardSeats: document.getElementById('board-seats'),
  factionSummary: document.getElementById('faction-summary'),
  turnBanner: document.getElementById('turn-banner'),
  identityDisplay: document.getElementById('identity-display'),
  handContainer: document.getElementById('hand-cards'),
  handHint: document.getElementById('hand-hint'),
  selectedCards: document.getElementById('selected-cards'),
  targetList: document.getElementById('target-list'),
  btnSubmitChallenge: document.getElementById('btn-submit-challenge'),
  btnClearSelection: document.getElementById('btn-clear-selection'),
  logsList: document.getElementById('logs-list'),

  defenseModal: document.getElementById('defense-modal'),
  defenseTitle: document.getElementById('defense-title'),
  defenseDescription: document.getElementById('defense-description'),
  defenseOptions: document.getElementById('defense-options'),
  btnDefenseConfirm: document.getElementById('btn-defense-confirm'),
  btnDefensePass: document.getElementById('btn-defense-pass'),

  toast: document.getElementById('toast'),
};

function resolveKind(kind) {
  if (typeof kind === 'number') return kind;
  const parsed = Number(kind);
  return Number.isNaN(parsed) ? kind : parsed;
}

function getHandCardByIndex(index) {
  const hand = state.privateSnapshot?.hand || [];
  return hand.find((card) => card.index === index) || null;
}

function getSelectedHandCards() {
  const hand = state.privateSnapshot?.hand || [];
  const selected = Array.from(state.selectedCards)
    .map((idx) => hand.find((card) => card.index === idx))
    .filter(Boolean);
  selected.sort((a, b) => a.index - b.index);
  return selected;
}

function validateCardCombination(cards, { mode = 'attack', identity } = {}) {
  if (!cards || cards.length === 0) {
    return '請選擇至少一張牌';
  }
  if (cards.length > 5) {
    return '最多選擇 5 張牌';
  }

  const normalized = cards.map((card) => ({ card, kind: resolveKind(card.kind) }));
  const first = normalized[0];

  if (mode === 'attack' && first.kind === CARD_KIND.VACCINE) {
    return '疫苗僅能在防守時使用';
  }
  if (mode === 'attack' && first.kind === CARD_KIND.ZOMBIE && identity !== '僵屍') {
    return '僅有僵屍可以使用僵屍牌';
  }

  if (first.kind === CARD_KIND.NUMBER) {
    const suit = first.card.suit;
    for (const entry of normalized) {
      if (entry.kind !== CARD_KIND.NUMBER) {
        return '數字牌不可與特殊牌混出';
      }
      if (entry.card.suit !== suit) {
        return `請選擇同一花色（${suit}）`;
      }
    }
    return null;
  }

  if (first.kind === CARD_KIND.SHOTGUN || first.kind === CARD_KIND.ZOMBIE || first.kind === CARD_KIND.VACCINE) {
    if (normalized.length > 1) {
      return '特殊牌需單獨出牌';
    }
    return null;
  }

  if (new Set(normalized.map((entry) => entry.kind)).size > 1) {
    return '選取的牌型不相容';
  }

  return null;
}

function setView(view) {
  elements.lobbyView.classList.toggle('hidden', view !== 'lobby');
  elements.roomView.classList.toggle('hidden', view !== 'room');
  elements.gameView.classList.toggle('hidden', view !== 'game');
}

function showToast(message, duration = 2800) {
  if (!elements.toast) return;
  elements.toast.textContent = message;
  elements.toast.classList.add('show');
  if (showToast.timer) {
    clearTimeout(showToast.timer);
  }
  showToast.timer = setTimeout(() => {
    elements.toast.classList.remove('show');
  }, duration);
}

function setSession(token, username) {
  state.sessionToken = token;
  state.accountName = username;
  try {
    localStorage.setItem(SESSION_KEY, token);
  } catch (err) {
    console.warn('無法儲存會話', err);
  }
}

function clearSession() {
  state.sessionToken = null;
  state.accountName = '';
  try {
    localStorage.removeItem(SESSION_KEY);
  } catch (err) {
    console.warn('無法清除會話', err);
  }
}

function setDisplayName(name) {
  const value = (name || '').trim();
  state.playerName = value;
  try {
    localStorage.setItem(DISPLAY_KEY, value);
  } catch (err) {
    console.warn('無法儲存顯示名稱', err);
  }
  if (elements.authDisplay && document.activeElement !== elements.authDisplay) {
    elements.authDisplay.value = value;
  }
  if (elements.currentPlayerName) {
    elements.currentPlayerName.textContent = value || '-';
  }
}

async function requestAuth(endpoint, payload) {
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || '操作失敗');
  }
  return data;
}

async function fetchProfile(token) {
  const response = await fetch('/api/profile', {
    headers: { Authorization: `Bearer ${token}` },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || '取得會話資訊失敗');
  }
  return data;
}

function connect(displayName) {
  if (!state.sessionToken) {
    elements.loginOverlay.classList.remove('hidden');
    return;
  }

  const resolvedName = (displayName || state.playerName || state.accountName || '玩家').trim() || '玩家';
  setDisplayName(resolvedName);

  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }

  if (state.ws) {
    try {
      state.ws.close();
    } catch (err) {
      console.warn('關閉舊連線失敗', err);
    }
  }

  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const params = new URLSearchParams();
  params.set('name', resolvedName);
  params.set('token', state.token);
  params.set('auth', state.sessionToken);
  if (state.roomId) {
    params.set('room', state.roomId);
  }
  const ws = new WebSocket(`${proto}://${window.location.host}/ws?${params.toString()}`);

  ws.onopen = () => {
    state.ws = ws;
    state.roomId = state.roomId || null;
    elements.loginOverlay.classList.add('hidden');
    setView('lobby');
    sendMessage({ type: 'lobby_list', payload: {} });
  };

  ws.onmessage = handleMessage;

  ws.onclose = () => {
    if (state.ws === ws) {
      state.ws = null;
    }
    if (!state.sessionToken) {
      elements.loginOverlay.classList.remove('hidden');
      return;
    }
    if (state.reconnectTimer) return;
    showToast('連線中斷，將嘗試重新連線…', 2600);
    state.reconnectTimer = setTimeout(() => {
      state.reconnectTimer = null;
      connect();
    }, 2000);
  };

  ws.onerror = (err) => {
    console.error('WebSocket 錯誤', err);
  };
}

function sendMessage(message) {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
    console.warn('WebSocket 尚未連線', message);
    return;
  }
  state.ws.send(JSON.stringify(message));
}

function handleMessage(event) {
  let message;
  try {
    message = JSON.parse(event.data);
  } catch (err) {
    console.warn('無法解析伺服器訊息', err, event.data);
    return;
  }

  const { type, payload } = message;
  switch (type) {
    case 'welcome':
      handleWelcome(payload || {});
      break;
    case 'lobby_rooms':
      handleLobbyRooms(payload || {});
      break;
    case 'lobby_state':
      handleLobbyState(payload || {});
      break;
    case 'public_state':
      handlePublicState(payload || {});
      break;
    case 'private_state':
      handlePrivateState(payload || {});
      break;
    case 'defense_prompt':
      handleDefensePrompt(payload || {});
      break;
    case 'turn_prompt':
      showToast('輪到你行動', 2000);
      break;
    case 'log':
      if (payload?.message) {
        appendLog(payload.message);
      }
      break;
    case 'error':
      if (payload?.message) {
        showToast(payload.message, 3600);
      }
      break;
    case 'private_info':
      if (payload?.message) {
        showToast(payload.message, 3600);
      }
      break;
    default:
      console.debug('未處理訊息', message);
      break;
  }
}

function handleWelcome(payload) {
  state.roomId = payload.roomId || null;
  state.roomName = payload.roomName || '';
  state.roomStatus = payload.status || 'lobby';
  state.seatIndex = typeof payload.seatIndex === 'number' ? payload.seatIndex : -1;
  state.hostSeat = typeof payload.hostSeat === 'number' ? payload.hostSeat : state.hostSeat;
  if (payload.token) {
    state.token = String(payload.token);
    try {
      localStorage.setItem(TOKEN_KEY, state.token);
    } catch (err) {
      console.warn('無法保存座位識別', err);
    }
  }

  if (payload.displayName) {
    setDisplayName(payload.displayName);
  }

  try {
    const url = new URL(window.location.href);
    if (state.roomId) {
      url.searchParams.set('room', state.roomId);
    } else {
      url.searchParams.delete('room');
    }
    window.history.replaceState({}, '', url.toString());
  } catch (err) {
    console.warn('無法更新網址列', err);
  }

  if (state.roomId) {
    const inGameView = state.roomStatus === 'running' || state.roomStatus === 'finished';
    setView(inGameView ? 'game' : 'room');
  } else {
    setView('lobby');
  }
}

function handleLobbyRooms(payload) {
  state.lobbyRooms = Array.isArray(payload.rooms) ? payload.rooms : [];
  renderLobby();
}

function handleLobbyState(payload) {
  if (!payload || payload.roomId !== state.roomId) return;
  state.roomState = payload;
  state.roomName = payload.roomName || state.roomName;
  const nextStatus = payload.status || state.roomStatus;
  state.roomStatus = nextStatus;
  state.hostSeat = typeof payload.hostSeat === 'number' ? payload.hostSeat : state.hostSeat;
  renderRoom();
  const showGame = nextStatus === 'running' || nextStatus === 'finished';
  setView(showGame ? 'game' : 'room');
}

function handlePublicState(payload) {
  state.roomState = payload;
  state.roomName = payload.roomName || state.roomName;
  const previousStatus = state.roomStatus;
  const nextStatus = payload.status || previousStatus;
  state.roomStatus = nextStatus;
  state.hostSeat = typeof payload.hostSeat === 'number' ? payload.hostSeat : state.hostSeat;
  state.publicGame = payload.publicGame || null;

  if (nextStatus === 'finished') {
    if (previousStatus !== 'finished' && !state.postGameMessage) {
      state.postGameMessage = '對局結束，稍後返回大廳';
    }
  } else if (state.postGameMessage) {
    state.postGameMessage = '';
  }

  const targetView = nextStatus === 'running' || nextStatus === 'finished' ? 'game' : 'room';
  setView(targetView);
  renderRoom();
  renderGame();
}

function handlePrivateState(payload) {
  const snapshot = payload?.snapshot;
  if (!snapshot || Object.keys(snapshot).length === 0) {
    state.privateSnapshot = null;
    state.selectedCards.clear();
    updateSelectionUI();
    renderGame();
    return;
  }
  state.privateSnapshot = snapshot;
  state.seatIndex = snapshot.playerId ?? state.seatIndex;
  state.challengeTarget = null;
  state.selectedCards.clear();
  updateSelectionUI();
  renderGame();
}

function handleDefensePrompt(payload) {
  state.pendingDefense = payload;
  state.defenseSelection.clear();
  if (!payload || !payload.options) {
    closeDefenseModal();
    return;
  }
  const attackerName = payload.attackerName || '對手';
  const suit = payload.suit ? `花色 ${payload.suit}` : '特殊牌';
  const maxSelectable = payload.maxSelectable || 5;
  elements.defenseTitle.textContent = '防守選擇';
  elements.defenseDescription.textContent = `${attackerName} 的出牌：${describeCards(payload.attackCards || [])}。可選擇最多 ${maxSelectable} 張 ${suit}。`;
  renderDefenseOptions(payload.options || [], maxSelectable);
  elements.defenseModal.classList.remove('hidden');
}

function describeCards(cards) {
  if (!cards || !cards.length) return '—';
  return cards.map((card) => card.label || translateCard(card)).join('、');
}

function renderDefenseOptions(options, maxSelectable) {
  elements.defenseOptions.innerHTML = '';
  options.forEach((card) => {
    const div = document.createElement('div');
    div.className = 'card';
    div.dataset.index = card.index;
    div.dataset.kind = String(card.kind);
    div.textContent = translateCard(card);
    div.addEventListener('click', () => {
      toggleDefenseSelection(card.index, maxSelectable, div);
    });
    elements.defenseOptions.append(div);
  });
  updateDefenseButtons(maxSelectable);
}

function toggleDefenseSelection(index, maxSelectable, element) {
  if (state.defenseSelection.has(index)) {
    state.defenseSelection.delete(index);
    element.classList.remove('selected');
  } else {
    if (state.defenseSelection.size >= maxSelectable) {
      showToast(`最多只能選 ${maxSelectable} 張牌`, 1800);
      return;
    }
    state.defenseSelection.add(index);
    element.classList.add('selected');
  }
  updateDefenseButtons(maxSelectable);
}

function updateDefenseButtons(maxSelectable) {
  if (state.defenseSelection.size === 0) {
    elements.btnDefenseConfirm.textContent = '棄權';
  } else {
    elements.btnDefenseConfirm.textContent = `出牌（${state.defenseSelection.size}/${maxSelectable}）`;
  }
}

function closeDefenseModal() {
  elements.defenseModal.classList.add('hidden');
  elements.defenseOptions.innerHTML = '';
  state.pendingDefense = null;
  state.defenseSelection.clear();
}

function renderLobby() {
  elements.roomCount.textContent = `${state.lobbyRooms.length} 間房間`;
  elements.roomList.innerHTML = '';
  if (state.lobbyRooms.length === 0) {
    const hint = document.createElement('div');
    hint.className = 'hint';
    hint.textContent = '目前尚無房間，建立一間新房間並邀請朋友吧。';
    elements.roomList.append(hint);
    return;
  }
  state.lobbyRooms.forEach((room) => {
    const card = document.createElement('div');
    card.className = 'room-card';
    const title = document.createElement('h3');
    title.textContent = room.name || '未命名房間';
    const meta = document.createElement('div');
    meta.className = 'meta';
    meta.innerHTML = `房主：${room.host || '未知'}<br>狀態：${translateStatus(room.status)}<br>人數：${room.players || 0} / ${room.capacity || 8}`;
    const btn = document.createElement('button');
    btn.type = 'button';
    const canJoin = room.status === 'lobby' && (room.players || 0) < (room.capacity || 8);
    btn.textContent = canJoin ? '加入房間' : '無法加入';
    btn.disabled = !canJoin;
    btn.addEventListener('click', () => {
      sendMessage({ type: 'room_join', payload: { roomId: room.roomId } });
    });
    card.append(title, meta, btn);
    elements.roomList.append(card);
  });
}

function renderRoom() {
  if (!state.roomState) return;
  elements.roomTitle.textContent = state.roomName || '-';
  elements.roomStatusBadge.textContent = translateStatus(state.roomStatus);
  const seats = Array.isArray(state.roomState.seats) ? state.roomState.seats : [];
  elements.hostIndicator.textContent = `房主：${state.roomState.hostSeat >= 0 && state.roomState.hostSeat < seats.length ? seats[state.roomState.hostSeat].name : '-'}`;
  elements.seatGrid.innerHTML = '';
  seats.forEach((seat) => {
    const card = document.createElement('div');
    card.className = 'seat-card';
    if (seat.index === state.hostSeat) card.classList.add('host');
    if (seat.index === state.seatIndex) card.classList.add('me');
    if (seat.isBot) card.classList.add('bot');
    if (!seat.filled) card.classList.add('empty');
    if (seat.alive === false) card.classList.add('dead');

    const label = document.createElement('div');
    label.className = 'label';
    label.textContent = `座位 #${seat.index}`;
    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = seat.name || `座位 #${seat.index}`;
    const status = document.createElement('div');
    status.className = 'status';
    if (!seat.filled) {
      status.textContent = '狀態：空位';
    } else if (seat.alive === false) {
      status.textContent = '狀態：淘汰';
    } else {
      status.textContent = seat.isBot ? '狀態：機器人' : '狀態：待命';
    }

    card.append(label, name, status);
    elements.seatGrid.append(card);
  });
}

function renderGame() {
  if (!state.roomState) return;
  elements.gameRoomName.textContent = state.roomName || '-';
  elements.gameSeatIndex.textContent = state.seatIndex >= 0 ? `#${state.seatIndex}` : '-';
  renderBoard();
  renderHand();
  renderTargets();
  renderLogs();
  updateSelectionUI();

  if (state.publicGame?.snapshot) {
    elements.infoRound.textContent = state.publicGame.snapshot.round ?? '-';
    elements.maxRounds.textContent = state.publicGame.snapshot.maxRounds ?? '12';
    const turnIdx = state.publicGame.currentTurn;
    elements.infoTurn.textContent = turnIdx === state.seatIndex ? '你' : seatName(turnIdx);
  } else {
    elements.infoRound.textContent = '-';
    elements.infoTurn.textContent = '-';
  }

  if (state.roomStatus === 'running') {
    elements.factionSummary.textContent = '陣營情報保密中';
  } else if (state.roomStatus === 'finished') {
    elements.factionSummary.textContent = '對局已結束';
  } else {
    elements.factionSummary.textContent = '等待房主開始對戰';
  }

  const myTurn = state.publicGame && state.publicGame.currentTurn === state.seatIndex;
  if (state.roomStatus === 'finished') {
    const message = state.postGameMessage || '對局已結束，稍後返回大廳';
    elements.turnBanner.textContent = message;
    elements.turnBanner.classList.remove('hidden');
  } else {
    elements.turnBanner.textContent = '輪到你行動';
    elements.turnBanner.classList.toggle('hidden', !myTurn);
  }
}

function renderBoard() {
  elements.boardSeats.innerHTML = '';
  const seats = Array.isArray(state.roomState?.seats) ? state.roomState.seats : [];
  const count = seats.length || 8;
  seats.forEach((seat, idx) => {
    const card = document.createElement('div');
    card.className = 'seat-card';
    card.dataset.seat = seat.index;
    const angle = (360 / count) * idx;
    card.style.setProperty('--angle', `${angle}deg`);

    if (seat.index === state.hostSeat) card.classList.add('host');
    if (seat.index === state.seatIndex) card.classList.add('me');
    if (seat.isBot) card.classList.add('bot');
    if (!seat.filled) card.classList.add('empty');
    if (seat.alive === false) card.classList.add('dead');
    if (state.publicGame && state.publicGame.currentTurn === seat.index) {
      card.classList.add('current-turn');
    }

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = seat.name || `座位 #${seat.index}`;

    const status = document.createElement('div');
    status.className = 'status';
    if (!seat.filled) {
      status.textContent = '空位';
    } else if (seat.alive === false) {
      status.textContent = '淘汰';
    } else {
      status.textContent = seat.isBot ? '機器人' : '存活';
    }

    const info = document.createElement('div');
    info.className = 'meta-row';
    const handSize = seat.hand ?? '?';
    info.textContent = `手牌：${seat.alive === false ? 0 : handSize}`;

    card.append(name, status, info);
    elements.boardSeats.append(card);
  });
  updateBoardSelection();
}

function renderHand() {
  elements.handContainer.innerHTML = '';
  if (!state.privateSnapshot) {
    elements.identityDisplay.textContent = '身份：未知';
    return;
  }
  elements.identityDisplay.textContent = `身份：${state.privateSnapshot.identity}`;
  const cards = Array.isArray(state.privateSnapshot.hand) ? state.privateSnapshot.hand : [];
  cards.forEach((card) => {
    const div = document.createElement('div');
    div.className = 'card';
    div.dataset.index = card.index;
    div.dataset.kind = String(card.kind);
    div.textContent = translateCard(card);
    if (state.selectedCards.has(card.index)) {
      div.classList.add('selected');
    }
    div.addEventListener('click', () => {
      toggleCardSelection(card.index, div);
    });
    elements.handContainer.append(div);
  });
  updateSelectionUI();
}

function renderTargets() {
  elements.targetList.innerHTML = '';
  const seats = Array.isArray(state.roomState?.seats) ? state.roomState.seats : [];
  seats.forEach((seat) => {
    const card = document.createElement('div');
    card.className = 'target-card';
    card.dataset.seat = seat.index;

    const alive = seat.alive !== false;
    const selectable = alive && seat.filled && seat.index !== state.seatIndex;
    if (!selectable) {
      card.classList.add('disabled');
    }
    const title = document.createElement('div');
    title.className = 'name';
    title.textContent = seat.name || `座位 ${seat.index}`;
    const info = document.createElement('div');
    info.className = 'status';
    info.textContent = selectable ? '可挑戰' : (alive ? '不可選擇' : '已淘汰');

    card.append(title, info);
    card.addEventListener('click', () => {
      if (!selectable) return;
      state.challengeTarget = seat.index;
      updateTargetSelection();
      updateSelectionUI();
    });
    elements.targetList.append(card);
  });
  updateTargetSelection();
}

function updateTargetSelection() {
  const cards = elements.targetList.querySelectorAll('.target-card');
  cards.forEach((card) => {
    const idx = Number(card.dataset.seat);
    card.classList.toggle('selected', idx === state.challengeTarget);
  });
  updateBoardSelection();
}

function updateBoardSelection() {
  const seatCards = elements.boardSeats?.querySelectorAll('.seat-card');
  if (!seatCards) return;
  seatCards.forEach((card) => {
    const idx = Number(card.dataset.seat);
    card.classList.toggle('selected-target', state.challengeTarget !== null && idx === state.challengeTarget);
  });
}

function toggleCardSelection(index, element) {
  const card = getHandCardByIndex(index);
  if (!card) {
    return;
  }
  if (state.selectedCards.has(index)) {
    state.selectedCards.delete(index);
    element.classList.remove('selected');
  } else {
    const prospective = [...getSelectedHandCards(), card];
    const error = validateCardCombination(prospective, { mode: 'attack', identity: state.privateSnapshot?.identity });
    if (error) {
      showToast(error, 2400);
      return;
    }
    state.selectedCards.add(index);
    element.classList.add('selected');
  }
  updateSelectionUI();
}

function updateSelectionUI() {
  elements.selectedCards.innerHTML = '';
  const selectedCards = getSelectedHandCards();
  selectedCards.forEach((card) => {
    const div = document.createElement('div');
    div.className = 'card';
    div.dataset.kind = String(card.kind);
    div.textContent = translateCard(card);
    elements.selectedCards.append(div);
  });

  const validationError = selectedCards.length > 0
    ? validateCardCombination(selectedCards, { mode: 'attack', identity: state.privateSnapshot?.identity })
    : null;

  if (elements.handHint) {
    if (selectedCards.length === 0) {
      elements.handHint.textContent = '點擊手牌以選取，多選上限 5 張';
      elements.handHint.classList.remove('warning');
    } else if (validationError) {
      elements.handHint.textContent = validationError;
      elements.handHint.classList.add('warning');
    } else {
      const first = selectedCards[0];
      const kind = resolveKind(first.kind);
      if (kind === CARD_KIND.NUMBER) {
        elements.handHint.textContent = `已選 ${selectedCards.length} 張 ${first.suit || ''} 數字牌`;
      } else {
        elements.handHint.textContent = `已選 ${translateCard(first)}，隨時可以出牌`;
      }
      elements.handHint.classList.remove('warning');
    }
  }

  const myTurn = state.publicGame && state.publicGame.currentTurn === state.seatIndex;
  const gameActive = state.roomStatus === 'running';
  const canSubmit = gameActive && myTurn && state.challengeTarget !== null && selectedCards.length > 0 && !validationError;
  elements.btnSubmitChallenge.disabled = !canSubmit;
}

function submitChallenge() {
  if (state.roomStatus !== 'running') {
    showToast('對局目前無法發起挑戰');
    return;
  }
  if (state.challengeTarget === null) {
    showToast('請先選擇挑戰目標');
    return;
  }
  const selectedCards = getSelectedHandCards();
  if (selectedCards.length === 0) {
    showToast('請選擇手牌');
    return;
  }
  const error = validateCardCombination(selectedCards, { mode: 'attack', identity: state.privateSnapshot?.identity });
  if (error) {
    showToast(error, 3200);
    return;
  }
  const cards = selectedCards.map((card) => card.index).sort((a, b) => a - b);
  sendMessage({
    type: 'action_challenge',
    payload: { targetId: state.challengeTarget, cards },
  });
  state.selectedCards.clear();
  updateSelectionUI();
}

function appendLog(text) {
  const time = new Date();
  state.logs.push({ text, time });
  if (state.logs.length > 200) {
    state.logs = state.logs.slice(-200);
  }
  renderLogs();
  if (text.startsWith('對局結束')) {
    state.postGameMessage = text;
    renderGame();
  }
}

function renderLogs() {
  elements.logsList.innerHTML = '';
  state.logs.slice(-40).forEach((entry) => {
    const li = document.createElement('li');
    li.className = 'log-entry';
    const time = document.createElement('div');
    time.className = 'time';
    time.textContent = entry.time.toLocaleTimeString('zh-TW', { hour12: false });
    const text = document.createElement('div');
    text.className = 'text';
    text.textContent = entry.text;
    li.append(time, text);
    elements.logsList.append(li);
  });
  elements.logsList.scrollTop = elements.logsList.scrollHeight;
}

function seatName(index) {
  if (!state.roomState?.seats) return `座位 ${index}`;
  const seat = state.roomState.seats.find((s) => s.index === index);
  return seat ? seat.name || `座位 ${index}` : `座位 ${index}`;
}

function translateStatus(status) {
  switch (status) {
    case 'lobby':
      return '待機中';
    case 'running':
      return '對戰中';
    case 'finished':
      return '已結束';
    default:
      return status || '-';
  }
}

function translateCard(card) {
  const kind = typeof card.kind === 'number' ? card.kind : Number(card.kind);
  switch (kind) {
    case 0:
      return `${card.suit || ''}${card.value || ''}`;
    case 1:
      return '僵屍牌';
    case 2:
      return '獵槍牌';
    case 3:
      return '疫苗牌';
    default:
      return card.label || `${card.suit || ''}${card.value || ''}`;
  }
}

function restoreSession() {
  if (!state.sessionToken) {
    elements.loginOverlay.classList.remove('hidden');
    if (elements.authUsername) elements.authUsername.focus();
    return;
  }
  fetchProfile(state.sessionToken)
    .then((profile) => {
      setSession(state.sessionToken, profile.username);
      if (!state.playerName) {
        setDisplayName(profile.username);
      }
      elements.loginOverlay.classList.add('hidden');
      connect(state.playerName);
    })
    .catch((err) => {
      console.warn('恢復登入狀態失敗', err);
      clearSession();
      elements.loginOverlay.classList.remove('hidden');
      if (elements.authUsername) elements.authUsername.focus();
    });
}

function registerEventListeners() {
  if (elements.authForm) {
    elements.authForm.addEventListener('submit', async (evt) => {
      evt.preventDefault();
      const username = elements.authUsername.value.trim();
      const password = elements.authPassword.value;
      const display = elements.authDisplay.value.trim();
      if (!username || !password) {
        showToast('請輸入帳號與密碼');
        return;
      }
      elements.btnAuthSubmit.disabled = true;
      try {
        const endpoint = state.authMode === 'login' ? '/api/login' : '/api/register';
        const data = await requestAuth(endpoint, { username, password });
        setSession(data.token, data.username);
        elements.authPassword.value = '';
        setDisplayName(display || data.username);
        elements.loginOverlay.classList.add('hidden');
        connect(display || data.username);
      } catch (err) {
        showToast(err.message || '操作失敗', 4000);
      } finally {
        elements.btnAuthSubmit.disabled = false;
      }
    });
  }

  if (elements.btnAuthToggle) {
    elements.btnAuthToggle.addEventListener('click', () => {
      state.authMode = state.authMode === 'login' ? 'register' : 'login';
      elements.btnAuthSubmit.textContent = state.authMode === 'login' ? '登入' : '註冊';
      elements.btnAuthToggle.textContent = state.authMode === 'login' ? '切換至註冊' : '切換至登入';
      elements.authHint.textContent = state.authMode === 'login' ? '還沒有帳號？立即註冊一個。' : '已經有帳號？改為登入。';
    });
  }

  elements.btnRefreshLobby?.addEventListener('click', () => {
    sendMessage({ type: 'lobby_list', payload: {} });
  });

  elements.createRoomForm?.addEventListener('submit', (evt) => {
    evt.preventDefault();
    const name = elements.createRoomName.value.trim() || '未命名房間';
    sendMessage({ type: 'room_create', payload: { name } });
    elements.createRoomName.value = '';
  });

  elements.addBotForm?.addEventListener('submit', (evt) => {
    evt.preventDefault();
    const name = elements.botName.value.trim();
    sendMessage({ type: 'room_add_bot', payload: name ? { name } : {} });
    elements.botName.value = '';
  });

  elements.btnRemoveBot?.addEventListener('click', () => {
    const seat = elements.botSeatSelect.value;
    if (!seat) {
      showToast('沒有可移除的機器人');
      return;
    }
    sendMessage({ type: 'room_remove_bot', payload: { seat: Number(seat) } });
  });

  elements.btnStartGame?.addEventListener('click', () => {
    sendMessage({ type: 'start_game', payload: {} });
  });

  elements.btnCopyInvite?.addEventListener('click', async () => {
    if (!state.roomId) {
      showToast('尚未進入房間');
      return;
    }
    try {
      const url = new URL(window.location.href);
      url.searchParams.set('room', state.roomId);
      await navigator.clipboard.writeText(url.toString());
      showToast('已複製邀請連結。');
    } catch (err) {
      console.warn('複製連結失敗', err);
      showToast('請手動複製網址。', 3600);
    }
  });

  elements.btnLeaveRoom?.addEventListener('click', leaveRoom);
  elements.btnLeaveGame?.addEventListener('click', leaveRoom);

  elements.btnSubmitChallenge?.addEventListener('click', submitChallenge);
  elements.btnClearSelection?.addEventListener('click', () => {
    state.selectedCards.clear();
    updateSelectionUI();
  });

  elements.btnDefenseConfirm?.addEventListener('click', () => {
    if (!state.pendingDefense) {
      closeDefenseModal();
      return;
    }
    const cards = Array.from(state.defenseSelection).sort((a, b) => a - b);
    sendMessage({ type: 'action_defense', payload: { cards } });
    closeDefenseModal();
  });

  elements.btnDefensePass?.addEventListener('click', () => {
    if (!state.pendingDefense) {
      closeDefenseModal();
      return;
    }
    sendMessage({ type: 'action_defense', payload: { cards: [] } });
    closeDefenseModal();
  });

  document.addEventListener('keydown', (evt) => {
    if (evt.key === 'Escape') {
      closeDefenseModal();
    }
  });
}

function leaveRoom() {
  if (!state.sessionToken) {
    showToast('尚未登入');
    return;
  }
  sendMessage({ type: 'room_leave', payload: {} });
  state.roomId = null;
  state.roomState = null;
  state.publicGame = null;
  state.privateSnapshot = null;
  state.selectedCards.clear();
  setView('lobby');
  sendMessage({ type: 'lobby_list', payload: {} });
}

setView('lobby');
registerEventListeners();
restoreSession();
