package web

import (
	"net/http"
	"sort"
	"strconv"

	"local-tracker/internal/domain"
)

// dashStats is the overview strip of the panel.
type dashStats struct {
	Total    int
	Done     int
	Progress int
	Pending  int
}

// dashEntry is one completed title with its short day label.
type dashEntry struct {
	itemView
	DayLabel string
	sortKey  string
}

// dashGoal is one goal with its precomputed rollup for the panel.
type dashGoal struct {
	ID      domain.GoalID
	Title   string
	Couple  bool
	Done    int
	Total   int
	Percent int
}

// handleDashboard renders the overview: the featured couple goal, the catalog
// counters, what is in progress, the active goals with their rollups, the
// latest completions and the latest additions. Every number comes from the
// same services the other pages use, so the panel can never disagree with
// them. The individual sections are skipped when a collaborator is not wired.
func handleDashboard(cat Catalog, goals Goals, members Membership, progress Progress, tmpls templates, canBackup bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := ActorFrom(r.Context())
		data := pageData{Title: "Panel", UserID: actor.ID(), CanBackup: canBackup}

		if cat != nil && progress != nil {
			items, err := cat.List(r.Context(), actor)
			if err != nil {
				fail(w, r, err)
				return
			}
			stats := dashStats{Total: len(items)}
			var inProgress, added []itemView
			var completed []dashEntry
			for _, item := range items {
				v := toView(item)
				added = append(added, v)
				tilt, err := progress.Tilt(r.Context(), actor, 0, item.ID)
				if err != nil {
					fail(w, r, err)
					return
				}
				switch domain.DeriveState(tilt) {
				case domain.StateCompleted:
					stats.Done++
					if day := tiltDay(tilt); day != "" {
						completed = append(completed, dashEntry{itemView: v, DayLabel: shortDay(day), sortKey: day})
					}
				case domain.StateInProgress:
					stats.Progress++
					inProgress = append(inProgress, v)
				default:
					stats.Pending++
				}
			}
			sort.Slice(added, func(i, j int) bool { return added[i].CreatedAt.After(added[j].CreatedAt) })
			sort.Slice(completed, func(i, j int) bool { return completed[i].sortKey > completed[j].sortKey })
			if len(inProgress) > 6 {
				inProgress = inProgress[:6]
			}
			if len(added) > 4 {
				added = added[:4]
			}
			if len(completed) > 3 {
				completed = completed[:3]
			}
			data.Stats = &stats
			data.InProgress = inProgress
			data.RecentAdded = added
			data.RecentDone = completed
		}

		if goals != nil && members != nil {
			list, err := goals.List(r.Context(), actor)
			if err != nil {
				fail(w, r, err)
				return
			}
			for _, goal := range list {
				memberList, err := members.ListMembers(r.Context(), actor, goal.ID)
				if err != nil {
					fail(w, r, err)
					return
				}
				memberDone := 0
				for _, m := range memberList {
					if m.Tilt != nil && m.Tilt.Done {
						memberDone++
					}
				}
				doneCount, total := domain.Rollup(goal.Target, memberDone, len(memberList))
				entry := dashGoal{ID: goal.ID, Title: goal.Title, Couple: goal.OwnerUserID == 0, Done: doneCount, Total: total}
				if total > 0 {
					entry.Percent = doneCount * 100 / total
				}
				if goal.ExternalKey == "top100" {
					top := entry
					data.TopGoal = &top
					continue
				}
				data.DashGoals = append(data.DashGoals, entry)
			}
		}
		render(w, tmpls, "dashboard", http.StatusOK, data)
	}
}

// tiltDay picks the display date of a completed tilt: the end date when
// present, the start date otherwise.
func tiltDay(p domain.Progress) string {
	if p.EndDate != nil {
		return p.EndDate.String()
	}
	if p.StartDate != nil {
		return p.StartDate.String()
	}
	return ""
}

// shortDay renders "2026-08-14" as "14 ago" using the shared month table.
func shortDay(iso string) string {
	if len(iso) < 10 {
		return iso
	}
	month, _ := strconv.Atoi(iso[5:7])
	if month < 1 || month > 12 {
		return iso
	}
	return iso[8:10] + " " + mesesCortos[month-1]
}
