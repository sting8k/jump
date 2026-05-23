package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sting8k/jump/services/jumpd/internal/presence"
	"github.com/sting8k/jump/services/jumpd/internal/store"
)

// testConfig uses short durations for fast tests.
func testConfig() Config {
	return Config{
		GracePeriod:               50 * time.Millisecond,
		IdleThreshold:             2 * time.Minute,
		ActivityNtfyCooldown:      100 * time.Millisecond,
		NotifyRateLimit:           3,
		NotifyRateWindow:          2 * time.Minute,
		WorkspaceNotifyRateLimit:  2,
		WorkspaceNotifyRateWindow: time.Minute,
	}
}

func nowSecs() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

// testEnv bundles a store, presence table, and router for testing.
type testEnv struct {
	store    *store.Store
	presence *presence.Table
	router   *Router
	cancel   context.CancelFunc
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s := store.New()
	router := (*Router)(nil)
	p := presence.New(presence.Callbacks{
		OnClientFocused: func(_ string) {
			if router != nil {
				router.CancelAll()
			}
		},
		OnSessionSelected: func(_, sessID string) {
			if router != nil {
				router.CancelForSession(sessID)
			}
		},
	})
	router = New(p, s, testConfig())
	ctx, cancel := context.WithCancel(context.Background())
	go router.Run(ctx)

	t.Cleanup(func() { cancel() })
	return &testEnv{store: s, presence: p, router: router, cancel: cancel}
}

// addClient adds a focused, granted client to the presence table with a nil conn.
// The nil conn means sendJSON will log an error but not crash — sufficient for
// testing routing decisions.
func (e *testEnv) addClient(id, deviceType string) {
	e.presence.Add(&presence.Client{
		ID:                     id,
		DeviceType:             deviceType,
		NotificationPermission: "granted",
		LastInteraction:        nowSecs(),
		ConnectedAt:            time.Now(),
	})
}

// upsertSession creates or updates a session in the store.
func (e *testEnv) upsertSession(id string, working, unread, alive bool) {
	e.upsertSessionInWorkspace(id, "", working, unread, alive)
}

func (e *testEnv) upsertSessionInWorkspace(id, workspace string, working, unread, alive bool) {
	var status *store.Status
	if working {
		status = &store.Status{Label: "working", Working: true}
	} else {
		status = &store.Status{Label: "idle", Working: false}
	}
	e.store.Upsert(store.Session{
		ID:            id,
		Title:         "test-" + id,
		Alive:         alive,
		Status:        status,
		Unread:        unread,
		WorkspaceRoot: workspace,
		StartedAt:     time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
	})
}

func TestTransition_WorkingToIdle_SchedulesNotification(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	// Client is not focused → notifications should fire.
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	// Seed session as working.
	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond) // let router process

	// Transition to idle.
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if !hasPending {
		t.Fatal("expected a pending notification for s1 after working→idle transition")
	}
}

func TestTransition_UnreadFlip_SchedulesNotification(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.upsertSession("s1", false, true, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if !hasPending {
		t.Fatal("expected a pending notification for s1 after unread flip")
	}
}

func TestNoOSNotification_WhenFocused(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         true,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if hasPending {
		t.Fatal("should not schedule OS notification when a client is focused")
	}
}

func TestInAppNotification_WhenFocusedElsewhere(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:           true,
		SelectedSessionID: "other",
		LastInteraction:   nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	pendingCount := len(env.router.pending)
	activeCount := len(env.router.active)
	env.router.mu.Unlock()

	if pendingCount != 0 {
		t.Fatal("focused in-app path should not schedule an OS notification")
	}
	if activeCount != 1 {
		t.Fatalf("expected one active in-app notification, got %d", activeCount)
	}

	env.presence.Update("c1", presence.ClientState{
		Focused:           true,
		SelectedSessionID: "s1",
		LastInteraction:   nowSecs(),
	})

	env.router.mu.Lock()
	activeAfterSelect := len(env.router.active)
	env.router.mu.Unlock()
	if activeAfterSelect != 0 {
		t.Fatal("selecting the session should cancel active in-app notification")
	}
}

func TestNtfyPublishesWhenFocusedElsewhere(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:           true,
		SelectedSessionID: "other",
		LastInteraction:   nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)

	select {
	case got := <-received:
		if got != "[session] session finished" {
			t.Fatalf("ntfy body = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected ntfy publish while focused elsewhere")
	}
}

func TestAllowDeliveryWorkspaceLimitDoesNotCorruptSessionHistory(t *testing.T) {
	env := newTestEnv(t)
	r := env.router
	r.config.NotifyRateLimit = 3
	r.config.NotifyRateWindow = 2 * time.Minute
	r.config.WorkspaceNotifyRateLimit = 1
	r.config.WorkspaceNotifyRateWindow = time.Minute

	now := time.Unix(1_700_000_000, 0)
	expired := now.Add(-3 * time.Minute)
	recent := now.Add(-30 * time.Second)

	r.mu.Lock()
	r.deliveryHistory["s1"] = []time.Time{expired, recent}
	r.workspaceDeliveryHistory["ws"] = []time.Time{recent}
	allowed := r.allowDeliveryLocked("s1", "ws", now)
	sessionHistory := append([]time.Time(nil), r.deliveryHistory["s1"]...)
	r.mu.Unlock()

	if allowed {
		t.Fatal("expected workspace rate limit to reject delivery")
	}
	if len(sessionHistory) != 1 || !sessionHistory[0].Equal(recent) {
		t.Fatalf("session history after workspace rejection = %v, want only recent timestamp", sessionHistory)
	}

	next := now.Add(2 * time.Minute)
	r.mu.Lock()
	r.workspaceDeliveryHistory["ws"] = nil
	allowed = r.allowDeliveryLocked("s1", "ws", next)
	sessionHistory = append([]time.Time(nil), r.deliveryHistory["s1"]...)
	r.mu.Unlock()

	if !allowed {
		t.Fatal("expected later delivery to be allowed after workspace cap clears")
	}
	if len(sessionHistory) != 1 || !sessionHistory[0].Equal(next) {
		t.Fatalf("session history after later delivery = %v, want only current timestamp", sessionHistory)
	}
}

func TestNotifyRateLimitCapsActivityNtfy(t *testing.T) {
	received := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.ActivityNtfyCooldown = 0
	env.router.config.WorkspaceNotifyRateLimit = 10
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()

	env.upsertSession("s1", false, true, true)
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 4; i++ {
		env.store.Broadcast(store.Event{Type: "session-activity", ID: "s1"})
	}

	deadline := time.After(time.Second)
	count := 0
	for count < 3 {
		select {
		case <-received:
			count++
		case <-deadline:
			t.Fatalf("received %d ntfy messages, want 3", count)
		}
	}

	select {
	case got := <-received:
		t.Fatalf("unexpected ntfy beyond rate limit: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNtfyDoesNotDuplicateUnreadTransitionWithImmediateActivity(t *testing.T) {
	received := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.ActivityNtfyCooldown = time.Minute
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:           true,
		SelectedSessionID: "other",
		LastInteraction:   nowSecs(),
	})

	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, true, true)
	env.store.Broadcast(store.Event{Type: "session-activity", ID: "s1"})

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("expected ntfy for unread transition")
	}

	select {
	case got := <-received:
		t.Fatalf("unexpected duplicate ntfy after immediate activity: %q", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWorkspaceRateLimitCapsActivityNtfy(t *testing.T) {
	received := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.ActivityNtfyCooldown = 0
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()

	for _, id := range []string{"s1", "s2", "s3"} {
		env.upsertSessionInWorkspace(id, "/tmp/gmux", false, true, true)
	}
	time.Sleep(20 * time.Millisecond)
	for _, id := range []string{"s1", "s2", "s3"} {
		env.store.Broadcast(store.Event{Type: "session-activity", ID: id})
	}

	deadline := time.After(time.Second)
	count := 0
	for count < 2 {
		select {
		case <-received:
			count++
		case <-deadline:
			t.Fatalf("received %d ntfy messages, want 2", count)
		}
	}

	select {
	case got := <-received:
		t.Fatalf("unexpected ntfy beyond workspace rate limit: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWorkspaceRateLimitCapsCoalescedNtfy(t *testing.T) {
	received := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()
	events := []*pendingNotif{
		{sessionID: "s1", workspace: "gmux"},
		{sessionID: "s2", workspace: "gmux"},
		{sessionID: "s3", workspace: "gmux"},
	}

	env.router.fireCoalesced(events)
	env.router.fireCoalesced(events)
	env.router.fireCoalesced(events)

	deadline := time.After(time.Second)
	count := 0
	for count < 2 {
		select {
		case <-received:
			count++
		case <-deadline:
			t.Fatalf("received %d coalesced ntfy messages, want 2", count)
		}
	}

	select {
	case got := <-received:
		t.Fatalf("unexpected coalesced ntfy beyond workspace rate limit: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNtfyDoesNotPublishActivityWhenSessionRead(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()

	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)
	env.store.Broadcast(store.Event{Type: "session-activity", ID: "s1"})

	select {
	case got := <-received:
		t.Fatalf("unexpected ntfy body for read session: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNtfyDoesNotPublishActivityWhileWorking(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()

	env.upsertSession("s1", true, true, true)
	time.Sleep(20 * time.Millisecond)
	env.store.Broadcast(store.Event{Type: "session-activity", ID: "s1"})

	select {
	case got := <-received:
		t.Fatalf("unexpected ntfy body for working session activity: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNtfyPublishesOnActivityWhenUnreadAlreadyTrue(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	env := newTestEnv(t)
	env.router.config.NtfyProvider = func() NtfyConfig {
		return NtfyConfig{Enabled: true, ServerURL: srv.URL, TopicID: "jump-topic"}
	}
	env.router.config.HTTPClient = srv.Client()

	env.upsertSession("s1", false, true, true)
	time.Sleep(20 * time.Millisecond)
	env.store.Broadcast(store.Event{Type: "session-activity", ID: "s1"})

	select {
	case got := <-received:
		if got != "[session] new output" {
			t.Fatalf("ntfy body = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected ntfy publish for repeated unread activity")
	}
}

func TestNoNotification_WhenViewing(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:           true,
		SelectedSessionID: "s1",
		LastInteraction:   nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if hasPending {
		t.Fatal("should not schedule notification when client is viewing the session")
	}
}

func TestNewSession_NoTransition(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	// First time seeing this session — already idle. Should not fire.
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if hasPending {
		t.Fatal("should not fire for a new session that starts idle")
	}
}

func TestCancelAllPending_OnFocus(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	// Verify pending exists.
	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()
	if !hasPending {
		t.Fatal("expected pending notification before focus")
	}

	// User focuses jump → should cancel all pending.
	env.presence.Update("c1", presence.ClientState{
		Focused:         true,
		LastInteraction: nowSecs(),
	})

	env.router.mu.Lock()
	_, stillPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if stillPending {
		t.Fatal("pending notification should have been cancelled on focus")
	}
}

func TestCancelForSession_OnSelect(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	// User selects s1 → should cancel pending for s1.
	env.presence.Update("c1", presence.ClientState{
		Focused:           false,
		SelectedSessionID: "s1",
		LastInteraction:   nowSecs(),
	})

	env.router.mu.Lock()
	_, stillPending := env.router.pending["s1"]
	env.router.mu.Unlock()

	if stillPending {
		t.Fatal("pending notification should have been cancelled when session selected")
	}
}

func TestAck_RemovesActiveNotification(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(90 * time.Millisecond)

	var notifID string
	env.router.mu.Lock()
	for id := range env.router.active {
		notifID = id
		break
	}
	env.router.mu.Unlock()

	if notifID == "" {
		t.Fatal("expected active notification after grace period")
	}

	env.router.Ack(notifID)

	env.router.mu.Lock()
	_, stillActive := env.router.active[notifID]
	env.router.mu.Unlock()
	if stillActive {
		t.Fatal("ack should remove active notification")
	}
}

func TestCancelAll_OnFocusCancelsActiveNotification(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(90 * time.Millisecond)

	env.router.mu.Lock()
	activeBefore := len(env.router.active)
	env.router.mu.Unlock()
	if activeBefore == 0 {
		t.Fatal("expected active notification before focus")
	}

	env.presence.Update("c1", presence.ClientState{
		Focused:         true,
		LastInteraction: nowSecs(),
	})

	env.router.mu.Lock()
	activeAfter := len(env.router.active)
	env.router.mu.Unlock()
	if activeAfter != 0 {
		t.Fatal("focus should cancel active notifications")
	}
}

func TestGracePeriod_FiresAfterDelay(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)
	env.upsertSession("s1", false, false, true)
	time.Sleep(20 * time.Millisecond)

	// Should be pending now, not yet fired.
	env.router.mu.Lock()
	_, hasPending := env.router.pending["s1"]
	env.router.mu.Unlock()
	if !hasPending {
		t.Fatal("expected pending notification")
	}

	// Wait for grace period to expire (50ms + margin).
	time.Sleep(80 * time.Millisecond)

	// Should have been removed from pending (fired or dropped).
	env.router.mu.Lock()
	_, stillPending := env.router.pending["s1"]
	env.router.mu.Unlock()
	if stillPending {
		t.Fatal("notification should have fired after grace period")
	}
}

func TestFinishedPreferredOverUnread(t *testing.T) {
	env := newTestEnv(t)
	env.addClient("c1", "desktop")
	env.presence.Update("c1", presence.ClientState{
		Focused:         false,
		LastInteraction: nowSecs(),
	})

	// Start working.
	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)

	// Unread arrives first.
	env.upsertSession("s1", true, true, true)
	time.Sleep(20 * time.Millisecond)

	// Then finishes.
	env.upsertSession("s1", false, true, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	p, ok := env.router.pending["s1"]
	notifType := ""
	if ok {
		notifType = p.notifType
	}
	env.router.mu.Unlock()

	if !ok {
		t.Fatal("expected pending notification")
	}
	if notifType != "finished" {
		t.Fatalf("expected 'finished' to override 'unread', got %q", notifType)
	}
}

func TestSessionRemove_CleansUpPrevState(t *testing.T) {
	env := newTestEnv(t)

	env.upsertSession("s1", true, false, true)
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, exists := env.router.prevState["s1"]
	env.router.mu.Unlock()
	if !exists {
		t.Fatal("prevState should contain s1")
	}

	env.store.Remove("s1")
	time.Sleep(20 * time.Millisecond)

	env.router.mu.Lock()
	_, exists = env.router.prevState["s1"]
	env.router.mu.Unlock()
	if exists {
		t.Fatal("prevState should be cleaned up after session-remove")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m 0s"},
		{65 * time.Minute, "1h 5m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
