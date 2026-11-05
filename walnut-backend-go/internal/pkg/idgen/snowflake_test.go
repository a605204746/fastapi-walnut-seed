package idgen

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestBitLayoutConstants 锁住位分配。
//
// 这些常量是与另两端的硬契约：改动会让同一 ID 在三端解析出不同的
// 时间戳/机器位，跨端引用（日志里记录的 ID、导出的 Excel）全部对不上。
func TestBitLayoutConstants(t *testing.T) {
	if Epoch != 1577836800000 {
		t.Errorf("Epoch = %d，必须为 1577836800000（2020-01-01 UTC，与 Python _EPOCH 一致）", Epoch)
	}
	if workerIDShift != 12 {
		t.Errorf("workerIDShift = %d，期望 12", workerIDShift)
	}
	if timestampShift != 22 {
		t.Errorf("timestampShift = %d，期望 22 (= sequenceBits 12 + workerIDBits 10)", timestampShift)
	}
	if MaxWorkerID != 1023 {
		t.Errorf("MaxWorkerID = %d，期望 1023", MaxWorkerID)
	}
	if MaxSequence != 4095 {
		t.Errorf("MaxSequence = %d，期望 4095", MaxSequence)
	}
}

// TestDecodePythonGeneratedID 用 **Python 端实测生成的真实 ID** 验证位布局互通。
//
// 样本取自 2026-11-02 在 Python 端执行
// `[IdGeneratorUtil.next_long_id() for _ in range(3)]` 的输出。
// 若 Go 的移位/掩码与 Python 不一致，反解出的时间戳会落在荒谬的年份，
// 机器位也会超出 0-1023。
func TestDecodePythonGeneratedID(t *testing.T) {
	samples := []int64{
		904941282285273088,
		904941282285273089,
		904941282285273090,
	}

	for _, id := range samples {
		ts := TimestampFromID(id)
		worker := WorkerIDFromID(id)
		seq := SequenceFromID(id)

		// 时间戳必须落在合理区间：epoch(2020) 之后、当前时间附近
		if ts.Year() < 2020 || ts.Year() > 2100 {
			t.Errorf("ID %d 反解出的时间 %v 不合理（位布局可能与 Python 不一致）", id, ts)
		}
		// 样本生成于 2026-11-02，允许 ±1 天误差（时区/时钟漂移）
		want := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
		if diff := ts.Sub(want); diff < -48*time.Hour || diff > 48*time.Hour {
			t.Errorf("ID %d 反解时间 %v 与样本生成日期 2026-11-02 相差过远", id, ts)
		}
		if worker < 0 || worker > MaxWorkerID {
			t.Errorf("ID %d 反解机器位 %d 超出 0-%d", id, worker, MaxWorkerID)
		}
		if seq < 0 || seq > MaxSequence {
			t.Errorf("ID %d 反解序列号 %d 超出 0-%d", id, seq, MaxSequence)
		}
		t.Logf("ID %d → 时间 %s / 机器位 %d / 序列号 %d", id, ts.Format(time.RFC3339), worker, seq)
	}

	// 连续三个 ID 应只差序列号（同毫秒内生成）
	if samples[1]-samples[0] != 1 || samples[2]-samples[1] != 1 {
		t.Errorf("连续 ID 差值应均为 1（同毫秒序列号递增），实际 %d / %d",
			samples[1]-samples[0], samples[2]-samples[1])
	}
}

// TestIDMagnitudeExceedsJSSafe 确认雪花 ID 恒超出 JS 安全整数范围。
//
// 这是 idtype.JSInt 必须把 ID 序列化为字符串的**前提事实**（见实施计划 §9.1）。
// 若哪天 ID 变小到安全范围内，前端契约就要重新评估。
func TestIDMagnitudeExceedsJSSafe(t *testing.T) {
	const jsSafeMax int64 = 9007199254740991

	g, err := New(1)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	for i := 0; i < 100; i++ {
		id := g.NextID()
		if id <= jsSafeMax {
			t.Fatalf("ID %d 未超出 JS 安全整数上限 %d，前端会丢失精度", id, jsSafeMax)
		}
	}
	id := g.NextID()
	t.Logf("样本 ID %d 是 JS 安全上限的 %.1f 倍（%d 位十进制）",
		id, float64(id)/float64(jsSafeMax), len(strconv.FormatInt(id, 10)))
}

// TestNewRejectsOutOfRange 验证越界机器位被拒绝而不是静默掩码。
//
// 静默掩码会让越界值回绕成**另一个实例**的机器位，两实例产生相同 ID
// 却毫无报错——这是最难排查的一类故障。
func TestNewRejectsOutOfRange(t *testing.T) {
	for _, bad := range []int{-1, 1024, 99999} {
		if _, err := New(bad); err == nil {
			t.Errorf("New(%d) 应当失败（合法范围 0-%d）", bad, MaxWorkerID)
		}
	}
	for _, ok := range []int{0, 1, 512, 1023} {
		if _, err := New(ok); err != nil {
			t.Errorf("New(%d) 不应失败: %v", ok, err)
		}
	}
}

// TestMonotonicAndUnique 验证单生成器的 ID 严格递增且唯一。
func TestMonotonicAndUnique(t *testing.T) {
	g, err := New(7)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	const n = 20000
	seen := make(map[int64]struct{}, n)
	prev := int64(-1)
	for i := 0; i < n; i++ {
		id := g.NextID()
		if id <= prev {
			t.Fatalf("第 %d 个 ID %d 未严格大于前一个 %d", i, id, prev)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("第 %d 个 ID %d 重复", i, id)
		}
		seen[id] = struct{}{}
		prev = id
		if WorkerIDFromID(id) != 7 {
			t.Fatalf("ID %d 的机器位 = %d，期望 7", id, WorkerIDFromID(id))
		}
	}
	t.Logf("生成 %d 个 ID，严格递增且全部唯一", n)
}

// TestConcurrentUniqueness 验证并发安全（Python 端用 threading.Lock，Go 用 sync.Mutex）。
//
// ⚠️ 这条用例必须在 CI 里配 `-race` 跑才能真正验证 R8；本机 Windows 无 gcc
// 跑不了 -race（见实施计划 R18），故 CI 门禁不可省。
func TestConcurrentUniqueness(t *testing.T) {
	g, err := New(3)
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	const (
		goroutines = 16
		perG       = 2000
	)
	var (
		mu   sync.Mutex
		seen = make(map[int64]struct{}, goroutines*perG)
		wg   sync.WaitGroup
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]int64, 0, perG)
			for j := 0; j < perG; j++ {
				local = append(local, g.NextID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("并发下产生重复 ID: %d", id)
					return
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perG {
		t.Errorf("唯一 ID 数 = %d，期望 %d", len(seen), goroutines*perG)
	}
	t.Logf("%d 协程 × %d 次 = %d 个 ID，无重复", goroutines, perG, len(seen))
}

// TestWorkerIDIsolation 验证不同机器位产生的 ID 不冲突（多副本部署的基础）。
func TestWorkerIDIsolation(t *testing.T) {
	g1, _ := New(1)
	g2, _ := New(2)

	const n = 5000
	set := make(map[int64]struct{}, 2*n)
	for i := 0; i < n; i++ {
		id1, id2 := g1.NextID(), g2.NextID()
		if _, dup := set[id1]; dup {
			t.Fatalf("worker 1 内部重复: %d", id1)
		}
		set[id1] = struct{}{}
		if _, dup := set[id2]; dup {
			t.Fatalf("worker 2 与已有 ID 冲突: %d", id2)
		}
		set[id2] = struct{}{}
	}
	if len(set) != 2*n {
		t.Errorf("两个机器位共产生 %d 个唯一 ID，期望 %d", len(set), 2*n)
	}
}

// TestDeriveWorkerIDInRange 验证回退派生的机器位落在合法区间。
func TestDeriveWorkerIDInRange(t *testing.T) {
	id := deriveWorkerID()
	if id < 0 || id > MaxWorkerID {
		t.Errorf("deriveWorkerID() = %d，超出 0-%d", id, MaxWorkerID)
	}
	ip := localIP()
	if ip == "" {
		t.Error("localIP() 返回空串")
	}
	t.Logf("回退派生：本机 IP=%s PID=%d → workerID=%d", ip, os.Getpid(), id)

	// 同一进程内多次调用应稳定（IP 与 PID 都不变）
	if again := deriveWorkerID(); again != id {
		t.Errorf("deriveWorkerID() 不稳定：%d vs %d", id, again)
	}
}

// TestNewAutoUsesConfigured 验证配置优先于回退派生。
func TestNewAutoUsesConfigured(t *testing.T) {
	configured := 42
	g, err := NewAuto(&configured, nil)
	if err != nil {
		t.Fatalf("NewAuto 失败: %v", err)
	}
	if g.WorkerID() != 42 {
		t.Errorf("WorkerID = %d，期望 42（配置应优先于回退派生）", g.WorkerID())
	}

	// 越界配置必须报错，不能静默回退到派生
	bad := 2000
	if _, err := NewAuto(&bad, nil); err == nil {
		t.Error("SNOWFLAKE_WORKER_ID=2000 越界，应当报错")
	}
}

// TestUUID 验证 32 位无横线 UUID（对象存储文件 key 用）。
func TestUUID(t *testing.T) {
	u := UUID()
	if len(u) != 32 {
		t.Errorf("UUID 长度 = %d，期望 32（无横线）", len(u))
	}
	for _, c := range u {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("UUID 含非小写十六进制字符: %q", u)
			break
		}
	}
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		v := UUID()
		if _, dup := seen[v]; dup {
			t.Fatalf("UUID 重复: %s", v)
		}
		seen[v] = struct{}{}
	}
}

func TestPrefixedVariants(t *testing.T) {
	if got := UUIDWithPrefix("file_"); len(got) != 5+32 || got[:5] != "file_" {
		t.Errorf("UUIDWithPrefix = %q", got)
	}
	if got := IDWithPrefix("ord_"); len(got) < 5 || got[:4] != "ord_" {
		t.Errorf("IDWithPrefix = %q", got)
	}
}

// TestSetDefaultIsRaceFree 验证全局单例的并发读写安全。
//
// 配 -race 运行时，这条用例会真正检测 SetDefault / Default 的数据竞争。
func TestSetDefaultIsRaceFree(t *testing.T) {
	g, _ := New(9)
	SetDefault(g)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if got := Default(); got.WorkerID() != 9 {
					t.Errorf("Default() 返回了错误的生成器: workerID=%d", got.WorkerID())
					return
				}
				_ = NextID()
			}
		}()
	}
	wg.Wait()
}
