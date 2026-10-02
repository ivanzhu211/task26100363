package seminarattribution

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidInput 表示输入批次不合法。
var ErrInvalidInput = errors.New("invalid batch input")

// ErrInfeasible 表示分摊约束无可行解（理论上不会发生，属于内部保护）。
var ErrInfeasible = errors.New("no feasible allocation")

// lcmDenominator 是 1..10 的最小公倍数，用于把理论汇总表示为精确整数运算。
const lcmDenominator = 2520

// Allocate 对一个完整统计批次做收入归因分摊。
//
// 返回结果满足：
//   - 每条关联项取理论归因额向下或向上取整后的值；
//   - 每笔商机的分摊合计精确等于成交金额；
//   - 每场研讨会的实际汇总等于其精确理论汇总向下或向上取整后的值；
//   - 同一输入集合结果唯一，与商机及关联研讨会的输入顺序无关。
func Allocate(deals []Deal) (*Result, error) {
	norm, err := normalize(deals)
	if err != nil {
		return nil, err
	}
	return solve(norm), nil
}

// normalized 是排序、校验后的批次内部表示。
type normalized struct {
	deals      []Deal         // 按 ID 排序，每个 Deal.SeminarIDs 已排序去重
	seminars   []string       // 排序后的研讨会 ID
	seminarIdx map[string]int // 研讨会 ID -> 下标
	links      [][]int        // links[i][j]: deals[i] 第 j 个关联研讨会的下标
	quota      []int64        // quota[i] = amount/k（理论归因额向下取整）
	rem        []int64        // rem[i] = amount%k（该笔商机需额外分出的单位数）
	theoryRem  []int64        // theoryRem[j]: 研讨会 j 理论汇总的小数部分，以 1/2520 为单位
}

// normalize 校验输入并排序，使后续计算与输入顺序无关。
func normalize(deals []Deal) (*normalized, error) {
	sorted := make([]Deal, len(deals))
	copy(sorted, deals)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].ID < sorted[b].ID })

	seminarSet := make(map[string]struct{})
	nm := &normalized{
		deals: sorted,
		links: make([][]int, len(sorted)),
	}
	for i, d := range sorted {
		if d.ID == "" {
			return nil, fmt.Errorf("%w: deal at position %d has empty ID", ErrInvalidInput, i)
		}
		if i > 0 && sorted[i-1].ID == d.ID {
			return nil, fmt.Errorf("%w: duplicate deal ID %q", ErrInvalidInput, d.ID)
		}
		if d.AmountCents < 0 {
			return nil, fmt.Errorf("%w: deal %q has negative amount %d", ErrInvalidInput, d.ID, d.AmountCents)
		}
		k := len(d.SeminarIDs)
		if k < 1 || k > 10 {
			return nil, fmt.Errorf("%w: deal %q must reference 1..10 seminars, got %d", ErrInvalidInput, d.ID, k)
		}
		sids := make([]string, k)
		copy(sids, d.SeminarIDs)
		sort.Strings(sids)
		for j := 1; j < k; j++ {
			if sids[j-1] == sids[j] {
				return nil, fmt.Errorf("%w: deal %q references seminar %q more than once", ErrInvalidInput, d.ID, sids[j])
			}
		}
		for _, s := range sids {
			if s == "" {
				return nil, fmt.Errorf("%w: deal %q references an empty seminar ID", ErrInvalidInput, d.ID)
			}
			seminarSet[s] = struct{}{}
		}
		sorted[i].SeminarIDs = sids
	}

	nm.seminars = make([]string, 0, len(seminarSet))
	for s := range seminarSet {
		nm.seminars = append(nm.seminars, s)
	}
	sort.Strings(nm.seminars)
	nm.seminarIdx = make(map[string]int, len(nm.seminars))
	for j, s := range nm.seminars {
		nm.seminarIdx[s] = j
	}

	nm.quota = make([]int64, len(sorted))
	nm.rem = make([]int64, len(sorted))
	nm.theoryRem = make([]int64, len(nm.seminars))
	for i, d := range sorted {
		k := int64(len(d.SeminarIDs))
		nm.quota[i] = d.AmountCents / k
		nm.rem[i] = d.AmountCents % k
		nm.links[i] = make([]int, len(d.SeminarIDs))
		for j, s := range d.SeminarIDs {
			sj := nm.seminarIdx[s]
			nm.links[i][j] = sj
			// rem*(2520/k) 是整数：k 整除 2520。
			nm.theoryRem[sj] += nm.rem[i] * (lcmDenominator / k)
		}
	}
	return nm, nil
}

// solve 用带下界的最大流求出满足全部约束的整数分摊。
//
// 归约：商机 i 的理论归因额为 quota[i]（余数 rem[i] 需以 0/1 单位分给其关联研讨会）；
// 研讨会 j 的理论汇总为 Q_j + R_j/2520，其中 Q_j = 关联商机的 quota 之和、
// R_j = theoryRem[j]，因此实际汇总允许区间为 [Q_j+lb_j, Q_j+ub_j]，
// lb_j = R_j/2520，ub_j = lb_j（R_j 整除 2520 时）或 lb_j+1。
// 在“超出 quota 的 0/1 单位”上求可行流即得分摊。
func solve(nm *normalized) *Result {
	numDeals := len(nm.deals)
	numSeminars := len(nm.seminars)

	// 节点编号：商机 [0,n)，研讨会 [n,n+m)，SS, TT, 超级源 S'，超级汇 T'。
	ss := numDeals + numSeminars
	tt := ss + 1
	superS := tt + 1
	superT := superS + 1
	mf := newMaxFlow(superT + 1)

	demand := make([]int64, superT+1)

	// 商机 -> 研讨会：容量 1（仅当该商机有余数单位可分配），记录边下标用于回推。
	edgeIdx := make([][]int, numDeals)
	for i := range nm.deals {
		edgeIdx[i] = make([]int, len(nm.links[i]))
		if nm.rem[i] == 0 {
			continue
		}
		for j, sj := range nm.links[i] {
			edgeIdx[i][j] = mf.addEdge(i, numDeals+sj, 1)
		}
	}

	// SS -> 商机 i：下界=上界=rem[i]（该商机必须分出 rem[i] 个单位）。
	// 用需求数组表达，无需实际边。
	var need int64
	for i := range nm.deals {
		if nm.rem[i] > 0 {
			demand[i] += nm.rem[i]
			demand[ss] -= nm.rem[i]
		}
	}

	// 研讨会 j -> TT：下界 lb_j，上界 ub_j。
	theoryFloor := make([]int64, numSeminars)
	theoryCeil := make([]int64, numSeminars)
	for j := range nm.seminars {
		lb := nm.theoryRem[j] / lcmDenominator
		ub := lb
		if nm.theoryRem[j]%lcmDenominator != 0 {
			ub = lb + 1
		}
		// theoryFloor/ceil 先存超出 Q_j 的部分，最后统一加 Q_j。
		theoryFloor[j] = lb
		theoryCeil[j] = ub
		if ub > lb {
			mf.addEdge(numDeals+j, tt, ub-lb)
		}
		if lb > 0 {
			demand[tt] += lb
			demand[numDeals+j] -= lb
		}
	}

	// TT -> SS：无限容量，构成循环流。
	mf.addEdge(tt, ss, 1<<62)

	// 超级源/汇：满足所有节点需求。
	for v := 0; v <= tt; v++ {
		if demand[v] > 0 {
			mf.addEdge(superS, v, demand[v])
			need += demand[v]
		} else if demand[v] < 0 {
			mf.addEdge(v, superT, -demand[v])
		}
	}

	if flow := mf.maxFlow(superS, superT); flow != need {
		// 理论分数流恒可行，整数可行解必存在（完全单模），到达此处说明内部错误。
		panic(fmt.Sprintf("%v: flow %d, need %d", ErrInfeasible, flow, need))
	}

	// 回推分摊结果。
	alloc := make([][]int64, numDeals)
	for i := range nm.deals {
		alloc[i] = make([]int64, len(nm.links[i]))
		for j := range nm.links[i] {
			alloc[i][j] = nm.quota[i]
			if nm.rem[i] > 0 {
				// 该边初始容量为 1，已用流量 = 1 - 剩余容量。
				alloc[i][j] += 1 - mf.residualCap(i, edgeIdx[i][j])
			}
		}
	}

	// 汇总理论边界：加回各研讨会的 quota 之和 Q_j。
	bySeminar := make([][]seminarItem, numSeminars)
	for i := range nm.deals {
		for j, sj := range nm.links[i] {
			theoryFloor[sj] += nm.quota[i]
			theoryCeil[sj] += nm.quota[i]
			bySeminar[sj] = append(bySeminar[sj], seminarItem{deal: i, slot: j})
		}
	}

	dealIndex := make(map[string]int, numDeals)
	for i, d := range nm.deals {
		dealIndex[d.ID] = i
	}
	return &Result{
		deals:       nm.deals,
		dealIndex:   dealIndex,
		seminars:    nm.seminars,
		alloc:       alloc,
		bySeminar:   bySeminar,
		theoryFloor: theoryFloor,
		theoryCeil:  theoryCeil,
	}
}
