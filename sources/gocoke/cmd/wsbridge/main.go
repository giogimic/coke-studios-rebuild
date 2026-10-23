package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()

	log.Printf("Parsed address flag: %q", *addr)

	// WebSocket realtime TCP proxy for browser clients (bridge XMLSocket over WS)
	http.HandleFunc("/ws/realtime", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		host := q.Get("host")
		port := q.Get("port")
		if host == "" || port == "" {
			http.Error(w, "missing host or port", http.StatusBadRequest)
			return
		}
		// Upgrade
		c, err := websocketAccept(w, r)
		if err != nil {
			log.Printf("ws accept error: %v", err)
			return
		}
		defer c.Close()

		// Best-effort preflight to Flash policy port 843
		if err := preflightFlashPolicy(host); err != nil {
			log.Printf("policy preflight (843) error: %v", err)
		}

		// Dial TCP target
		log.Printf("Connecting to game server: %s:%s", host, port)
		backend, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 8*time.Second)
		if err != nil {
			log.Printf("Failed to connect to game server: %v", err)
			_ = c.CloseWithStatus(1011, "tcp dial failed")
			return
		}
		defer func() {
			log.Printf("Closing connection to game server")
			backend.Close()
		}()

		// Flash policy-file handshake on game port as well
		_, _ = backend.Write([]byte("<policy-file-request/>\x00"))

		policyDone := make(chan struct{}, 1)

		// TCP -> WS: frame by NUL and forward as binary messages
		go func() {
			buf := make([]byte, 4096)
			partial := make([]byte, 0, 8192)
			policyMode := true
			for {
				n, err := backend.Read(buf)
				if n > 0 {
					partial = append(partial, buf[:n]...)
					for {
						if policyMode {
							// Try to detect end of policy XML without relying on NUL
							endTag := []byte("</cross-domain-policy>")
							if i := indexSubslice(partial, endTag); i >= 0 {
								end := i + len(endTag)
								frame := make([]byte, end)
								copy(frame, partial[:end])
								partial = partial[end:]
								log.Printf("Policy(game-port): %s", string(frame))
								_ = websocketWriteBinary(context.Background(), c, frame)
								policyMode = false
								select {
								case policyDone <- struct{}{}:
								default:
								}
								continue
							}
						}
						idx := indexByte(partial, 0x00)
						if idx < 0 {
							break
						}
						frame := make([]byte, idx)
						copy(frame, partial[:idx])
						partial = partial[idx+1:]
						if len(frame) == 0 {
							log.Printf("Server->Client: [EMPTY RESPONSE - %d bytes]", len(frame))
						} else {
							log.Printf("Server->Client: [%d bytes] %s", len(frame), string(frame))
						}
						_ = websocketWriteBinary(context.Background(), c, frame)
					}
				}
				if err != nil {
					log.Printf("TCP read error: %v", err)
					return
				}
			}
		}()

		// WS -> TCP: strictly wait for policy before forwarding any client messages
		<-policyDone
		for {
			msg, err := websocketRead(context.Background(), c)
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				return
			}
			if len(msg) == 0 {
				continue
			}
			// Ensure each forwarded frame ends with NUL terminator
			if msg[len(msg)-1] != 0x00 {
				msg = append(msg, 0x00)
			}
			log.Printf("Client->Server: %s", string(msg[:len(msg)-1])) // Log without NUL
			_, _ = backend.Write(msg)
		}
	})

	log.Printf("WebSocket bridge listening at http://%s/ws/realtime", *addr)
	log.Printf("Starting server on address: %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func indexByte(b []byte, target byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == target {
			return i
		}
	}
	return -1
}

func indexSubslice(haystack []byte, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// --- Minimal RFC6455 upgrader (limited scope for dev proxy) ---

type upgradedConn struct {
	netConn net.Conn
	rw      *bufio.ReadWriter
}

func (c *upgradedConn) Close() error                                  { return c.netConn.Close() }
func (c *upgradedConn) CloseWithStatus(code int, reason string) error { return c.Close() }

func (c *upgradedConn) WriteBinary(p []byte) error {
	// Frame: FIN|binary, no mask, payload len, payload
	hdr := []byte{0x82}
	var lenField []byte
	l := len(p)
	switch {
	case l < 126:
		lenField = []byte{byte(l)}
	case l <= 0xFFFF:
		lenField = []byte{126, byte(l >> 8), byte(l)}
	default:
		lenField = []byte{127, 0, 0, 0, 0, byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
	}
	if _, err := c.rw.Write(hdr); err != nil {
		return err
	}
	if _, err := c.rw.Write(lenField); err != nil {
		return err
	}
	if _, err := c.rw.Write(p); err != nil {
		return err
	}
	return c.rw.Flush()
}

func (c *upgradedConn) ReadBinary() ([]byte, error) {
	br := c.rw
	b1, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	b2, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	opcode := b1 & 0x0F
	masked := (b2 & 0x80) != 0
	if opcode == 0x8 {
		return nil, io.EOF
	}
	if opcode != 0x2 && opcode != 0x1 && opcode != 0x0 {
		return nil, errors.New("unsupported opcode")
	}
	payloadLen := int(b2 & 0x7F)
	switch payloadLen {
	case 126:
		b, _ := br.Peek(2)
		_, _ = br.Discard(2)
		payloadLen = int(b[0])<<8 | int(b[1])
	case 127:
		b, _ := br.Peek(8)
		_, _ = br.Discard(8)
		payloadLen = int(b[4])<<24 | int(b[5])<<16 | int(b[6])<<8 | int(b[7])
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(br, maskKey[:]); err != nil {
			return nil, err
		}
	}
	buf := make([]byte, payloadLen)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, err
	}
	if masked {
		for i := 0; i < payloadLen; i++ {
			buf[i] ^= maskKey[i%4]
		}
	}
	return buf, nil
}

func acceptRFC6455(w http.ResponseWriter, r *http.Request) (*upgradedConn, error) {
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("hijack not supported")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(hash[:])
	conn, rw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	rw.WriteString("Upgrade: websocket\r\n")
	rw.WriteString("Connection: Upgrade\r\n")
	rw.WriteString("Sec-WebSocket-Accept: ")
	rw.WriteString(accept)
	rw.WriteString("\r\n\r\n")
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &upgradedConn{netConn: conn, rw: rw}, nil
}

// websocketAccept upgrades the connection using RFC6455 minimal handshake.
func websocketAccept(w http.ResponseWriter, r *http.Request) (*upgradedConn, error) {
	return acceptRFC6455(w, r)
}

func websocketWriteBinary(ctx context.Context, c *upgradedConn, payload []byte) error {
	return c.WriteBinary(payload)
}

func websocketRead(ctx context.Context, c *upgradedConn) ([]byte, error) {
	return c.ReadBinary()
}

// preflightFlashPolicy dials host:843 and requests the Flash crossdomain policy.
func preflightFlashPolicy(host string) error {
	d, err := net.DialTimeout("tcp", net.JoinHostPort(host, "843"), 2*time.Second)
	if err != nil {
		return err
	}
	defer d.Close()
	_ = d.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = d.Write([]byte("<policy-file-request/>\x00"))
	buf := make([]byte, 4096)
	n, _ := d.Read(buf)
	if n > 0 {
		log.Printf("Policy(843): %s", string(buf[:n]))
	}
	return nil
}
