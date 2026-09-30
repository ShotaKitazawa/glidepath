package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ShotaKitazawa/glidepath/internal/calc"
	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

// educationStages are walked in order to auto-generate expense_forecasts
// rows (SPEC.md section 5) whenever a family member is added.
var educationStages = []string{"幼稚園", "小学校", "中学校", "高校", "大学"}

// educationTrackPatterns maps a UI-facing "when do we switch to private"
// pattern to the 公立/私立 track for each pre-university stage. 大学 is not
// in this map — it's a separate selection (university_track), since its
// track names (国公立/私立文系/私立理系) don't fit this 公立/私立 axis.
var educationTrackPatterns = map[string]map[string]string{
	"all_public":       {"幼稚園": "公立", "小学校": "公立", "中学校": "公立", "高校": "公立"},
	"private_from_jhs": {"幼稚園": "公立", "小学校": "公立", "中学校": "私立", "高校": "私立"},
	"private_from_hs":  {"幼稚園": "公立", "小学校": "公立", "中学校": "公立", "高校": "私立"},
	"all_private":      {"幼稚園": "私立", "小学校": "私立", "中学校": "私立", "高校": "私立"},
}

// registerFamily wires the family/education-forecast mutations. Family data
// is displayed on GET /assumptions (registerAssumptions), not its own page
// — it's a rarely-changed input, not something the user checks day to day.
func registerFamily(mux *http.ServeMux, q *sqlcgen.Queries) {
	mux.HandleFunc("POST /family", familyCreate(q))
	mux.HandleFunc("POST /family/forecasts/{id}", forecastOverride(q))
	mux.HandleFunc("POST /family/members/{id}/delete", familyDelete(q))
}

type familyMemberView struct {
	ID        int64
	Relation  string
	Birth     string
	Forecasts []forecastView
}

type forecastView struct {
	ID         int64
	Stage      string
	Track      string
	StartAge   int64
	EndAge     int64
	AnnualCost int64
	IsOverride bool
}

// loadFamilyView reads all family members and their expense forecasts for
// display on GET /assumptions.
func loadFamilyView(ctx context.Context, q *sqlcgen.Queries) ([]familyMemberView, error) {
	members, err := q.ListFamilyMembers(ctx)
	if err != nil {
		return nil, err
	}

	views := make([]familyMemberView, 0, len(members))
	for _, m := range members {
		forecasts, err := q.ListExpenseForecastsByFamilyMember(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		fv := make([]forecastView, 0, len(forecasts))
		for _, f := range forecasts {
			fv = append(fv, forecastView{
				ID:         f.ID,
				Stage:      f.Stage,
				Track:      f.Track,
				StartAge:   f.StartAge,
				EndAge:     f.EndAge,
				AnnualCost: toThousandYen(int(f.AnnualCost)),
				IsOverride: f.IsOverride,
			})
		}
		views = append(views, familyMemberView{
			ID:        m.ID,
			Relation:  m.Relation,
			Birth:     m.BirthMonth.Format("2006-01"),
			Forecasts: fv,
		})
	}
	return views, nil
}

func familyCreate(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		relation := r.FormValue("relation")
		if relation == "" {
			renderAssumptionsError(w, r.Context(), q, "続柄を入力してください")
			return
		}
		birth, err := time.Parse("2006-01", r.FormValue("birth_month"))
		if err != nil {
			renderAssumptionsError(w, r.Context(), q, "生年月の形式が不正です")
			return
		}
		stageTracks, ok := educationTrackPatterns[r.FormValue("track_pattern")]
		if !ok {
			stageTracks = educationTrackPatterns["all_public"]
		}
		universityTrack := r.FormValue("university_track")
		if universityTrack == "" {
			universityTrack = "国公立"
		}

		member, err := q.CreateFamilyMember(r.Context(), sqlcgen.CreateFamilyMemberParams{
			Relation:   relation,
			BirthMonth: birth,
		})
		if err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("保存に失敗しました: %v", err))
			return
		}

		for _, stage := range educationStages {
			stageTrack := stageTracks[stage]
			if stage == "大学" {
				stageTrack = universityTrack
			}
			annualCost, ok := calc.DefaultAnnualEducationCost(stage, stageTrack)
			if !ok {
				continue
			}
			startAge, endAge, ok := calc.EducationAgeRange(stage)
			if !ok {
				continue
			}
			if _, err := q.CreateExpenseForecast(r.Context(), sqlcgen.CreateExpenseForecastParams{
				FamilyMemberID: member.ID,
				Stage:          stage,
				Track:          stageTrack,
				AnnualCost:     int64(annualCost),
				StartAge:       int64(startAge),
				EndAge:         int64(endAge),
			}); err != nil {
				renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("教育費予測の作成に失敗しました: %v", err))
				return
			}
		}

		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

func forecastOverride(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		annualCost, err := strconv.Atoi(r.FormValue("annual_cost"))
		if err != nil {
			renderAssumptionsError(w, r.Context(), q, "金額の形式が不正です")
			return
		}
		if _, err := q.UpdateExpenseForecastOverride(r.Context(), sqlcgen.UpdateExpenseForecastOverrideParams{
			ID:         int64(id),
			AnnualCost: fromThousandYen(annualCost),
		}); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("更新に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}

func familyDelete(q *sqlcgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := q.DeleteFamilyMember(r.Context(), int64(id)); err != nil {
			renderAssumptionsError(w, r.Context(), q, fmt.Sprintf("削除に失敗しました: %v", err))
			return
		}
		http.Redirect(w, r, "/assumptions", http.StatusSeeOther)
	}
}
