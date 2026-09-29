package calc

import "testing"

func TestPercentile(t *testing.T) {
	values := []int{10, 1, 5, 9, 3, 7, 2, 8, 4, 6} // 1..10 shuffled

	tests := []struct {
		p    float64
		want int
	}{
		{0, 1},
		{0.5, 6}, // nearest-rank on 10 sorted values
		{1, 10},
	}
	for _, tt := range tests {
		got := Percentile(values, tt.p)
		if got != tt.want {
			t.Errorf("Percentile(values, %v) = %d, want %d", tt.p, got, tt.want)
		}
	}
}

func TestPercentile_Empty(t *testing.T) {
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil, 0.5) = %d, want 0", got)
	}
}

func TestPercentile_DoesNotMutateInput(t *testing.T) {
	values := []int{3, 1, 2}
	_ = Percentile(values, 0.5)
	if values[0] != 3 || values[1] != 1 || values[2] != 2 {
		t.Errorf("Percentile mutated its input: %v", values)
	}
}
