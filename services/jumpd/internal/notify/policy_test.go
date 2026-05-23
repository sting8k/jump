package notify

import (
	"reflect"
	"testing"

	"github.com/sting8k/jump/services/jumpd/internal/store"
)

func intentKinds(intents []notificationIntent) []notificationKind {
	kinds := make([]notificationKind, len(intents))
	for i, intent := range intents {
		kinds[i] = intent.kind
	}
	return kinds
}

func TestSessionNotificationIntentsMatrix(t *testing.T) {
	tests := []struct {
		name string
		prev sessionSnapshot
		cur  sessionSnapshot
		sess store.Session
		want []notificationKind
	}{
		{
			name: "working live session finished",
			prev: sessionSnapshot{Working: true, Alive: true},
			cur:  sessionSnapshot{Working: false, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true},
			want: []notificationKind{notificationFinished},
		},
		{
			name: "working dead session exits silently",
			prev: sessionSnapshot{Working: true, Alive: true},
			cur:  sessionSnapshot{Working: false, Alive: false},
			sess: store.Session{ID: "s1", Title: "session", Alive: false},
			want: []notificationKind{},
		},
		{
			name: "unread flips on",
			prev: sessionSnapshot{Unread: false, Alive: true},
			cur:  sessionSnapshot{Unread: true, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true, Unread: true},
			want: []notificationKind{notificationUnread},
		},
		{
			name: "finish and unread produces both in stable order",
			prev: sessionSnapshot{Working: true, Unread: false, Alive: true},
			cur:  sessionSnapshot{Working: false, Unread: true, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true, Unread: true},
			want: []notificationKind{notificationFinished, notificationUnread},
		},
		{
			name: "working starts without notify",
			prev: sessionSnapshot{Working: false, Alive: true},
			cur:  sessionSnapshot{Working: true, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true, Status: &store.Status{Working: true}},
			want: []notificationKind{},
		},
		{
			name: "unread already true does not duplicate",
			prev: sessionSnapshot{Unread: true, Alive: true},
			cur:  sessionSnapshot{Unread: true, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true, Unread: true},
			want: []notificationKind{},
		},
		{
			name: "mark read does not notify",
			prev: sessionSnapshot{Unread: true, Alive: true},
			cur:  sessionSnapshot{Unread: false, Alive: true},
			sess: store.Session{ID: "s1", Title: "session", Alive: true, Unread: false},
			want: []notificationKind{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intentKinds(sessionNotificationIntents(tt.prev, tt.cur, tt.sess))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("intents = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionNotificationIntentBodyUsesStatusLabel(t *testing.T) {
	intents := sessionNotificationIntents(
		sessionSnapshot{Unread: false, Alive: true},
		sessionSnapshot{Unread: true, Alive: true},
		store.Session{ID: "s1", Title: "session", Alive: true, Unread: true, Status: &store.Status{Label: "blocked"}},
	)
	if len(intents) != 1 || intents[0].body != "blocked" {
		t.Fatalf("intents = %+v, want unread body from status label", intents)
	}
}

func TestNotificationDeliveryModeMatrix(t *testing.T) {
	tests := []struct {
		name    string
		viewing bool
		focused bool
		want    notificationDeliveryMode
	}{
		{name: "viewing session suppresses even when focused", viewing: true, focused: true, want: deliverySuppress},
		{name: "viewing session suppresses when background app", viewing: true, focused: false, want: deliverySuppress},
		{name: "focused elsewhere uses in-app path", viewing: false, focused: true, want: deliveryFocused},
		{name: "not focused defers to ntfy/os path", viewing: false, focused: false, want: deliveryDeferred},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notificationDeliveryModeFor(tt.viewing, tt.focused); got != tt.want {
				t.Fatalf("notificationDeliveryModeFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActivityNotificationAllowedMatrix(t *testing.T) {
	tests := []struct {
		name    string
		sess    store.Session
		viewing bool
		want    bool
	}{
		{name: "unread idle background session", sess: store.Session{ID: "s1", Alive: true, Unread: true}, want: true},
		{name: "viewing session suppresses", sess: store.Session{ID: "s1", Alive: true, Unread: true}, viewing: true, want: false},
		{name: "read session suppresses", sess: store.Session{ID: "s1", Alive: true, Unread: false}, want: false},
		{name: "dead session suppresses", sess: store.Session{ID: "s1", Alive: false, Unread: true}, want: false},
		{name: "working session waits for finished transition", sess: store.Session{ID: "s1", Alive: true, Unread: true, Status: &store.Status{Working: true}}, want: false},
		{name: "error unread idle can notify", sess: store.Session{ID: "s1", Alive: true, Unread: true, Status: &store.Status{Error: true}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activityNotificationAllowed(tt.sess, tt.viewing); got != tt.want {
				t.Fatalf("activityNotificationAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
