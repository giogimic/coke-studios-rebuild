package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

var denizenSeq int64

type DenizenRecord struct {
	ID           string
	ScreenName   string
	PasswordHash string
	Secret       string
	Gender       int
	Mission      string
	AvatarDef    string
	Decibels     int
	VCoins       int
	LastSeenIn   string
}

type SessionRecord struct {
	Token      string
	DenizenID  string
	ScreenName string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

type StudioRecord struct {
	ID        string
	OwnerID   string
	OwnerName string
	Name      string
	Layout    int
	RoomType  string
	Wallpaper int
	Floor     int
	Doorbell  int
	IsPublic  bool
}

type BackpackItemRecord struct {
	ID            int
	DenizenID     string
	CatalogItemID int
	Properties    map[string]any
}

type PlacedFurniRecord struct {
	ID            int
	StudioID      string
	CatalogItemID int
	PosID         string
	X             int
	Y             int
	Z             int
	HO            int
	Dir           int
	State         int
	IAOT          bool
	Properties    map[string]any
}

type FriendRecord struct {
	ScreenName string `json:"screen_name"`
	Gender     int    `json:"gender"`
	Mission    string `json:"mission"`
	AvatarDef  string `json:"avatar_def"`
	Online     bool   `json:"online"`
	LastSeenIn string `json:"last_seen_in"`
	Status     string `json:"status"`
}

type MessageRecord struct {
	ID            int       `json:"id"`
	SenderName    string    `json:"sender"`
	RecipientName string    `json:"recipient"`
	Message       string    `json:"message"`
	SentAt        time.Time `json:"sent_at"`
	IsRead        bool      `json:"is_read"`
}

type Top40Entry struct {
	Rank        int    `json:"rank"`
	SongTitle   string `json:"title"`
	CreatorName string `json:"creator"`
	Votes       int    `json:"votes"`
	Award       string `json:"award,omitempty"`
}

type VegoSanResult struct {
	Win          bool   `json:"win"`
	Draw         bool   `json:"draw"`
	PlayerMove   string `json:"player_move"`
	OpponentMove string `json:"opponent_move"`
	Message      string `json:"message"`
	Streak       int    `json:"streak"`
	AwardedDB    int    `json:"awarded_db"`
	PrizeItem    string `json:"prize_item,omitempty"`
	PrizeItemID  int    `json:"prize_item_id,omitempty"`
}

type Store struct {
	db     *sql.DB
	driver string // "sqlite" or "mysql"
	mu     sync.Mutex
}

func OpenStore(connStr string) (*Store, error) {
	driver := "sqlite"
	dsn := connStr

	// Check if MySQL / MariaDB connection
	envDriver := os.Getenv("DB_DRIVER")
	if envDriver == "mysql" || envDriver == "mariadb" || strings.HasPrefix(connStr, "mysql://") || strings.Contains(connStr, "@tcp(") {
		driver = "mysql"
		dsn = strings.TrimPrefix(connStr, "mysql://")
		if !strings.Contains(dsn, "parseTime=true") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
	} else if strings.HasPrefix(connStr, "sqlite://") {
		dsn = strings.TrimPrefix(connStr, "sqlite://")
	}

	if driver == "sqlite" && !strings.Contains(dsn, "?") && dsn != ":memory:" {
		dsn += "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database (%s): %w", driver, err)
	}

	// Retry connection with backoff for MariaDB startup in Docker
	if driver == "mysql" {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
		for i := 0; i < 15; i++ {
			if err = db.Ping(); err == nil {
				break
			}
			log.Printf("[DB] Waiting for MariaDB connection... (%d/15): %v", i+1, err)
			time.Sleep(2 * time.Second)
		}
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to connect to MariaDB: %w", err)
		}
	} else {
		db.SetMaxOpenConns(1)
	}

	s := &Store{db: db, driver: driver}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	log.Printf("[DB] Storage initialized successfully using %s", strings.ToUpper(driver))
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	var queries []string

	if s.driver == "mysql" {
		queries = []string{
			`CREATE TABLE IF NOT EXISTS denizens (
				id VARCHAR(64) PRIMARY KEY,
				screen_name VARCHAR(64) UNIQUE NOT NULL,
				password_hash VARCHAR(255) NOT NULL DEFAULT '',
				secret VARCHAR(128) NOT NULL,
				gender INT NOT NULL DEFAULT 1,
				mission VARCHAR(255) NOT NULL DEFAULT 'Coke Music',
				avatar_def TEXT NOT NULL,
				decibels INT NOT NULL DEFAULT 5000,
				vcoins INT NOT NULL DEFAULT 500,
				last_seen_in VARCHAR(64) NOT NULL DEFAULT 'VIS$0',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS studios (
				id VARCHAR(64) PRIMARY KEY,
				owner_id VARCHAR(64) NOT NULL,
				owner_name VARCHAR(64) NOT NULL,
				name VARCHAR(128) NOT NULL,
				layout INT NOT NULL DEFAULT 3,
				room_type VARCHAR(64) NOT NULL DEFAULT 'studio_model_c',
				wallpaper INT DEFAULT 0,
				floor INT DEFAULT 0,
				doorbell INT DEFAULT 0,
				is_public TINYINT(1) DEFAULT 0
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS backpack_items (
				id INT AUTO_INCREMENT PRIMARY KEY,
				denizen_id VARCHAR(64) NOT NULL,
				catalog_item_id INT NOT NULL,
				properties_json TEXT NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS placed_furniture (
				id INT AUTO_INCREMENT PRIMARY KEY,
				studio_id VARCHAR(64) NOT NULL,
				catalog_item_id INT NOT NULL,
				pos_id VARCHAR(64) NOT NULL,
				x INT NOT NULL,
				y INT NOT NULL,
				z INT NOT NULL,
				ho INT NOT NULL DEFAULT 0,
				dir INT NOT NULL DEFAULT 2,
				state INT NOT NULL DEFAULT 0,
				iaot TINYINT(1) NOT NULL DEFAULT 0,
				properties_json TEXT NOT NULL,
				UNIQUE KEY uq_studio_pos (studio_id, pos_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS songs (
				id INT AUTO_INCREMENT PRIMARY KEY,
				denizen_id VARCHAR(64) NOT NULL,
				title VARCHAR(128) NOT NULL,
				artist VARCHAR(128) NOT NULL,
				edl_string TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS red_caps (
				pin_code VARCHAR(32) PRIMARY KEY,
				reward_vcoins INT NOT NULL DEFAULT 50,
				reward_item_id INT DEFAULT 0,
				redeemed_by VARCHAR(64),
				redeemed_at TIMESTAMP NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS messenger_friends (
				denizen_id VARCHAR(64) NOT NULL,
				friend_id VARCHAR(64) NOT NULL,
				status VARCHAR(32) NOT NULL DEFAULT 'accepted',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (denizen_id, friend_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS messenger_messages (
				id INT AUTO_INCREMENT PRIMARY KEY,
				sender_name VARCHAR(64) NOT NULL,
				recipient_name VARCHAR(64) NOT NULL,
				message TEXT NOT NULL,
				sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				is_read TINYINT(1) DEFAULT 0
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS daily_coke_drinks (
				denizen_id VARCHAR(64) NOT NULL,
				drink_date VARCHAR(16) NOT NULL,
				count INT NOT NULL DEFAULT 1,
				PRIMARY KEY (denizen_id, drink_date)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS top40_votes (
				id INT AUTO_INCREMENT PRIMARY KEY,
				creator_name VARCHAR(64) NOT NULL,
				song_title VARCHAR(128) NOT NULL,
				voter_name VARCHAR(64) NOT NULL,
				voted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS vegosan_streaks (
				denizen_id VARCHAR(64) PRIMARY KEY,
				streak INT NOT NULL DEFAULT 0,
				total_wins INT NOT NULL DEFAULT 0,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS sessions (
				token VARCHAR(128) PRIMARY KEY,
				denizen_id VARCHAR(64) NOT NULL,
				screen_name VARCHAR(64) NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				expires_at TIMESTAMP NOT NULL,
				INDEX idx_sessions_screen (screen_name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS purchase_tokens (
				token VARCHAR(128) PRIMARY KEY,
				denizen_id VARCHAR(64) NOT NULL,
				possession_id INT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		}
	} else {
		queries = []string{
			`CREATE TABLE IF NOT EXISTS denizens (
				id TEXT PRIMARY KEY,
				screen_name TEXT UNIQUE NOT NULL,
				password_hash TEXT DEFAULT '',
				secret TEXT NOT NULL,
				gender INTEGER NOT NULL DEFAULT 1,
				mission TEXT NOT NULL DEFAULT 'Coke Music',
				avatar_def TEXT NOT NULL,
				decibels INTEGER NOT NULL DEFAULT 5000,
				vcoins INTEGER NOT NULL DEFAULT 500,
				last_seen_in TEXT NOT NULL DEFAULT 'VIS$0',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS studios (
				id TEXT PRIMARY KEY,
				owner_id TEXT NOT NULL,
				owner_name TEXT NOT NULL,
				name TEXT NOT NULL,
				layout INTEGER NOT NULL DEFAULT 3,
				room_type TEXT NOT NULL DEFAULT 'studio_model_c',
				wallpaper INTEGER DEFAULT 0,
				floor INTEGER DEFAULT 0,
				doorbell INTEGER DEFAULT 0,
				is_public BOOLEAN DEFAULT 0
			);`,

			`CREATE TABLE IF NOT EXISTS backpack_items (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				denizen_id TEXT NOT NULL,
				catalog_item_id INTEGER NOT NULL,
				properties_json TEXT NOT NULL DEFAULT '{}'
			);`,

			`CREATE TABLE IF NOT EXISTS placed_furniture (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				studio_id TEXT NOT NULL,
				catalog_item_id INTEGER NOT NULL,
				pos_id TEXT NOT NULL,
				x INTEGER NOT NULL,
				y INTEGER NOT NULL,
				z INTEGER NOT NULL,
				ho INTEGER NOT NULL DEFAULT 0,
				dir INTEGER NOT NULL DEFAULT 2,
				state INTEGER NOT NULL DEFAULT 0,
				iaot BOOLEAN NOT NULL DEFAULT 0,
				properties_json TEXT NOT NULL DEFAULT '{}',
				UNIQUE(studio_id, pos_id)
			);`,

			`CREATE TABLE IF NOT EXISTS songs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				denizen_id TEXT NOT NULL,
				title TEXT NOT NULL,
				artist TEXT NOT NULL,
				edl_string TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS red_caps (
				pin_code TEXT PRIMARY KEY,
				reward_vcoins INTEGER NOT NULL DEFAULT 50,
				reward_item_id INTEGER DEFAULT 0,
				redeemed_by TEXT,
				redeemed_at TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS messenger_friends (
				denizen_id TEXT NOT NULL,
				friend_id TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'accepted',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (denizen_id, friend_id)
			);`,

			`CREATE TABLE IF NOT EXISTS messenger_messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				sender_name TEXT NOT NULL,
				recipient_name TEXT NOT NULL,
				message TEXT NOT NULL,
				sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				is_read BOOLEAN DEFAULT 0
			);`,

			`CREATE TABLE IF NOT EXISTS daily_coke_drinks (
				denizen_id TEXT NOT NULL,
				drink_date TEXT NOT NULL,
				count INTEGER NOT NULL DEFAULT 1,
				PRIMARY KEY (denizen_id, drink_date)
			);`,

			`CREATE TABLE IF NOT EXISTS top40_votes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				creator_name TEXT NOT NULL,
				song_title TEXT NOT NULL,
				voter_name TEXT NOT NULL,
				voted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS vegosan_streaks (
				denizen_id TEXT PRIMARY KEY,
				streak INTEGER NOT NULL DEFAULT 0,
				total_wins INTEGER NOT NULL DEFAULT 0,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS sessions (
				token TEXT PRIMARY KEY,
				denizen_id TEXT NOT NULL,
				screen_name TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				expires_at TIMESTAMP NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_screen ON sessions(screen_name);`,

			`CREATE TABLE IF NOT EXISTS purchase_tokens (
				token TEXT PRIMARY KEY,
				denizen_id TEXT NOT NULL,
				possession_id INTEGER NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);`,
		}
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration query failed: %w (%s)", err, q)
		}
	}

	// Safe column migration for existing databases
	if s.driver == "mysql" {
		_, _ = s.db.Exec("ALTER TABLE denizens ADD COLUMN password_hash VARCHAR(255) NOT NULL DEFAULT ''")
	} else {
		_, _ = s.db.Exec("ALTER TABLE denizens ADD COLUMN password_hash TEXT DEFAULT ''")
	}

	// Seed public default studios
	defaultStudios := []StudioRecord{
		{ID: "VIS$0", OwnerID: "system", OwnerName: "Coke Studios", Name: "Central Park Studio", Layout: 3, RoomType: "studio_model_c", IsPublic: true},
		{ID: "VIS$1", OwnerID: "system", OwnerName: "Coke Studios", Name: "San Francisco Studio", Layout: 3, RoomType: "studio_model_c", IsPublic: true},
		{ID: "VIS$2", OwnerID: "system", OwnerName: "Coke Studios", Name: "London Studio", Layout: 3, RoomType: "studio_model_c", IsPublic: true},
		{ID: "studio_1", OwnerID: "system", OwnerName: "Adam", Name: "Private Studio #1", Layout: 3, RoomType: "studio_model_c", IsPublic: false},
	}

	for _, st := range defaultStudios {
		query := `INSERT OR IGNORE INTO studios (id, owner_id, owner_name, name, layout, room_type, is_public) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`
		if s.driver == "mysql" {
			query = `INSERT IGNORE INTO studios (id, owner_id, owner_name, name, layout, room_type, is_public) 
				VALUES (?, ?, ?, ?, ?, ?, ?)`
		}
		_, _ = s.db.Exec(query, st.ID, st.OwnerID, st.OwnerName, st.Name, st.Layout, st.RoomType, st.IsPublic)
	}

	// Seed starter red caps promotion codes if empty
	sampleCaps := []string{"COKE-STUDIOS-2026", "SAINTS-GAMING", "RED-CAP-BEATS-01", "MYCOKE-RETRO-VIP", "SUMMER-CHILL-99"}
	for _, capCode := range sampleCaps {
		query := `INSERT OR IGNORE INTO red_caps (pin_code, reward_vcoins, reward_item_id) VALUES (?, 100, 52)`
		if s.driver == "mysql" {
			query = `INSERT IGNORE INTO red_caps (pin_code, reward_vcoins, reward_item_id) VALUES (?, 100, 52)`
		}
		_, _ = s.db.Exec(query, capCode)
	}

	return nil
}

func (s *Store) GetOrCreateDenizen(screenName, gender, avatarDef, mission string) (*DenizenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanName := strings.TrimSpace(screenName)
	if cleanName == "" {
		cleanName = "Adam"
	}

	var d DenizenRecord
	row := s.db.QueryRow(`SELECT id, screen_name, COALESCE(password_hash, ''), secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in 
		FROM denizens WHERE LOWER(screen_name) = LOWER(?)`, cleanName)

	err := row.Scan(&d.ID, &d.ScreenName, &d.PasswordHash, &d.Secret, &d.Gender, &d.Mission, &d.AvatarDef, &d.Decibels, &d.VCoins, &d.LastSeenIn)
	if err == nil {
		newSecret := "coke_secret_" + fmt.Sprintf("%x", time.Now().UnixNano())
		_, _ = s.db.Exec("UPDATE denizens SET secret = ? WHERE id = ?", newSecret, d.ID)
		d.Secret = newSecret
		return &d, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	newSeq := atomic.AddInt64(&denizenSeq, 1)
	newID := fmt.Sprintf("usr_%d_%d", time.Now().UnixNano(), newSeq)
	newSecret := fmt.Sprintf("coke_secret_%x_%d", time.Now().UnixNano(), newSeq)
	g := 1
	if strings.ToUpper(gender) == "F" || gender == "2" {
		g = 2
	}
	if avatarDef == "" {
		avatarDef = "ch=01/01/01&fa=01/01/01&cl=01/01/01"
	}
	if mission == "" {
		mission = "Coke Music Reborn"
	}

	d = DenizenRecord{
		ID:           newID,
		ScreenName:   cleanName,
		PasswordHash: "",
		Secret:       newSecret,
		Gender:       g,
		Mission:      mission,
		AvatarDef:    avatarDef,
		Decibels:     5000,
		VCoins:       500,
		LastSeenIn:   "VIS$0",
	}

	_, err = s.db.Exec(`INSERT INTO denizens 
		(id, screen_name, password_hash, secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ScreenName, d.PasswordHash, d.Secret, d.Gender, d.Mission, d.AvatarDef, d.Decibels, d.VCoins, d.LastSeenIn)
	if err != nil {
		return nil, err
	}

	log.Printf("[DB] Created new denizen: %s (ID: %s, Starting dB: %d, Starting V-Coins: %d)", d.ScreenName, d.ID, d.Decibels, d.VCoins)
	return &d, nil
}

func (s *Store) RegisterDenizen(screenName, password, gender, avatarDef, mission string) (*DenizenRecord, error) {
	cleanName := strings.TrimSpace(screenName)
	if cleanName == "" {
		return nil, errors.New("screen name is required")
	}
	if password == "" {
		return nil, errors.New("password is required")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var existingID string
	err = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", cleanName).Scan(&existingID)
	if err == nil {
		return nil, ErrDenizenExists
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	newSeq := atomic.AddInt64(&denizenSeq, 1)
	newID := fmt.Sprintf("usr_%d_%d", time.Now().UnixNano(), newSeq)
	newSecret := fmt.Sprintf("coke_secret_%x_%d", time.Now().UnixNano(), newSeq)
	g := 1
	if strings.ToUpper(gender) == "F" || gender == "2" {
		g = 2
	}
	if avatarDef == "" {
		avatarDef = "ch=01/01/01&fa=01/01/01&cl=01/01/01"
	}
	if mission == "" {
		mission = "Coke Music"
	}

	d := DenizenRecord{
		ID:           newID,
		ScreenName:   cleanName,
		PasswordHash: hash,
		Secret:       newSecret,
		Gender:       g,
		Mission:      mission,
		AvatarDef:    avatarDef,
		Decibels:     5000,
		VCoins:       500,
		LastSeenIn:   "VIS$0",
	}

	_, err = s.db.Exec(`INSERT INTO denizens 
		(id, screen_name, password_hash, secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ScreenName, d.PasswordHash, d.Secret, d.Gender, d.Mission, d.AvatarDef, d.Decibels, d.VCoins, d.LastSeenIn)
	if err != nil {
		return nil, err
	}

	log.Printf("[DB] Registered new denizen: %s (ID: %s)", d.ScreenName, d.ID)
	return &d, nil
}

func (s *Store) VerifyCredentials(screenName, password string) (*DenizenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanName := strings.TrimSpace(screenName)
	if cleanName == "" {
		return nil, ErrInvalidCredentials
	}

	var d DenizenRecord
	row := s.db.QueryRow(`SELECT id, screen_name, COALESCE(password_hash, ''), secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in 
		FROM denizens WHERE LOWER(screen_name) = LOWER(?)`, cleanName)

	if err := row.Scan(&d.ID, &d.ScreenName, &d.PasswordHash, &d.Secret, &d.Gender, &d.Mission, &d.AvatarDef, &d.Decibels, &d.VCoins, &d.LastSeenIn); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if d.PasswordHash != "" {
		if !VerifyPassword(password, d.PasswordHash) {
			return nil, ErrInvalidCredentials
		}
	} else {
		// Existing prototype account without password: only allowed if password is empty
		if password != "" {
			return nil, ErrInvalidCredentials
		}
	}

	return &d, nil
}

func (s *Store) GetDenizenByName(screenName string) (*DenizenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var d DenizenRecord
	row := s.db.QueryRow(`SELECT id, screen_name, COALESCE(password_hash, ''), secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in 
		FROM denizens WHERE LOWER(screen_name) = LOWER(?)`, strings.TrimSpace(screenName))
	if err := row.Scan(&d.ID, &d.ScreenName, &d.PasswordHash, &d.Secret, &d.Gender, &d.Mission, &d.AvatarDef, &d.Decibels, &d.VCoins, &d.LastSeenIn); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDenizenByID(id string) (*DenizenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var d DenizenRecord
	row := s.db.QueryRow(`SELECT id, screen_name, COALESCE(password_hash, ''), secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in 
		FROM denizens WHERE id = ?`, strings.TrimSpace(id))
	if err := row.Scan(&d.ID, &d.ScreenName, &d.PasswordHash, &d.Secret, &d.Gender, &d.Mission, &d.AvatarDef, &d.Decibels, &d.VCoins, &d.LastSeenIn); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDenizenBySecret(secret string) (*DenizenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var d DenizenRecord
	row := s.db.QueryRow(`SELECT id, screen_name, COALESCE(password_hash, ''), secret, gender, mission, avatar_def, decibels, vcoins, last_seen_in 
		FROM denizens WHERE secret = ?`, strings.TrimSpace(secret))
	if err := row.Scan(&d.ID, &d.ScreenName, &d.PasswordHash, &d.Secret, &d.Gender, &d.Mission, &d.AvatarDef, &d.Decibels, &d.VCoins, &d.LastSeenIn); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) CreateSession(denizenID, screenName string, duration time.Duration) (*SessionRecord, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}
	if duration == 0 {
		duration = defaultSessionTTL
	}
	now := time.Now().UTC()
	expiresAt := now.Add(duration)

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO sessions (token, denizen_id, screen_name, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`
	_, err = s.db.Exec(query, token, denizenID, screenName, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to insert session: %w", err)
	}

	return &SessionRecord{
		Token:      token,
		DenizenID:  denizenID,
		ScreenName: screenName,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Store) GetSession(token string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sess SessionRecord
	var createdStr, expiresStr string
	row := s.db.QueryRow(`SELECT token, denizen_id, screen_name, created_at, expires_at FROM sessions WHERE token = ?`, strings.TrimSpace(token))
	if err := row.Scan(&sess.Token, &sess.DenizenID, &sess.ScreenName, &createdStr, &expiresStr); err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expiresStr)
	if sess.ExpiresAt.IsZero() {
		sess.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
	}
	return &sess, nil
}

func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, strings.TrimSpace(token))
	return err
}

func (s *Store) CleanExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	return err
}

func (s *Store) UpdateDenizenProfile(screenName string, gender int, avatarDef, mission string) (*DenizenRecord, error) {
	cleanName := strings.TrimSpace(screenName)
	if cleanName == "" {
		cleanName = "Adam"
	}

	d, err := s.GetOrCreateDenizen(cleanName, fmt.Sprintf("%d", gender), avatarDef, mission)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE denizens SET gender = ?, avatar_def = ?, mission = ? WHERE id = ?`
	if _, err := s.db.Exec(query, gender, avatarDef, mission, d.ID); err != nil {
		return nil, err
	}

	d.Gender = gender
	d.AvatarDef = avatarDef
	d.Mission = mission
	return d, nil
}

func (s *Store) GetBalance(screenName string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var db, vc int
	row := s.db.QueryRow("SELECT decibels, vcoins FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName))
	if err := row.Scan(&db, &vc); err != nil {
		return 5000, 500, nil
	}
	return db, vc, nil
}

func (s *Store) AdjustBalance(screenName string, decibelsDelta, vcoinsDelta int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE denizens 
		SET decibels = CASE WHEN decibels + ? < 0 THEN 0 ELSE decibels + ? END,
		    vcoins = CASE WHEN vcoins + ? < 0 THEN 0 ELSE vcoins + ? END
		WHERE LOWER(screen_name) = LOWER(?)`
	_, err := s.db.Exec(query, decibelsDelta, decibelsDelta, vcoinsDelta, vcoinsDelta, strings.TrimSpace(screenName))
	if err != nil {
		return 0, err
	}

	var newDB int
	_ = s.db.QueryRow("SELECT decibels FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&newDB)
	return newDB, nil
}

func (s *Store) RedeemRedCap(screenName, pinCode string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanCode := strings.ToUpper(strings.TrimSpace(pinCode))
	var rewardVC, rewardItem int
	var redeemedBy sql.NullString

	row := s.db.QueryRow("SELECT reward_vcoins, reward_item_id, redeemed_by FROM red_caps WHERE pin_code = ?", cleanCode)
	if err := row.Scan(&rewardVC, &rewardItem, &redeemedBy); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("invalid or non-existent bottle cap PIN code")
		}
		return 0, 0, err
	}

	if redeemedBy.Valid && redeemedBy.String != "" {
		return 0, 0, fmt.Errorf("this bottle cap has already been redeemed by %s", redeemedBy.String)
	}

	// Mark redeemed
	_, err := s.db.Exec("UPDATE red_caps SET redeemed_by = ?, redeemed_at = CURRENT_TIMESTAMP WHERE pin_code = ?",
		screenName, cleanCode)
	if err != nil {
		return 0, 0, err
	}

	// Credit V-Coins
	_, _ = s.db.Exec("UPDATE denizens SET vcoins = vcoins + ? WHERE LOWER(screen_name) = LOWER(?)", rewardVC, strings.TrimSpace(screenName))

	// If cap carries a furniture reward (e.g. Vending machine), grant it
	if rewardItem > 0 {
		var denizenID string
		_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
		if denizenID != "" {
			_, _ = s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, '{}')",
				denizenID, rewardItem)
		}
	}

	return rewardVC, rewardItem, nil
}

func (s *Store) GetBackpack(screenName string) ([]BackpackItemRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query("SELECT id, catalog_item_id, properties_json FROM backpack_items WHERE denizen_id = ? ORDER BY id ASC", denizenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []BackpackItemRecord
	for rows.Next() {
		var it BackpackItemRecord
		var pJson string
		if err := rows.Scan(&it.ID, &it.CatalogItemID, &pJson); err == nil {
			it.DenizenID = denizenID
			it.Properties = make(map[string]any)
			_ = json.Unmarshal([]byte(pJson), &it.Properties)
			items = append(items, it)
		}
	}
	return items, nil
}

func (s *Store) AddBackpackItem(screenName string, catalogItemID int, properties map[string]any) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return 0, err
	}

	pJson, _ := json.Marshal(properties)
	res, err := s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?)",
		denizenID, catalogItemID, string(pJson))
	if err != nil {
		return 0, err
	}
	lid, _ := res.LastInsertId()
	return int(lid), nil
}

func (s *Store) RemoveBackpackItem(itemID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM backpack_items WHERE id = ?", itemID)
	return err
}

func (s *Store) PurchaseItemAtomic(screenName string, catalogItemID, price int, properties map[string]any, idempotencyKey string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	var currentDB int
	err := s.db.QueryRow("SELECT id, decibels FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID, &currentDB)
	if err != nil {
		return 0, 0, err
	}

	// Idempotency check: if token was already processed for this denizen, return existing possession without charging again!
	if idempotencyKey != "" {
		var existingPosID int
		err := s.db.QueryRow("SELECT possession_id FROM purchase_tokens WHERE token = ? AND denizen_id = ?", idempotencyKey, denizenID).Scan(&existingPosID)
		if err == nil && existingPosID > 0 {
			return existingPosID, currentDB, nil
		}
	}

	if currentDB < price {
		return 0, currentDB, ErrInsufficientFunds
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// Atomic deduction with conditional check
	res, err := tx.Exec("UPDATE denizens SET decibels = decibels - ? WHERE id = ? AND decibels >= ?", price, denizenID, price)
	if err != nil {
		return 0, 0, err
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		return 0, currentDB, ErrInsufficientFunds
	}

	if properties == nil {
		properties = make(map[string]any)
	}
	pJson, _ := json.Marshal(properties)

	res, err = tx.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?)",
		denizenID, catalogItemID, string(pJson))
	if err != nil {
		return 0, 0, err
	}
	lid, _ := res.LastInsertId()
	posID := int(lid)

	if idempotencyKey != "" {
		_, _ = tx.Exec("INSERT INTO purchase_tokens (token, denizen_id, possession_id) VALUES (?, ?, ?)",
			idempotencyKey, denizenID, posID)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}

	newBalance := currentDB - price
	return posID, newBalance, nil
}

func (s *Store) PlaceBackpackItem(studioID, ownerName, posID string, catalogItemID, x, y, z, ho, dir, state int, iaot bool, properties map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Verify studio exists and player is the owner
	var studioOwner string
	var isPublic bool
	err := s.db.QueryRow("SELECT owner_name, is_public FROM studios WHERE id = ?", studioID).Scan(&studioOwner, &isPublic)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrStudioNotFound
		}
		return err
	}
	if isPublic {
		return ErrNotStudioOwner
	}
	if !strings.EqualFold(strings.TrimSpace(studioOwner), strings.TrimSpace(ownerName)) {
		return ErrNotStudioOwner
	}

	// 2. Check if item is already placed in this studio (reposition / rotate)
	var existingPlacedID int
	err = s.db.QueryRow("SELECT id FROM placed_furniture WHERE studio_id = ? AND pos_id = ?", studioID, posID).Scan(&existingPlacedID)
	if err == nil && existingPlacedID > 0 {
		pJson, _ := json.Marshal(properties)
		_, err = s.db.Exec(`UPDATE placed_furniture 
			SET x = ?, y = ?, z = ?, ho = ?, dir = ?, state = ?, iaot = ?, properties_json = ? 
			WHERE studio_id = ? AND pos_id = ?`,
			x, y, z, ho, dir, state, iaot, string(pJson), studioID, posID)
		return err
	}

	// 3. New placement from backpack
	var denizenID string
	err = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(ownerName)).Scan(&denizenID)
	if err != nil {
		return err
	}

	intPosID, _ := strconv.Atoi(posID)
	var bpCatalogItemID int
	var bpPropsJson string
	if intPosID > 0 {
		err = s.db.QueryRow("SELECT catalog_item_id, properties_json FROM backpack_items WHERE id = ? AND denizen_id = ?",
			intPosID, denizenID).Scan(&bpCatalogItemID, &bpPropsJson)
	} else {
		err = sql.ErrNoRows
	}
	if err != nil {
		return ErrItemNotOwned
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete from backpack
	_, err = tx.Exec("DELETE FROM backpack_items WHERE id = ? AND denizen_id = ?", intPosID, denizenID)
	if err != nil {
		return err
	}

	pJson := bpPropsJson
	if properties != nil && len(properties) > 0 {
		m, _ := json.Marshal(properties)
		pJson = string(m)
	}
	if catalogItemID == 0 {
		catalogItemID = bpCatalogItemID
	}

	_, err = tx.Exec(`INSERT INTO placed_furniture 
		(studio_id, catalog_item_id, pos_id, x, y, z, ho, dir, state, iaot, properties_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		studioID, catalogItemID, posID, x, y, z, ho, dir, state, iaot, pJson)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) PickupFurnitureToBackpack(studioID, ownerName, posID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Verify studio exists and player is the owner
	var studioOwner string
	var isPublic bool
	err := s.db.QueryRow("SELECT owner_name, is_public FROM studios WHERE id = ?", studioID).Scan(&studioOwner, &isPublic)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrStudioNotFound
		}
		return err
	}
	if isPublic {
		return ErrNotStudioOwner
	}
	if !strings.EqualFold(strings.TrimSpace(studioOwner), strings.TrimSpace(ownerName)) {
		return ErrNotStudioOwner
	}

	// 2. Locate placed furniture
	var catalogItemID int
	var propertiesJson string
	err = s.db.QueryRow("SELECT catalog_item_id, properties_json FROM placed_furniture WHERE studio_id = ? AND pos_id = ?",
		studioID, posID).Scan(&catalogItemID, &propertiesJson)
	if err != nil {
		return fmt.Errorf("furniture not found in studio")
	}

	var denizenID string
	err = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(ownerName)).Scan(&denizenID)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete from placed_furniture
	_, err = tx.Exec("DELETE FROM placed_furniture WHERE studio_id = ? AND pos_id = ?", studioID, posID)
	if err != nil {
		return err
	}

	// Reinsert into backpack preserving original ID if numeric
	intPosID, _ := strconv.Atoi(posID)
	if intPosID > 0 {
		_, err = tx.Exec("INSERT INTO backpack_items (id, denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?, ?)",
			intPosID, denizenID, catalogItemID, propertiesJson)
	} else {
		_, err = tx.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?)",
			denizenID, catalogItemID, propertiesJson)
	}
	if err != nil {
		_, err = tx.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?)",
			denizenID, catalogItemID, propertiesJson)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetRoomFurniture(studioID string) ([]PlacedFurniRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id, catalog_item_id, pos_id, x, y, z, ho, dir, state, iaot, properties_json 
		FROM placed_furniture WHERE studio_id = ? ORDER BY id ASC`, studioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PlacedFurniRecord
	for rows.Next() {
		var pf PlacedFurniRecord
		var pJson string
		if err := rows.Scan(&pf.ID, &pf.CatalogItemID, &pf.PosID, &pf.X, &pf.Y, &pf.Z, &pf.HO, &pf.Dir, &pf.State, &pf.IAOT, &pJson); err == nil {
			pf.StudioID = studioID
			pf.Properties = make(map[string]any)
			_ = json.Unmarshal([]byte(pJson), &pf.Properties)
			list = append(list, pf)
		}
	}
	return list, nil
}

func (s *Store) PlaceFurniture(studioID, posID string, catalogItemID, x, y, z, ho, dir, state int, iaot bool, properties map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pJson, _ := json.Marshal(properties)
	var query string
	if s.driver == "mysql" {
		query = `INSERT INTO placed_furniture 
			(studio_id, catalog_item_id, pos_id, x, y, z, ho, dir, state, iaot, properties_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE 
				catalog_item_id = VALUES(catalog_item_id),
				x = VALUES(x), y = VALUES(y), z = VALUES(z),
				ho = VALUES(ho), dir = VALUES(dir), state = VALUES(state),
				iaot = VALUES(iaot), properties_json = VALUES(properties_json)`
	} else {
		query = `INSERT INTO placed_furniture 
			(studio_id, catalog_item_id, pos_id, x, y, z, ho, dir, state, iaot, properties_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(studio_id, pos_id) DO UPDATE SET
				catalog_item_id = excluded.catalog_item_id,
				x = excluded.x, y = excluded.y, z = excluded.z,
				ho = excluded.ho, dir = excluded.dir, state = excluded.state,
				iaot = excluded.iaot, properties_json = excluded.properties_json`
	}

	_, err := s.db.Exec(query, studioID, catalogItemID, posID, x, y, z, ho, dir, state, iaot, string(pJson))
	return err
}

func (s *Store) RemoveFurniture(studioID, posID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM placed_furniture WHERE studio_id = ? AND pos_id = ?", studioID, posID)
	return err
}

func (s *Store) RotateFurniture(studioID, posID string, dir, state, ho int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE placed_furniture 
		SET dir = ?, state = ?, ho = ? 
		WHERE studio_id = ? AND pos_id = ?`,
		dir, state, ho, studioID, posID)
	return err
}

func (s *Store) GetStudios() ([]StudioRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT id, owner_id, owner_name, name, layout, room_type, wallpaper, floor, doorbell, is_public FROM studios ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []StudioRecord
	for rows.Next() {
		var st StudioRecord
		if err := rows.Scan(&st.ID, &st.OwnerID, &st.OwnerName, &st.Name, &st.Layout, &st.RoomType, &st.Wallpaper, &st.Floor, &st.Doorbell, &st.IsPublic); err == nil {
			list = append(list, st)
		}
	}
	return list, nil
}

func (s *Store) GetStudio(studioID string) (*StudioRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var st StudioRecord
	row := s.db.QueryRow("SELECT id, owner_id, owner_name, name, layout, room_type, wallpaper, floor, doorbell, is_public FROM studios WHERE id = ?", strings.TrimSpace(studioID))
	if err := row.Scan(&st.ID, &st.OwnerID, &st.OwnerName, &st.Name, &st.Layout, &st.RoomType, &st.Wallpaper, &st.Floor, &st.Doorbell, &st.IsPublic); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) GetFriends(screenName string) ([]FriendRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return nil, err
	}

	query := `SELECT d.screen_name, d.gender, d.mission, d.avatar_def, d.last_seen_in, mf.status
		FROM messenger_friends mf
		JOIN denizens d ON d.id = mf.friend_id
		WHERE mf.denizen_id = ?
		ORDER BY d.screen_name ASC`
	rows, err := s.db.Query(query, denizenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []FriendRecord
	for rows.Next() {
		var f FriendRecord
		if err := rows.Scan(&f.ScreenName, &f.Gender, &f.Mission, &f.AvatarDef, &f.LastSeenIn, &f.Status); err == nil {
			f.Online = true
			friends = append(friends, f)
		}
	}
	return friends, nil
}

func (s *Store) AddFriend(user1, user2 string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id1, id2 string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user1)).Scan(&id1)
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user2)).Scan(&id2)
	if id1 == "" || id2 == "" || id1 == id2 {
		return fmt.Errorf("invalid friend users")
	}

	q1 := "INSERT OR IGNORE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
	q2 := "INSERT OR IGNORE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
	if s.driver == "mysql" {
		q1 = "INSERT IGNORE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
		q2 = "INSERT IGNORE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
	}
	_, _ = s.db.Exec(q1, id1, id2)
	_, _ = s.db.Exec(q2, id2, id1)
	return nil
}

func (s *Store) RemoveFriend(user1, user2 string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id1, id2 string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user1)).Scan(&id1)
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user2)).Scan(&id2)
	if id1 == "" || id2 == "" {
		return nil
	}
	_, _ = s.db.Exec("DELETE FROM messenger_friends WHERE (denizen_id = ? AND friend_id = ?) OR (denizen_id = ? AND friend_id = ?)",
		id1, id2, id2, id1)
	return nil
}

func (s *Store) SendMessage(sender, recipient, message string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec("INSERT INTO messenger_messages (sender_name, recipient_name, message) VALUES (?, ?, ?)",
		strings.TrimSpace(sender), strings.TrimSpace(recipient), message)
	if err != nil {
		return 0, err
	}
	lid, _ := res.LastInsertId()
	return int(lid), nil
}

func (s *Store) GetMessages(recipient string, unreadOnly bool) ([]MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "SELECT id, sender_name, recipient_name, message, sent_at, is_read FROM messenger_messages WHERE LOWER(recipient_name) = LOWER(?)"
	if unreadOnly {
		query += " AND is_read = 0"
	}
	query += " ORDER BY id DESC"

	rows, err := s.db.Query(query, strings.TrimSpace(recipient))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.ID, &m.SenderName, &m.RecipientName, &m.Message, &m.SentAt, &m.IsRead); err == nil {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

func (s *Store) MarkMessageRead(messageID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE messenger_messages SET is_read = 1 WHERE id = ?", messageID)
	return err
}

func (s *Store) UpdateStudioDecor(studioID string, wallpaper, floor, doorbell int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := "UPDATE studios SET wallpaper = ?, floor = ?, doorbell = ? WHERE id = ?"
	_, err := s.db.Exec(query, wallpaper, floor, doorbell, studioID)
	return err
}

func (s *Store) CreateStudio(studioID, ownerID, ownerName, studioName, roomType string, layout int) (*StudioRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if studioID == "" {
		studioID = fmt.Sprintf("studio_%s", strings.ToLower(strings.TrimSpace(ownerName)))
	}
	if studioName == "" {
		studioName = fmt.Sprintf("%s's Studio", ownerName)
	}
	if roomType == "" {
		roomType = "studio_model_c"
	}
	if layout <= 0 {
		layout = 3
	}

	var query string
	if s.driver == "mysql" {
		query = `INSERT INTO studios (id, owner_id, owner_name, name, layout, room_type, is_public) 
			VALUES (?, ?, ?, ?, ?, ?, 0) 
			ON DUPLICATE KEY UPDATE name = VALUES(name), room_type = VALUES(room_type), layout = VALUES(layout)`
	} else {
		query = `INSERT INTO studios (id, owner_id, owner_name, name, layout, room_type, is_public) 
			VALUES (?, ?, ?, ?, ?, ?, 0) 
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, room_type = excluded.room_type, layout = excluded.layout`
	}

	_, err := s.db.Exec(query, studioID, ownerID, ownerName, studioName, layout, roomType)
	if err != nil {
		return nil, err
	}

	return &StudioRecord{
		ID:        studioID,
		OwnerID:   ownerID,
		OwnerName: ownerName,
		Name:      studioName,
		Layout:    layout,
		RoomType:  roomType,
		IsPublic:  false,
	}, nil
}


func (s *Store) RecordDailyCokeDrink(screenName string) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return 0, false, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	var count int
	err = s.db.QueryRow("SELECT count FROM daily_coke_drinks WHERE denizen_id = ? AND drink_date = ?", denizenID, today).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}

	if err == sql.ErrNoRows {
		count = 0
	}

	if count >= 10 {
		return count, false, nil
	}

	count++
	if s.driver == "mysql" {
		_, err = s.db.Exec(`INSERT INTO daily_coke_drinks (denizen_id, drink_date, count) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE count = ?`, denizenID, today, count, count)
	} else {
		_, err = s.db.Exec(`INSERT INTO daily_coke_drinks (denizen_id, drink_date, count) VALUES (?, ?, ?)
			ON CONFLICT(denizen_id, drink_date) DO UPDATE SET count = excluded.count`, denizenID, today, count)
	}
	if err != nil {
		return 0, false, err
	}

	// Award 10 dB per drink
	_, _ = s.db.Exec("UPDATE denizens SET decibels = decibels + 10 WHERE id = ?", denizenID)
	return count, true, nil
}

func (s *Store) RecordChartVote(creatorName, songTitle, voterName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT INTO top40_votes (creator_name, song_title, voter_name) VALUES (?, ?, ?)",
		strings.TrimSpace(creatorName), strings.TrimSpace(songTitle), strings.TrimSpace(voterName))
	return err
}

func (s *Store) GetTop40(period string) ([]Top40Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	timeFilter := "datetime(voted_at) >= datetime('now', '-1 day')"
	if s.driver == "mysql" {
		timeFilter = "voted_at >= NOW() - INTERVAL 1 DAY"
		if strings.ToLower(period) == "weekly" {
			timeFilter = "voted_at >= NOW() - INTERVAL 7 DAY"
		} else if strings.ToLower(period) == "monthly" {
			timeFilter = "voted_at >= NOW() - INTERVAL 30 DAY"
		}
	} else {
		if strings.ToLower(period) == "weekly" {
			timeFilter = "datetime(voted_at) >= datetime('now', '-7 days')"
		} else if strings.ToLower(period) == "monthly" {
			timeFilter = "datetime(voted_at) >= datetime('now', '-30 days')"
		}
	}

	query := fmt.Sprintf(`SELECT creator_name, song_title, COUNT(*) as vote_count 
		FROM top40_votes 
		WHERE %s 
		GROUP BY creator_name, song_title 
		ORDER BY vote_count DESC LIMIT 40`, timeFilter)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Top40Entry
	rank := 1
	for rows.Next() {
		var e Top40Entry
		if err := rows.Scan(&e.CreatorName, &e.SongTitle, &e.Votes); err == nil {
			e.Rank = rank
			if rank == 1 && strings.ToLower(period) == "weekly" {
				e.Award = "Gold Record"
			} else if rank == 1 && strings.ToLower(period) == "monthly" {
				e.Award = "Platinum Record"
			}
			list = append(list, e)
			rank++
		}
	}

	// If empty, return sample authentic charts
	if len(list) == 0 {
		list = []Top40Entry{
			{Rank: 1, SongTitle: "Cherry Coke Grooves", CreatorName: "DJ-Cherry", Votes: 42, Award: "Gold Record"},
			{Rank: 2, SongTitle: "Neon Nights (Saints Remix)", CreatorName: "Adam", Votes: 38},
			{Rank: 3, SongTitle: "Red Room Anthem", CreatorName: "CokeMaster", Votes: 29},
			{Rank: 4, SongTitle: "Rooftop Sunset Beat", CreatorName: "SummerVibe", Votes: 21},
		}
	}

	return list, nil
}

func (s *Store) ProcessVegoSanMove(screenName, playerMove string) (*VegoSanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)

	pMove := strings.ToLower(strings.TrimSpace(playerMove))
	validMoves := []string{"slap", "grab", "thrust", "sweep", "block"}
	valid := false
	for _, m := range validMoves {
		if m == pMove {
			valid = true
			break
		}
	}
	if !valid {
		pMove = "slap"
	}

	// 5-move circular counter rules:
	// slap beats sweep & block
	// grab beats slap & thrust
	// thrust beats slap & sweep
	// sweep beats grab & block
	// block beats grab & thrust
	aiMove := validMoves[time.Now().UnixNano()%int64(len(validMoves))]

	res := &VegoSanResult{
		PlayerMove:   pMove,
		OpponentMove: aiMove,
	}

	if pMove == aiMove {
		res.Draw = true
		res.Message = fmt.Sprintf("Both chose %s! It's a standoff!", pMove)
	} else {
		win := false
		switch pMove {
		case "slap":
			win = (aiMove == "sweep" || aiMove == "block")
		case "grab":
			win = (aiMove == "slap" || aiMove == "thrust")
		case "thrust":
			win = (aiMove == "slap" || aiMove == "sweep")
		case "sweep":
			win = (aiMove == "grab" || aiMove == "block")
		case "block":
			win = (aiMove == "grab" || aiMove == "thrust")
		}

		res.Win = win
		if win {
			res.Message = fmt.Sprintf("Victory! Your %s counters V-ego San's %s!", pMove, aiMove)
		} else {
			res.Message = fmt.Sprintf("Defeat! V-ego San's %s overpowered your %s!", aiMove, pMove)
		}
	}

	if denizenID != "" {
		var streak, totalWins int
		_ = s.db.QueryRow("SELECT streak, total_wins FROM vegosan_streaks WHERE denizen_id = ?", denizenID).Scan(&streak, &totalWins)
		if res.Win {
			streak++
			totalWins++
			rewardDB := streak * 10
			res.AwardedDB = rewardDB
			res.Streak = streak
			_, _ = s.db.Exec("UPDATE denizens SET decibels = decibels + ? WHERE id = ?", rewardDB, denizenID)

			// Streak milestone furniture prizes
			if streak == 3 {
				res.PrizeItem = "Tatami Mat"
				res.PrizeItemID = 93
				_, _ = s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, 93, '{}')", denizenID)
			} else if streak == 5 {
				res.PrizeItem = "Rice Paper Divider"
				res.PrizeItemID = 94
				_, _ = s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, 94, '{}')", denizenID)
			} else if streak == 10 {
				res.PrizeItem = "Ceremonial Gong"
				res.PrizeItemID = 95
				_, _ = s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, 95, '{}')", denizenID)
			}
		} else if !res.Draw {
			streak = 0
			res.Streak = 0
		}

		if s.driver == "mysql" {
			_, _ = s.db.Exec(`INSERT INTO vegosan_streaks (denizen_id, streak, total_wins, updated_at) 
				VALUES (?, ?, ?, CURRENT_TIMESTAMP) 
				ON DUPLICATE KEY UPDATE streak = VALUES(streak), total_wins = VALUES(total_wins), updated_at = CURRENT_TIMESTAMP`,
				denizenID, streak, totalWins)
		} else {
			_, _ = s.db.Exec(`INSERT INTO vegosan_streaks (denizen_id, streak, total_wins, updated_at) 
				VALUES (?, ?, ?, CURRENT_TIMESTAMP) 
				ON CONFLICT(denizen_id) DO UPDATE SET streak = excluded.streak, total_wins = excluded.total_wins, updated_at = CURRENT_TIMESTAMP`,
				denizenID, streak, totalWins)
		}
	}

	return res, nil
}
