package store

// ApplyAttentionStatus applies the persistent status part of the attention
// lifecycle. An empty status means idle/clear; working and error states stay
// explicit because they drive dots and notification transitions.
func (s *Session) ApplyAttentionStatus(status *Status) {
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
	s.ApplyAttentionStatus(status)
	if unread != nil && allowUnread {
		s.ApplyAttentionUnread(*unread)
	}
}

// MarkAttentionRead consumes user-facing attention for a session view. It
// clears unread and error badges but never clears working state.
func (s *Session) MarkAttentionRead() bool {
	changed := s.Unread || (s.Status != nil && s.Status.Error)
	s.ApplyAttentionUnread(false)
	return changed
}

// ClearAttentionStatus removes runtime status when a session is known dead or
// idle. It does not mark unread/read.
func (s *Session) ClearAttentionStatus() {
	s.Status = nil
}
