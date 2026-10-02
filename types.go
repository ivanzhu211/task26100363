// Package seminarattribution 提供研讨会收入归因分摊能力。
//
// 上层将一个完整统计批次的已成交商机传入 Allocate，得到确定性的
// 整数美分分摊结果；报表（完整批次 / 单笔商机 / 选定研讨会）均从
// 同一份分摊结果派生，全部数据保存在内存中。
package seminarattribution

// Deal 是一笔已成交商机。
type Deal struct {
	// ID 是商机的唯一且稳定的标识。
	ID string
	// AmountCents 是成交金额，非负整数美分。
	AmountCents int64
	// SeminarIDs 是影响成交的研讨会标识，1 至 10 个且互不相同。
	SeminarIDs []string
}

// ItemAllocation 是一条关联项的分摊结果：某笔商机分给某场研讨会的金额。
type ItemAllocation struct {
	DealID      string
	SeminarID   string
	AmountCents int64
}

// DealAllocation 是单笔商机的分摊视图。
type DealAllocation struct {
	DealID      string
	AmountCents int64
	Items       []ItemAllocation // 按研讨会标识排序
}

// SeminarSummary 是单场研讨会的汇总视图。
type SeminarSummary struct {
	SeminarID string
	// TotalCents 是本批次实际分摊合计。
	TotalCents int64
	// TheoreticalFloorCents / TheoreticalCeilCents 是精确理论汇总的下/上取整。
	TheoreticalFloorCents int64
	TheoreticalCeilCents  int64
	// Items 是贡献该研讨会的关联项，按商机标识排序。
	Items []ItemAllocation
}

// BatchReport 是完整批次报表。
type BatchReport struct {
	Deals    []DealAllocation // 按商机标识排序
	Seminars []SeminarSummary // 按研讨会标识排序
}

// Result 是一次分摊的结果，所有报表视图共享同一份数据。
type Result struct {
	deals       []Deal          // 按 ID 排序，SeminarIDs 已排序
	dealIndex   map[string]int  // 商机 ID -> deals 下标
	seminars    []string        // 排序后的研讨会 ID
	alloc       [][]int64       // alloc[i][j]: deals[i] 分给其第 j 个研讨会的金额
	bySeminar   [][]seminarItem // bySeminar[j]: 研讨会 j 的全部关联项（按商机 ID 序）
	theoryFloor []int64         // 每场研讨会理论汇总下取整
	theoryCeil  []int64         // 每场研讨会理论汇总上取整
}

// seminarItem 定位一条关联项：第 deal 笔商机的第 slot 个关联。
type seminarItem struct {
	deal int
	slot int
}
