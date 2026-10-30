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

	"gocoke/internal/client/minigames/sumo"
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
	Status        string `json:"status"`
	Result        string `json:"result"`
	Win           bool   `json:"win"`
	Draw          bool   `json:"draw"`
	PlayerMove    string `json:"player_move"`
	OpponentMove  string `json:"opponent_move"`
	MasterMove    string `json:"masterMove"`
	Commentary    string `json:"commentary"`
	Message       string `json:"message"`
	Streak        int    `json:"streak"`
	AwardedDB     int    `json:"awarded_db"`
	TotalDecibels int    `json:"totalDecibels"`
	PrizeItem     string `json:"prize_item,omitempty"`
	PrizeItemID   int    `json:"prize_item_id,omitempty"`
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

			`CREATE TABLE IF NOT EXISTS call_for_help (
				id INT AUTO_INCREMENT PRIMARY KEY,
				reporter_name VARCHAR(64) NOT NULL,
				suspect_name VARCHAR(64),
				room_id VARCHAR(64),
				message TEXT NOT NULL,
				status VARCHAR(32) DEFAULT 'open',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

			`CREATE TABLE IF NOT EXISTS daily_minigame_earnings (
				denizen_id VARCHAR(64) NOT NULL,
				game VARCHAR(64) NOT NULL,
				earnings_date VARCHAR(16) NOT NULL,
				total_earned INT NOT NULL DEFAULT 0,
				PRIMARY KEY (denizen_id, game, earnings_date)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

			`CREATE TABLE IF NOT EXISTS postcards (
				id INT AUTO_INCREMENT PRIMARY KEY,
				sender_name VARCHAR(64) NOT NULL,
				sender_email VARCHAR(128) NOT NULL,
				recipient_name VARCHAR(64) NOT NULL,
				recipient_email VARCHAR(128) NOT NULL,
				movie_filename VARCHAR(64) NOT NULL,
				message_text TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				INDEX idx_postcards_recipient (recipient_name)
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

			`CREATE TABLE IF NOT EXISTS call_for_help (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				reporter_name TEXT NOT NULL,
				suspect_name TEXT,
				room_id TEXT,
				message TEXT NOT NULL,
				status TEXT DEFAULT 'open',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

			`CREATE TABLE IF NOT EXISTS daily_minigame_earnings (
				denizen_id TEXT NOT NULL,
				game TEXT NOT NULL,
				earnings_date TEXT NOT NULL,
				total_earned INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (denizen_id, game, earnings_date)
			);`,

			`CREATE TABLE IF NOT EXISTS postcards (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				sender_name TEXT NOT NULL,
				sender_email TEXT NOT NULL,
				recipient_name TEXT NOT NULL,
				recipient_email TEXT NOT NULL,
				movie_filename TEXT NOT NULL,
				message_text TEXT NOT NULL,
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
		return nil, fmt.Errorf("screenName cannot be empty")
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
		return nil, fmt.Errorf("screenName cannot be empty")
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

func (s *Store) GetBackpackItem(screenName string, posID int) (*BackpackItemRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	var catID int
	var pJson string
	err = s.db.QueryRow("SELECT catalog_item_id, properties_json FROM backpack_items WHERE id = ? AND denizen_id = ?", posID, denizenID).Scan(&catID, &pJson)
	if err != nil {
		return nil, err
	}

	props := make(map[string]any)
	if pJson != "" {
		_ = json.Unmarshal([]byte(pJson), &props)
	}
	return &BackpackItemRecord{
		ID:            posID,
		DenizenID:     denizenID,
		CatalogItemID: catID,
		Properties:    props,
	}, nil
}

func (s *Store) SwapBackpackItemsAtomic(traderName string, traderItemIDs []int, tradeeName string, tradeeItemIDs []int) ([]BackpackItemRecord, []BackpackItemRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var traderID, tradeeID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(traderName)).Scan(&traderID)
	if err != nil {
		return nil, nil, fmt.Errorf("trader %q not found: %w", traderName, err)
	}
	err = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(tradeeName)).Scan(&tradeeID)
	if err != nil {
		return nil, nil, fmt.Errorf("tradee %q not found: %w", tradeeName, err)
	}
	if traderID == tradeeID {
		return nil, nil, fmt.Errorf("cannot trade with oneself")
	}

	// Verify no duplicate IDs in offers
	seenTrader := make(map[int]bool)
	for _, id := range traderItemIDs {
		if seenTrader[id] {
			return nil, nil, fmt.Errorf("duplicate item %d in trader offer", id)
		}
		seenTrader[id] = true
	}
	seenTradee := make(map[int]bool)
	for _, id := range tradeeItemIDs {
		if seenTradee[id] {
			return nil, nil, fmt.Errorf("duplicate item %d in tradee offer", id)
		}
		seenTradee[id] = true
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var traderReceived []BackpackItemRecord
	var tradeeReceived []BackpackItemRecord

	// 1. Move trader items to tradee
	for _, id := range traderItemIDs {
		var catID int
		var pJson string
		err := tx.QueryRow("SELECT catalog_item_id, properties_json FROM backpack_items WHERE id = ? AND denizen_id = ?", id, traderID).Scan(&catID, &pJson)
		if err != nil {
			return nil, nil, fmt.Errorf("item %d is not owned by %s or not in backpack: %w", id, traderName, err)
		}

		props := make(map[string]any)
		if pJson != "" {
			_ = json.Unmarshal([]byte(pJson), &props)
		}
		// Update owner to tradee, strictly PRESERVING original song author/artist
		props["ownerName"] = tradeeName

		updatedJSON, _ := json.Marshal(props)
		_, err = tx.Exec("UPDATE backpack_items SET denizen_id = ?, properties_json = ? WHERE id = ?", tradeeID, string(updatedJSON), id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to transfer item %d to %s: %w", id, tradeeName, err)
		}

		tradeeReceived = append(tradeeReceived, BackpackItemRecord{
			ID:            id,
			DenizenID:     tradeeID,
			CatalogItemID: catID,
			Properties:    props,
		})
	}

	// 2. Move tradee items to trader
	for _, id := range tradeeItemIDs {
		var catID int
		var pJson string
		err := tx.QueryRow("SELECT catalog_item_id, properties_json FROM backpack_items WHERE id = ? AND denizen_id = ?", id, tradeeID).Scan(&catID, &pJson)
		if err != nil {
			return nil, nil, fmt.Errorf("item %d is not owned by %s or not in backpack: %w", id, tradeeName, err)
		}

		props := make(map[string]any)
		if pJson != "" {
			_ = json.Unmarshal([]byte(pJson), &props)
		}
		// Update owner to trader, strictly PRESERVING original song author/artist
		props["ownerName"] = traderName

		updatedJSON, _ := json.Marshal(props)
		_, err = tx.Exec("UPDATE backpack_items SET denizen_id = ?, properties_json = ? WHERE id = ?", traderID, string(updatedJSON), id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to transfer item %d to %s: %w", id, traderName, err)
		}

		traderReceived = append(traderReceived, BackpackItemRecord{
			ID:            id,
			DenizenID:     traderID,
			CatalogItemID: catID,
			Properties:    props,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return traderReceived, tradeeReceived, nil
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

func (s *Store) UpdatePlacedFurniture(studioID, posID string, x, y, z, dir, state, ho int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE placed_furniture 
		SET x = ?, y = ?, z = ?, dir = ?, state = ?, ho = ? 
		WHERE studio_id = ? AND pos_id = ?`,
		x, y, z, dir, state, ho, studioID, posID)
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
		WHERE mf.denizen_id = ? AND mf.status = 'accepted'
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

func (s *Store) AddFriendRequest(fromUser, toUser string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id1, id2 string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(fromUser)).Scan(&id1)
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(toUser)).Scan(&id2)
	if id1 == "" || id2 == "" || id1 == id2 {
		return fmt.Errorf("invalid friend users")
	}

	// Check if already friends
	var status string
	err := s.db.QueryRow("SELECT status FROM messenger_friends WHERE denizen_id = ? AND friend_id = ?", id1, id2).Scan(&status)
	if err == nil && status == "accepted" {
		return nil
	}

	q := "INSERT OR REPLACE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'pending')"
	if s.driver == "mysql" {
		q = "INSERT INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'pending') ON DUPLICATE KEY UPDATE status = 'pending'"
	}
	_, err = s.db.Exec(q, id1, id2)
	return err
}

func (s *Store) AcceptFriendRequest(user1, user2 string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id1, id2 string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user1)).Scan(&id1)
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(user2)).Scan(&id2)
	if id1 == "" || id2 == "" || id1 == id2 {
		return fmt.Errorf("invalid friend users")
	}

	q1 := "INSERT OR REPLACE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
	q2 := "INSERT OR REPLACE INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted')"
	if s.driver == "mysql" {
		q1 = "INSERT INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted') ON DUPLICATE KEY UPDATE status = 'accepted'"
		q2 = "INSERT INTO messenger_friends (denizen_id, friend_id, status) VALUES (?, ?, 'accepted') ON DUPLICATE KEY UPDATE status = 'accepted'"
	}
	_, _ = s.db.Exec(q1, id1, id2)
	_, _ = s.db.Exec(q2, id2, id1)
	return nil
}

func (s *Store) RejectFriendRequest(fromUser, toUser string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id1, id2 string
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(fromUser)).Scan(&id1)
	_ = s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(toUser)).Scan(&id2)
	if id1 == "" || id2 == "" {
		return nil
	}

	_, err := s.db.Exec("DELETE FROM messenger_friends WHERE (denizen_id = ? AND friend_id = ? AND status = 'pending') OR (denizen_id = ? AND friend_id = ? AND status = 'pending')",
		id1, id2, id2, id1)
	return err
}

func (s *Store) GetPendingFriendRequests(screenName string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT d.screen_name FROM messenger_friends mf
		JOIN denizens d ON d.id = mf.denizen_id
		WHERE mf.friend_id = ? AND mf.status = 'pending'
		ORDER BY d.screen_name ASC`, denizenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requesters []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			requesters = append(requesters, name)
		}
	}
	return requesters, nil
}

func (s *Store) RecordCallForHelp(reporter, suspect, roomID, message string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec("INSERT INTO call_for_help (reporter_name, suspect_name, room_id, message) VALUES (?, ?, ?, ?)",
		strings.TrimSpace(reporter), strings.TrimSpace(suspect), strings.TrimSpace(roomID), strings.TrimSpace(message))
	if err != nil {
		return 0, err
	}
	lid, _ := res.LastInsertId()
	return int(lid), nil
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
		baseID := fmt.Sprintf("studio_%s", strings.ToLower(strings.TrimSpace(ownerName)))
		var exists int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM studios WHERE id = ?", baseID).Scan(&exists)
		if exists == 0 {
			studioID = baseID
		} else {
			var count int
			_ = s.db.QueryRow("SELECT COUNT(*) FROM studios WHERE LOWER(owner_name) = LOWER(?)", strings.TrimSpace(ownerName)).Scan(&count)
			studioID = fmt.Sprintf("%s_%d", baseID, count+1)
		}
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

func (s *Store) ModifyStudio(studioID, ownerName, studioName, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if ownerName != "" {
		_, err = s.db.Exec("UPDATE studios SET name = ? WHERE id = ? AND LOWER(owner_name) = LOWER(?)", studioName, studioID, ownerName)
	} else {
		_, err = s.db.Exec("UPDATE studios SET name = ? WHERE id = ?", studioName, studioID)
	}
	return err
}

func (s *Store) DeleteStudio(studioID, ownerName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return any placed furniture to backpack
	_, _ = s.db.Exec("UPDATE backpack_items SET studio_id = '', placed = 0, x = 0, y = 0, z = 0 WHERE studio_id = ?", studioID)
	var err error
	if ownerName != "" {
		_, err = s.db.Exec("DELETE FROM studios WHERE id = ? AND LOWER(owner_name) = LOWER(?)", studioID, ownerName)
	} else {
		_, err = s.db.Exec("DELETE FROM studios WHERE id = ?", studioID)
	}
	return err
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

	return list, nil
}

type SongRecord struct {
	ID        int
	DenizenID string
	Title     string
	Artist    string
	EDLString string
	CreatedAt string
}

func (s *Store) SaveSong(screenName, title, edl string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	err := s.db.QueryRow("SELECT id FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID)
	if err != nil {
		denizenID = "usr_guest"
	}

	res, err := s.db.Exec("INSERT INTO songs (denizen_id, title, artist, edl_string) VALUES (?, ?, ?, ?)",
		denizenID, strings.TrimSpace(title), strings.TrimSpace(screenName), strings.TrimSpace(edl))
	if err != nil {
		return 0, err
	}
	lid, _ := res.LastInsertId()
	return int(lid), nil
}

func (s *Store) GetSong(id int) (*SongRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var r SongRecord
	err := s.db.QueryRow("SELECT id, denizen_id, title, artist, edl_string, created_at FROM songs WHERE id = ?", id).
		Scan(&r.ID, &r.DenizenID, &r.Title, &r.Artist, &r.EDLString, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetSongsByArtist(screenName string) ([]SongRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT id, denizen_id, title, artist, edl_string, created_at FROM songs WHERE LOWER(artist) = LOWER(?) ORDER BY id DESC",
		strings.TrimSpace(screenName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SongRecord
	for rows.Next() {
		var r SongRecord
		if err := rows.Scan(&r.ID, &r.DenizenID, &r.Title, &r.Artist, &r.EDLString, &r.CreatedAt); err == nil {
			list = append(list, r)
		}
	}
	return list, nil
}

func (s *Store) ProcessVegoSanMove(screenName, playerMove string) (*VegoSanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var denizenID string
	var currentDB int
	err := s.db.QueryRow("SELECT id, decibels FROM denizens WHERE LOWER(screen_name) = LOWER(?)", strings.TrimSpace(screenName)).Scan(&denizenID, &currentDB)
	if err != nil || denizenID == "" {
		return nil, fmt.Errorf("user not found: %s", screenName)
	}

	pMoveStr := strings.ToLower(strings.TrimSpace(playerMove))
	var pMove sumo.Move
	switch pMoveStr {
	case "1", "head_hit", "head", "push", "oshidashi":
		pMove = sumo.MoveHeadHit
	case "2", "body_blaster", "body", "slap", "tsuppari":
		pMove = sumo.MoveBodyBlaster
	case "3", "overhead_toss", "toss", "throw", "uwatenage", "grab":
		pMove = sumo.MoveOverheadToss
	case "4", "flying_kick", "kick", "dodge", "hikitoshi", "sweep":
		pMove = sumo.MoveFlyingKick
	case "5", "flying_punch", "punch", "special", "coke_power", "block":
		pMove = sumo.MoveFlyingPunch
	default:
		pMove = sumo.MoveBodyBlaster
	}

	aiMove := sumo.RandomMove()
	roundRes := sumo.EvaluateRound(pMove, aiMove)

	res := &VegoSanResult{
		Status:       "success",
		PlayerMove:   playerMove,
		OpponentMove: strings.ToLower(strings.ReplaceAll(aiMove.String(), " ", "_")),
		MasterMove:   aiMove.String(),
	}

	var streak, totalWins int
	_ = s.db.QueryRow("SELECT streak, total_wins FROM vegosan_streaks WHERE denizen_id = ?", denizenID).Scan(&streak, &totalWins)

	switch roundRes {
	case sumo.ResultDraw:
		res.Result = "draw"
		res.Draw = true
		res.Streak = streak
		res.TotalDecibels = currentDB
		res.Commentary = fmt.Sprintf("Both chose %s! Standoff in the dohyo!", pMove.String())
		res.Message = res.Commentary

	case sumo.ResultPlayer1Wins:
		streak++
		totalWins++
		res.Result = "win"
		res.Win = true
		res.Streak = streak

		tier := sumo.GetRewardForStreak(streak)
		rewardDB := 25
		if tier != nil && tier.RewardDB > 0 {
			rewardDB = tier.RewardDB
		}
		res.AwardedDB = rewardDB
		currentDB += rewardDB
		res.TotalDecibels = currentDB

		_, _ = s.db.Exec("UPDATE denizens SET decibels = ? WHERE id = ?", currentDB, denizenID)

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

		if tier != nil && tier.RewardFurni > 0 {
			res.PrizeItem = tier.RewardName
			res.PrizeItemID = tier.RewardFurni
			props := map[string]any{"sumo_prize": true, "streak": streak, "name": tier.RewardName}
			propsJSON, _ := json.Marshal(props)
			_, _ = s.db.Exec("INSERT INTO backpack_items (denizen_id, catalog_item_id, properties_json) VALUES (?, ?, ?)",
				denizenID, tier.RewardFurni, string(propsJSON))
		}

		res.Commentary = fmt.Sprintf("Victory! Your %s counters Master Kenji's %s! (+%d dB)", pMove.String(), aiMove.String(), rewardDB)
		res.Message = res.Commentary

	case sumo.ResultPlayer2Wins:
		res.Result = "lose"
		res.Win = false
		res.Streak = 0
		res.TotalDecibels = currentDB
		_, _ = s.db.Exec("UPDATE vegosan_streaks SET streak = 0, updated_at = CURRENT_TIMESTAMP WHERE denizen_id = ?", denizenID)
		res.Commentary = fmt.Sprintf("Defeat! Master Kenji's %s overpowered your %s! Try again!", aiMove.String(), pMove.String())
		res.Message = res.Commentary
	}

	return res, nil
}

func (s *Store) AddDenizenDecibels(denizenID string, amount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE denizens SET decibels = decibels + ? WHERE id = ?", amount, denizenID)
	return err
}

func (s *Store) ResetDenizenSumoStreak(denizenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := "UPDATE vegosan_streaks SET streak = 0, updated_at = CURRENT_TIMESTAMP WHERE denizen_id = ?"
	_, err := s.db.Exec(query, denizenID)
	return err
}

func (s *Store) IncrementDenizenSumoStreak(denizenID string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var streak, totalWins int
	_ = s.db.QueryRow("SELECT streak, total_wins FROM vegosan_streaks WHERE denizen_id = ?", denizenID).Scan(&streak, &totalWins)
	streak++
	totalWins++

	if s.driver == "mysql" {
		_, err := s.db.Exec(`INSERT INTO vegosan_streaks (denizen_id, streak, total_wins, updated_at) 
			VALUES (?, ?, ?, CURRENT_TIMESTAMP) 
			ON DUPLICATE KEY UPDATE streak = VALUES(streak), total_wins = VALUES(total_wins), updated_at = CURRENT_TIMESTAMP`,
			denizenID, streak, totalWins)
		return streak, totalWins, err
	}

	_, err := s.db.Exec(`INSERT INTO vegosan_streaks (denizen_id, streak, total_wins, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP) 
		ON CONFLICT(denizen_id) DO UPDATE SET streak = excluded.streak, total_wins = excluded.total_wins, updated_at = CURRENT_TIMESTAMP`,
		denizenID, streak, totalWins)
	return streak, totalWins, err
}

func (s *Store) GetDailyMinigameEarnings(denizenID, game string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	var earned int
	err := s.db.QueryRow("SELECT total_earned FROM daily_minigame_earnings WHERE denizen_id = ? AND game = ? AND earnings_date = ?",
		denizenID, game, today).Scan(&earned)
	if err != nil {
		return 0, nil
	}
	return earned, nil
}

func (s *Store) RecordDailyMinigameEarnings(denizenID, game string, amount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if s.driver == "mysql" {
		_, err := s.db.Exec(`INSERT INTO daily_minigame_earnings (denizen_id, game, earnings_date, total_earned)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE total_earned = total_earned + VALUES(total_earned)`,
			denizenID, game, today, amount)
		return err
	}

	_, err := s.db.Exec(`INSERT INTO daily_minigame_earnings (denizen_id, game, earnings_date, total_earned)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(denizen_id, game, earnings_date) DO UPDATE SET total_earned = total_earned + excluded.total_earned`,
		denizenID, game, today, amount)
	return err
}

type PostcardRecord struct {
	ID             int64     `json:"id"`
	SenderName     string    `json:"senderName"`
	SenderEmail    string    `json:"senderEmail"`
	RecipientName  string    `json:"recipientName"`
	RecipientEmail string    `json:"recipientEmail"`
	MovieFilename  string    `json:"movieFilename"`
	MessageText    string    `json:"messageText"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (s *Store) SavePostcard(senderName, senderEmail, recipientName, recipientEmail, movieFilename, messageText string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "INSERT INTO postcards (sender_name, sender_email, recipient_name, recipient_email, movie_filename, message_text) VALUES (?, ?, ?, ?, ?, ?)"
	res, err := s.db.Exec(query, strings.TrimSpace(senderName), strings.TrimSpace(senderEmail), strings.TrimSpace(recipientName), strings.TrimSpace(recipientEmail), strings.TrimSpace(movieFilename), strings.TrimSpace(messageText))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetPostcards(recipientName string) ([]PostcardRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "SELECT id, sender_name, sender_email, recipient_name, recipient_email, movie_filename, message_text, created_at FROM postcards WHERE LOWER(recipient_name) = LOWER(?) ORDER BY id DESC"
	rows, err := s.db.Query(query, strings.TrimSpace(recipientName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PostcardRecord
	for rows.Next() {
		var pc PostcardRecord
		if err := rows.Scan(&pc.ID, &pc.SenderName, &pc.SenderEmail, &pc.RecipientName, &pc.RecipientEmail, &pc.MovieFilename, &pc.MessageText, &pc.CreatedAt); err == nil {
			list = append(list, pc)
		}
	}
	return list, nil
}

