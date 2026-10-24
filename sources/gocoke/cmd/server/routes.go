package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler constructs and returns the canonical HTTP routing mux with anti-caching middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 0. Denizen Registration & Legacy Login APIs
	mux.HandleFunc("/api/denizen/register", s.handleRegisterDenizen)
	mux.HandleFunc("/sf/ssldocs/registerDenizen.jsp", s.handleRegisterDenizen)
	mux.HandleFunc("/login/Login", s.handleLegacyLogin)
	mux.HandleFunc("/login/LoginPerson", s.handleLegacyLogin)

	// 1. Denizen login JSP
	mux.HandleFunc("/sf/ssldocs/loginDenizen.jsp", s.handleLoginDenizen)

	// 2. Flash Remoting AMF0 gateway
	mux.HandleFunc("/sf/gateway", s.handleAMFGateway)

	// 3. WebSocket Realtime Game Server
	mux.HandleFunc("/ws/realtime", s.handleWebSocketRealtime)

	// 4. Red Cap PIN code redemption HTTP API
	mux.HandleFunc("/sf/ssldocs/redeemCap.jsp", s.handleRedeemCap)
	mux.HandleFunc("/api/shop/purchase", s.handleShopPurchase)
	mux.HandleFunc("/api/catalogue/purchase", s.handleShopPurchase)

	// 5. Denizen Profile & Customizer APIs
	mux.HandleFunc("/api/denizen/profile", s.handleDenizenProfile)
	mux.HandleFunc("/api/denizen/update", s.handleUpdateDenizen)
	mux.HandleFunc("/sf/ssldocs/updateDenizen.jsp", s.handleUpdateDenizen)

	// 6. Messenger & Social APIs
	mux.HandleFunc("/api/messenger/friends", s.handleMessengerFriends)
	mux.HandleFunc("/api/messenger/friend/list", s.handleMessengerFriends)
	mux.HandleFunc("/api/messenger/send", s.handleMessengerSend)
	mux.HandleFunc("/api/messenger/messages", s.handleMessengerMessages)
	mux.HandleFunc("/api/messenger/friend/add", s.handleMessengerAddFriend)
	mux.HandleFunc("/api/messenger/friend/remove", s.handleMessengerRemoveFriend)

	// 6b. Postcard Flow (Historical /servlet/PostCard and REST API)
	mux.HandleFunc("/servlet/PostCard", s.handlePostcard)
	mux.HandleFunc("/api/postcard/send", s.handlePostcard)
	mux.HandleFunc("/postcardpreview.jsp", s.handlePostcardPreview)

	// 7. Studio Decor API
	mux.HandleFunc("/api/studio/decor", s.handleStudioDecor)

	// 8. In-Game CD Burning API
	mux.HandleFunc("/api/mixer/burn", s.handleMixerBurn)

	// 9. Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","mode":"standalone-server","project":"gocoke","storage":"%s"}`, s.store.driver)
	})

	// 10. Sound library resolver (maps un-prefixed WAV names to Director prefixed files)
	mux.HandleFunc("/Lingo/soundlibrary/", s.handleSoundLibrary)
	mux.HandleFunc("/sounds/", s.handleSoundLibrary)
	mux.HandleFunc("/soundlibrary/", s.handleSoundLibrary)

	repoRoot := findRepoRoot()
	cokephaseDir := filepath.Join(repoRoot, "sources", "cokephase")
	portalDir := filepath.Join(repoRoot, "tools", "portal")
	mixerDir := filepath.Join(repoRoot, "sources", "music-mixer", "dist")
	avatarDir := filepath.Join(repoRoot, "sources", "avatar-assets", "assets")
	gocokeWebDir := filepath.Join(repoRoot, "sources", "gocoke", "web")
	if fi, err := os.Stat(s.webRoot); err == nil && fi.IsDir() {
		absRoot, _ := filepath.Abs(s.webRoot)
		gocokeWebDir = absRoot
	}

	// 11. Top 40 Charts API
	mux.HandleFunc("/api/music/top40", s.handleTop40)

	// 12. V-ego San Mini-game API & Director Shockwave endpoints
	mux.HandleFunc("/api/minigames/vegosan/move", s.handleVegoSanMove)
	mux.HandleFunc("/api/minigames/sumo/match", s.handleSumoMatch)
	mux.HandleFunc("/gamesUser.do", s.handleGamesUserValidate)
	mux.HandleFunc("/sumo.do", s.handleSumoPrizeHTTP)
	mux.HandleFunc("/api/minigames/uncover/play", s.handleUncoverPlay)
	mux.HandleFunc("/api/minigames/quiz/question", s.handlePopQuizQuestion)
	mux.HandleFunc("/api/minigames/quiz/answer", s.handlePopQuizAnswer)

	// 13. CokePhase legacy and JSON endpoints (reference)
	mux.HandleFunc("/jsonreply/room.php", s.handleJsonRoom)
	mux.HandleFunc("/jsonreply/catalog.php", s.handleJsonCatalog(cokephaseDir))
	if fi, err := os.Stat(filepath.Join(cokephaseDir, "jsonreply")); err == nil && fi.IsDir() {
		mux.Handle("/jsonreply/", http.StripPrefix("/jsonreply/", http.FileServer(http.Dir(filepath.Join(cokephaseDir, "jsonreply")))))
	}

	// 14. CokePhase Engine & Client Assets (preserved as reference tool)
	if fi, err := os.Stat(cokephaseDir); err == nil && fi.IsDir() {
		mux.Handle("/phase/", http.StripPrefix("/phase/", http.FileServer(http.Dir(cokephaseDir))))
		mux.Handle("/engine/", http.StripPrefix("/engine/", http.FileServer(http.Dir(filepath.Join(cokephaseDir, "engine")))))
		mux.Handle("/gameClasses/", http.StripPrefix("/gameClasses/", http.FileServer(http.Dir(filepath.Join(cokephaseDir, "gameClasses")))))
		mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir(filepath.Join(cokephaseDir, "css")))))
		mux.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir(filepath.Join(cokephaseDir, "js")))))
		mux.HandleFunc("/ClientConfig.js", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(cokephaseDir, "ClientConfig.js"))
		})
		mux.HandleFunc("/client.js", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(cokephaseDir, "client.js"))
		})
		mux.HandleFunc("/index.js", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(cokephaseDir, "index.js"))
		})
	}

	// 15. GoCoke WASM Client (The Canonical Game Route)
	if fi, err := os.Stat(gocokeWebDir); err == nil && fi.IsDir() {
		mux.Handle("/game/", http.StripPrefix("/game/", http.FileServer(http.Dir(gocokeWebDir))))
	}

	// 16. Music Mixer App
	if fi, err := os.Stat(mixerDir); err == nil && fi.IsDir() {
		mux.Handle("/mixer/", http.StripPrefix("/mixer/", http.FileServer(http.Dir(mixerDir))))
	}

	// 17. Avatar Assets
	if fi, err := os.Stat(avatarDir); err == nil && fi.IsDir() {
		mux.Handle("/avatar-assets/", http.StripPrefix("/avatar-assets/", http.FileServer(http.Dir(avatarDir))))
	}

	// 18. Portal & Assets Cascade
	if fi, err := os.Stat(portalDir); err == nil && fi.IsDir() {
		mux.Handle("/portal/", http.StripPrefix("/portal/", http.FileServer(http.Dir(portalDir))))
	}

	screensaversDir := filepath.Join(repoRoot, "sources", "screensavers", "swf")
	if fi, err := os.Stat(screensaversDir); err == nil && fi.IsDir() {
		mux.Handle("/screensavers/", http.StripPrefix("/screensavers/", http.FileServer(http.Dir(screensaversDir))))
	}

	csWebsiteDir := filepath.Join(repoRoot, "sources", "cs-website", "www")

	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/images/")
		candidates := []string{
			filepath.Join(portalDir, "assets", "images", sub),
			filepath.Join(csWebsiteDir, "images", sub),
			filepath.Join(cokephaseDir, "assets", "images", sub),
		}
		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, cand)
				return
			}
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/assets/")
		candidates := []string{
			filepath.Join(portalDir, "assets", sub),
			filepath.Join(cokephaseDir, "assets", sub),
			filepath.Join(gocokeWebDir, "assets", sub),
		}
		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, cand)
				return
			}
		}
		http.NotFound(w, r)
	})

	// 19. Root fallback: serve tools/portal/index.html or gocoke index.html
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			portalIndex := filepath.Join(portalDir, "index.html")
			if fi, err := os.Stat(portalIndex); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, portalIndex)
				return
			}
			gocokeIndex := filepath.Join(gocokeWebDir, "index.html")
			if fi, err := os.Stat(gocokeIndex); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, gocokeIndex)
				return
			}
		}
		http.FileServer(http.Dir(gocokeWebDir)).ServeHTTP(w, r)
	})

	noCacheHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		mux.ServeHTTP(w, r)
	})

	return noCacheHandler
}
