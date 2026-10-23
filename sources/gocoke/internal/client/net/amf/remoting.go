package amf

// Minimal AMF0 remoting envelope encoder/decoder (client-side) sufficient to
// perform simple calls like FlashDenizenManager.getTip. This preserves the
// classic AMF0 Remoting envelope layout and encodes the message body as a
// StrictArray of parameters.
//
// NOTE: This is intentionally small and self-contained. We can later swap the
// value encoder/decoder to ossrs/go-oryx-lib if needed, while retaining this
// envelope logic.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// EncodeRemotingRequest builds an AMF0 Remoting envelope for a single message.
// - target: service and method, e.g., "FlashDenizenManager.getTip"
// - params: ordered parameters to the method
func EncodeRemotingRequest(target string, params []any) ([]byte, error) {
	// Body is a StrictArray of params
	body, err := encodeAMF0StrictArray(params)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	// AMF version (u16) = 0 for AMF0
	binary.Write(&out, binary.BigEndian, uint16(0))
	// Header count (u16) = 0
	binary.Write(&out, binary.BigEndian, uint16(0))
	// Message count (u16) = 1
	binary.Write(&out, binary.BigEndian, uint16(1))
	// Target (UTF-8, 2-byte length)
	writeUTF(&out, target)
	// Response URI (client-side correlation).
	// Conventionally "/1"; server ignores and responds with "/1/onResult".
	writeUTF(&out, "/1")
	// Body length (i32)
	binary.Write(&out, binary.BigEndian, int32(len(body)))
	// Body bytes
	out.Write(body)
	return out.Bytes(), nil
}

// DecodeRemotingResponse parses an AMF0 Remoting response envelope and returns
// the first message body decoded as a Go value.
func DecodeRemotingResponse(b []byte) (any, error) {
	p := &parser{b: b}
	// version
	if _, err := p.readU16(); err != nil {
		return nil, err
	}
	// headers
	if hc, err := p.readU16(); err != nil {
		return nil, err
	} else {
		for i := 0; i < int(hc); i++ {
			// header name
			if _, err := p.readUTF(); err != nil {
				return nil, err
			}
			// must understand (bool)
			if _, err := p.readU8(); err != nil {
				return nil, err
			}
			// header length (i32)
			l, err := p.readI32()
			if err != nil {
				return nil, err
			}
			if l < 0 {
				// Some servers send -1 for unknown header length; decode one AMF0 value to advance
				if _, consumed, err := decodeAMF0Value(p.b[p.i:], 0); err != nil {
					return nil, err
				} else {
					p.i += consumed
				}
			} else {
				if _, err := p.readN(int(l)); err != nil {
					return nil, err
				}
			}
		}
	}
	// messages
	mc, err := p.readU16()
	if err != nil {
		return nil, err
	}
	if mc < 1 {
		return nil, errors.New("amf: no messages in response")
	}
	// Only parse the first message
	target, err := p.readUTF()
	if err != nil {
		return nil, err
	} // e.g., /1/onResult
	respURI, err := p.readUTF()
	if err != nil {
		return nil, err
	} // often empty
	l, err := p.readI32()
	if err != nil {
		return nil, err
	}
	var body []byte
	if l <= 0 {
		// Unknown/zero length observed in some servers: use the rest of the buffer
		if p.i > len(p.b) {
			return nil, errors.New("amf: body offset past end")
		}
		body = p.b[p.i:]
		p.i = len(p.b)
	} else {
		body, err = p.readN(int(l))
		if err != nil {
			return nil, err
		}
	}
	// Body is an AMF0 value
	val, _, err := decodeAMF0Value(body, 0)
	if err != nil {
		// Debug aid: show first bytes when decode fails
		max := 64
		if len(body) < max {
			max = len(body)
		}
		return nil, fmt.Errorf("amf: body decode failed (len=%d target=%q resp=%q): % x", len(body), target, respURI, body[:max])
	}
	return val, nil
}

// ---- AMF0 minimal value codec ----

const (
	amf0Number    = 0x00
	amf0Boolean   = 0x01
	amf0String    = 0x02
	amf0Object    = 0x03
	amf0Null      = 0x05
	amf0ECMAArray = 0x08
	amf0ObjectEnd = 0x09
	amf0StrictArr = 0x0A
	amf0Date      = 0x0B
	amf0LongStr   = 0x0C
	amf0XMLDoc    = 0x0F
	amf0TypedObj  = 0x10
)

func encodeAMF0Value(v any) ([]byte, error) {
	var buf bytes.Buffer
	switch x := v.(type) {
	case nil:
		buf.WriteByte(amf0Null)
	case bool:
		buf.WriteByte(amf0Boolean)
		if x {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	case int:
		buf.WriteByte(amf0Number)
		var f float64 = float64(x)
		_ = binary.Write(&buf, binary.BigEndian, f)
	case int32:
		buf.WriteByte(amf0Number)
		var f float64 = float64(x)
		_ = binary.Write(&buf, binary.BigEndian, f)
	case float64:
		buf.WriteByte(amf0Number)
		_ = binary.Write(&buf, binary.BigEndian, x)
	case string:
		buf.WriteByte(amf0String)
		writeUTF(&buf, x)
	case []any:
		b, err := encodeAMF0StrictArray(x)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	case map[string]any:
		buf.WriteByte(amf0Object)
		for k, vv := range x {
			writeUTF(&buf, k)
			vb, err := encodeAMF0Value(vv)
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		// object end: 0x00 0x00 0x09
		buf.Write([]byte{0x00, 0x00, amf0ObjectEnd})
	default:
		return nil, errors.New("amf: unsupported type in encoder")
	}
	return buf.Bytes(), nil
}

func encodeAMF0StrictArray(arr []any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(amf0StrictArr)
	binary.Write(&buf, binary.BigEndian, uint32(len(arr)))
	for _, e := range arr {
		b, err := encodeAMF0Value(e)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

func decodeAMF0Value(b []byte, i int) (any, int, error) {
	if i >= len(b) {
		return nil, i, errors.New("amf: decode past end")
	}
	t := b[i]
	i++
	switch t {
	case amf0Null:
		return nil, i, nil
	case amf0Boolean:
		if i >= len(b) {
			return nil, i, errors.New("amf: bool truncated")
		}
		v := b[i] != 0
		return v, i + 1, nil
	case amf0Number:
		if i+8 > len(b) {
			return nil, i, errors.New("amf: number truncated")
		}
		bits := binary.BigEndian.Uint64(b[i : i+8])
		f := math.Float64frombits(bits)
		return f, i + 8, nil
	case amf0String:
		s, ni, err := readUTFFrom(b, i)
		return s, ni, err
	case amf0Object:
		m := map[string]any{}
		for {
			if i+2 > len(b) {
				return nil, i, errors.New("amf: object key trunc")
			}
			if b[i] == 0 && b[i+1] == 0 {
				i += 2
				if i >= len(b) || b[i] != amf0ObjectEnd {
					return nil, i, errors.New("amf: object end marker")
				}
				return m, i + 1, nil
			}
			key, ni, err := readUTFFrom(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni
			val, ni2, err := decodeAMF0Value(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni2
			m[key] = val
		}
	case amf0ECMAArray:
		if i+4 > len(b) {
			return nil, i, errors.New("amf: ecma array trunc")
		}
		_ = binary.BigEndian.Uint32(b[i : i+4])
		i += 4
		m := map[string]any{}
		for {
			if i+2 > len(b) {
				return nil, i, errors.New("amf: ecma array key trunc")
			}
			if b[i] == 0 && b[i+1] == 0 {
				i += 2
				if i >= len(b) || b[i] != amf0ObjectEnd {
					return nil, i, errors.New("amf: ecma array end marker")
				}
				return m, i + 1, nil
			}
			key, ni, err := readUTFFrom(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni
			val, ni2, err := decodeAMF0Value(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni2
			m[key] = val
		}
	case amf0StrictArr:
		if i+4 > len(b) {
			return nil, i, errors.New("amf: strict array trunc")
		}
		n := int(binary.BigEndian.Uint32(b[i : i+4]))
		i += 4
		out := make([]any, 0, n)
		for c := 0; c < n; c++ {
			v, ni, err := decodeAMF0Value(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni
			out = append(out, v)
		}
		return out, i, nil
	case amf0TypedObj:
		// Typed object: alias (UTF) followed by object (key/value pairs terminated by 00 00 09)
		alias, ni, err := readUTFFrom(b, i)
		if err != nil {
			return nil, i, err
		}
		i = ni
		m := map[string]any{"__alias": alias}
		for {
			if i+2 > len(b) {
				return nil, i, errors.New("amf: typed obj key trunc")
			}
			if b[i] == 0 && b[i+1] == 0 {
				i += 2
				if i >= len(b) || b[i] != amf0ObjectEnd {
					return nil, i, errors.New("amf: typed obj end marker")
				}
				return m, i + 1, nil
			}
			key, ni2, err := readUTFFrom(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni2
			val, ni3, err := decodeAMF0Value(b, i)
			if err != nil {
				return nil, i, err
			}
			i = ni3
			m[key] = val
		}
	case amf0LongStr:
		if i+4 > len(b) {
			return nil, i, errors.New("amf: long string len trunc")
		}
		l := int(binary.BigEndian.Uint32(b[i : i+4]))
		i += 4
		if i+l > len(b) {
			return nil, i, errors.New("amf: long string body past end")
		}
		return string(b[i : i+l]), i + l, nil
	case amf0Date:
		// AMF0 Date: 8-byte IEEE-754 (milliseconds since epoch) + 2-byte timezone (ignored)
		if i+8 > len(b) {
			return nil, i, errors.New("amf: date truncated")
		}
		bits := binary.BigEndian.Uint64(b[i : i+8])
		f := math.Float64frombits(bits)
		i += 8
		// Skip timezone
		if i+2 > len(b) {
			return nil, i, errors.New("amf: date tz trunc")
		}
		i += 2
		return f, i, nil
	default:
		return nil, i, errors.New("amf: unsupported type in decoder")
	}
}

// Helpers

type parser struct {
	b []byte
	i int
}

func (p *parser) readU8() (byte, error) {
	if p.i >= len(p.b) {
		return 0, errors.New("amf: u8 past end")
	}
	v := p.b[p.i]
	p.i++
	return v, nil
}
func (p *parser) readU16() (uint16, error) {
	if p.i+2 > len(p.b) {
		return 0, errors.New("amf: u16 past end")
	}
	v := binary.BigEndian.Uint16(p.b[p.i : p.i+2])
	p.i += 2
	return v, nil
}
func (p *parser) readI32() (int32, error) {
	if p.i+4 > len(p.b) {
		return 0, errors.New("amf: i32 past end")
	}
	v := int32(binary.BigEndian.Uint32(p.b[p.i : p.i+4]))
	p.i += 4
	return v, nil
}
func (p *parser) readN(n int) ([]byte, error) {
	if n < 0 {
		return nil, errors.New("amf: readN negative length")
	}
	if p.i+n > len(p.b) {
		return nil, errors.New("amf: readN past end")
	}
	v := p.b[p.i : p.i+n]
	p.i += n
	return v, nil
}
func (p *parser) readUTF() (string, error) {
	s, ni, err := readUTFFrom(p.b, p.i)
	p.i = ni
	return s, err
}

func writeUTF(buf *bytes.Buffer, s string) {
	binary.Write(buf, binary.BigEndian, uint16(len(s)))
	buf.WriteString(s)
}

func readUTFFrom(b []byte, i int) (string, int, error) {
	if i+2 > len(b) {
		return "", i, errors.New("amf: utf len past end")
	}
	l := int(binary.BigEndian.Uint16(b[i : i+2]))
	i += 2
	if i+l > len(b) {
		return "", i, errors.New("amf: utf body past end")
	}
	return string(b[i : i+l]), i + l, nil
}

// EncodeRemotingResponse builds an AMF0 Remoting response envelope for a value.
func EncodeRemotingResponse(target string, val any) ([]byte, error) {
	valBytes, err := encodeAMF0Value(val)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, uint16(0))
	binary.Write(&out, binary.BigEndian, uint16(0))
	binary.Write(&out, binary.BigEndian, uint16(1))
	writeUTF(&out, target)
	writeUTF(&out, "null")
	binary.Write(&out, binary.BigEndian, int32(len(valBytes)))
	out.Write(valBytes)
	return out.Bytes(), nil
}

// ParseRemotingTarget parses target and responseURI from an AMF0 envelope.
func ParseRemotingTarget(b []byte) (string, string, error) {
	p := &parser{b: b}
	if _, err := p.readU16(); err != nil {
		return "", "", err
	}
	hc, err := p.readU16()
	if err != nil {
		return "", "", err
	}
	for i := 0; i < int(hc); i++ {
		if _, err := p.readUTF(); err != nil {
			return "", "", err
		}
		if _, err := p.readU8(); err != nil {
			return "", "", err
		}
		l, err := p.readI32()
		if err != nil {
			return "", "", err
		}
		if l >= 0 {
			if _, err := p.readN(int(l)); err != nil {
				return "", "", err
			}
		}
	}
	mc, err := p.readU16()
	if err != nil || mc < 1 {
		return "", "", errors.New("amf: no messages")
	}
	target, err := p.readUTF()
	if err != nil {
		return "", "", err
	}
	respURI, err := p.readUTF()
	if err != nil {
		return "", "", err
	}
	return target, respURI, nil
}

