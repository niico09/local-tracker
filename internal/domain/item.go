package domain

import "time"

// ItemID identifies a catalog item.
type ItemID int64

// Kind is the catalog item type.
type Kind string

const (
	// KindSeries is a TV series.
	KindSeries Kind = "series"
	// KindMovie is a film.
	KindMovie Kind = "movie"
	// KindBook is a book.
	KindBook Kind = "book"
	// KindCourse is a course.
	KindCourse Kind = "course"
)

// Kinds lists the allowed item types in display order.
func Kinds() []Kind { return []Kind{KindSeries, KindMovie, KindBook, KindCourse} }

// Valid reports whether k is one of the four allowed kinds (I10).
func (k Kind) Valid() bool {
	switch k {
	case KindSeries, KindMovie, KindBook, KindCourse:
		return true
	}
	return false
}

// Item is one catalog entry. OwnerUserID 0 means the item is shared by the
// couple; a non-zero owner makes it personal to that profile (G1).
type Item struct {
	ID          ItemID
	Title       string
	Kind        Kind
	Year        *int
	ExternalID  string
	CoverPath   string // filename only, relative to data/uploads/
	OwnerUserID UserID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Subject exposes the item's ownership facts to the single access rule. Items
// never carry a read-only grant, so a partner simply cannot see a personal item.
func (i Item) Subject() Subject { return Subject{OwnerID: i.OwnerUserID} }
