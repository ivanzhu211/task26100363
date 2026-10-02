package attribution

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	perfDeals    = 50000
	perfSeminars = 5000
	perfLinks    = 300000
	perfMaxAmt   = 1000000000000

	perfTimeLimit   = 20 * time.Second
	perfMemoryLimit = 256 << 20 // 256 MiB
)

// perfBatch 生成达到验收规模的确定性批次：50000 笔商机、
// 5000 场研讨会、300000 条关联、单笔金额最高 10^12 美分。
func perfBatch() []Deal {
	rng := rand.New(rand.NewSource(20261003))
	deals := make([]Deal, perfDeals)
	per := perfLinks / perfDeals // 每笔 6 条关联
	for i := range deals {
		perm := rng.Perm(perfSeminars)[:per]
		ids := make([]string, per)
		for j, s := range perm {
			ids[j] = fmt.Sprintf("sem-%05d", s)
		}
		deals[i] = Deal{
			ID:          fmt.Sprintf("deal-%06d", i),
			AmountCents: rng.Int63n(perfMaxAmt + 1),
			SeminarIDs:  ids,
		}
	}
	return deals
}

// TestPerfWorker 在子进程中运行：生成报表、核对全部金额约束、输出耗时。
func TestPerfWorker(t *testing.T) {
	if os.Getenv("ATTRIBUTION_PERF_WORKER") != "1" {
		t.Skip("仅由 TestPerformance 以子进程方式运行")
	}
	deals := perfBatch()
	start := time.Now()
	r, err := GenerateReport(deals)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	verifyReport(t, deals, r)
	fmt.Printf("DURATION_MS=%d\n", elapsed.Milliseconds())
}

// TestPerformance 在子进程中跑满规模用例，记录生成耗时与峰值内存，
// 并核对全部金额约束（由子进程内的 verifyReport 完成）。
func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式下跳过性能测试")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestPerfWorker$", "-test.v")
	cmd.Env = append(os.Environ(), "ATTRIBUTION_PERF_WORKER=1")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	runErr := cmd.Run()
	wall := time.Since(start)
	if runErr != nil {
		t.Fatalf("性能子进程失败: %v\n%s", runErr, out.String())
	}

	var duration time.Duration
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "DURATION_MS=") {
			ms, _ := strconv.Atoi(strings.TrimPrefix(line, "DURATION_MS="))
			duration = time.Duration(ms) * time.Millisecond
		}
	}

	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Fatal("无法读取子进程资源用量")
	}
	peakBytes := ru.Maxrss
	if runtime.GOOS == "linux" {
		peakBytes *= 1024 // Linux 的 Maxrss 单位为 KiB
	}

	t.Logf("规模: %d 商机 / %d 研讨会 / %d 关联", perfDeals, perfSeminars, perfLinks)
	t.Logf("报表生成耗时: %v（子进程总耗时 %v，上限 %v）", duration, wall.Round(time.Millisecond), perfTimeLimit)
	t.Logf("子进程峰值内存: %.1f MiB（上限 %d MiB）", float64(peakBytes)/(1<<20), perfMemoryLimit>>20)

	if duration > perfTimeLimit {
		t.Errorf("生成耗时 %v 超过上限 %v", duration, perfTimeLimit)
	}
	if peakBytes > perfMemoryLimit {
		t.Errorf("峰值内存 %d 字节超过上限 %d 字节", peakBytes, perfMemoryLimit)
	}
}
