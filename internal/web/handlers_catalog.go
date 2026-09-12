package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"local-tracker/internal/domain"
	"local-tracker/internal/service"
)

// Catalog is the catalog use-case surface the HTTP layer depends on.
type Catalog interface {
	List(ctx context.Context, a domain.Actor) ([]domain.Item, error)
	Get(ctx context.Context, a domain.Actor, id domain.ItemID) (domain.Item, error)
	Create(ctx context.Context, a domain.Actor, in service.ItemInput) (domain.Item, error)
	Update(ctx context.Context, a domain.Actor, id domain.ItemID, in service.ItemInput) (domain.Item, error)
	Delete(ctx context.Context, a domain.Actor, id domain.ItemID) error
	SetCover(ctx context.Context, a domain.Actor, id domain.ItemID, data []byte) error
}

// CoverFiles opens stored cover files for serving. Open must reject any name
// outside the generated filename shape.
type CoverFiles interface {
	Open(name string) (*os.File, error)
}

// itemView is a catalog item plus the browser-facing cover URL.
type itemView struct {
	domain.Item
	CoverURL string
}

// itemForm keeps submitted values so a rejected form re-renders with input.
type itemForm struct {
	Title      string
	Kind       domain.Kind
	Year       string
	ExternalID string
	Personal   bool
}

func toView(it domain.Item) itemView {
	v := itemView{Item: it}
	if it.CoverPath != "" {
		v.CoverURL = "/uploads/" + it.CoverPath
	}
	return v
}

func formFrom(r *http.Request) itemForm {
	return itemForm{
		Title:      r.FormValue("title"),
		Kind:       domain.Kind(r.FormValue("kind")),
		Year:       r.FormValue("year"),
		ExternalID: r.FormValue("external_id"),
		Personal:   r.FormValue("personal") != "",
	}
}

// actorOf reads the actor installed by the Authenticate middleware. RequireAuth
// guarantees a valid actor on every catalog route.
func actorOf(r *http.Request) domain.Actor {
	a, _ := ActorFrom(r.Context())
	return a
}

// itemID parses the {id} wildcard, rejecting anything that is not a positive id.
func itemID(r *http.Request) (domain.ItemID, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return domain.ItemID(id), true
}

// parseYear accepts an optional four-digit year.
func parseYear(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	y, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: year must be a number", domain.ErrValidation)
	}
	return &y, nil
}

func handleCatalogList(cat Catalog, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := cat.List(r.Context(), actorOf(r))
		if err != nil {
			fail(w, r, err)
			return
		}
		views := make([]itemView, 0, len(items))
		for _, it := range items {
			views = append(views, toView(it))
		}
		render(w, tmpls, "catalog_list", http.StatusOK, pageData{Title: "Catalog", Items: views})
	}
}

func handleCatalogNew(tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, tmpls, "catalog_new", http.StatusOK, pageData{Title: "Add item", Kinds: domain.Kinds()})
	}
}

func handleCatalogCreate(cat Catalog, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		form := formFrom(r)
		year, err := parseYear(form.Year)
		if err != nil {
			render(w, tmpls, "catalog_new", http.StatusUnprocessableEntity,
				pageData{Title: "Add item", Error: err.Error(), Kinds: domain.Kinds(), Form: form})
			return
		}
		item, err := cat.Create(r.Context(), actorOf(r), service.ItemInput{
			Title: form.Title, Kind: form.Kind, Year: year, ExternalID: form.ExternalID, Personal: form.Personal,
		})
		if err != nil {
			if errors.Is(err, domain.ErrValidation) {
				render(w, tmpls, "catalog_new", http.StatusUnprocessableEntity,
					pageData{Title: "Add item", Error: err.Error(), Kinds: domain.Kinds(), Form: form})
				return
			}
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", item.ID), http.StatusSeeOther)
	}
}

func handleCatalogDetail(cat Catalog, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		item, err := cat.Get(r.Context(), actorOf(r), id)
		if err != nil {
			fail(w, r, err)
			return
		}
		v := toView(item)
		render(w, tmpls, "catalog_detail", http.StatusOK, pageData{Title: item.Title, Item: &v})
	}
}

func handleCatalogEdit(cat Catalog, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		item, err := cat.Get(r.Context(), actorOf(r), id)
		if err != nil {
			fail(w, r, err)
			return
		}
		form := itemForm{Title: item.Title, Kind: item.Kind, ExternalID: item.ExternalID, Personal: item.OwnerUserID != 0}
		if item.Year != nil {
			form.Year = strconv.Itoa(*item.Year)
		}
		v := toView(item)
		render(w, tmpls, "catalog_edit", http.StatusOK,
			pageData{Title: "Edit " + item.Title, Item: &v, Kinds: domain.Kinds(), Form: form})
	}
}

func handleCatalogUpdate(cat Catalog, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		form := formFrom(r)
		year, err := parseYear(form.Year)
		if err != nil {
			render(w, tmpls, "catalog_edit", http.StatusUnprocessableEntity,
				pageData{Title: "Edit item", Error: err.Error(), Kinds: domain.Kinds(), Form: form})
			return
		}
		item, err := cat.Update(r.Context(), actorOf(r), id, service.ItemInput{
			Title: form.Title, Kind: form.Kind, Year: year, ExternalID: form.ExternalID, Personal: form.Personal,
		})
		if err != nil {
			if errors.Is(err, domain.ErrValidation) {
				render(w, tmpls, "catalog_edit", http.StatusUnprocessableEntity,
					pageData{Title: "Edit item", Error: err.Error(), Kinds: domain.Kinds(), Form: form})
				return
			}
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", item.ID), http.StatusSeeOther)
	}
}

func handleCatalogDelete(cat Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if err := cat.Delete(r.Context(), actorOf(r), id); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
	}
}

// handleCatalogCover accepts a multipart upload. MaxBytesReader bounds the body
// before anything is parsed, and the service re-checks size and MIME type, so a
// rejected upload never writes a file.
func handleCatalogCover(cat Catalog, max int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if max <= 0 {
			max = 5 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, max)
		if err := r.ParseMultipartForm(max); err != nil {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
		file, _, err := r.FormFile("cover")
		if err != nil {
			http.Error(w, "a cover file is required", http.StatusUnprocessableEntity)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := cat.SetCover(r.Context(), actorOf(r), id, data); err != nil {
			if errors.Is(err, domain.ErrValidation) {
				http.Error(w, "invalid cover image", http.StatusUnprocessableEntity)
				return
			}
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", id), http.StatusSeeOther)
	}
}

// handleUploads serves stored covers. The store validates the generated name
// shape, so anything else becomes a 404 before a path is ever joined.
func handleUploads(covers CoverFiles) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := covers.Open(r.PathValue("name"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, r, r.PathValue("name"), info.ModTime(), f)
	}
}
