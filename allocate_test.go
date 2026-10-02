package seminarattribution

import (
	"math/big"
	"math/rand"
	"testing"
)

// verifyAllocation 用独立的精确计算（big.Rat）核对分摊结果的全部金额约束：
// 每条关联项取值范围、每笔商机守恒、每场研讨会汇总边界。
func verifyAllocation(t *testing.T, deals []Deal, res *Result) {
	t.Helper()

	// 逐笔核对：每条关联项 ∈ {floor(amount/k), ceil(amount/k)}，合计 == amount。
	for i, d := range res.deals {
		k := int64(len(d.SeminarIDs))
		lo := d.AmountCents / k
		hi := lo
		if d.AmountCents%k != 0 {
			hi = lo + 1
		}
		var sum int64
		for j := range d.SeminarIDs {
			got := res.alloc[i][j]
			if got != lo && got != hi {
				t.Errorf("deal %s seminar %s: alloc %d not in {%d,%d}",
					d.ID, d.SeminarIDs[j], got, lo, hi)
			}
			sum += got
		}
		if sum != d.AmountCents {
			t.Errorf("deal %s: alloc sum %d != amount %d", d.ID, sum, d.AmountCents)
		}
	}

	// 逐场核对：用 big.Rat 独立累加精确理论汇总，实际汇总必须等于其下/上取整。
	theory := make(map[string]*big.Rat)
	actual := make(map[string]int64)
	for i, d := range res.deals {
		k := int64(len(d.SeminarIDs))
		share := new(big.Rat).SetFrac(big.NewInt(d.AmountCents), big.NewInt(k))
		for j, sid := range d.SeminarIDs {
			if _, ok := theory[sid]; !ok {
				theory[sid] = new(big.Rat)
			}
			theory[sid].Add(theory[sid], share)
			actual[sid] += res.alloc[i][j]
		}
	}
	if len(theory) != len(res.seminars) {
		t.Fatalf("seminar count mismatch: theory %d, result %d", len(theory), len(res.seminars))
	}
	for j, sid := range res.seminars {
		tr := theory[sid]
		floor := new(big.Int).Quo(tr.Num(), tr.Denom()) // 非负，Quo 即向下取整
		ceil := new(big.Int).Set(floor)
		if !tr.IsInt() {
			ceil.Add(ceil, big.NewInt(1))
		}
		got := actual[sid]
		if got != floor.Int64() && got != ceil.Int64() {
			t.Errorf("seminar %s: total %d not in {%s,%s} (theory=%s)",
				sid, got, floor, ceil, tr.RatString())
		}
		if tr.IsInt() && got != floor.Int64() {
			t.Errorf("seminar %s: theory is integer %s but total is %d", sid, floor, got)
		}
		if res.theoryFloor[j] != floor.Int64() || res.theoryCeil[j] != ceil.Int64() {
			t.Errorf("seminar %s: reported bounds [%d,%d], exact [%s,%s]",
				sid, res.theoryFloor[j], res.theoryCeil[j], floor, ceil)
		}
	}
}

// TestAcceptanceThreeOneCentDeals 是验收用例：三笔 1 美分商机分别关联
// A与B、B与C、A与C，每笔合计 1 美分，每场汇总 1 美分。
func TestAcceptanceThreeOneCentDeals(t *testing.T) {
	deals := []Deal{
		{ID: "d1", AmountCents: 1, SeminarIDs: []string{"A", "B"}},
		{ID: "d2", AmountCents: 1, SeminarIDs: []string{"B", "C"}},
		{ID: "d3", AmountCents: 1, SeminarIDs: []string{"A", "C"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	verifyAllocation(t, deals, res)

	rep := res.FullReport()
	for _, da := range rep.Deals {
		var sum int64
		for _, it := range da.Items {
			sum += it.AmountCents
		}
		if sum != 1 {
			t.Errorf("deal %s: sum %d, want 1", da.DealID, sum)
		}
	}
	if len(rep.Seminars) != 3 {
		t.Fatalf("want 3 seminars, got %d", len(rep.Seminars))
	}
	for _, ss := range rep.Seminars {
		if ss.TotalCents != 1 {
			t.Errorf("seminar %s: total %d, want 1", ss.SeminarID, ss.TotalCents)
		}
	}
}

// TestMixedAssociationCounts 覆盖不同关联数量混合（1..10）。
func TestMixedAssociationCounts(t *testing.T) {
	deals := []Deal{
		{ID: "d01", AmountCents: 7, SeminarIDs: []string{"S1"}},
		{ID: "d02", AmountCents: 100, SeminarIDs: []string{"S1", "S2"}},
		{ID: "d03", AmountCents: 999, SeminarIDs: []string{"S1", "S2", "S3"}},
		{ID: "d04", AmountCents: 5, SeminarIDs: []string{"S2", "S3", "S4", "S5"}},
		{ID: "d05", AmountCents: 12345, SeminarIDs: []string{"S1", "S3", "S5", "S6", "S7"}},
		{ID: "d06", AmountCents: 13, SeminarIDs: []string{"S1", "S2", "S3", "S4", "S5", "S6"}},
		{ID: "d07", AmountCents: 7777777, SeminarIDs: []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7"}},
		{ID: "d08", AmountCents: 11, SeminarIDs: []string{"S2", "S4", "S6", "S8", "S9", "S10", "S11", "S12"}},
		{ID: "d09", AmountCents: 3, SeminarIDs: []string{"S1", "S4", "S7", "S10", "S12", "S13", "S14", "S15", "S16"}},
		{ID: "d10", AmountCents: 987654321, SeminarIDs: []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10"}},
		{ID: "d11", AmountCents: 0, SeminarIDs: []string{"S3", "S13"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	verifyAllocation(t, deals, res)
}

// TestExactIntegerTheoryTotals 构造理论汇总恰为整数的场景，实际汇总必须等于该整数。
func TestExactIntegerTheoryTotals(t *testing.T) {
	deals := []Deal{
		// S1 理论汇总 = 1/2 + 1/2 = 1，恰为整数。
		{ID: "a", AmountCents: 1, SeminarIDs: []string{"S1", "S2"}},
		{ID: "b", AmountCents: 1, SeminarIDs: []string{"S1", "S3"}},
		// S4 理论汇总 = 10/5 + 6/3 = 4，恰为整数。
		{ID: "c", AmountCents: 10, SeminarIDs: []string{"S4", "S5", "S6", "S7", "S8"}},
		{ID: "d", AmountCents: 6, SeminarIDs: []string{"S4", "S9", "S10"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	verifyAllocation(t, deals, res)

	sr, err := res.SeminarReport("S1", "S4")
	if err != nil {
		t.Fatalf("SeminarReport: %v", err)
	}
	for _, ss := range sr {
		if ss.TheoreticalFloorCents != ss.TheoreticalCeilCents {
			t.Errorf("seminar %s: expected integer theory, got [%d,%d]",
				ss.SeminarID, ss.TheoreticalFloorCents, ss.TheoreticalCeilCents)
		}
		if ss.TotalCents != ss.TheoreticalFloorCents {
			t.Errorf("seminar %s: total %d != integer theory %d",
				ss.SeminarID, ss.TotalCents, ss.TheoreticalFloorCents)
		}
	}
}

// TestDisconnectedComponents 覆盖互不相连的多组关联。
func TestDisconnectedComponents(t *testing.T) {
	deals := []Deal{
		{ID: "g1d1", AmountCents: 3, SeminarIDs: []string{"G1A", "G1B"}},
		{ID: "g1d2", AmountCents: 4, SeminarIDs: []string{"G1B", "G1C"}},
		{ID: "g2d1", AmountCents: 100, SeminarIDs: []string{"G2A"}},
		{ID: "g2d2", AmountCents: 7, SeminarIDs: []string{"G2A", "G2B", "G2C"}},
		{ID: "g3d1", AmountCents: 1, SeminarIDs: []string{"G3A", "G3B", "G3C", "G3D"}},
		{ID: "g3d2", AmountCents: 2, SeminarIDs: []string{"G3A", "G3D"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	verifyAllocation(t, deals, res)
}

// TestLargeAmounts 覆盖大金额场景（单笔最高 1e12 美分）。
func TestLargeAmounts(t *testing.T) {
	deals := []Deal{
		{ID: "big1", AmountCents: 1000000000000, SeminarIDs: []string{"L1", "L2", "L3"}},
		{ID: "big2", AmountCents: 999999999999, SeminarIDs: []string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}},
		{ID: "big3", AmountCents: 1000000000000, SeminarIDs: []string{"L2", "L4", "L8", "L9", "L10"}},
		{ID: "small", AmountCents: 1, SeminarIDs: []string{"L1", "L10"}},
	}
	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	verifyAllocation(t, deals, res)
}

// TestOrderIndependence 打乱商机顺序与关联研讨会顺序，分摊结果必须不变。
func TestOrderIndependence(t *testing.T) {
	base := []Deal{
		{ID: "d1", AmountCents: 1, SeminarIDs: []string{"A", "B"}},
		{ID: "d2", AmountCents: 1, SeminarIDs: []string{"B", "C"}},
		{ID: "d3", AmountCents: 1, SeminarIDs: []string{"A", "C"}},
		{ID: "d4", AmountCents: 1234567, SeminarIDs: []string{"A", "D", "E"}},
		{ID: "d5", AmountCents: 88, SeminarIDs: []string{"C", "D", "F", "G"}},
		{ID: "d6", AmountCents: 99999999999, SeminarIDs: []string{"E", "F"}},
		{ID: "d7", AmountCents: 5, SeminarIDs: []string{"G"}},
	}
	ref, err := Allocate(base)
	if err != nil {
		t.Fatalf("Allocate base: %v", err)
	}
	refAlloc := allocMap(ref)

	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 20; trial++ {
		shuffled := make([]Deal, len(base))
		perm := rng.Perm(len(base))
		for i, p := range perm {
			d := base[p]
			sids := make([]string, len(d.SeminarIDs))
			copy(sids, d.SeminarIDs)
			rng.Shuffle(len(sids), func(a, b int) { sids[a], sids[b] = sids[b], sids[a] })
			d.SeminarIDs = sids
			shuffled[i] = d
		}
		got, err := Allocate(shuffled)
		if err != nil {
			t.Fatalf("Allocate shuffled: %v", err)
		}
		gotAlloc := allocMap(got)
		for key, want := range refAlloc {
			if gotAlloc[key] != want {
				t.Fatalf("trial %d: item %v = %d, want %d", trial, key, gotAlloc[key], want)
			}
		}
	}
}

// allocMap 把分摊结果展开为 (dealID, seminarID) -> 金额 的映射，便于比较。
type allocKey struct {
	deal, seminar string
}

func allocMap(res *Result) map[allocKey]int64 {
	m := make(map[allocKey]int64)
	for i, d := range res.deals {
		for j, sid := range d.SeminarIDs {
			m[allocKey{d.ID, sid}] = res.alloc[i][j]
		}
	}
	return m
}

// TestInvalidInput 覆盖非法输入。
func TestInvalidInput(t *testing.T) {
	cases := map[string][]Deal{
		"empty deal ID":     {{ID: "", AmountCents: 1, SeminarIDs: []string{"A"}}},
		"duplicate deal ID": {{ID: "x", AmountCents: 1, SeminarIDs: []string{"A"}}, {ID: "x", AmountCents: 2, SeminarIDs: []string{"B"}}},
		"negative amount":   {{ID: "x", AmountCents: -1, SeminarIDs: []string{"A"}}},
		"no seminars":       {{ID: "x", AmountCents: 1, SeminarIDs: nil}},
		"too many seminars": {{ID: "x", AmountCents: 1, SeminarIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}}},
		"duplicate seminar": {{ID: "x", AmountCents: 1, SeminarIDs: []string{"A", "A"}}},
		"empty seminar ID":  {{ID: "x", AmountCents: 1, SeminarIDs: []string{""}}},
	}
	for name, deals := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Allocate(deals); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

// TestEmptyBatch 空批次应返回空报表而非报错。
func TestEmptyBatch(t *testing.T) {
	res, err := Allocate(nil)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	rep := res.FullReport()
	if len(rep.Deals) != 0 || len(rep.Seminars) != 0 {
		t.Fatalf("expected empty report, got %+v", rep)
	}
}
