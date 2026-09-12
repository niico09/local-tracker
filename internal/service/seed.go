package service

import (
	"context"

	"local-tracker/internal/domain"
)

// SeedService applies the embedded Top 100 list. The loader is injected so the
// service depends only on domain and never on the data-file package.
type SeedService struct {
	repo domain.SeedRepository
	load func() ([]domain.SeedEntry, error)
}

// NewSeed wires the seed use case.
func NewSeed(repo domain.SeedRepository, load func() ([]domain.SeedEntry, error)) *SeedService {
	return &SeedService{repo: repo, load: load}
}

// Run loads the list and applies it with the trusted system actor, exactly as
// the first-run setup does before anyone has logged in.
func (s *SeedService) Run(ctx context.Context) (domain.SeedResult, error) {
	entries, err := s.load()
	if err != nil {
		return domain.SeedResult{}, err
	}
	return s.repo.Seed(ctx, domain.SystemActor(), entries)
}
