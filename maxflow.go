package seminarattribution

// maxFlow 是内部使用的确定性最大流实现（Dinic 算法，int64 容量）。
//
// 只要按相同顺序建图，算法过程与结果完全确定，
// 用于求解带下界的整数分摊可行性问题。
type maxFlow struct {
	n     int
	graph [][]mfEdge // 邻接表；反向边通过 to/rev 索引引用
	level []int
	iter  []int
}

type mfEdge struct {
	to  int
	rev int // 在 graph[to] 中反向边的下标
	cap int64
}

func newMaxFlow(n int) *maxFlow {
	return &maxFlow{
		n:     n,
		graph: make([][]mfEdge, n),
		level: make([]int, n),
		iter:  make([]int, n),
	}
}

// addEdge 添加一条 from -> to、容量为 cap 的边，返回该边在 graph[from] 中的下标。
func (f *maxFlow) addEdge(from, to int, cap int64) int {
	fwd := mfEdge{to: to, rev: len(f.graph[to]), cap: cap}
	rev := mfEdge{to: from, rev: len(f.graph[from]), cap: 0}
	f.graph[from] = append(f.graph[from], fwd)
	f.graph[to] = append(f.graph[to], rev)
	return len(f.graph[from]) - 1
}

// residualCap 读取建图后某条正向边的剩余容量（用于回推分摊结果）。
func (f *maxFlow) residualCap(from, idx int) int64 {
	return f.graph[from][idx].cap
}

// maxFlow 计算 s -> t 的最大流并返回流量。
func (f *maxFlow) maxFlow(s, t int) int64 {
	var flow int64
	for f.bfs(s, t) {
		for i := range f.iter {
			f.iter[i] = 0
		}
		for {
			pushed := f.dfs(s, t, 1<<62)
			if pushed == 0 {
				break
			}
			flow += pushed
		}
	}
	return flow
}

func (f *maxFlow) bfs(s, t int) bool {
	for i := range f.level {
		f.level[i] = -1
	}
	f.level[s] = 0
	queue := make([]int, 0, f.n)
	queue = append(queue, s)
	for head := 0; head < len(queue); head++ {
		v := queue[head]
		for _, e := range f.graph[v] {
			if e.cap > 0 && f.level[e.to] < 0 {
				f.level[e.to] = f.level[v] + 1
				queue = append(queue, e.to)
			}
		}
	}
	return f.level[t] >= 0
}

func (f *maxFlow) dfs(v, t int, up int64) int64 {
	if v == t {
		return up
	}
	for ; f.iter[v] < len(f.graph[v]); f.iter[v]++ {
		idx := f.iter[v]
		e := &f.graph[v][idx]
		if e.cap > 0 && f.level[v] < f.level[e.to] {
			d := f.dfs(e.to, t, min64(up, e.cap))
			if d > 0 {
				e.cap -= d
				f.graph[e.to][e.rev].cap += d
				return d
			}
		}
	}
	return 0
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
