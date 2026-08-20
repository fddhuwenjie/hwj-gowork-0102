package store

import "context"

func (s *Store) Migrate(ctx context.Context) error {
	return Migrate(ctx, s.DB)
}
