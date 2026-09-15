package user

import "context"

// Conditional writes make overlapping requests monotonic without a process
// lock. The caller throttles browser reads; completed generations always count.
func (s *Store) MarkActive(ctx context.Context, userID string, at int64) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET last_active_at = ? WHERE id = ? AND last_active_at < ?`, at, userID, at)
	return err
}
