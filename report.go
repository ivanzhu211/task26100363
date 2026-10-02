package seminarattribution

import (
	"errors"
	"fmt"
	"sort"
)

// ErrUnknownDeal 表示查询的商机不存在。
var ErrUnknownDeal = errors.New("unknown deal")

// ErrUnknownSeminar 表示查询的研讨会不存在。
var ErrUnknownSeminar = errors.New("unknown seminar")

// dealView 生成单笔商机的分摊视图（内部共享实现）。
func (r *Result) dealView(i int) DealAllocation {
	d := r.deals[i]
	items := make([]ItemAllocation, len(d.SeminarIDs))
	for j, sid := range d.SeminarIDs {
		items[j] = ItemAllocation{
			DealID:      d.ID,
			SeminarID:   sid,
			AmountCents: r.alloc[i][j],
		}
	}
	return DealAllocation{
		DealID:      d.ID,
		AmountCents: d.AmountCents,
		Items:       items,
	}
}

// seminarView 生成单场研讨会的汇总视图（内部共享实现）。
func (r *Result) seminarView(j int) SeminarSummary {
	total := int64(0)
	items := make([]ItemAllocation, 0, len(r.bySeminar[j]))
	for _, ref := range r.bySeminar[j] {
		amount := r.alloc[ref.deal][ref.slot]
		items = append(items, ItemAllocation{
			DealID:      r.deals[ref.deal].ID,
			SeminarID:   r.seminars[j],
			AmountCents: amount,
		})
		total += amount
	}
	return SeminarSummary{
		SeminarID:             r.seminars[j],
		TotalCents:            total,
		TheoreticalFloorCents: r.theoryFloor[j],
		TheoreticalCeilCents:  r.theoryCeil[j],
		Items:                 items,
	}
}

// FullReport 生成完整批次报表。
func (r *Result) FullReport() *BatchReport {
	rep := &BatchReport{
		Deals:    make([]DealAllocation, len(r.deals)),
		Seminars: make([]SeminarSummary, len(r.seminars)),
	}
	for i := range r.deals {
		rep.Deals[i] = r.dealView(i)
	}
	for j := range r.seminars {
		rep.Seminars[j] = r.seminarView(j)
	}
	return rep
}

// DealReport 查看单笔商机的分摊。
func (r *Result) DealReport(dealID string) (*DealAllocation, error) {
	i, ok := r.dealIndex[dealID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDeal, dealID)
	}
	view := r.dealView(i)
	return &view, nil
}

// SeminarReport 查看选定研讨会的汇总；筛选只影响展示，
// 不会把被隐藏研讨会的收入重新分给剩余研讨会。
// 结果按研讨会标识排序，重复的标识只返回一次。
func (r *Result) SeminarReport(seminarIDs ...string) ([]SeminarSummary, error) {
	idx := make(map[string]int, len(r.seminars))
	for j, s := range r.seminars {
		idx[s] = j
	}
	seen := make(map[string]struct{}, len(seminarIDs))
	picked := make([]int, 0, len(seminarIDs))
	for _, id := range seminarIDs {
		j, ok := idx[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownSeminar, id)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		picked = append(picked, j)
	}
	sort.Ints(picked)
	out := make([]SeminarSummary, len(picked))
	for k, j := range picked {
		out[k] = r.seminarView(j)
	}
	return out, nil
}
