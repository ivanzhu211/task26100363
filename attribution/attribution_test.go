package attribution

import (
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

// verifyReport 用独立的精确计算（big.Rat）核对报表的全部金额约束。
func verifyReport(t *testing.T, deals []Deal, r *Report) {
	t.Helper()

	dealByID := make(map[string]Deal, len(deals))
	for _, d := range deals {
		dealByID[d.ID] = d
	}

	// 每个关联项的取值范围与逐笔守恒。
	allocByDeal := make(map[string][]Allocation)
	allocBySeminar := make(map[string]int64)
	linked := make(map[string]map[string]bool)
	for _, a := range r.Allocations() {
		d, ok := dealByID[a.DealID]
		if !ok {
			t.Fatalf("分摊引用了不存在的商机 %q", a.DealID)
		}
		k := int64(len(d.SeminarIDs))
		floor, rem := d.AmountCents/k, d.AmountCents%k
		ceil := floor
		if rem > 0 {
			ceil = floor + 1
		}
		if a.AmountCents != floor && a.AmountCents != ceil {
			t.Errorf("商机 %q 在 %q 分摊 %d，超出 [%d, %d]", a.DealID, a.SeminarID, a.AmountCents, floor, ceil)
		}
		found := false
		for _, id := range d.SeminarIDs {
			if id == a.SeminarID {
				found = true
			}
		}
		if !found {
			t.Errorf("商机 %q 向未关联的研讨会 %q 分摊", a.DealID, a.SeminarID)
		}
		if linked[a.DealID] == nil {
			linked[a.DealID] = make(map[string]bool)
		}
		if linked[a.DealID][a.SeminarID] {
			t.Errorf("商机 %q 对研讨会 %q 重复分摊", a.DealID, a.SeminarID)
		}
		linked[a.DealID][a.SeminarID] = true
		allocByDeal[a.DealID] = append(allocByDeal[a.DealID], a)
		allocBySeminar[a.SeminarID] += a.AmountCents
	}
	for _, d := range deals {
		items := allocByDeal[d.ID]
		if len(items) != len(d.SeminarIDs) {
			t.Errorf("商机 %q 分摊项数 %d，关联数 %d", d.ID, len(items), len(d.SeminarIDs))
		}
		var sum int64
		for _, a := range items {
			sum += a.AmountCents
		}
		if sum != d.AmountCents {
			t.Errorf("商机 %q 分摊合计 %d，成交金额 %d", d.ID, sum, d.AmountCents)
		}
		// 单笔查询视图与完整报表一致。
		view, ok := r.DealAllocations(d.ID)
		if !ok {
			t.Errorf("DealAllocations(%q) 未找到", d.ID)
		} else if !reflect.DeepEqual(view, itemsSorted(items)) && !reflect.DeepEqual(view, items) {
			t.Errorf("DealAllocations(%q) 与完整报表不一致", d.ID)
		}
	}

	// 每场研讨会的理论汇总（big.Rat 精确计算）与实际汇总边界。
	theory := make(map[string]*big.Rat)
	for _, d := range deals {
		k := big.NewInt(int64(len(d.SeminarIDs)))
		share := new(big.Rat).SetFrac(big.NewInt(d.AmountCents), k)
		for _, id := range d.SeminarIDs {
			if theory[id] == nil {
				theory[id] = new(big.Rat)
			}
			theory[id].Add(theory[id], share)
		}
	}
	for id, tr := range theory {
		got := allocBySeminar[id]
		floor := new(big.Int).Quo(tr.Num(), tr.Denom()) // 非负，Quo 即向下取整
		isInt := tr.IsInt()
		lo, hi := floor, new(big.Int).Add(floor, big.NewInt(1))
		if isInt {
			hi = floor
		}
		g := big.NewInt(got)
		if g.Cmp(lo) < 0 || g.Cmp(hi) > 0 {
			t.Errorf("研讨会 %q 实际汇总 %d，理论 %s，允许 [%s, %s]", id, got, tr.RatString(), lo, hi)
		}
		// 汇总视图与分摊明细一致。
		summary, ok := r.SeminarSummary(id)
		if !ok {
			t.Errorf("SeminarSummary(%q) 未找到", id)
			continue
		}
		if summary.AllocatedCents != got {
			t.Errorf("研讨会 %q 汇总视图 %d，明细合计 %d", id, summary.AllocatedCents, got)
		}
		// 报表中的理论值与 big.Rat 独立计算一致。
		reconstructed := new(big.Rat).SetInt64(summary.TheoreticalFloorCents)
		reconstructed.Add(reconstructed, new(big.Rat).SetFrac(
			big.NewInt(summary.TheoreticalRemNum), big.NewInt(summary.TheoreticalRemDen)))
		if reconstructed.Cmp(tr) != 0 {
			t.Errorf("研讨会 %q 理论汇总报表值 %s，独立计算 %s", id, reconstructed.RatString(), tr.RatString())
		}
		if isInt && summary.AllocatedCents != floor.Int64() {
			t.Errorf("研讨会 %q 理论汇总为整数 %s，实际必须为整数，实际 %d", id, floor, summary.AllocatedCents)
		}
	}
}

func itemsSorted(items []Allocation) []Allocation {
	out := make([]Allocation, len(items))
	copy(out, items)
	return out
}

// TestTriangle 验收案例：三笔 1 美分商机，AB / BC / AC。
func TestTriangle(t *testing.T) {
	deals := []Deal{
		{ID: "d1", AmountCents: 1, SeminarIDs: []string{"A", "B"}},
		{ID: "d2", AmountCents: 1, SeminarIDs: []string{"B", "C"}},
		{ID: "d3", AmountCents: 1, SeminarIDs: []string{"A", "C"}},
	}
	r, err := GenerateReport(deals)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	for _, d := range deals {
		items, _ := r.DealAllocations(d.ID)
		var sum int64
		for _, a := range items {
			sum += a.AmountCents
		}
		if sum != 1 {
			t.Errorf("商机 %q 合计 %d，期望 1", d.ID, sum)
		}
	}
	for _, id := range []string{"A", "B", "C"} {
		s, ok := r.SeminarSummary(id)
		if !ok {
			t.Fatalf("缺少研讨会 %q 汇总", id)
		}
		if s.AllocatedCents != 1 {
			t.Errorf("研讨会 %q 汇总 %d，期望 1", id, s.AllocatedCents)
		}
	}
	verifyReport(t, deals, r)
}

// TestMixedAndDisconnected 覆盖不同关联数量混合、理论汇总恰为整数、
// 互不相连的多组关联及大金额场景。
func TestMixedAndDisconnected(t *testing.T) {
	deals := []Deal{
		// 组 1：混合关联数，含大金额。
		{ID: "g1-a", AmountCents: 1000000000000, SeminarIDs: []string{"S1", "S2", "S3"}},
		{ID: "g1-b", AmountCents: 7, SeminarIDs: []string{"S1"}},
		{ID: "g1-c", AmountCents: 10, SeminarIDs: []string{"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8", "S9", "S10"}},
		{ID: "g1-d", AmountCents: 999999999999, SeminarIDs: []string{"S2", "S4"}},
		// 组 2：与组 1 不相连；S11 理论汇总恰为整数（1/2+3/6+... 设计如下）。
		{ID: "g2-a", AmountCents: 1, SeminarIDs: []string{"S11", "S12"}},
		{ID: "g2-b", AmountCents: 1, SeminarIDs: []string{"S11", "S13"}},
		{ID: "g2-c", AmountCents: 3, SeminarIDs: []string{"S12", "S13"}},
		// 组 3：单笔零金额。
		{ID: "g3-a", AmountCents: 0, SeminarIDs: []string{"S20", "S21"}},
	}
	r, err := GenerateReport(deals)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	verifyReport(t, deals, r)

	// 组 2 中 S11 理论汇总 = 1/2 + 1/2 = 1，恰为整数，实际必须等于 1。
	s, ok := r.SeminarSummary("S11")
	if !ok {
		t.Fatal("缺少 S11 汇总")
	}
	if s.TheoreticalRemNum != 0 {
		t.Fatalf("S11 理论汇总应为整数，余数 %d/%d", s.TheoreticalRemNum, s.TheoreticalRemDen)
	}
	if s.AllocatedCents != 1 {
		t.Errorf("S11 理论汇总恰为整数 1，实际 %d", s.AllocatedCents)
	}
}

// TestShuffleInvariance 打乱商机顺序与关联研讨会顺序，分摊结果不变。
func TestShuffleInvariance(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	base := randomBatch(rng, 2000, 200, 8)
	r1, err := GenerateReport(base)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	for trial := 0; trial < 5; trial++ {
		shuffled := shuffleBatch(rng, base)
		r2, err := GenerateReport(shuffled)
		if err != nil {
			t.Fatalf("GenerateReport: %v", err)
		}
		if !reflect.DeepEqual(r1.Allocations(), r2.Allocations()) {
			t.Fatalf("第 %d 次打乱后分摊结果改变", trial)
		}
		if !reflect.DeepEqual(r1.Summaries(), r2.Summaries()) {
			t.Fatalf("第 %d 次打乱后汇总结果改变", trial)
		}
	}
	verifyReport(t, base, r1)
}

// TestFilterViewConsistency 筛选报表只影响展示，原有分摊不变。
func TestFilterViewConsistency(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	deals := randomBatch(rng, 500, 50, 5)
	r, err := GenerateReport(deals)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	full := r.Summaries()
	// 随机选一半研讨会做筛选视图。
	var picked []string
	for i, s := range full {
		if i%2 == 0 {
			picked = append(picked, s.SeminarID)
		}
	}
	filtered := r.SeminarSummaries(picked)
	if len(filtered) != len(picked) {
		t.Fatalf("筛选视图数量 %d，期望 %d", len(filtered), len(picked))
	}
	byID := make(map[string]SeminarSummary, len(full))
	for _, s := range full {
		byID[s.SeminarID] = s
	}
	for _, s := range filtered {
		if s != byID[s.SeminarID] {
			t.Errorf("筛选视图中 %q 的汇总与完整报表不一致", s.SeminarID)
		}
	}
	// 筛选后被隐藏研讨会的收入不得重新分配：完整报表保持不变。
	if !reflect.DeepEqual(r.Summaries(), full) {
		t.Error("筛选操作后完整报表发生变化")
	}
	verifyReport(t, deals, r)
}

// TestValidation 输入校验。
func TestValidation(t *testing.T) {
	cases := []struct {
		name  string
		deals []Deal
	}{
		{"空商机标识", []Deal{{ID: "", AmountCents: 1, SeminarIDs: []string{"A"}}}},
		{"商机标识重复", []Deal{
			{ID: "x", AmountCents: 1, SeminarIDs: []string{"A"}},
			{ID: "x", AmountCents: 2, SeminarIDs: []string{"B"}},
		}},
		{"负金额", []Deal{{ID: "x", AmountCents: -1, SeminarIDs: []string{"A"}}}},
		{"无关联研讨会", []Deal{{ID: "x", AmountCents: 1, SeminarIDs: nil}}},
		{"超过10个研讨会", []Deal{{ID: "x", AmountCents: 1, SeminarIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}}}},
		{"研讨会标识重复", []Deal{{ID: "x", AmountCents: 1, SeminarIDs: []string{"A", "A"}}}},
		{"空研讨会标识", []Deal{{ID: "x", AmountCents: 1, SeminarIDs: []string{""}}}},
	}
	for _, c := range cases {
		if _, err := GenerateReport(c.deals); err == nil {
			t.Errorf("%s：期望返回错误", c.name)
		}
	}
}

// TestEmptyBatch 空批次合法。
func TestEmptyBatch(t *testing.T) {
	r, err := GenerateReport(nil)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if len(r.Allocations()) != 0 || len(r.Summaries()) != 0 {
		t.Error("空批次应产生空报表")
	}
}

// randomBatch 生成确定性随机批次。
func randomBatch(rng *rand.Rand, dealCount, seminarCount, maxLinks int) []Deal {
	deals := make([]Deal, dealCount)
	for i := range deals {
		k := 1 + rng.Intn(maxLinks)
		perm := rng.Perm(seminarCount)[:k]
		ids := make([]string, k)
		for j, s := range perm {
			ids[j] = fmt.Sprintf("sem-%05d", s)
		}
		deals[i] = Deal{
			ID:          fmt.Sprintf("deal-%06d", i),
			AmountCents: rng.Int63n(1000000000000),
			SeminarIDs:  ids,
		}
	}
	return deals
}

// shuffleBatch 打乱商机顺序及每笔商机的研讨会顺序，返回副本。
func shuffleBatch(rng *rand.Rand, deals []Deal) []Deal {
	out := make([]Deal, len(deals))
	for i, d := range deals {
		ids := make([]string, len(d.SeminarIDs))
		copy(ids, d.SeminarIDs)
		rng.Shuffle(len(ids), func(a, b int) { ids[a], ids[b] = ids[b], ids[a] })
		out[i] = Deal{ID: d.ID, AmountCents: d.AmountCents, SeminarIDs: ids}
	}
	rng.Shuffle(len(out), func(a, b int) { out[a], out[b] = out[b], out[a] })
	return out
}
