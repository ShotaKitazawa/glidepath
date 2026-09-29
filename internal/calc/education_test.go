package calc

import "testing"

func TestDefaultAnnualEducationCost(t *testing.T) {
	tests := []struct {
		name    string
		stage   string
		track   string
		wantYen int
		wantOK  bool
	}{
		{"kindergarten public", "幼稚園", "公立", 185_000, true},
		{"kindergarten private", "幼稚園", "私立", 347_000, true},
		{"elementary public", "小学校", "公立", 336_000, true},
		{"elementary private", "小学校", "私立", 1_828_000, true},
		{"junior high public", "中学校", "公立", 542_000, true},
		{"junior high private", "中学校", "私立", 1_560_000, true},
		{"high school public", "高校", "公立", 590_000, true},
		{"high school private", "高校", "私立", 1_020_000, true},
		{"university national", "大学", "国公立", 4_830_000 / 4, true},
		{"university private humanities", "大学", "私立文系", 6_900_000 / 4, true},
		{"university private science", "大学", "私立理系", 8_220_000 / 4, true},
		{"unknown stage", "予備校", "公立", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DefaultAnnualEducationCost(tt.stage, tt.track)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantYen {
				t.Errorf("got %d, want %d", got, tt.wantYen)
			}
		})
	}
}

func TestEducationAgeRange(t *testing.T) {
	tests := []struct {
		stage        string
		wantStartAge int
		wantEndAge   int
		wantOK       bool
	}{
		{"幼稚園", 3, 5, true},
		{"小学校", 6, 11, true},
		{"中学校", 12, 14, true},
		{"高校", 15, 17, true},
		{"大学", 18, 21, true},
		{"不明", 0, 0, false},
	}

	for _, tt := range tests {
		start, end, ok := EducationAgeRange(tt.stage)
		if ok != tt.wantOK || start != tt.wantStartAge || end != tt.wantEndAge {
			t.Errorf("EducationAgeRange(%q) = (%d, %d, %v), want (%d, %d, %v)",
				tt.stage, start, end, ok, tt.wantStartAge, tt.wantEndAge, tt.wantOK)
		}
	}
}
