package system_api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真实落盘的样子：core.MyLog.Format 会把颜色码一起写进文件
const sampleInfo = "[2026-09-18 00:00:01] \x1b[36m[info]\x1b[0m [network.go:73,linkstar/modules/stun.updateNetworkAddress] \x1b[36m 通过 STUN 服务器 stun.hot-chilli.net:3478 获取到的网络地址: LocalIP=192.168.100.187, PublicIP=14.19.90.21 \x1b[0m\n"

const sampleErr = "[2026-09-18 00:00:11] \x1b[31m[error]\x1b[0m [forward.go:67,linkstar/modules/stun.ForwardTCP] \x1b[31m 连接内网目标失败 [127.0.0.1:22]: dial tcp \x1b[0m\n"

// TestParseLog 颜色码要剥干净，时间/级别/调用位置/正文要各归各位。
// 对不上格式的行是正文里带了换行，得接回上一条，不能当成新的一条、更不能丢。
func TestParseLog(t *testing.T) {
	raw := sampleInfo + sampleErr + "  堆栈第二行\n" + "\n"

	got := parseLog([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("解析出 %d 条，想要 2 条：%+v", len(got), got)
	}

	if strings.Contains(got[0].Message, "\x1b") {
		t.Errorf("正文里还留着颜色码：%q", got[0].Message)
	}
	if got[0].Time != "2026-09-18 00:00:01" {
		t.Errorf("time = %q", got[0].Time)
	}
	if got[0].Level != "info" {
		t.Errorf("level = %q", got[0].Level)
	}
	if got[0].Caller != "network.go:73,linkstar/modules/stun.updateNetworkAddress" {
		t.Errorf("caller = %q", got[0].Caller)
	}
	if !strings.HasPrefix(got[0].Message, "通过 STUN 服务器") || strings.HasSuffix(got[0].Message, " ") {
		t.Errorf("message = %q", got[0].Message)
	}

	if got[1].Level != "error" {
		t.Errorf("level = %q", got[1].Level)
	}
	if !strings.HasSuffix(got[1].Message, "堆栈第二行") {
		t.Errorf("续行没接回上一条：%q", got[1].Message)
	}
}

// TestParseLogNoCaller SetReportCaller(false) 时没有调用位置那一段，不能因此把正文吞掉
func TestParseLogNoCaller(t *testing.T) {
	got := parseLog([]byte("[2026-09-18 10:00:00] [warn] 端口被占用\n"))
	if len(got) != 1 {
		t.Fatalf("解析出 %d 条", len(got))
	}
	if got[0].Caller != "" || got[0].Message != "端口被占用" || got[0].Level != "warn" {
		t.Errorf("%+v", got[0])
	}
}

// TestReadTailDropsPartialLine 从中间切进来时第一行是半截的，必须丢掉，
// 否则页面上第一条会是一段没头没尾的乱码。
func TestReadTailDropsPartialLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "info.log")
	if err := os.WriteFile(p, []byte("第一行很长很长很长\n第二行\n第三行\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 只给 12 字节，切点必然落在第一行中间
	got, err := readTail(p, 12)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "第一行") {
		t.Errorf("半截的第一行没丢掉：%q", got)
	}
	if !strings.Contains(string(got), "第三行") {
		t.Errorf("尾部内容丢了：%q", got)
	}
}

// TestReadTailWholeFile 文件比上限小就整个读，一个字都不能少
func TestReadTailWholeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "info.log")
	content := "第一行\n第二行\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := readTail(p, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("readTail = %q, 想要 %q", got, content)
	}
}

// TestDayRe 这个值会拼进文件路径，白名单必须挡住往上跳目录
func TestDayRe(t *testing.T) {
	ok := []string{"2026-09-18", "2026-01-01"}
	bad := []string{"", "..", "../..", "2026-09-18/../..", "logs", "2026-9-18", "2026-09-18 "}

	for _, s := range ok {
		if !dayRe.MatchString(s) {
			t.Errorf("%q 应该放行", s)
		}
	}
	for _, s := range bad {
		if dayRe.MatchString(s) {
			t.Errorf("%q 应该挡住", s)
		}
	}
}
