package seminarattribution

import (
	"testing"
)

func reportFixture(t *testing.T) ([]Deal, *Result) {
	t.Helper()
	deals := []Deal{
		{ID: "d1", AmountCents: 1, SeminarIDs: []string{"A", "B"}},
		{ID: "d2", AmountCents: 1, SeminarIDs: []string{"B", "C"}},
		{ID: "d3", AmountCents: 1, SeminarIDs: []string{"A", "C"}},
		{ID: "d4", AmountCents: 1000003, SeminarIDs: []string{"A", "D", "E"}},
		{ID: "d5", AmountCents: 42, SeminarIDs: []string{"B", "D"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	return deals, res
}

// TestViewsShareSameAllocation 三种查看方式必须对应同一份分摊结果。
func TestViewsShareSameAllocation(t *testing.T) {
	_, res := reportFixture(t)
	full := res.FullReport()

	// FullReport 与 DealReport 一致。
	for _, da := range full.Deals {
		single, err := res.DealReport(da.DealID)
		if err != nil {
			t.Fatalf("DealReport(%s): %v", da.DealID, err)
		}
		if single.AmountCents != da.AmountCents || len(single.Items) != len(da.Items) {
			t.Fatalf("deal %s: view mismatch", da.DealID)
		}
		for i := range da.Items {
			if single.Items[i] != da.Items[i] {
				t.Errorf("deal %s item %d: %+v != %+v", da.DealID, i, single.Items[i], da.Items[i])
			}
		}
		var sum int64
		for _, it := range single.Items {
			sum += it.AmountCents
		}
		if sum != single.AmountCents {
			t.Errorf("deal %s: items sum %d != amount %d", da.DealID, sum, single.AmountCents)
		}
	}

	// FullReport 与 SeminarReport 一致，且汇总等于关联项之和。
	sr, err := res.SeminarReport("A", "B", "C", "D", "E")
	if err != nil {
		t.Fatalf("SeminarReport: %v", err)
	}
	if len(sr) != len(full.Seminars) {
		t.Fatalf("seminar count %d != %d", len(sr), len(full.Seminars))
	}
	for i := range sr {
		if sr[i].SeminarID != full.Seminars[i].SeminarID ||
			sr[i].TotalCents != full.Seminars[i].TotalCents ||
			len(sr[i].Items) != len(full.Seminars[i].Items) {
			t.Errorf("seminar view %d mismatch", i)
		}
		var sum int64
		for _, it := range sr[i].Items {
			sum += it.AmountCents
		}
		if sum != sr[i].TotalCents {
			t.Errorf("seminar %s: items sum %d != total %d", sr[i].SeminarID, sum, sr[i].TotalCents)
		}
	}
}

// TestFilteringDoesNotReallocate 筛选只影响展示：隐藏部分研讨会后，
// 剩余研讨会的关联项金额与汇总不得改变。
func TestFilteringDoesNotReallocate(t *testing.T) {
	_, res := reportFixture(t)
	full := res.FullReport()
	fullByID := make(map[string]SeminarSummary)
	for _, ss := range full.Seminars {
		fullByID[ss.SeminarID] = ss
	}

	subset, err := res.SeminarReport("B", "D")
	if err != nil {
		t.Fatalf("SeminarReport: %v", err)
	}
	if len(subset) != 2 {
		t.Fatalf("want 2 seminars, got %d", len(subset))
	}
	for _, ss := range subset {
		want := fullByID[ss.SeminarID]
		if ss.TotalCents != want.TotalCents {
			t.Errorf("seminar %s: filtered total %d != full total %d",
				ss.SeminarID, ss.TotalCents, want.TotalCents)
		}
		if len(ss.Items) != len(want.Items) {
			t.Errorf("seminar %s: filtered items %d != full items %d",
				ss.SeminarID, len(ss.Items), len(want.Items))
		}
		for i := range ss.Items {
			if ss.Items[i] != want.Items[i] {
				t.Errorf("seminar %s item %d changed after filtering", ss.SeminarID, i)
			}
		}
	}
}

// TestReportErrors 覆盖未知标识与重复查询。
func TestReportErrors(t *testing.T) {
	_, res := reportFixture(t)

	if _, err := res.DealReport("nope"); err == nil {
		t.Errorf("DealReport: expected error for unknown deal")
	}
	if _, err := res.SeminarReport("A", "nope"); err == nil {
		t.Errorf("SeminarReport: expected error for unknown seminar")
	}

	// 重复标识只返回一次。
	sr, err := res.SeminarReport("A", "A", "B")
	if err != nil {
		t.Fatalf("SeminarReport: %v", err)
	}
	if len(sr) != 2 || sr[0].SeminarID != "A" || sr[1].SeminarID != "B" {
		t.Errorf("unexpected dedup result: %+v", sr)
	}
}
