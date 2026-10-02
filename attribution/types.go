// Package attribution 提供研讨会收入归因分摊能力。
//
// 上层将一个完整统计批次的已成交商机传入，库内完成等权归因的整数美分分摊，
// 并生成可追溯的批次报表。所有数据均保存在内存中。
package attribution

// Deal 表示一笔已成交商机。
type Deal struct {
	// ID 为唯一且稳定的商机标识。
	ID string
	// AmountCents 为成交金额，非负整数美分。
	AmountCents int64
	// SeminarIDs 为影响成交的研讨会标识，1 至 10 个且互不相同。
	SeminarIDs []string
}

// Allocation 表示一笔商机在一场研讨会上的实际分摊结果。
type Allocation struct {
	DealID      string
	SeminarID   string
	AmountCents int64
}

// SeminarSummary 表示一场研讨会在整个批次中的汇总。
type SeminarSummary struct {
	SeminarID string
	// AllocatedCents 为实际汇总（整数美分）。
	AllocatedCents int64
	// 理论汇总为 TheoreticalFloorCents + TheoreticalRemNum/TheoreticalRemDen。
	TheoreticalFloorCents int64
	TheoreticalRemNum     int64
	TheoreticalRemDen     int64
	// DealCount 为关联商机数量。
	DealCount int
}

// Report 为一个统计批次的完整分摊报表。
// 报表生成后内容固定，所有查询视图对应同一份分摊结果。
type Report struct {
	allocations  []Allocation
	summaries    []SeminarSummary
	dealIndex    map[string][]Allocation
	seminarIndex map[string]SeminarSummary
}

// Allocations 返回完整批次的分摊明细（按商机标识、研讨会标识排序）。
func (r *Report) Allocations() []Allocation {
	out := make([]Allocation, len(r.allocations))
	copy(out, r.allocations)
	return out
}

// Summaries 返回完整批次的研讨会汇总（按研讨会标识排序）。
func (r *Report) Summaries() []SeminarSummary {
	out := make([]SeminarSummary, len(r.summaries))
	copy(out, r.summaries)
	return out
}

// DealAllocations 查看单笔商机的分摊明细。
func (r *Report) DealAllocations(dealID string) ([]Allocation, bool) {
	items, ok := r.dealIndex[dealID]
	if !ok {
		return nil, false
	}
	out := make([]Allocation, len(items))
	copy(out, items)
	return out, true
}

// SeminarSummary 查看单场研讨会的汇总。
func (r *Report) SeminarSummary(seminarID string) (SeminarSummary, bool) {
	s, ok := r.seminarIndex[seminarID]
	return s, ok
}

// SeminarSummaries 查看选定研讨会的汇总；筛选只影响展示，不改变分摊结果。
// 未找到的研讨会标识会被跳过。
func (r *Report) SeminarSummaries(seminarIDs []string) []SeminarSummary {
	out := make([]SeminarSummary, 0, len(seminarIDs))
	for _, id := range seminarIDs {
		if s, ok := r.seminarIndex[id]; ok {
			out = append(out, s)
		}
	}
	return out
}
