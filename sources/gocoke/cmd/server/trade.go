package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// TradeSession tracks active trade negotiations between two players in a room
type TradeSession struct {
	RoomID       string
	TraderName   string
	TradeeName   string
	TraderItems  []int // posIDs in backpack_items
	TradeeItems  []int // posIDs in backpack_items
	TraderAgreed bool
	TradeeAgreed bool
	mu           sync.Mutex
}

// TradeManager coordinates in-room trading sessions and enforces atomic completion
type TradeManager struct {
	mu            sync.RWMutex
	sessions      map[string]*TradeSession // keyed by lowercase screenName -> active session
	store         *Store
	broadcastRoom func(roomID string, xml string)
	sendPlayer    func(screenName string, xml string) error
}

func NewTradeManager(store *Store, broadcastRoom func(string, string), sendPlayer func(string, string) error) *TradeManager {
	return &TradeManager{
		sessions:      make(map[string]*TradeSession),
		store:         store,
		broadcastRoom: broadcastRoom,
		sendPlayer:    sendPlayer,
	}
}

// GetSession returns the active trade session for a player if present
func (tm *TradeManager) GetSession(screenName string) *TradeSession {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.sessions[strings.ToLower(strings.TrimSpace(screenName))]
}

// StartTrade initiates a new trade session between trader and tradee in roomID
func (tm *TradeManager) StartTrade(roomID, traderName, tradeeName string) (*TradeSession, error) {
	traderClean := strings.TrimSpace(traderName)
	tradeeClean := strings.TrimSpace(tradeeName)
	if traderClean == "" || tradeeClean == "" {
		return nil, fmt.Errorf("invalid trader or tradee name")
	}
	if strings.EqualFold(traderClean, tradeeClean) {
		return nil, fmt.Errorf("cannot trade with oneself")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	k1 := strings.ToLower(traderClean)
	k2 := strings.ToLower(tradeeClean)
	if tm.sessions[k1] != nil {
		return nil, fmt.Errorf("player %s is already in a trade", traderClean)
	}
	if tm.sessions[k2] != nil {
		return nil, fmt.Errorf("player %s is already in a trade", tradeeClean)
	}

	sess := &TradeSession{
		RoomID:      roomID,
		TraderName:  traderClean,
		TradeeName:  tradeeClean,
		TraderItems: make([]int, 0, 6),
		TradeeItems: make([]int, 0, 6),
	}
	tm.sessions[k1] = sess
	tm.sessions[k2] = sess

	// Broadcast initial trade display (window open) to both players
	displXML := tm.buildDisplXML(sess)
	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(traderClean, displXML)
		_ = tm.sendPlayer(tradeeClean, displXML)
	} else if tm.broadcastRoom != nil {
		tm.broadcastRoom(roomID, displXML)
	}

	return sess, nil
}

// AddItem adds an owned item from backpack into the player's trade offer
func (tm *TradeManager) AddItem(screenName string, posID int) error {
	sess := tm.GetSession(screenName)
	if sess == nil {
		return fmt.Errorf("player %s is not in an active trade", screenName)
	}

	// Verify that the player authoritatively owns this item in their backpack
	item, err := tm.store.GetBackpackItem(screenName, posID)
	if err != nil || item == nil {
		return fmt.Errorf("item %d is not available in %s's backpack: %w", posID, screenName, err)
	}

	sess.mu.Lock()
	isTrader := strings.EqualFold(sess.TraderName, screenName)
	if isTrader {
		if len(sess.TraderItems) >= 6 {
			sess.mu.Unlock()
			return fmt.Errorf("trade offer is full (max 6 items)")
		}
		for _, id := range sess.TraderItems {
			if id == posID {
				sess.mu.Unlock()
				return fmt.Errorf("item %d is already in trade offer", posID)
			}
		}
		sess.TraderItems = append(sess.TraderItems, posID)
	} else {
		if len(sess.TradeeItems) >= 6 {
			sess.mu.Unlock()
			return fmt.Errorf("trade offer is full (max 6 items)")
		}
		for _, id := range sess.TradeeItems {
			if id == posID {
				sess.mu.Unlock()
				return fmt.Errorf("item %d is already in trade offer", posID)
			}
		}
		sess.TradeeItems = append(sess.TradeeItems, posID)
	}

	// Changing offered items automatically resets agreements on both sides
	sess.TraderAgreed = false
	sess.TradeeAgreed = false
	sess.mu.Unlock()

	displXML := tm.buildDisplXML(sess)
	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(sess.TraderName, displXML)
		_ = tm.sendPlayer(sess.TradeeName, displXML)
	} else if tm.broadcastRoom != nil {
		tm.broadcastRoom(sess.RoomID, displXML)
	}

	return nil
}

// AgreeTrade marks agreement for player; if both have agreed, executes atomic completion
func (tm *TradeManager) AgreeTrade(screenName string) (bool, error) {
	sess := tm.GetSession(screenName)
	if sess == nil {
		return false, fmt.Errorf("player %s is not in an active trade", screenName)
	}

	sess.mu.Lock()
	if strings.EqualFold(sess.TraderName, screenName) {
		sess.TraderAgreed = true
	} else if strings.EqualFold(sess.TradeeName, screenName) {
		sess.TradeeAgreed = true
	}
	bothAgreed := sess.TraderAgreed && sess.TradeeAgreed
	sess.mu.Unlock()

	displXML := tm.buildDisplXML(sess)
	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(sess.TraderName, displXML)
		_ = tm.sendPlayer(sess.TradeeName, displXML)
	} else if tm.broadcastRoom != nil {
		tm.broadcastRoom(sess.RoomID, displXML)
	}

	if bothAgreed {
		if err := tm.CompleteTrade(sess); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// DisagreeTrade revokes agreement for player
func (tm *TradeManager) DisagreeTrade(screenName string) error {
	sess := tm.GetSession(screenName)
	if sess == nil {
		return fmt.Errorf("player %s is not in an active trade", screenName)
	}

	sess.mu.Lock()
	if strings.EqualFold(sess.TraderName, screenName) {
		sess.TraderAgreed = false
	} else if strings.EqualFold(sess.TradeeName, screenName) {
		sess.TradeeAgreed = false
	}
	sess.mu.Unlock()

	displXML := tm.buildDisplXML(sess)
	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(sess.TraderName, displXML)
		_ = tm.sendPlayer(sess.TradeeName, displXML)
	} else if tm.broadcastRoom != nil {
		tm.broadcastRoom(sess.RoomID, displXML)
	}

	return nil
}

// CancelTrade cancels the trade session and returns offered items untouched
func (tm *TradeManager) CancelTrade(screenName string) error {
	sess := tm.GetSession(screenName)
	if sess == nil {
		return nil
	}

	tm.mu.Lock()
	delete(tm.sessions, strings.ToLower(sess.TraderName))
	delete(tm.sessions, strings.ToLower(sess.TradeeName))
	tm.mu.Unlock()

	cancelXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CNCL/></Room.Action>", sess.RoomID)
	cancelXMLAlt := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CN/></Room.Action>", sess.RoomID)
	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(sess.TraderName, cancelXML)
		_ = tm.sendPlayer(sess.TraderName, cancelXMLAlt)
		_ = tm.sendPlayer(sess.TradeeName, cancelXML)
		_ = tm.sendPlayer(sess.TradeeName, cancelXMLAlt)
	} else if tm.broadcastRoom != nil {
		tm.broadcastRoom(sess.RoomID, cancelXML)
	}

	return nil
}

// AbortTradeForPlayer safely cleans up a trade if a player disconnects or leaves the room
func (tm *TradeManager) AbortTradeForPlayer(screenName string) {
	_ = tm.CancelTrade(screenName)
}

// CompleteTrade executes atomic database transfer and broadcasts completion packets
func (tm *TradeManager) CompleteTrade(sess *TradeSession) error {
	tm.mu.Lock()
	delete(tm.sessions, strings.ToLower(sess.TraderName))
	delete(tm.sessions, strings.ToLower(sess.TradeeName))
	tm.mu.Unlock()

	sess.mu.Lock()
	traderItems := append([]int(nil), sess.TraderItems...)
	tradeeItems := append([]int(nil), sess.TradeeItems...)
	sess.mu.Unlock()

	traderRecv, tradeeRecv, err := tm.store.SwapBackpackItemsAtomic(sess.TraderName, traderItems, sess.TradeeName, tradeeItems)
	if err != nil {
		// If transaction fails (e.g. concurrent pickup or sold), cancel trade cleanly
		cancelXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CNCL/></Room.Action>", sess.RoomID)
		if tm.sendPlayer != nil {
			_ = tm.sendPlayer(sess.TraderName, cancelXML)
			_ = tm.sendPlayer(sess.TradeeName, cancelXML)
		} else if tm.broadcastRoom != nil {
			tm.broadcastRoom(sess.RoomID, cancelXML)
		}
		return fmt.Errorf("trade transfer failed: %w", err)
	}

	// Send TD_CPLT containing the items received by each respective party
	traderCpltXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CPLT>%s</TD_CPLT></Room.Action>", sess.RoomID, tm.buildCpltItemsXML(traderRecv))
	tradeeCpltXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CPLT>%s</TD_CPLT></Room.Action>", sess.RoomID, tm.buildCpltItemsXML(tradeeRecv))

	if tm.sendPlayer != nil {
		_ = tm.sendPlayer(sess.TraderName, traderCpltXML)
		_ = tm.sendPlayer(sess.TradeeName, tradeeCpltXML)
	}

	// Broadcast generic completion packet to room
	genericCplt := fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_CPLT/></Room.Action>", sess.RoomID)
	if tm.broadcastRoom != nil {
		tm.broadcastRoom(sess.RoomID, genericCplt)
	}

	return nil
}

func (tm *TradeManager) buildDisplXML(sess *TradeSession) string {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Extract comma-separated catalog item IDs for trader and tradee
	traderCIDs := make([]string, 0, len(sess.TraderItems))
	for _, posID := range sess.TraderItems {
		if it, err := tm.store.GetBackpackItem(sess.TraderName, posID); err == nil && it != nil {
			traderCIDs = append(traderCIDs, strconv.Itoa(it.CatalogItemID))
		}
	}

	tradeeCIDs := make([]string, 0, len(sess.TradeeItems))
	for _, posID := range sess.TradeeItems {
		if it, err := tm.store.GetBackpackItem(sess.TradeeName, posID); err == nil && it != nil {
			tradeeCIDs = append(tradeeCIDs, strconv.Itoa(it.CatalogItemID))
		}
	}

	traderItemsStr := strings.Join(traderCIDs, ",")
	tradeeItemsStr := strings.Join(tradeeCIDs, ",")

	traderAgreesStr := "false"
	if sess.TraderAgreed {
		traderAgreesStr = "true"
	}
	tradeeAgreesStr := "false"
	if sess.TradeeAgreed {
		tradeeAgreesStr = "true"
	}

	return fmt.Sprintf("<Room.Action roomID=\"%s\"><TD_DISPL tdr=\"%s\" tde=\"%s\" tdr_a=\"%s\" tde_a=\"%s\" tdr_i=\"%s\" tde_i=\"%s\"/></Room.Action>",
		sess.RoomID, sess.TraderName, sess.TradeeName, traderAgreesStr, tradeeAgreesStr, traderItemsStr, tradeeItemsStr)
}

func (tm *TradeManager) buildCpltItemsXML(items []BackpackItemRecord) string {
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(fmt.Sprintf("<P id=\"%d\" cid=\"%d\">[:]</P>", it.ID, it.CatalogItemID))
	}
	return sb.String()
}
