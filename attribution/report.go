package attribution

import (
	"fmt"
	"sort"
)

// lcmDen 为 1..10 的最小公倍数。关联数量不超过 10，
// 因此任何理论归因额都可以表示为 lcmDen 分之一的整数倍，
// 全部中间计算使用整数完成，不引入浮点误差。
const lcmDen = 2520

// GenerateReport 对一个完整统计批次执行分摊并生成报表。
// 同一输入集合（与顺序无关）必然得到相同结果。
func GenerateReport(deals []Deal) (*Report, error) {
	return buildReport(deals)
}

func buildReport(input []Deal) (*Report, error) {
	deals, err := normalizeDeals(input)
	if err != nil {
		return nil, err
	}

	seminarIDs := collectSeminarIDs(deals)
	seminarIdx := make(map[string]int, len(seminarIDs))
	for i, id := range seminarIDs {
		seminarIdx[id] = i
	}

	n := len(deals)
	m := len(seminarIDs)

	// 逐笔理论值：floor = amount/k，余数 rem = amount%k 即该笔需分配的余数个数。
	remainder := make([]int64, n)
	// 每场研讨会的理论汇总：base + remNum/lcmDen。
	base := make([]int64, m)
	remNum := make([]int64, m)
	dealCount := make([]int, m)
	// 每个商机按研讨会序号升序的关联列表（不改动调用方入参）。
	sortedSems := make([][]int, n)
	for i, d := range deals {
		k := int64(len(d.SeminarIDs))
		q := d.AmountCents / k
		remainder[i] = d.AmountCents % k
		sems := make([]int, 0, len(d.SeminarIDs))
		for _, id := range d.SeminarIDs {
			s := seminarIdx[id]
			sems = append(sems, s)
			base[s] += q
			remNum[s] += remainder[i] * (lcmDen / k)
			dealCount[s]++
		}
		sort.Ints(sems)
		sortedSems[i] = sems
	}

	// 每场研讨会实际汇总允许区间 [lower, upper]（相对 base 的余数个数）。
	lower := make([]int64, m)
	upper := make([]int64, m)
	for s := 0; s < m; s++ {
		lower[s] = remNum[s] / lcmDen
		if remNum[s]%lcmDen == 0 {
			upper[s] = lower[s]
		} else {
			upper[s] = lower[s] + 1
		}
	}

	bonus, err := solveBonuses(n, m, remainder, sortedSems, lower, upper)
	if err != nil {
		return nil, err
	}

	report := &Report{
		allocations:  make([]Allocation, 0, n*4),
		summaries:    make([]SeminarSummary, 0, m),
		dealIndex:    make(map[string][]Allocation, n),
		seminarIndex: make(map[string]SeminarSummary, m),
	}
	allocated := make([]int64, m)
	for i, d := range deals {
		k := int64(len(d.SeminarIDs))
		q := d.AmountCents / k
		for j, s := range sortedSems[i] {
			a := Allocation{
				DealID:      d.ID,
				SeminarID:   seminarIDs[s],
				AmountCents: q + bonus[i][j],
			}
			report.allocations = append(report.allocations, a)
			report.dealIndex[d.ID] = append(report.dealIndex[d.ID], a)
			allocated[s] += a.AmountCents
		}
	}
	for s := 0; s < m; s++ {
		rem := remNum[s] % lcmDen
		summary := SeminarSummary{
			SeminarID:             seminarIDs[s],
			AllocatedCents:        allocated[s],
			TheoreticalFloorCents: base[s] + remNum[s]/lcmDen,
			TheoreticalRemNum:     rem,
			TheoreticalRemDen:     lcmDen,
			DealCount:             dealCount[s],
		}
		report.summaries = append(report.summaries, summary)
		report.seminarIndex[summary.SeminarID] = summary
	}
	return report, nil
}

// normalizeDeals 校验输入并按商机标识排序，返回排序后的副本。
func normalizeDeals(input []Deal) ([]Deal, error) {
	deals := make([]Deal, len(input))
	copy(deals, input)
	seen := make(map[string]struct{}, len(deals))
	for i, d := range deals {
		if d.ID == "" {
			return nil, fmt.Errorf("attribution: 第 %d 笔商机标识为空", i)
		}
		if _, dup := seen[d.ID]; dup {
			return nil, fmt.Errorf("attribution: 商机标识 %q 重复", d.ID)
		}
		seen[d.ID] = struct{}{}
		if d.AmountCents < 0 {
			return nil, fmt.Errorf("attribution: 商机 %q 金额为负", d.ID)
		}
		if len(d.SeminarIDs) == 0 || len(d.SeminarIDs) > 10 {
			return nil, fmt.Errorf("attribution: 商机 %q 关联研讨会数量 %d 不在 1..10 范围", d.ID, len(d.SeminarIDs))
		}
		semSeen := make(map[string]struct{}, len(d.SeminarIDs))
		for _, id := range d.SeminarIDs {
			if id == "" {
				return nil, fmt.Errorf("attribution: 商机 %q 存在空研讨会标识", d.ID)
			}
			if _, dup := semSeen[id]; dup {
				return nil, fmt.Errorf("attribution: 商机 %q 的研讨会标识 %q 重复", d.ID, id)
			}
			semSeen[id] = struct{}{}
		}
	}
	sort.Slice(deals, func(a, b int) bool { return deals[a].ID < deals[b].ID })
	return deals, nil
}

// collectSeminarIDs 汇总全部研讨会标识并排序。
func collectSeminarIDs(deals []Deal) []string {
	set := make(map[string]struct{})
	for _, d := range deals {
		for _, id := range d.SeminarIDs {
			set[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// solveBonuses 求解每个关联项是否取上取整（余数 1 分给谁）。
//
// 建模为带下界的可行流：
//   - 源到商机 i：下界=上界=remainder[i]（该笔必须分出的余数个数）
//   - 商机 i 到研讨会 s：容量 1（每个关联项至多向上取整一次）
//   - 研讨会 s 到汇：下界 lower[s]、上界 upper[s]（汇总边界）
//
// 约束矩阵全单位模且分数解（每项取理论小数部分）存在，
// 因此整数可行解必然存在；排序后的固定建图顺序保证结果确定。
func solveBonuses(n, m int, remainder []int64, sortedSems [][]int, lower, upper []int64) ([][]int64, error) {
	const (
		nodeS = 0
		nodeT = 1
	)
	dealNode := func(i int) int { return 2 + i }
	semNode := func(s int) int { return 2 + n + s }
	nodeSS := 2 + n + m
	nodeTT := nodeSS + 1
	nodeCount := nodeTT + 1

	linkCount := 0
	for _, sems := range sortedSems {
		linkCount += len(sems)
	}
	d := newDinic(nodeCount, n+linkCount+m+n+m+1)

	balance := make([]int64, nodeCount)
	// 源到商机：下界=上界=remainder，容量 0，仅体现到节点平衡。
	for i := 0; i < n; i++ {
		if remainder[i] == 0 {
			continue
		}
		balance[nodeS] -= remainder[i]
		balance[dealNode(i)] += remainder[i]
	}
	// 商机到研讨会：容量 1，记录边编号以读取结果。
	edgeID := make([][]int32, n)
	for i := 0; i < n; i++ {
		edgeID[i] = make([]int32, len(sortedSems[i]))
		for j, s := range sortedSems[i] {
			edgeID[i][j] = d.addEdge(dealNode(i), semNode(s), 1)
		}
	}
	// 研讨会到汇：下界 lower、上界 upper。
	for s := 0; s < m; s++ {
		if lower[s] > 0 {
			balance[semNode(s)] -= lower[s]
			balance[nodeT] += lower[s]
		}
		if upper[s] > lower[s] {
			d.addEdge(semNode(s), nodeT, upper[s]-lower[s])
		}
	}
	// 汇到源的无穷边，闭合循环流。
	var totalRem int64
	for i := 0; i < n; i++ {
		totalRem += remainder[i]
	}
	d.addEdge(nodeT, nodeS, totalRem)

	// 下界变换：超级源/超级汇。
	var need int64
	for v := 0; v < nodeCount; v++ {
		switch {
		case balance[v] > 0:
			d.addEdge(nodeSS, v, balance[v])
			need += balance[v]
		case balance[v] < 0:
			d.addEdge(v, nodeTT, -balance[v])
		}
	}
	if got := d.maxFlow(nodeSS, nodeTT); got != need {
		return nil, fmt.Errorf("attribution: 内部错误，余数分配不可行 (%d/%d)", got, need)
	}

	bonus := make([][]int64, n)
	for i := 0; i < n; i++ {
		bonus[i] = make([]int64, len(sortedSems[i]))
		for j, id := range edgeID[i] {
			bonus[i][j] = d.flow(id, 1)
		}
	}
	return bonus, nil
}
