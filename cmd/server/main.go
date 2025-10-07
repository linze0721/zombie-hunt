package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"zombierush/internal/server"
	serverstore "zombierush/internal/server/store"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

type profileResponse struct {
	Username string `json:"username"`
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP 服務監聽位址")
	webDir := flag.String("web", "web", "前端靜態資源目錄")
	dataDir := flag.String("data", "data", "資料存放目錄")
	flag.Parse()

	dbPath := filepath.Join(*dataDir, "zombierush.db")
	store, err := serverstore.New(dbPath)
	if err != nil {
		log.Fatalf("初始化資料庫失敗: %v", err)
	}
	defer func() {
		if cerr := store.Close(); cerr != nil {
			log.Printf("關閉資料庫時發生錯誤: %v", cerr)
		}
	}()

	hub := server.NewHub()

	http.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "僅支援 POST")
			return
		}
		var req authRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "請提供帳號與密碼")
			return
		}
		user, err := store.CreateUser(req.Username, req.Password)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "已存在") {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		token, err := store.CreateSession(user.ID, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "建立會話失敗")
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Token: token, Username: user.Username})
	})

	http.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "僅支援 POST")
			return
		}
		var req authRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "請提供帳號與密碼")
			return
		}
		user, err := store.Authenticate(req.Username, req.Password)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		token, err := store.CreateSession(user.ID, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "建立會話失敗")
			return
		}
		writeJSON(w, http.StatusOK, authResponse{Token: token, Username: user.Username})
	})

	http.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "僅支援 GET")
			return
		}
		token := parseAuthHeader(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "缺少會話資訊")
			return
		}
		user, err := store.GetUserBySession(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, profileResponse{Username: user.Username})
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		authToken := strings.TrimSpace(r.URL.Query().Get("auth"))
		if authToken == "" {
			authToken = parseAuthHeader(r)
		}
		if authToken == "" {
			http.Error(w, "未登入", http.StatusUnauthorized)
			return
		}

		user, err := store.GetUserBySession(authToken)
		if err != nil {
			http.Error(w, "會話無效", http.StatusUnauthorized)
			return
		}

		roomID := strings.TrimSpace(r.URL.Query().Get("room"))
		displayName := strings.TrimSpace(r.URL.Query().Get("name"))
		if displayName == "" {
			displayName = user.Username
		}
		if len(displayName) > 24 {
			displayName = displayName[:24]
		}
		seatToken := strings.TrimSpace(r.URL.Query().Get("token"))
		if seatToken == "" {
			seatToken = fmt.Sprintf("seat-%d", time.Now().UnixNano())
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket 升級失敗: %v", err)
			return
		}

		client := server.NewWebClient(conn, hub, user.ID, user.Username, displayName, seatToken)
		hub.RegisterLobbyClient(client)
		if roomID != "" {
			if err := hub.JoinRoom(roomID, client); err != nil {
				_ = conn.WriteJSON(server.ServerMessage{Type: "error", Payload: server.ErrorPayload{Message: err.Error()}})
			}
		}

		go client.WritePump()
		client.ReadPump()
	})

	staticDir := http.Dir(filepath.Join(*webDir, "static"))
	staticFS := http.FileServer(staticDir)
	http.Handle("/static/", http.StripPrefix("/static/", staticFS))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(*webDir, "index.html"))
	})

	log.Printf("《僵屍狩獵》伺服器啟動於 %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("HTTP 服務啟動失敗: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("回傳 JSON 失敗: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseAuthHeader(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		if cookie, err := r.Cookie("session_token"); err == nil {
			return strings.TrimSpace(cookie.Value)
		}
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
