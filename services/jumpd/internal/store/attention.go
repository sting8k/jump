package store

import (
	"log"
	"reflect"
)

// ApplyAttentionStatus applies the persistent status part of the attention
// lifecycle. An empty status means idle/clear; working and error states stay
// explicit because they drive dots and notification transitions.
func (s *Session) ApplyAttentionStatus(status *Status) {
	s.applyAttentionStatus(status)
}

func (s *Session) ApplyAttentionStatusFrom(source string, status *Status) {
	s.traceAttention(source, func() { s.applyAttentionStatus(status) })
}

func (s *Session) applyAttentionStatus(status *Status) {
	if status == nil {
		return
	}
	if status.Label == "" && !status.Working && !status.Error {
		s.Status = nil
		return
	}
	copy := *status
	s.Status = &copy
}

// ApplyAttentionUnread applies the unread/read part of the attention
// lifecycle. Clearing unread also consumes an error badge without touching an
// active working status.
func (s *Session) ApplyAttentionUnread(unread bool) {
	s.applyAttentionUnread(unread)
}

func (s *Session) ApplyAttentionUnreadFrom(source string, unread bool) {
	s.traceAttention(source, func() { s.applyAttentionUnread(unread) })
}

func (s *Session) applyAttentionUnread(unread bool) {
	s.Unread = unread
	if !unread && s.Status != nil && s.Status.Error {
		if s.Status.Label == "" && !s.Status.Working {
			s.Status = nil
			return
		}
		s.Status.Error = false
	}
}

// ApplyAttentionUpdate applies a status/unread pair from an adapter or runner
// event. When allowUnread is false, unread changes are intentionally ignored
// (for example, full historical reads that should not resurrect old output).
func (s *Session) ApplyAttentionUpdate(status *Status, unread *bool, allowUnread bool) {
	s.applyAttentionUpdate(status, unread, allowUnread)
}

func (s *Session) ApplyAttentionUpdateFrom(source string, status *Status, unread *bool, allowUnread bool) {
	s.traceAttention(source, func() { s.applyAttentionUpdate(status, unread, allowUnread) })
}

func (s *Session) applyAttentionUpdate(status *Status, unread *bool, allowUnread bool) {
	s.applyAttentionStatus(status)
	if unread != nil && allowUnread {
		s.applyAttentionUnread(*unread)
	}
}

// MarkAttentionRead consumes user-facing attention for a session view. It
// clears unread and error badges but never clears working state.
func (s *Session) MarkAttentionRead() bool {
	changed := s.attentionChangedAfter(func() { s.applyAttentionUnread(false) })
	return changed
}

func (s *Session) MarkAttentionReadFrom(source string) bool {
	changed := false
	s.traceAttention(source, func() {
		changed = s.attentionChangedAfter(func() { s.applyAttentionUnread(false) })
	})
	return changed
}

// ClearAttentionStatus removes runtime status when a session is known dead or
// idle. It does not mark unread/read.
func (s *Session) ClearAttentionStatus() {
	s.Status = nil
}

func (s *Session) ClearAttentionStatusFrom(source string) {
	s.traceAttention(source, func() { s.Status = nil })
}

func (s *Session) traceAttention(source string, fn func()) {
	prevStatus := cloneStatus(s.Status)
	prevUnread := s.Unread
	fn()
	if source == "" || attentionEqual(prevStatus, prevUnread, s.Status, s.Unread) {
		return
	}
	log.Printf("attention: %s source=%s status=%s->%s unread=%t->%t", s.ID, source, statusLogValue(prevStatus), statusLogValue(s.Status), prevUnread, s.Unread)
}

func (s *Session) attentionChangedAfter(fn func()) bool {
	prevStatus := cloneStatus(s.Status)
	prevUnread := s.Unread
	fn()
	return !attentionEqual(prevStatus, prevUnread, s.Status, s.Unread)
}

func attentionEqual(aStatus *Status, aUnread bool, bStatus *Status, bUnread bool) bool {
	return aUnread == bUnread && reflect.DeepEqual(aStatus, bStatus)
}

func cloneStatus(status *Status) *Status {
	if status == nil {
		return nil
	}
	copy := *status
	return &copy
}

func statusLogValue(status *Status) any {
	if status == nil {
		return "idle"
	}
	return *status
}
