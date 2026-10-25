//go:build !js || !wasm

package realtime

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Client implements the legacy Shockwave XMLSocket protocol used by Coke Studios.
// It frames messages as: sha1(xml + trip + secret) + " " + trip(7-width, space-padded) + " " + xml + "\x00".
// The server responds with NUL-terminated XML documents and occasional <Accepted msgID="..."/> acks.
type Client struct {
	conn   net.Conn
	mu     sync.Mutex
	trip   uint64
	msgID  uint64
	secret string
	closed bool

	// Logging only; optional
	logger *log.Logger

	// Ack tracking for <Accepted msgID="n"/>
	pendingAcks map[uint64]chan struct{}
}

// New constructs a Client. Secret is the session Secret from loginDenizen.jsp.
func New(secret string, logger *log.Logger) *Client {
	return &Client{secret: secret, logger: logger, pendingAcks: make(map[uint64]chan struct{}), trip: 1}
}

// Connect dials the TCP server at host:port with a timeout and starts a reader loop.
func (c *Client) Connect(ctx context.Context, host string, port string) error {
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.closed = false
	c.mu.Unlock()

	go c.readLoop()
	return nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendLogin sends a System.Login request with binhex-encoded username and password (secret).
// The server decodes using the FFDK text/binhex codec.
func (c *Client) SendLogin(username string, secret string) error {
	// Encode as uppercase hex of ASCII bytes
	encUser := strings.ToUpper(hex.EncodeToString([]byte(username)))
	encPass := strings.ToUpper(hex.EncodeToString([]byte(secret)))
	// Include msgID so server can ACK
	id := c.nextMsgID()
	xml := fmt.Sprintf("<System.Login msgID=\"%d\"><Username encoding=\"text/binhex\">%s</Username><Password encoding=\"text/binhex\">%s</Password></System.Login>", id, encUser, encPass)
	return c.SendXML(xml)
}

// EnterRoom sends the Room.Enter with AV_EN child to initiate studio bootstrapping.
func (c *Client) EnterRoom(roomID string) error {
	id := c.nextMsgID()
	xml := fmt.Sprintf("<Room.Enter msgID=\"%d\" roomID=\"%s\"><AV_EN/></Room.Enter>", id, roomID)
	return c.SendXML(xml)
}

// SendXML frames and writes a single XML message.
func (c *Client) SendXML(xml string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return fmt.Errorf("connection not open")
	}
	// Prepare trip counter padded to width 7 with spaces (left-padded)
	tripStr := fmt.Sprintf("%d", c.trip)
	if n := 7 - len(tripStr); n > 0 {
		tripStr = strings.Repeat(" ", n) + tripStr
	}
	// Hash: sha1(xml + tripStr + secret), lower-case hex
	h := sha1.New()
	h.Write([]byte(xml))
	h.Write([]byte(tripStr))
	h.Write([]byte(c.secret))
	sum := hex.EncodeToString(h.Sum(nil))
	framed := sum + " " + tripStr + " " + xml + "\x00"
	if c.logger != nil && strings.Contains(xml, "<System.Login") {
		// Debug: show hash and trip used for login
		secretPreview := c.secret
		if len(secretPreview) > 8 {
			secretPreview = secretPreview[:8]
		}
		c.logger.Printf("Realtime OUT LOGIN: sha1=%s trip=[%s] xmlLen=%d secret[0:8]=%q", sum, tripStr, len(xml), secretPreview)
	}
	// Increment trip for next send
	c.trip++
	// Write
	_, err := c.conn.Write([]byte(framed))
	return err
}

func (c *Client) readLoop() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	r := bufio.NewReader(conn)
	for {
		payload, err := r.ReadBytes(0x00)
		if err != nil {
			if c.logger != nil {
				c.logger.Printf("Realtime: read error: %v", err)
			}
			_ = c.Close()
			return
		}
		// Drop trailing NUL
		if len(payload) > 0 && payload[len(payload)-1] == 0x00 {
			payload = payload[:len(payload)-1]
		}
		if len(payload) == 0 {
			continue
		}
		// Detect <Accepted msgID> and resolve waiter if any
		if id, ok := parseAcceptedMsgID(payload); ok {
			c.mu.Lock()
			ch := c.pendingAcks[id]
			if ch != nil {
				delete(c.pendingAcks, id)
				c.mu.Unlock()
				select {
				case ch <- struct{}{}:
				default:
				}
			} else {
				c.mu.Unlock()
			}
		}
		// Log xml. In future: route to handlers.
		if c.logger != nil {
			c.logger.Printf("Realtime IN: %s", string(payload))
		}
	}
}

func (c *Client) nextMsgID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgID++
	return c.msgID
}

// SendLoginAndWait sends System.Login and waits for the server <Accepted msgID> or context cancellation.
func (c *Client) SendLoginAndWait(ctx context.Context, username string, secret string) error {
	encUser := strings.ToUpper(hex.EncodeToString([]byte(username)))
	encPass := strings.ToUpper(hex.EncodeToString([]byte(secret)))
	id := c.nextMsgID()
	xml := fmt.Sprintf("<System.Login msgID=\"%d\"><Username encoding=\"text/binhex\">%s</Username><Password encoding=\"text/binhex\">%s</Password></System.Login>", id, encUser, encPass)

	ch := make(chan struct{}, 1)
	c.mu.Lock()
	if c.pendingAcks == nil {
		c.pendingAcks = make(map[uint64]chan struct{})
	}
	c.pendingAcks[id] = ch
	c.mu.Unlock()

	if err := c.SendXML(xml); err != nil {
		c.mu.Lock()
		delete(c.pendingAcks, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pendingAcks, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// parseAcceptedMsgID extracts msgID from an <Accepted msgID="n"/> payload.
func parseAcceptedMsgID(b []byte) (uint64, bool) {
	needle := []byte("<Accepted msgID=\"")
	for i := 0; i+len(needle) <= len(b); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if b[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			k := i + len(needle)
			var id uint64
			for ; k < len(b); k++ {
				c := b[k]
				if c >= '0' && c <= '9' {
					id = id*10 + uint64(c-'0')
					continue
				}
				if c == '"' {
					return id, true
				}
				break
			}
		}
	}
	return 0, false
}

// SendMessengerMessage stubs instant message send for non-wasm
func (c *Client) SendMessengerMessage(targetName, message string) error {
	xml := fmt.Sprintf("<Room.Action roomID=\"$LOBBY$\"><MMS_MSG to=\"%s\" msg=\"%s\"/></Room.Action>", targetName, message)
	return c.SendXML(xml)
}

// SendFriendAccept stubs friend accept for non-wasm
func (c *Client) SendFriendAccept(fromUser string) error {
	xml := fmt.Sprintf("<Room.Action roomID=\"$LOBBY$\"><MMS_ACC from=\"%s\"/></Room.Action>", fromUser)
	return c.SendXML(xml)
}

// SendFriendReject stubs friend reject for non-wasm
func (c *Client) SendFriendReject(fromUser string) error {
	xml := fmt.Sprintf("<Room.Action roomID=\"$LOBBY$\"><MMS_REJ from=\"%s\"/></Room.Action>", fromUser)
	return c.SendXML(xml)
}
