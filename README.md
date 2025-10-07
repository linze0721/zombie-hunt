# 僵屍狩獵（ZombiesRush）

a game from alice in borderland

## 核心特色

- **即時大廳與房間管理**：採用 `gorilla/websocket` 建立長連線，支援建立房間、加入/離開與座位同步更新。
- **完整身分驗證流程**：透過 SQLite 儲存帳號與加鹽密碼雜湊，提供註冊、登入與會話管理 API。
- **十二回合推理對戰**：內建桌遊引擎（`internal/game`），模擬人類與僵屍陣營的對抗規則、牌組管理與勝負判定。
- **Bot 支援**：房主可在房間中新增/移除機器人座位，快速補齊人數體驗完整對戰。
- **純前端 UI**：不依賴框架，使用原生 HTML5/CSS/JavaScript 完成登入、房間、大廳到對戰界面。

## 目錄導覽

```
cmd/
  server/        # HTTP + WebSocket 主伺服器入口
  zombiehunt/    # CLI 範例（提示使用網頁版）
data/            # 預設 SQLite 資料庫位置
internal/
  game/          # 桌遊核心規則與狀態管理
  server/        # 大廳、房間、訊息格式與機器人
    store/       # 使用者與會話資料存取層
web/
  index.html     # SPA 入口頁
  static/        # 前端腳本與樣式
```

## 環境需求

- Go 1.22 以上（`go.mod` 設定為 1.24，建議啟用 `GOTOOLCHAIN=auto` 以使用新工具鏈）
- SQLite（隨 `github.com/mattn/go-sqlite3` 動態連結）
- 現代瀏覽器（Chrome、Edge、Firefox 等）

## 快速開始

1. 下載相依套件：
   ```bash
   go mod download
   ```
2. 啟動伺服器：
   ```bash
   go run ./cmd/server --addr :8080 --web ./web --data ./data
   ```
3. 於瀏覽器開啟 `http://localhost:8080`，註冊或登入帳號後即可建立房間、邀請朋友或加入 Bot 進行遊戲。

### 伺服器旗標

| 旗標 | 預設值 | 說明 |
| ---- | ------ | ---- |
| `--addr` | `:8080` | HTTP 服務監聽位址 |
| `--web` | `web` | 靜態資源目錄（需包含 `index.html` 與 `static/`） |
| `--data` | `data` | SQLite 資料庫存放目錄 |

資料庫初次啟動時會自動建立 `zombierush.db` 並初始化 `users` / `sessions` 資料表。

## 遊戲流程速覽

1. **登入/註冊**：透過 `/api/login` 與 `/api/register` 取得會話 Token。
2. **大廳互動**：WebSocket 訂閱 `/ws`，取得房間列表與座位資訊。
3. **建立或加入房間**：房主可控制 Bot 與開始遊戲；玩家以座位對應身份資訊同步更新。
4. **對戰進行**：行動、挑戰、防守等訊息皆透過 WebSocket 發送，後端由 `internal/game` 判斷結果並廣播。


歡迎針對桌遊規則、前端體驗或後端架構持續優化，共同打造更完整的線上推理桌遊平台。
