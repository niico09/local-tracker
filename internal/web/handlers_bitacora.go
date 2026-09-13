package web

import (
	"net/http"
	"sort"
	"strconv"

	"local-tracker/internal/domain"
)

// bitacoraEntry is one finished title with its completion date.
type bitacoraEntry struct {
	ID        domain.ItemID
	Title     string
	Kind      domain.Kind
	Year      *int
	CoverURL  string
	DayLabel  string
	OwnerNote string
	SortKey   string
}

// bitacoraMonth groups entries under one month headline.
type bitacoraMonth struct {
	Label   string
	Count   int
	Entries []bitacoraEntry
}

var mesesLargos = [...]string{"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio", "Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre"}
var mesesCortos = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// handleBitacora lists every completed title of the actor's visible catalog,
// grouped by the month of its completion date (end date preferred, else the
// start date). It is a read-only view over existing progress data.
func handleBitacora(cat Catalog, progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := actorOf(r)
		items, err := cat.List(r.Context(), a)
		if err != nil {
			fail(w, r, err)
			return
		}

		groups := map[string][]bitacoraEntry{}
		total := 0
		for _, it := range items {
			tilt, err := progress.Tilt(r.Context(), a, 0, it.ID)
			if err != nil {
				fail(w, r, err)
				return
			}
			if !tilt.Done {
				continue
			}
			date := tilt.EndDate
			if date == nil {
				date = tilt.StartDate
			}
			if date == nil {
				continue
			}
			iso := date.String()
			if len(iso) < 10 {
				continue
			}
			owner := "Compartido"
			if tilt.OwnerUserID != 0 {
				owner = "Personal"
			}
			entry := bitacoraEntry{
				ID:        it.ID,
				Title:     it.Title,
				Kind:      it.Kind,
				Year:      it.Year,
				OwnerNote: owner,
				SortKey:   iso,
			}
			if it.CoverPath != "" {
				entry.CoverURL = "/uploads/" + it.CoverPath
			}
			month, _ := strconv.Atoi(iso[5:7])
			if month >= 1 && month <= 12 {
				entry.DayLabel = iso[8:10] + " " + mesesCortos[month-1]
			} else {
				entry.DayLabel = iso
			}
			key := iso[:7]
			groups[key] = append(groups[key], entry)
			total++
		}

		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))

		data := pageData{Title: "Bitácora", BitacoraTotal: total}
		for _, key := range keys {
			entries := groups[key]
			sort.Slice(entries, func(i, j int) bool { return entries[i].SortKey > entries[j].SortKey })
			month, _ := strconv.Atoi(key[5:7])
			year, _ := strconv.Atoi(key[:4])
			label := key
			if month >= 1 && month <= 12 {
				label = mesesLargos[month-1] + " " + strconv.Itoa(year)
			}
			data.Bitacora = append(data.Bitacora, bitacoraMonth{Label: label, Count: len(entries), Entries: entries})
		}
		render(w, tmpls, "bitacora", http.StatusOK, data)
	}
}
