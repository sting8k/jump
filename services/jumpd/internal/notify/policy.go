package notify

import "github.com/sting8k/jump/services/jumpd/internal/store"

type notificationKind string

const (
	notificationFinished notificationKind = "finished"
	notificationUnread   notificationKind = "unread"
)

type notificationIntent struct {
	kind  notificationKind
	title string
	body  string
}

func sessionNotificationIntents(prev, cur sessionSnapshot, sess store.Session) []notificationIntent {
	intents := make([]notificationIntent, 0, 2)
	if prev.Working && !cur.Working && cur.Alive {
		intents = append(intents, notificationIntent{
			kind:  notificationFinished,
			title: sess.Title,
			body:  formatFinishedBody(sess),
		})
	}
	if !prev.Unread && cur.Unread {
		body := "New output"
		if sess.Status != nil && sess.Status.Label != "" {
			body = sess.Status.Label
		}
		intents = append(intents, notificationIntent{
			kind:  notificationUnread,
			title: sess.Title,
			body:  body,
		})
	}
	return intents
}

type notificationDeliveryMode int

const (
	deliverySuppress notificationDeliveryMode = iota
	deliveryFocused
	deliveryDeferred
)

func notificationDeliveryModeFor(viewingSession, jumpFocused bool) notificationDeliveryMode {
	if viewingSession {
		return deliverySuppress
	}
	if jumpFocused {
		return deliveryFocused
	}
	return deliveryDeferred
}

func activityNotificationAllowed(sess store.Session, viewingSession bool) bool {
	if viewingSession || !sess.Alive || !sess.Unread {
		return false
	}
	if sess.Status != nil && sess.Status.Working {
		return false
	}
	return true
}
