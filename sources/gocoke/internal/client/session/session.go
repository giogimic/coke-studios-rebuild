package session

import (
	"gocoke/internal/client/net/realtime"
	"sync"
)

var (
	mu         sync.RWMutex
	screenName string
	secret     string
	gatewayURL string
	rtClient   *realtime.Client
	rtReady    bool
	rtHost     string
	rtPort     string
	avatarDef  string
	mission    string
	gender     string
)

func SetCredentials(sn, sec string) {
	mu.Lock()
	screenName = sn
	secret = sec
	mu.Unlock()
}

func GetCredentials() (string, string) {
	mu.RLock()
	sn := screenName
	sec := secret
	mu.RUnlock()
	return sn, sec
}

func SetUserProfile(def, m, g string) {
	mu.Lock()
	avatarDef = def
	mission = m
	gender = g
	mu.Unlock()
}

func GetUserProfile() (string, string, string) {
	mu.RLock()
	d := avatarDef
	m := mission
	g := gender
	mu.RUnlock()
	return d, m, g
}

func SetGatewayURL(u string) {
	mu.Lock()
	gatewayURL = u
	mu.Unlock()
}

func GetGatewayURL() string {
	mu.RLock()
	u := gatewayURL
	mu.RUnlock()
	return u
}

func SetRealtimeClient(c *realtime.Client) {
	mu.Lock()
	rtClient = c
	mu.Unlock()
}

func GetRealtimeClient() *realtime.Client {
	mu.RLock()
	c := rtClient
	mu.RUnlock()
	return c
}

func SetRealtimeReady(ready bool) {
	mu.Lock()
	rtReady = ready
	mu.Unlock()
}

func IsRealtimeReady() bool {
	mu.RLock()
	r := rtReady
	mu.RUnlock()
	return r
}

func SetRealtimeEndpoint(host, port string) {
	mu.Lock()
	rtHost = host
	rtPort = port
	mu.Unlock()
}

func GetRealtimeEndpoint() (string, string) {
	mu.RLock()
	h := rtHost
	p := rtPort
	mu.RUnlock()
	return h, p
}
