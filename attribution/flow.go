package attribution

// dinic 实现 Dinic 最大流算法。节点编号固定、边按加入顺序存储，
// 因此在相同输入下结果确定。
type dinic struct {
	head  []int32 // 每个节点的第一条边，-1 表示无
	to    []int32 // 边的终点
	next  []int32 // 同一起点的下一条边
	cap   []int64 // 剩余容量
	level []int32
	it    []int32 // 当前弧优化：每个节点当前尝试的边
}

func newDinic(nodeCount int, edgeHint int) *dinic {
	head := make([]int32, nodeCount)
	for i := range head {
		head[i] = -1
	}
	return &dinic{
		head:  head,
		to:    make([]int32, 0, 2*edgeHint),
		next:  make([]int32, 0, 2*edgeHint),
		cap:   make([]int64, 0, 2*edgeHint),
		level: make([]int32, nodeCount),
		it:    make([]int32, nodeCount),
	}
}

// addEdge 加入一条 from->to、容量为 cap 的边，并返回其正向边编号。
func (d *dinic) addEdge(from, to int, capacity int64) int32 {
	id := int32(len(d.to))
	d.to = append(d.to, int32(to))
	d.next = append(d.next, d.head[from])
	d.cap = append(d.cap, capacity)
	d.head[from] = id
	d.to = append(d.to, int32(from))
	d.next = append(d.next, d.head[to])
	d.cap = append(d.cap, 0)
	d.head[to] = id + 1
	return id
}

// flow 返回当前正向边的流量（即初始容量减剩余容量）。
func (d *dinic) flow(edgeID int32, initialCap int64) int64 {
	return initialCap - d.cap[edgeID]
}

func (d *dinic) bfs(source, sink int) bool {
	for i := range d.level {
		d.level[i] = -1
	}
	queue := make([]int, 0, 64)
	d.level[source] = 0
	queue = append(queue, source)
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for e := d.head[v]; e != -1; e = d.next[e] {
			if d.cap[e] > 0 && d.level[d.to[e]] < 0 {
				d.level[d.to[e]] = d.level[v] + 1
				queue = append(queue, int(d.to[e]))
			}
		}
	}
	return d.level[sink] >= 0
}

func (d *dinic) dfs(v, sink int, f int64) int64 {
	if v == sink {
		return f
	}
	for ; d.it[v] != -1; d.it[v] = d.next[d.it[v]] {
		e := d.it[v]
		w := int(d.to[e])
		if d.cap[e] > 0 && d.level[w] == d.level[v]+1 {
			pushed := d.dfs(w, sink, minInt64(f, d.cap[e]))
			if pushed > 0 {
				d.cap[e] -= pushed
				d.cap[e^1] += pushed
				return pushed
			}
		}
	}
	return 0
}

// maxFlow 求 source 到 sink 的最大流。
func (d *dinic) maxFlow(source, sink int) int64 {
	var total int64
	for d.bfs(source, sink) {
		copy(d.it, d.head)
		for {
			pushed := d.dfs(source, sink, int64(1)<<62)
			if pushed == 0 {
				break
			}
			total += pushed
		}
	}
	return total
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
