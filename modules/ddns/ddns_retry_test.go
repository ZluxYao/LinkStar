package ddns

import (
	"testing"
	"time"

	"linkstar/modules/ddns/model"
)

// TestRetryAfterBacksOffOnFailure 没同步成的记录得比正常间隔早得多地再试一次。
//
// 开机时 STUN 还在挑服务器，DDNS 这一轮必然取不到公网 IP。按正常间隔算就是
// 干等 5 分钟，这 5 分钟里域名一直指着旧地址——用户看到的就是「等了好久才同步」。
func TestRetryAfterBacksOffOnFailure(t *testing.T) {
	s := &Scheduler{failures: make(map[uint]int)}
	interval := 5 * time.Minute

	if got := s.retryAfter(1, interval); got != interval {
		t.Fatalf("没失败过应按正常间隔，实际 %v", got)
	}

	s.noteResult(1, false)
	if got := s.retryAfter(1, interval); got != failRetryBase {
		t.Fatalf("第一次失败应等 %v，实际 %v", failRetryBase, got)
	}

	s.noteResult(1, false)
	if got := s.retryAfter(1, interval); got != 2*failRetryBase {
		t.Fatalf("第二次失败应翻倍到 %v，实际 %v", 2*failRetryBase, got)
	}

	// 一直错的（token 填错、域名不在这个账号下）不能越试越勤，最多退到正常间隔
	for i := 0; i < 20; i++ {
		s.noteResult(1, false)
	}
	if got := s.retryAfter(1, interval); got != interval {
		t.Fatalf("连续失败封顶应为正常间隔 %v，实际 %v", interval, got)
	}

	// 成功一次就清零，回到正常节奏
	s.noteResult(1, true)
	if got := s.retryAfter(1, interval); got != interval {
		t.Fatalf("成功后应回到正常间隔，实际 %v", got)
	}
}

// TestRetryAfterIsPerRecord 一条记录配错了，不该把别的记录也拖慢
func TestRetryAfterIsPerRecord(t *testing.T) {
	s := &Scheduler{failures: make(map[uint]int)}
	interval := 5 * time.Minute

	s.noteResult(1, false)
	if got := s.retryAfter(2, interval); got != interval {
		t.Fatalf("另一条记录应按正常间隔，实际 %v", got)
	}
}

// TestDueUsesLastCheckAt 到期判断看的是「上次尝试」的时间
func TestDueUsesLastCheckAt(t *testing.T) {
	now := time.Now()
	r := &model.DDNSRecord{LastCheckAt: now.Add(-40 * time.Second)}

	if due(r, 5*time.Minute, now) {
		t.Fatal("才过 40s，按 5 分钟算不该到期")
	}
	if !due(r, 30*time.Second, now) {
		t.Fatal("才过 40s，按 30s 的重试节奏应该到期")
	}
}

// TestNormalInterval 间隔没填按 5 分钟算
func TestNormalInterval(t *testing.T) {
	if got := normalInterval(0); got != 5*time.Minute {
		t.Fatalf("留空应为 5 分钟，实际 %v", got)
	}
	if got := normalInterval(-1); got != 5*time.Minute {
		t.Fatalf("负数应为 5 分钟，实际 %v", got)
	}
	if got := normalInterval(60); got != time.Minute {
		t.Fatalf("填 60 应为 1 分钟，实际 %v", got)
	}
}
