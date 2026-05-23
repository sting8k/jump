package notify

import (
	"testing"

	"github.com/sting8k/jump/services/jumpd/internal/store"
)

func TestSessionNotificationIntentsBackgroundFinishUnread(t *testing.T) {
	sess := store.Session{ID: "s1", Title: "session", Alive: true, Unread: true}
	intents := sessionNotificationIntents(
		sessionSnapshot{Working: true, Unread: false, Alive: true},
		sessionSnapshot{Working: false, Unread: true, Alive: true},
		sess,
	)

	if len(intents) != 2 {
		t.Fatalf("len(intents) = %d, want 2", len(intents))
	}
	if intents[0].kind != notificationFinished || intents[1].kind != notificationUnread {
		t.Fatalf("intents = %+v, want finished then unread", intents)
	}
}

func TestNotificationDeliveryModeFor(t *testing.T) {
	tests := []struct {
		name    string
		viewing bool
		focused bool
		want    notificationDeliveryMode
	}{
		{name: "viewing session", viewing: true, focused: true, want: deliverySuppress},
		{name: "focused elsewhere", viewing: false, focused: true, want: deliveryFocused},
		{name: "not focused", viewing: false, focused: false, want: deliveryDeferred},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notificationDeliveryModeFor(tt.viewing, tt.focused); got != tt.want {
				t.Fatalf("notificationDeliveryModeFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActivityNotificationAllowed(t *testing.T) {
	unreadIdle := store.Session{ID: "s1", Alive: true, Unread: true}
	if !activityNotificationAllowed(unreadIdle, false) {
		t.Fatal("expected unread idle background session activity to notify")
	}
	if activityNotificationAllowed(unreadIdle, true) {
		t.Fatal("viewing the session must suppress activity notification")
	}
	if activityNotificationAllowed(store.Session{ID: "s1", Alive: true, Unread: false}, false) {
		t.Fatal("read session activity must not notify")
	}
	if activityNotificationAllowed(store.Session{ID: "s1", Alive: true, Unread: true, Status: &store.Status{Working: true}}, false) {
		t.Fatal("working session activity must wait for the finished transition")
	}
}
