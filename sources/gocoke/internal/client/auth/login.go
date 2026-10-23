package auth

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LoginResult struct {
	XMLName          xml.Name `xml:"LoginResult"`
	ErrorCode        int      `xml:"ErrorCode"`
	ErrorMessage     string   `xml:"ErrorMessage"`
	Gender           string   `xml:"Gender"`
	AvatarDefinition string   `xml:"AvatarDefinition"`
	ScreenName       string   `xml:"ScreenName"`
	MissionStatement string   `xml:"MissionStatement"`
	LastSeenIn       string   `xml:"LastSeenIn"`
	Banned           string   `xml:"Banned"`
	Secret           string   `xml:"Secret"`
}

// FetchLoginDenizen calls /sf/ssldocs/loginDenizen.jsp with the provided sid
// using the legacy parameter name "sidempty" and parses the XML into LoginResult.
func FetchLoginDenizen(httpClient *http.Client, base string, sid string) (*LoginResult, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	// Ensure base has no trailing slash issues and append path predictably
	base = strings.TrimRight(base, "/")
	endpoint := base + "/sf/ssldocs/loginDenizen.jsp"
	q := url.Values{}
	q.Set("sidempty", sid)
	full := endpoint + "?" + q.Encode()

	resp, err := httpClient.Get(full)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("loginDenizen http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("loginDenizen empty body")
	}
	// First try strict XML decode
	var lr LoginResult
	dec := xml.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&lr); err != nil {
		// Fallback: lenient tag extraction (server may return unescaped '&' in AvatarDefinition)
		ls, ferr := parseLoginResultLenient(string(b))
		if ferr != nil {
			// Prepare a sanitized snippet for diagnostics without leaking Secret
			s := string(b)
			if i := strings.Index(s, "<Secret>"); i >= 0 {
				if j := strings.Index(s[i:], "</Secret>"); j > 0 {
					closeIdx := i + j + len("</Secret>")
					s = s[:i] + "<Secret>REDACTED</Secret>" + s[closeIdx:]
				}
			}
			snippet := s
			if len(snippet) > 256 {
				snippet = snippet[:256]
			}
			return nil, fmt.Errorf("loginDenizen xml decode failed: %w; body=%q", err, snippet)
		}
		return &ls, nil
	}
	return &lr, nil
}

func parseLoginResultLenient(s string) (LoginResult, error) {
	var out LoginResult
	// Basic presence check
	if !strings.Contains(s, "<LoginResult>") {
		return out, fmt.Errorf("no LoginResult root")
	}
	out.ErrorMessage = between(s, "ErrorMessage")
	out.Gender = between(s, "Gender")
	out.AvatarDefinition = between(s, "AvatarDefinition")
	out.ScreenName = between(s, "ScreenName")
	out.MissionStatement = between(s, "MissionStatement")
	out.LastSeenIn = between(s, "LastSeenIn")
	out.Banned = between(s, "Banned")
	out.Secret = between(s, "Secret")
	if ec := between(s, "ErrorCode"); ec != "" {
		// tolerate parse failure silently; leave zero
		if n, err := atoiSafe(ec); err == nil {
			out.ErrorCode = n
		}
	}
	return out, nil
}

func between(s, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	i += len(open)
	j := strings.Index(s[i:], close)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

func atoiSafe(s string) (int, error) {
	// minimal, avoid strconv to keep dependencies small here
	var n int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("nan")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
