package seminarattribution

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

const (
	scaleDeals       = 50000
	scaleSeminars    = 5000
	scaleLinks       = 300000
	scaleMaxAmount   = 1000000000000 // 1e12 美分
	scaleTimeLimit   = 20 * time.Second
	scaleMemoryLimit = 256 << 20 // 256 MiB
)

// makeScaleBatch 生成确定性的满规模批次：50000 笔商机、5000 场研讨会、
// 300000 条关联（每笔 6 条），金额混合大金额（最高 1e12）与小金额。
func makeScaleBatch(seed int64) []Deal {
	rng := rand.New(rand.NewSource(seed))
	deals := make([]Deal, scaleDeals)
	linksPerDeal := scaleLinks / scaleDeals
	for i := range deals {
		var amount int64
		switch i % 10 {
		case 0:
			amount = rng.Int63n(scaleMaxAmount) + 1 // 大金额
		case 1:
			amount = rng.Int63n(1000) // 小金额，制造大量余数
		default:
			amount = rng.Int63n(1000000)
		}
		sids := make([]string, linksPerDeal)
		// 确定性伪随机选取互不相同的研讨会。
		start := rng.Intn(scaleSeminars)
		step := 1 + rng.Intn(97)
		seen := make(map[int]struct{}, linksPerDeal)
		for j := 0; j < linksPerDeal; j++ {
			s := (start + j*step) % scaleSeminars
			for {
				if _, dup := seen[s]; !dup {
					break
				}
				s = (s + 1) % scaleSeminars
			}
			seen[s] = struct{}{}
			sids[j] = fmt.Sprintf("SEM-%05d", s)
		}
		deals[i] = Deal{
			ID:          fmt.Sprintf("DEAL-%06d", i),
			AmountCents: amount,
			SeminarIDs:  sids,
		}
	}
	return deals
}

// peakMemSampler 在后台周期采样 HeapAlloc，返回峰值。
func peakMemSampler(interval time.Duration) (stop func() uint64) {
	var peak uint64
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var ms runtime.MemStats
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak {
					peak = ms.HeapAlloc
				}
			}
		}
	}()
	return func() uint64 {
		close(done)
		// 再补一次采样，避免漏掉结束前的峰值。
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.HeapAlloc > peak {
			peak = ms.HeapAlloc
		}
		return peak
	}
}

// TestScale 满规模测试：核对全部金额约束，并记录生成耗时与峰值内存。
func TestScale(t *testing.T) {
	deals := makeScaleBatch(20261003)

	runtime.GC()
	stopSampler := peakMemSampler(2 * time.Millisecond)
	start := time.Now()

	res, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	report := res.FullReport()

	elapsed := time.Since(start)
	peak := stopSampler()

	t.Logf("scale: %d deals, %d seminars, %d links", len(deals), scaleSeminars, scaleLinks)
	t.Logf("scale: allocate+full report took %v (limit %v)", elapsed, scaleTimeLimit)
	t.Logf("scale: peak HeapAlloc %.1f MiB (limit %d MiB)", float64(peak)/float64(uint64(1)<<20), scaleMemoryLimit>>20)

	if elapsed > scaleTimeLimit {
		t.Errorf("too slow: %v > %v", elapsed, scaleTimeLimit)
	}
	if peak > scaleMemoryLimit {
		t.Errorf("peak memory %d bytes exceeds %d bytes", peak, scaleMemoryLimit)
	}
	if len(report.Deals) != scaleDeals || len(report.Seminars) != scaleSeminars {
		t.Fatalf("report size: %d deals, %d seminars", len(report.Deals), len(report.Seminars))
	}

	// 核对全部金额约束（逐笔守恒、逐项取值范围、逐场汇总边界）。
	verifyAllocation(t, deals, res)

	// 核对报表视图与内部分摊一致（抽样 + 汇总守恒）。
	var grandTotal int64
	for _, ss := range report.Seminars {
		var sum int64
		for _, it := range ss.Items {
			sum += it.AmountCents
		}
		if sum != ss.TotalCents {
			t.Errorf("seminar %s: items sum %d != total %d", ss.SeminarID, sum, ss.TotalCents)
		}
		if ss.TotalCents != ss.TheoreticalFloorCents && ss.TotalCents != ss.TheoreticalCeilCents {
			t.Errorf("seminar %s: total %d outside [%d,%d]",
				ss.SeminarID, ss.TotalCents, ss.TheoreticalFloorCents, ss.TheoreticalCeilCents)
		}
		grandTotal += ss.TotalCents
	}
	var dealTotal int64
	for _, d := range deals {
		dealTotal += d.AmountCents
	}
	if grandTotal != dealTotal {
		t.Errorf("grand total %d != sum of deal amounts %d", grandTotal, dealTotal)
	}
}

// TestScaleOrderIndependence 满规模批次打乱输入顺序后结果必须一致。
func TestScaleOrderIndependence(t *testing.T) {
	deals := makeScaleBatch(777)
	ref, err := Allocate(deals)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	refAlloc := allocMap(ref)

	rng := rand.New(rand.NewSource(99))
	shuffled := make([]Deal, len(deals))
	perm := rng.Perm(len(deals))
	for i, p := range perm {
		d := deals[p]
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
	if len(gotAlloc) != len(refAlloc) {
		t.Fatalf("alloc size %d != %d", len(gotAlloc), len(refAlloc))
	}
	for key, want := range refAlloc {
		if gotAlloc[key] != want {
			t.Fatalf("item %v = %d, want %d", key, gotAlloc[key], want)
		}
	}
}
