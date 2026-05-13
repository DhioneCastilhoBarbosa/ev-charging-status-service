package csmsstomp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const defaultStompHeartbeatMs = 10000

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// DialConfig conexão STOMP sobre SockJS ao CSMS Move.
type DialConfig struct {
	Host             string
	SockJSPrefix     string // ex.: "/ws"
	UseTLS           bool
	BearerToken      string // JWT (sem prefixo "Bearer ")
	ChargeBoxUUIDs   []string
	StompHeartbeatMs int
}

// RunConsumer mantém uma sessão até ctx cancelar, erro de leitura ou falha de dial.
// onMessage é chamado em série (mesma goroutine do read loop); deve retornar rápido.
func RunConsumer(ctx context.Context, cfg DialConfig, onMessage func(StatusEvent)) error {
	if len(cfg.ChargeBoxUUIDs) == 0 {
		return fmt.Errorf("csmsstomp: no charge box uuids to subscribe")
	}
	hb := cfg.StompHeartbeatMs
	if hb <= 0 {
		hb = defaultStompHeartbeatMs
	}
	token := strings.TrimSpace(cfg.BearerToken)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return fmt.Errorf("csmsstomp: empty bearer token")
	}

	scheme := "ws"
	if cfg.UseTLS {
		scheme = "wss"
	}
	prefix := strings.TrimSuffix(cfg.SockJSPrefix, "/")
	if prefix == "" {
		prefix = "/ws"
	}

	probeSockJSInfo(cfg.UseTLS, cfg.Host, prefix)

	serverID := randomServerID()
	sessionID := randomSessionID()
	wsURL := url.URL{
		Scheme: scheme,
		Host:   cfg.Host,
		Path:   fmt.Sprintf("%s/%s/%s/websocket", prefix, serverID, sessionID),
	}
	log.Printf("[csms-stomp] connecting %s", wsURL.String())

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL.String(), nil)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return fmt.Errorf("dial: HTTP %d %s: %w", resp.StatusCode, string(body), err)
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	stopWatchdog := make(chan struct{})
	defer close(stopWatchdog)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"),
				time.Now().Add(time.Second),
			)
			_ = conn.Close()
		case <-stopWatchdog:
		}
	}()

	if err := waitForOpen(ctx, conn); err != nil {
		return fmt.Errorf("sockjs open: %w", err)
	}
	// waitForOpen usa deadlines curtos por leitura; limpar para o loop STOMP poder bloquear indefinidamente.
	_ = conn.SetReadDeadline(time.Time{})

	if err := sendSockJSFrame(conn, buildConnectFrame(cfg.Host, hb, token)); err != nil {
		return fmt.Errorf("stomp connect: %w", err)
	}

	for i, uuid := range cfg.ChargeBoxUUIDs {
		subID := fmt.Sprintf("sub-%d", i)
		if err := sendSockJSFrame(conn, buildSubscribeFrame(subID, uuid)); err != nil {
			return fmt.Errorf("subscribe %s: %w", uuid, err)
		}
		log.Printf("[csms-stomp] subscribed id=%s destination=/topic/status/chargeBox/%s", subID, uuid)
	}

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go runHeartbeat(heartbeatCtx, conn, hb)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}
		processSockJS(string(msg), onMessage)
	}
}

func runHeartbeat(ctx context.Context, conn *websocket.Conn, intervalMs int) {
	t := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := sendSockJSFrame(conn, "\n"); err != nil {
				log.Printf("[csms-stomp] heartbeat write: %v", err)
				return
			}
		}
	}
}

func processSockJS(message string, onMessage func(StatusEvent)) {
	if message == "" {
		return
	}
	switch message[0] {
	case 'o', 'h':
		return
	case 'c':
		log.Printf("[csms-stomp] sockjs close: %s", message)
	case 'a':
		var frames []string
		if err := json.Unmarshal([]byte(message[1:]), &frames); err != nil {
			log.Printf("[csms-stomp] decode sockjs a[]: %v", err)
			return
		}
		for _, f := range frames {
			processStomp(f, onMessage)
		}
	}
}

func processStomp(frame string, onMessage func(StatusEvent)) {
	cmdEnd := strings.Index(frame, "\n")
	if cmdEnd < 0 {
		return
	}
	command := frame[:cmdEnd]
	switch command {
	case "CONNECTED":
		log.Println("[csms-stomp] STOMP CONNECTED")
	case "ERROR":
		log.Printf("[csms-stomp] STOMP ERROR:\n%s", frame)
	case "MESSAGE":
		body := extractStompBody(frame)
		var ev StatusEvent
		if err := json.Unmarshal([]byte(body), &ev); err != nil {
			log.Printf("[csms-stomp] MESSAGE json: %v body=%s", err, truncateForLog(body, 400))
			return
		}
		if strings.TrimSpace(ev.ChargeBoxUUID) == "" {
			log.Printf("[csms-stomp] MESSAGE sem chargeBoxUuid no JSON (chargeBoxId=%q) body=%s",
				ev.ChargeBoxID, truncateForLog(body, 400))
		}
		onMessage(ev)
	}
}

func sendSockJSFrame(conn *websocket.Conn, frame string) error {
	payload, err := json.Marshal([]string{frame})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func waitForOpen(ctx context.Context, conn *websocket.Conn) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := time.Until(deadline)
		if chunk > 2*time.Second {
			chunk = 2 * time.Second
		}
		_ = conn.SetReadDeadline(time.Now().Add(chunk))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if len(msg) > 0 && msg[0] == 'o' {
			return nil
		}
	}
	return fmt.Errorf("timeout waiting sockjs open")
}

func extractStompBody(frame string) string {
	sep := strings.Index(frame, "\n\n")
	if sep < 0 {
		return ""
	}
	body := frame[sep+2:]
	body = strings.TrimRight(body, "\x00")
	body = strings.TrimRight(body, "\n")
	return body
}

func buildConnectFrame(host string, stompHeartbeatMs int, jwt string) string {
	return fmt.Sprintf(
		"CONNECT\naccept-version:1.2\nhost:%s\nheart-beat:%d,%d\nAuthorization:Bearer %s\nPlatform:API\n\n\x00",
		host, stompHeartbeatMs, stompHeartbeatMs, jwt,
	)
}

func buildSubscribeFrame(subID, chargeBoxUUID string) string {
	return fmt.Sprintf(
		"SUBSCRIBE\nid:%s\ndestination:/topic/status/chargeBox/%s\n\n\x00",
		subID, chargeBoxUUID,
	)
}

func probeSockJSInfo(useTLS bool, host, prefix string) {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	infoURL := fmt.Sprintf("%s://%s%s/info", scheme, host, strings.TrimSuffix(prefix, "/"))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(infoURL)
	if err != nil {
		log.Printf("[csms-stomp] probe /info: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[csms-stomp] probe /info status=%d body=%s", resp.StatusCode, string(body))
	}
}

func randomServerID() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return "000"
	}
	return fmt.Sprintf("%03d", n.Int64())
}

func randomSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}
