package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID       int64
	Username string
	Created  time.Time
}

func New(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("db 路徑不可為空")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("建立資料目錄失敗: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("開啟資料庫失敗: %w", err)
	}

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化資料表失敗: %w", err)
	}
	return nil
}

func (s *Store) CreateUser(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("帳號不可為空")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("密碼長度至少 6 碼")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("加密密碼失敗: %w", err)
	}

	res, err := s.db.Exec(`INSERT INTO users(username, password_hash) VALUES(?, ?)`, username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("帳號已存在")
		}
		return nil, fmt.Errorf("建立使用者失敗: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("取得使用者 ID 失敗: %w", err)
	}

	user := &User{ID: id, Username: username, Created: time.Now()}
	return user, nil
}

func (s *Store) Authenticate(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("帳號不可為空")
	}

	row := s.db.QueryRow(`SELECT id, password_hash, created_at FROM users WHERE username = ?`, username)
	var (
		id      int64
		hash    string
		created time.Time
	)
	if err := row.Scan(&id, &hash, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("帳號或密碼錯誤")
		}
		return nil, fmt.Errorf("查詢使用者失敗: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("帳號或密碼錯誤")
	}

	return &User{ID: id, Username: username, Created: created}, nil
}

func (s *Store) CreateSession(userID int64, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}

	token, err := randomToken(32)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	exp := now.Add(ttl)
	if _, err := s.db.Exec(`INSERT INTO sessions(token, user_id, created_at, expires_at) VALUES(?, ?, ?, ?)`, token, userID, now, exp); err != nil {
		return "", fmt.Errorf("建立會話失敗: %w", err)
	}

	return token, nil
}

func (s *Store) GetUserBySession(token string) (*User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("缺少會話 token")
	}

	row := s.db.QueryRow(`SELECT u.id, u.username, u.created_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token = ? AND s.expires_at > ?`, token, time.Now().UTC())
	var (
		id       int64
		username string
		created  time.Time
	)
	if err := row.Scan(&id, &username, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("會話無效或已過期")
		}
		return nil, fmt.Errorf("查詢會話失敗: %w", err)
	}

	return &User{ID: id, Username: username, Created: created}, nil
}

func (s *Store) CleanupExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("清理過期會話失敗: %w", err)
	}
	return nil
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成亂數 token 失敗: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
