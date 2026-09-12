package service

import (
	"context"
	"fmt"
	"strings"

	"local-tracker/internal/domain"
)

// ItemInput is the validated user input for creating or updating an item.
type ItemInput struct {
	Title       string
	Kind        domain.Kind
	Year        *int
	ExternalID  string
	OwnerUserID domain.UserID
}

// CatalogService implements the catalog use cases on top of the item port.
type CatalogService struct {
	items  domain.ItemRepository
	covers *CoverStore
}

// NewCatalog wires the catalog use cases. covers may be nil for callers that
// never handle uploads.
func NewCatalog(items domain.ItemRepository, covers *CoverStore) *CatalogService {
	return &CatalogService{items: items, covers: covers}
}

// List returns the catalog as visible to the actor: shared items plus their own.
func (s *CatalogService) List(ctx context.Context, a domain.Actor) ([]domain.Item, error) {
	return s.items.List(ctx, a)
}

// Get returns one item or ErrForbidden when it is hidden from the actor.
func (s *CatalogService) Get(ctx context.Context, a domain.Actor, id domain.ItemID) (domain.Item, error) {
	return s.items.Get(ctx, a, id)
}

// Create validates input and inserts one item.
func (s *CatalogService) Create(ctx context.Context, a domain.Actor, in ItemInput) (domain.Item, error) {
	item, err := validateItem(a, in)
	if err != nil {
		return domain.Item{}, err
	}
	return s.items.Create(ctx, a, item)
}

// Update validates input and applies it to an existing item. Ownership and the
// cover are preserved: editing never reassigns an item or drops its cover.
func (s *CatalogService) Update(ctx context.Context, a domain.Actor, id domain.ItemID, in ItemInput) (domain.Item, error) {
	current, err := s.items.Get(ctx, a, id)
	if err != nil {
		return domain.Item{}, err
	}
	item, err := validateItem(a, in)
	if err != nil {
		return domain.Item{}, err
	}
	item.ID = current.ID
	item.CoverPath = current.CoverPath
	item.OwnerUserID = current.OwnerUserID
	if err := s.items.Update(ctx, a, item); err != nil {
		return domain.Item{}, err
	}
	return item, nil
}

// Delete removes an item and its cover file, if any.
func (s *CatalogService) Delete(ctx context.Context, a domain.Actor, id domain.ItemID) error {
	current, err := s.items.Get(ctx, a, id)
	if err != nil {
		return err
	}
	if err := s.items.Delete(ctx, a, id); err != nil {
		return err
	}
	if current.CoverPath != "" && s.covers != nil {
		_ = s.covers.Remove(current.CoverPath)
	}
	return nil
}

// SetCover validates and stores an uploaded cover, replacing any previous file.
func (s *CatalogService) SetCover(ctx context.Context, a domain.Actor, id domain.ItemID, data []byte) error {
	current, err := s.items.Get(ctx, a, id)
	if err != nil {
		return err
	}
	if s.covers == nil {
		return fmt.Errorf("%w: cover storage is unavailable", domain.ErrValidation)
	}
	name, err := s.covers.Save(data)
	if err != nil {
		return err
	}
	if err := s.items.SetCover(ctx, a, id, name); err != nil {
		_ = s.covers.Remove(name) // never leave an orphan file behind
		return err
	}
	if current.CoverPath != "" && current.CoverPath != name {
		_ = s.covers.Remove(current.CoverPath)
	}
	return nil
}

// validateItem enforces the catalog invariants and trims free text.
func validateItem(a domain.Actor, in ItemInput) (domain.Item, error) {
	if err := a.Require(); err != nil {
		return domain.Item{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return domain.Item{}, fmt.Errorf("%w: title is required", domain.ErrValidation)
	}
	if !in.Kind.Valid() {
		return domain.Item{}, fmt.Errorf("%w: unknown item type %q", domain.ErrValidation, in.Kind)
	}
	if in.Year != nil && (*in.Year < 1000 || *in.Year > 9999) {
		return domain.Item{}, fmt.Errorf("%w: year must be between 1000 and 9999", domain.ErrValidation)
	}
	return domain.Item{
		Title:       title,
		Kind:        in.Kind,
		Year:        in.Year,
		ExternalID:  strings.TrimSpace(in.ExternalID),
		OwnerUserID: in.OwnerUserID,
	}, nil
}
