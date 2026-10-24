package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"
)

// handleDoorbellKnock routes visitor knock notifications to the studio owner
func (s *Server) handleDoorbellKnock(player *Player, roomID string) {
	if roomID == "" {
		roomID = player.RoomID
	}
	if roomID == "" || roomID == "$LOBBY$" {
		return
	}

	rm, err := s.GetRoom(roomID)
	if err != nil || rm == nil {
		return
	}

	knockXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_KNOCK av=\"%s\"/></Room.Action>", rm.ID, player.ScreenName)
	rm.Broadcast(knockXML, player.ID)
	log.Printf("[DOORBELL] %s knocked on room %s (owner: %s)", player.ScreenName, rm.ID, rm.OwnerName)
}

// handleDoorbellResponse validates host permissions before admitting or denying a knocking visitor
func (s *Server) handleDoorbellResponse(player *Player, target string, allow bool) {
	roomID := player.RoomID
	if roomID == "" || roomID == "$LOBBY$" || target == "" {
		return
	}

	rm, err := s.GetRoom(roomID)
	if err != nil || rm == nil {
		return
	}

	// Host security check: only the studio owner can admit or deny visitors
	if !strings.EqualFold(rm.OwnerName, player.ScreenName) {
		log.Printf("[DOORBELL] Non-owner %s unauthorized to respond to doorbell in %s (owner: %s)",
			player.ScreenName, rm.ID, rm.OwnerName)
		return
	}

	s.mu.RLock()
	var targetPlayer *Player
	for _, p := range s.Players {
		if strings.EqualFold(p.ScreenName, target) {
			targetPlayer = p
			break
		}
	}
	s.mu.RUnlock()

	if targetPlayer != nil {
		if allow {
			_ = targetPlayer.Send(fmt.Sprintf("<Accepted roomID=\"%s\"/>", rm.ID))
			log.Printf("[DOORBELL] Owner %s admitted %s into %s", player.ScreenName, target, rm.ID)
		} else {
			_ = targetPlayer.Send("<Alert msg=\"The studio owner declined your knock.\"/>")
			log.Printf("[DOORBELL] Owner %s declined knock from %s into %s", player.ScreenName, target, rm.ID)
		}
	}
}

// handleStudioKick verifies host permissions before expelling a visitor to the lobby
func (s *Server) handleStudioKick(player *Player, target string) {
	roomID := player.RoomID
	if roomID == "" || roomID == "$LOBBY$" || target == "" {
		return
	}
	if strings.EqualFold(player.ScreenName, target) {
		return // cannot kick self
	}

	rm, err := s.GetRoom(roomID)
	if err != nil || rm == nil {
		return
	}

	// Host security check: only the studio owner can kick players
	if !strings.EqualFold(rm.OwnerName, player.ScreenName) {
		log.Printf("[MODERATION] Non-owner %s unauthorized to kick %s from %s (owner: %s)",
			player.ScreenName, target, rm.ID, rm.OwnerName)
		return
	}

	var kickedPlayer *Player
	rm.mu.Lock()
	for pid, p := range rm.Players {
		if strings.EqualFold(p.ScreenName, target) {
			kickedPlayer = p
			delete(rm.Players, pid)
			break
		}
	}
	rm.mu.Unlock()

	if kickedPlayer != nil {
		kickedPlayer.RoomID = "$LOBBY$"
		_ = kickedPlayer.Send(fmt.Sprintf("<KICKED roomID=\"%s\" by=\"%s\"/>", rm.ID, player.ScreenName))
		rmXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><AV_RM av=\"%s\"/></Room.Action>", rm.ID, target)
		rm.Broadcast(rmXML, "")
		log.Printf("[MODERATION] Host %s kicked %s from studio %s", player.ScreenName, target, rm.ID)
	}
}

// applyRealtimeStudioDecor enforces host ownership before applying live wallpaper/floor/doorbell updates
func (s *Server) applyRealtimeStudioDecor(player *Player, wp, fl, db int) {
	roomID := player.RoomID
	if roomID == "" || roomID == "$LOBBY$" {
		return
	}

	rm, err := s.GetRoom(roomID)
	if err != nil || rm == nil {
		return
	}

	// Host security check: only the studio owner can update room decor
	if !strings.EqualFold(rm.OwnerName, player.ScreenName) {
		log.Printf("[STUDIO] Non-owner %s unauthorized to update decor in %s (owner: %s)",
			player.ScreenName, rm.ID, rm.OwnerName)
		return
	}

	if wp <= 0 {
		wp = 1
	}
	if fl <= 0 {
		fl = 1
	}
	if db <= 0 {
		db = 1
	}

	rm.mu.Lock()
	rm.Wallpaper = wp
	rm.Floor = fl
	rm.Doorbell = db
	rm.mu.Unlock()

	_ = s.store.UpdateStudioDecor(rm.ID, wp, fl, db)
	decorXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ST_DECOR wp=\"%d\" fl=\"%d\" db=\"%d\"/></Room.Action>",
		rm.ID, wp, fl, db)
	rm.Broadcast(decorXML, "")
	log.Printf("[STUDIO] Owner %s updated decor for %s: wp=%d fl=%d db=%d", player.ScreenName, rm.ID, wp, fl, db)
}

// handleCallForHelp logs user reports and sends an authoritative acknowledgment
func (s *Server) handleCallForHelp(player *Player, suspect, msg string) {
	if msg == "" {
		return
	}
	reportID, err := s.store.RecordCallForHelp(player.ScreenName, suspect, player.RoomID, msg)
	if err != nil {
		log.Printf("[CFH] Error recording call for help: %v", err)
		return
	}

	ackXML := fmt.Sprintf("<Room.Action roomID=\"%s\"><ANS_CLL id=\"%d\"/></Room.Action>", player.RoomID, reportID)
	_ = player.Send(ackXML)
	_ = player.Send("<Alert msg=\"Your call for help has been received by moderators.\"/>")
	log.Printf("[CFH] Report #%d from %s against %s: %q", reportID, player.ScreenName, suspect, msg)
}

// handleMessengerRequest handles sending a friend request
func (s *Server) handleMessengerRequest(player *Player, to string) {
	to = strings.TrimSpace(to)
	if to == "" || strings.EqualFold(to, player.ScreenName) {
		return
	}

	if err := s.store.AddFriendRequest(player.ScreenName, to); err != nil {
		log.Printf("[MESSENGER] AddFriendRequest error: %v", err)
		return
	}

	// Send live notifications if recipient is online
	invXML := fmt.Sprintf("<MSGR_INV sn=\"%s\"/>", player.ScreenName)
	invAltXML := fmt.Sprintf("<MMS_REQ from=\"%s\"/>", player.ScreenName)
	s.notifyMessengerEvent(to, invXML)
	s.notifyMessengerEvent(to, invAltXML)

	log.Printf("[MESSENGER] Friend request from %s to %s", player.ScreenName, to)
}

// handleMessengerAccept handles accepting a pending friend request
func (s *Server) handleMessengerAccept(player *Player, from string) {
	from = strings.TrimSpace(from)
	if from == "" {
		return
	}

	if err := s.store.AcceptFriendRequest(player.ScreenName, from); err != nil {
		log.Printf("[MESSENGER] AcceptFriendRequest error: %v", err)
		return
	}

	// Notify both users that friendship is established
	adXML1 := fmt.Sprintf("<MSGR_AD_F sn=\"%s\" isOnline=\"true\"/>", from)
	adXML2 := fmt.Sprintf("<MSGR_AD_F sn=\"%s\" isOnline=\"true\"/>", player.ScreenName)
	altXML1 := fmt.Sprintf("<MMS_ACC from=\"%s\"/>", from)
	altXML2 := fmt.Sprintf("<MMS_ACC from=\"%s\"/>", player.ScreenName)

	_ = player.Send(adXML1)
	_ = player.Send(altXML1)
	s.notifyMessengerEvent(from, adXML2)
	s.notifyMessengerEvent(from, altXML2)

	log.Printf("[MESSENGER] Friendship accepted between %s and %s", player.ScreenName, from)
}

// handleMessengerReject handles declining a friend request
func (s *Server) handleMessengerReject(player *Player, from string) {
	from = strings.TrimSpace(from)
	if from == "" {
		return
	}

	if err := s.store.RejectFriendRequest(from, player.ScreenName); err != nil {
		log.Printf("[MESSENGER] RejectFriendRequest error: %v", err)
		return
	}

	rjXML := fmt.Sprintf("<MSGR_RJ_INV inviter=\"%s\" invitee=\"%s\"/>", from, player.ScreenName)
	_ = player.Send(rjXML)
	s.notifyMessengerEvent(from, rjXML)

	log.Printf("[MESSENGER] %s rejected friend request from %s", player.ScreenName, from)
}

// deliverRealtimeMessengerMessage stores and delivers instant messages
func (s *Server) deliverRealtimeMessengerMessage(player *Player, to, msgText string) {
	to = strings.TrimSpace(to)
	msgText = strings.TrimSpace(msgText)
	if to == "" || msgText == "" {
		return
	}

	msgID, err := s.store.SendMessage(player.ScreenName, to, msgText)
	if err != nil {
		log.Printf("[MESSENGER] SendMessage error: %v", err)
		return
	}

	hexMsg := hex.EncodeToString([]byte(msgText))
	sndXML := fmt.Sprintf("<MSGR_SND_M snd=\"%s\" rcv=\"%s\" id=\"%d\" m=\"%s\" msg=\"%s\"/>",
		player.ScreenName, to, msgID, hexMsg, msgText)
	altXML := fmt.Sprintf("<MMS_MSG from=\"%s\" msg=\"%s\"/>", player.ScreenName, msgText)

	s.notifyMessengerEvent(to, sndXML)
	s.notifyMessengerEvent(to, altXML)

	log.Printf("[MESSENGER] %s -> %s (#%d): %q", player.ScreenName, to, msgID, msgText)
}
