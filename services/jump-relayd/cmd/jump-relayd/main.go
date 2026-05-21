package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sting8k/jump/packages/relayproto"
	"nhooyr.io/websocket"
)

const maxHTTPBody = 16 << 20

func main() {
	listen := flag.String("listen", ":8791", "HTTP listen address")
	token := flag.String("token", "", "bearer token required for jumpd agent")
	tokenFile := flag.String("token-file", "", "file containing bearer token required for jumpd agent")
	agentPath := flag.String("agent-path", "/_jump/agent", "WebSocket path for the jumpd agent")
	flag.Parse()

	resolvedToken := strings.TrimSpace(*token)
	if resolvedToken == "" && strings.TrimSpace(*tokenFile) != "" {
		b, err := os.ReadFile(*tokenFile)
		if err != nil {
			log.Fatalf("read -token-file: %v", err)
		}
		resolvedToken = strings.TrimSpace(string(b))
	}
	if resolvedToken == "" {
		log.Fatal("-token or -token-file is required")
	}

	r := newRelay(resolvedToken)
	mux := http.NewServeMux()
	mux.HandleFunc(*agentPath, r.handleAgent)
	mux.HandleFunc("/", r.handleBrowser)

	log.Printf("jump-relayd listening on %s", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

type relay struct {
	token string
	mu    sync.Mutex
	agent *agentConn
}

func newRelay(token string) *relay { return &relay{token: token} }

func (r *relay) handleAgent(w http.ResponseWriter, req *http.Request) {
	if !r.authorized(req) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("agent accept: %v", err)
		return
	}
	conn.SetReadLimit(32 << 20)

	a := newAgentConn(conn)
	r.mu.Lock()
	old := r.agent
	r.agent = a
	r.mu.Unlock()
	if old != nil {
		old.close(websocket.StatusPolicyViolation, "replaced by new agent")
	}

	log.Printf("agent connected from %s", req.RemoteAddr)
	a.readLoop(req.Context())

	r.mu.Lock()
	if r.agent == a {
		r.agent = nil
	}
	r.mu.Unlock()
	a.failAll(errors.New("agent disconnected"))
	log.Printf("agent disconnected")
}

func (r *relay) handleBrowser(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/_jump/health" {
		r.handleHealth(w, req)
		return
	}
	agent := r.currentAgent()
	if agent == nil {
		writeError(w, http.StatusServiceUnavailable, "jump agent not connected")
		return
	}

	if websocketAcceptRequested(req) {
		agent.proxyWebSocket(w, req)
		return
	}
	agent.proxyHTTP(w, req)
}

func (r *relay) handleHealth(w http.ResponseWriter, _ *http.Request) {
	connected := r.currentAgent() != nil
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "agent_connected": connected})
}

func (r *relay) currentAgent() *agentConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agent
}

func (r *relay) authorized(req *http.Request) bool {
	got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(r.token)) == 1
}

type agentConn struct {
	conn *websocket.Conn

	sendMu sync.Mutex
	mu     sync.Mutex
	seq    uint64
	pend   map[string]chan relayproto.Frame
	ws     map[string]*websocket.Conn
}

func newAgentConn(conn *websocket.Conn) *agentConn {
	return &agentConn{conn: conn, pend: map[string]chan relayproto.Frame{}, ws: map[string]*websocket.Conn{}}
}

func (a *agentConn) nextID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	return fmt.Sprintf("r%d", a.seq)
}

func (a *agentConn) send(ctx context.Context, f relayproto.Frame) error {
	b, err := relayproto.Marshal(f)
	if err != nil {
		return err
	}
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	return a.conn.Write(ctx, websocket.MessageBinary, b)
}

func (a *agentConn) readLoop(ctx context.Context) {
	for {
		typ, data, err := a.conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageBinary {
			continue
		}
		f, err := relayproto.Unmarshal(data)
		if err != nil {
			log.Printf("relay frame decode: %v", err)
			continue
		}
		a.dispatch(ctx, f)
	}
}

func (a *agentConn) dispatch(ctx context.Context, f relayproto.Frame) {
	switch f.Type {
	case relayproto.TypeHTTPResp, relayproto.TypeWSResult:
		a.mu.Lock()
		ch := a.pend[f.ID]
		delete(a.pend, f.ID)
		a.mu.Unlock()
		if ch != nil {
			ch <- f
		}
	case relayproto.TypeWSData:
		a.mu.Lock()
		ws := a.ws[f.ID]
		a.mu.Unlock()
		if ws != nil {
			_ = ws.Write(ctx, websocket.MessageType(f.MessageType), f.Data)
		}
	case relayproto.TypeWSClose:
		a.mu.Lock()
		ws := a.ws[f.ID]
		delete(a.ws, f.ID)
		a.mu.Unlock()
		if ws != nil {
			_ = ws.Close(websocket.StatusNormalClosure, f.Error)
		}
	}
}

func (a *agentConn) proxyHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, maxHTTPBody+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read request body")
		return
	}
	if len(body) > maxHTTPBody {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	id := a.nextID()
	ch := make(chan relayproto.Frame, 1)
	a.mu.Lock()
	a.pend[id] = ch
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(req.Context(), 60*time.Second)
	defer cancel()
	if err := a.send(ctx, relayproto.Frame{Type: relayproto.TypeHTTPReq, ID: id, Method: req.Method, Path: req.URL.RequestURI(), Header: cloneHeader(req.Header), Body: body}); err != nil {
		a.removePending(id)
		writeError(w, http.StatusBadGateway, "agent send failed")
		return
	}

	select {
	case resp := <-ch:
		copyHeader(w.Header(), resp.Header)
		if resp.Status == 0 {
			resp.Status = http.StatusBadGateway
		}
		w.WriteHeader(resp.Status)
		_, _ = w.Write(resp.Body)
	case <-ctx.Done():
		a.removePending(id)
		writeError(w, http.StatusGatewayTimeout, "agent response timeout")
	}
}

func (a *agentConn) proxyWebSocket(w http.ResponseWriter, req *http.Request) {
	browser, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("browser ws accept: %v", err)
		return
	}
	browser.SetReadLimit(4 << 20)

	id := a.nextID()
	ch := make(chan relayproto.Frame, 1)
	a.mu.Lock()
	a.pend[id] = ch
	a.ws[id] = browser
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()
	if err := a.send(ctx, relayproto.Frame{Type: relayproto.TypeWSOpen, ID: id, Method: req.Method, Path: req.URL.RequestURI(), Header: cloneHeader(req.Header)}); err != nil {
		cancel()
		a.removeWS(id)
		_ = browser.Close(websocket.StatusInternalError, "agent send failed")
		return
	}

	select {
	case resp := <-ch:
		cancel()
		if resp.Error != "" || resp.Status >= 400 {
			a.removeWS(id)
			_ = browser.Close(websocket.StatusPolicyViolation, resp.Error)
			return
		}
	case <-ctx.Done():
		a.removePending(id)
		a.removeWS(id)
		_ = browser.Close(websocket.StatusTryAgainLater, "agent open timeout")
		return
	}

	defer func() {
		a.removeWS(id)
		_ = a.send(context.Background(), relayproto.Frame{Type: relayproto.TypeWSClose, ID: id})
	}()
	for {
		typ, data, err := browser.Read(req.Context())
		if err != nil {
			return
		}
		if err := a.send(req.Context(), relayproto.Frame{Type: relayproto.TypeWSData, ID: id, MessageType: int(typ), Data: data}); err != nil {
			return
		}
	}
}

func (a *agentConn) removePending(id string) {
	a.mu.Lock()
	delete(a.pend, id)
	a.mu.Unlock()
}

func (a *agentConn) removeWS(id string) {
	a.mu.Lock()
	ws := a.ws[id]
	delete(a.ws, id)
	a.mu.Unlock()
	if ws != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}
}

func (a *agentConn) failAll(err error) {
	a.mu.Lock()
	pend := a.pend
	ws := a.ws
	a.pend = map[string]chan relayproto.Frame{}
	a.ws = map[string]*websocket.Conn{}
	a.mu.Unlock()
	for _, ch := range pend {
		ch <- relayproto.Frame{Type: relayproto.TypeHTTPResp, Status: http.StatusBadGateway, Error: err.Error(), Body: []byte(err.Error())}
	}
	for _, c := range ws {
		_ = c.Close(websocket.StatusTryAgainLater, err.Error())
	}
}

func (a *agentConn) close(code websocket.StatusCode, reason string) {
	_ = a.conn.Close(code, reason)
}

func websocketAcceptRequested(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") && strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		if strings.EqualFold(k, "Connection") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		dst.Del(k)
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}
