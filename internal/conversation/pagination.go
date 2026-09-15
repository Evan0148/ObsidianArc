package conversation

import "context"

func (s *Store) ListPage(ctx context.Context, userID string, limit, offset int) ([]Conversation, int, error) {
	if limit <= 0 || limit > MaxListLimit {
		limit = DefaultListLimit
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.list(ctx, userID, limit, offset)
	return rows, total, err
}
