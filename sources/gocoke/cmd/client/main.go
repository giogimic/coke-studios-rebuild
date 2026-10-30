package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gocoke/internal/client/auth"
	"gocoke/internal/client/env"
	"gocoke/internal/client/scene"
	bootscene "gocoke/internal/client/scene/boot"
	entryscene "gocoke/internal/client/scene/entry"
	navigatorscene "gocoke/internal/client/scene/navigator"
	studioscene "gocoke/internal/client/scene/studio"
	"gocoke/internal/client/session"

	"github.com/hajimehoshi/ebiten/v2"
)

// version is set at build time via -ldflags
var version = "dev"

func main() {
	// Flags / config (Windows-first). Secrets are never logged.
	host := flag.String("host", "https://decibel.fun", "Backend base URL (scheme+host[:port])")
	gatewayPath := flag.String("gateway", "/sf/gateway", "Gateway path for AMF0 remoting")
	sw1 := flag.String("sw1", "", "Base64-encoded screen name (not logged)")
	sw2 := flag.String("sw2", "", "Session secret/sid (raw; NOT Base64)")
	user := flag.String("user", "", "Plain screen name (will be Base64-encoded to SW1 if sw1 not provided)")
	pass := flag.String("pass", "", "Plain session secret/sid (used as SW2 if sw2 not provided)")
	dataDir := flag.String("data", "data", "Path to data directory (Text_English.txt, etc)")
	flag.Parse()

	// In browser (WASM), auto-point to same origin to avoid CORS; the dev server proxies /sf/* to decibel.fun
	if origin := env.PageOrigin(); origin != "" {
		*host = origin
	}

	// In WASM mode, override SW1/SW2 from JavaScript globals if available
	readJSGlobals(sw1, sw2)

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("GoCoke client %s starting (Windows-first)", version)

	// Decode SW1; SW2 is used raw
	var screenName, secret string
	var encSW1, rawSW2 string
	// Prefer explicit sw1 if provided; otherwise derive from user via AS-compatible Base64
	if *sw1 != "" {
		encSW1 = *sw1
	} else if *user != "" {
		encSW1 = auth.ASBase64Encode(*user)
	}
	if encSW1 != "" {
		b, err := base64.StdEncoding.DecodeString(encSW1)
		if err != nil {
			log.Fatalf("invalid SW1: %v", err)
		}
		screenName = string(b)
	}
	// SW2/sid: raw (no Base64)
	if *sw2 != "" {
		rawSW2 = *sw2
	} else if *pass != "" {
		rawSW2 = *pass
	}

	// HTTP client with timeouts (cookie jar to be added when XML endpoints are used)
	httpClient := &http.Client{Timeout: 15 * time.Second}

	// Unconditional login attempt using SW2 as sid; print concise result
	if rawSW2 != "" {
		lr, err := auth.FetchLoginDenizen(httpClient, *host, rawSW2)
		if err != nil {
			log.Printf("loginDenizen failed: %v", err)
		} else {
			log.Printf("Login XML: ErrorCode=%d ScreenName=%q SecretLen=%d", lr.ErrorCode, lr.ScreenName, len(lr.Secret))
			if lr.ErrorCode == 0 {
				// Cache for later AMF calls
				screenName = lr.ScreenName
				secret = lr.Secret
				// Store in session for scenes
				session.SetCredentials(screenName, secret)
				session.SetUserProfile(lr.AvatarDefinition, lr.MissionStatement, lr.Gender)
			}
		}
	} else if screenName != "" {
		// Attempt session resolution using screenName
		lr, err := auth.FetchLoginDenizen(httpClient, *host, screenName)
		if err == nil && lr.ErrorCode == 0 {
			screenName = lr.ScreenName
			secret = lr.Secret
			session.SetCredentials(screenName, secret)
			session.SetUserProfile(lr.AvatarDefinition, lr.MissionStatement, lr.Gender)
		} else {
			session.SetCredentials(screenName, "")
		}
	}

	// Minimal Ebiten game loop stub
	ebiten.SetWindowTitle("GoCoke")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(760, 520)
	ebiten.SetTPS(120)

	sm := &scene.Manager{}
	g := &game{sm: sm, dataDir: *dataDir, httpClient: httpClient, launchRoom: env.LaunchRoom()}
	// Boot scene shows LOADER_TITLE/LOADING from Text_English.txt and calls getTip
	gatewayURL := fmt.Sprintf("%s%s", *host, *gatewayPath)
	session.SetGatewayURL(gatewayURL)
	g.gatewayURL = gatewayURL
	sm.Set(bootscene.New(*dataDir, httpClient, gatewayURL))
	if err := ebiten.RunGame(g); err != nil {
		log.Println("Ebiten terminated:", err)
		os.Exit(1)
	}
}

type game struct {
	launchRoom string
	sm         *scene.Manager
	dataDir    string
	httpClient *http.Client
	gatewayURL string
}

func (g *game) Update() error {
	// Check for scene transitions
	if bootScene, ok := g.sm.Current().(*bootscene.Scene); ok {
		if bootScene.IsComplete() {
			// Transition from boot to entry
			log.Println("Boot complete, transitioning to entry...")
			entry := entryscene.New(g.dataDir, bootScene.GetFeatureSet(), bootScene.GetStudioList())
			entry.SetCatalog(bootScene.GetCatalog())
			g.sm.Set(entry)
			if g.launchRoom != "" {
				entry.RequestInitialStudio(g.launchRoom)
				g.launchRoom = ""
			}
		}
	} else if entryScene, ok := g.sm.Current().(*entryscene.Scene); ok {
		if studioID := entryScene.ShouldEnterStudio(); studioID != "" {
			// Transition from entry to studio
			log.Printf("Entry requesting studio entry, transitioning to studio %s...", studioID)
			studio := entryScene.GetStudioByID(studioID)
			if studio != nil {
				studioScene, err := studioscene.New(g.dataDir, studio)
				if err != nil {
					log.Printf("Failed to create studio scene: %v", err)
				} else {
					// Inject catalog from entry and pass StudioList so navigator in studio has data
					studioScene.SetCatalog(entryScene.GetCatalog())
					studioScene.SetStudioList(entryScene.GetStudioList())
					// Register as active scene for realtime updates BEFORE room entry packets arrive
					studioscene.SetActiveScene(studioScene)
					// Inject current user name from session to enable local click movement
					if sn, sec := session.GetCredentials(); strings.TrimSpace(sn) != "" {
						studioScene.SetCurrentUserName(sn)
						def, mission, _ := session.GetUserProfile()
						studioScene.SetUserProfile(def, mission)
						// Load user's possessions from gateway
						studioScene.LoadPossessionsFromGateway(sn, sec, session.GetGatewayURL())
					}
					g.sm.Set(studioScene)

					// Trigger authoritative room entry once active scene is ready to receive presence/furni
					if rc := session.GetRealtimeClient(); rc != nil && session.IsRealtimeReady() {
						if err := rc.EnterRoom(studioID); err != nil {
							log.Printf("Realtime enter room failed during entry transition: %v", err)
						}
					}
				}
			} else {
				log.Printf("Studio %s not found", studioID)
			}
		} else if entryScene.ShouldShowNavigator() {
			// Transition from entry to navigator
			log.Println("Entry complete, transitioning to navigator...")
			navigator := navigatorscene.New(g.dataDir, entryScene.GetFeatureSet(), entryScene.GetStudioList())
			g.sm.Set(navigator)
		}
	} else if studioScene, ok := g.sm.Current().(*studioscene.Scene); ok {
		// Transition to another room via in-studio navigator
		if targetStudioID := studioScene.ShouldChangeStudio(); targetStudioID != "" {
			log.Printf("Studio changing room to %s...", targetStudioID)
			studioScene.ClearTargetStudio()
			studio := studioScene.GetStudioByID(targetStudioID)
			if studio != nil {
				newScene, err := studioscene.New(g.dataDir, studio)
				if err != nil {
					log.Printf("Failed to create studio scene: %v", err)
				} else {
					newScene.SetCatalog(studioScene.GetCatalog())
					newScene.SetStudioList(studioScene.GetStudioList())
					studioscene.SetActiveScene(newScene)
					if sn, sec := session.GetCredentials(); strings.TrimSpace(sn) != "" {
						newScene.SetCurrentUserName(sn)
						def, mission, _ := session.GetUserProfile()
						newScene.SetUserProfile(def, mission)
						newScene.LoadPossessionsFromGateway(sn, sec, session.GetGatewayURL())
					}
					g.sm.Set(newScene)

					if rc := session.GetRealtimeClient(); rc != nil && session.IsRealtimeReady() {
						if err := rc.EnterRoom(targetStudioID); err != nil {
							log.Printf("Realtime enter room failed during room transition: %v", err)
						}
					}
					return nil
				}
			} else {
				log.Printf("Target studio %s not found in studio list", targetStudioID)
			}
		}

		// Transition from studio back to boot (which will load entry/nav exactly as initial flow)
		if studioScene.ShouldExit() {
			log.Println("Studio requested exit, restarting boot -> entry flow...")
			studioscene.SetActiveScene(nil)
			// Use lobby variant: shorter status list and persistent tip
			g.sm.Set(bootscene.NewLobby(g.dataDir, g.httpClient, g.gatewayURL))
		}

		// Inject movement sender once per scene instance
		studioScene.SetMoveSender(func(roomID string, points [][2]int) {
			log.Printf("Main: sending AV_GSP for room %s with %d points", roomID, len(points))
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendMovePathGSP; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface{ SendMovePathGSP(string, [][2]int) error }); ok {
					_ = sender.SendMovePathGSP(roomID, points)
				}
			}
		})

		// Inject chat sender once per scene instance
		studioScene.SetChatSender(func(roomID string, message string, speechMode int) error {
			log.Printf("Main: sending CH for room %s with message %q (mode=%d)", roomID, message, speechMode)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendChat; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface {
					SendChat(string, string, int) error
				}); ok {
					return sender.SendChat(roomID, message, speechMode)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject furniture placement sender once per scene instance
		studioScene.SetFurnitureSender(func(roomID string, posID string, prodID string, x, y, z, dir, state, ho int) error {
			log.Printf("Main: sending POS_STD for room %s pos=%s prod=%s at client[row=%d,col=%d,y=%d] ho=%d dir=%d state=%d",
				roomID, posID, prodID, x, y, z, ho, dir, state)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendFurniturePlacement; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface {
					SendFurniturePlacement(string, string, string, int, int, int, int, int, int) error
				}); ok {
					// x=row, y=col, z=stackLevel(y in server terms)
					return sender.SendFurniturePlacement(roomID, posID, prodID, x, y, z, dir, state, ho)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject furniture pickup sender once per scene instance
		studioScene.SetPickupSender(func(roomID string, posID string) error {
			log.Printf("Main: sending POS_BKPK for room %s pos=%s (pickup to backpack)", roomID, posID)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendFurniturePickup; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface {
					SendFurniturePickup(string, string) error
				}); ok {
					return sender.SendFurniturePickup(roomID, posID)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject furniture rotation sender once per scene instance
		studioScene.SetRotateSender(func(roomID string, posID string, x, y, z, dir, state, ho int) error {
			log.Printf("Main: sending POS for room %s pos=%s rotation to dir=%d at client[row=%d,col=%d,y=%d] ho=%d state=%d",
				roomID, posID, dir, x, y, z, ho, state)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendFurnitureRotation; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface {
					SendFurnitureRotation(string, string, int, int, int, int, int, int) error
				}); ok {
					// x=row, y=col, z=stackLevel(y in server terms)
					return sender.SendFurnitureRotation(roomID, posID, x, y, z, dir, state, ho)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject avatar action sender once per scene instance
		studioScene.SetAvatarActionSender(func(act string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendAvatarAction(string, string) error }); ok {
					return sender.SendAvatarAction(studioScene.GetStudio().StudioID, act)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject friend request sender once per scene instance
		studioScene.SetFriendRequestSender(func(target string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendFriendRequest(string, string) error }); ok {
					return sender.SendFriendRequest(studioScene.GetStudio().StudioID, target)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject instant message sender once per scene instance
		studioScene.SetMessengerSender(func(to, message string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendMessengerMessage(string, string) error }); ok {
					return sender.SendMessengerMessage(to, message)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject friend accept sender once per scene instance
		studioScene.SetFriendAcceptSender(func(from string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendFriendAccept(string) error }); ok {
					return sender.SendFriendAccept(from)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject friend reject sender once per scene instance
		studioScene.SetFriendRejectSender(func(from string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendFriendReject(string) error }); ok {
					return sender.SendFriendReject(from)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject private whisper sender once per scene instance
		studioScene.SetWhisperSender(func(roomID, target, message string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface {
					SendWhisper(string, string, string) error
				}); ok {
					return sender.SendWhisper(roomID, target, message)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject host moderation kick sender once per scene instance
		studioScene.SetKickSender(func(roomID, target string) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendKickAvatar(string, string) error }); ok {
					return sender.SendKickAvatar(roomID, target)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject drink dispenser sender once per scene instance
		studioScene.SetDrinkDispenserSender(func(roomID string) error {
			log.Printf("Main: sending AV_DC for room %s (drink dispenser request)", roomID)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendDrinkDispenser; use interface assertion to avoid non-wasm compile error
				if sender, ok := any(rc).(interface {
					SendDrinkDispenser(string) error
				}); ok {
					return sender.SendDrinkDispenser(roomID)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject CD request (stereo) sender once per scene instance
		studioScene.SetCDRequestSender(func(roomID string, posID string) error {
			log.Printf("Main: sending CD_R for room %s pos=%s (stereo request)", roomID, posID)
			if rc := session.GetRealtimeClient(); rc != nil {
				// WASM realtime client implements SendCDRequest; use interface assertion
				if sender, ok := any(rc).(interface {
					SendCDRequest(string, string) error
				}); ok {
					return sender.SendCDRequest(roomID, posID)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject CD play (CD_P) sender once per scene instance
		studioScene.SetCDPlaySender(func(roomID string, posID string) error {
			log.Printf("Main: sending CD_P for room %s cdId=%s", roomID, posID)
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface {
					SendCDPlay(string, string) error
				}); ok {
					return sender.SendCDPlay(roomID, posID)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject thumbs up/down sender once per scene instance
		studioScene.SetThumbSender(func(up bool) error {
			if rc := session.GetRealtimeClient(); rc != nil {
				if up {
					if sender, ok := any(rc).(interface{ SendThumbsUp(string) error }); ok {
						return sender.SendThumbsUp(studioScene.GetStudio().StudioID)
					}
				} else {
					if sender, ok := any(rc).(interface{ SendThumbsDown(string) error }); ok {
						return sender.SendThumbsDown(studioScene.GetStudio().StudioID)
					}
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Inject CD end (CD_E) sender once per scene instance
		studioScene.SetCDEndSender(func(roomID string) error {
			log.Printf("Main: sending CD_E for room %s", roomID)
			if rc := session.GetRealtimeClient(); rc != nil {
				if sender, ok := any(rc).(interface{ SendCDEnd(string) error }); ok {
					return sender.SendCDEnd(roomID)
				}
			}
			return fmt.Errorf("realtime client not available")
		})

		// Wire trade callbacks to TradeManager
		if tm := studioScene.GetTradeManager(); tm != nil {
			tm.SetCallbacks(
				// TD_ST - Start trade
				func(targetName string) error {
					log.Printf("Main: sending TD_ST to %s", targetName)
					if rc := session.GetRealtimeClient(); rc != nil {
						if sender, ok := any(rc).(interface {
							SendTradeStart(string, string) error
						}); ok {
							return sender.SendTradeStart(studioScene.GetStudio().StudioID, targetName)
						}
					}
					return fmt.Errorf("realtime client not available")
				},
				// TD_AD - Add item to trade
				func(posID string) error {
					log.Printf("Main: sending TD_AD for pos=%s", posID)
					if rc := session.GetRealtimeClient(); rc != nil {
						if sender, ok := any(rc).(interface {
							SendTradeAddItem(string, string) error
						}); ok {
							return sender.SendTradeAddItem(studioScene.GetStudio().StudioID, posID)
						}
					}
					return fmt.Errorf("realtime client not available")
				},
				// TD_AG - Agree to trade
				func() error {
					log.Printf("Main: sending TD_AG")
					if rc := session.GetRealtimeClient(); rc != nil {
						if sender, ok := any(rc).(interface {
							SendTradeAgree(string) error
						}); ok {
							return sender.SendTradeAgree(studioScene.GetStudio().StudioID)
						}
					}
					return fmt.Errorf("realtime client not available")
				},
				// TD_CN - Cancel trade
				func() error {
					log.Printf("Main: sending TD_CN")
					if rc := session.GetRealtimeClient(); rc != nil {
						if sender, ok := any(rc).(interface {
							SendTradeCancel(string) error
						}); ok {
							return sender.SendTradeCancel(studioScene.GetStudio().StudioID)
						}
					}
					return fmt.Errorf("realtime client not available")
				},
			)
		}
	}

	if rc := session.GetRealtimeClient(); rc != nil {
		if flusher, ok := any(rc).(interface{ FlushRoomMessages() }); ok {
			flusher.FlushRoomMessages()
		}
	}
	return g.sm.Update()
}

func (g *game) Draw(screen *ebiten.Image) { g.sm.Draw(screen) }
func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.sm.Layout(outsideWidth, outsideHeight)
}
