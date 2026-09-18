package system_api

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"linkstar/utils/res"

	"github.com/gin-gonic/gin"
)

// logRoot 和 core.InitLogger 里 Myhook 的 logPath 一致，按天分目录
const logRoot = "logs"

// tailLimit 从文件尾部往回读多少字节。一天的 info.log 能到好几 MB，
// 整个读进内存再切没必要，够翻出一页就行。
const tailLimit = 2 << 20 // 2MB

// ansiRe 日志落盘时带着终端颜色码（写的时候是给控制台看的），送进浏览器前要剥掉
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// lineRe 一行的形状，对应 core.MyLog.Format：
// [2026-09-18 00:00:01] [info] [network.go:73,linkstar/modules/stun.updateNetworkAddress]  正文
// caller 那一段在 SetReportCaller(false) 时不会出现，所以是可选的。
var lineRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] \[(\w+)\](?: \[([^\]]*)\])?\s*(.*)$`)

// dayRe 日期目录名。同时兼作入参白名单——这个值会拼进文件路径，
// 不校验就等于把 logs/../../ 这种路径交给调用方随便写。
var dayRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Caller  string `json:"caller"`
	Message string `json:"message"`
}

// LogDaysView 有哪几天的日志，新的在前
func (SystemApi) LogDaysView(c *gin.Context) {
	items, err := os.ReadDir(logRoot)
	if err != nil {
		// 还没写过日志时目录不存在，这不是错误
		res.OkWithData([]string{}, c)
		return
	}

	days := make([]string, 0, len(items))
	for _, it := range items {
		if it.IsDir() && dayRe.MatchString(it.Name()) {
			days = append(days, it.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	res.OkWithData(days, c)
}

// LogListView 读某一天的日志。按 level / 关键字过滤，返回最新的 limit 条，新的在前。
func (SystemApi) LogListView(c *gin.Context) {
	day := c.Query("day")
	if day == "" {
		res.FailWithMsg("请指定日期", c)
		return
	}
	if !dayRe.MatchString(day) {
		res.FailWithMsg("日期格式不对", c)
		return
	}

	// 只有这两个文件，写死成白名单；err.log 是 info.log 里错误级别的副本
	name := "info.log"
	if c.Query("file") == "err" {
		name = "err.log"
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}

	level := strings.ToLower(strings.TrimSpace(c.Query("level")))
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))

	raw, err := readTail(filepath.Join(logRoot, day, name), tailLimit)
	if err != nil {
		if os.IsNotExist(err) {
			res.OkWithData(gin.H{"list": []LogEntry{}, "truncated": false}, c)
			return
		}
		res.FailWithMsg("读取日志失败："+err.Error(), c)
		return
	}

	entries := parseLog(raw)

	// 从最新一条往回收集，够 limit 条就停，不用把一整天都过一遍
	out := make([]LogEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if level != "" && e.Level != level {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(e.Message), keyword) &&
			!strings.Contains(strings.ToLower(e.Caller), keyword) {
			continue
		}
		out = append(out, e)
	}

	res.OkWithData(gin.H{
		"list": out,
		// 只读了尾部一段，前面还有没读到的，页面上要讲清楚免得用户以为日志就这些
		"truncated": len(raw) >= tailLimit,
	}, c)
}

// readTail 读文件末尾最多 max 字节。文件比 max 小就整个读。
func readTail(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := st.Size()
	offset := int64(0)
	if size > max {
		offset = size - max
		size = max
	}

	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, offset); err != nil && len(buf) == 0 {
		return nil, err
	}

	// 从中间切进来的话第一行多半是半截，丢掉
	if offset > 0 {
		if i := strings.IndexByte(string(buf), '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf, nil
}

// parseLog 把日志正文切成一条条。对不上格式的行当成上一条的续行
// （日志正文里带换行时会这样），没有上一条就原样留着，不丢内容。
func parseLog(raw []byte) []LogEntry {
	lines := strings.Split(ansiRe.ReplaceAllString(string(raw), ""), "\n")
	entries := make([]LogEntry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			if n := len(entries); n > 0 {
				entries[n-1].Message += "\n" + line
			} else {
				entries = append(entries, LogEntry{Message: line})
			}
			continue
		}

		entries = append(entries, LogEntry{
			Time:    m[1],
			Level:   strings.ToLower(m[2]),
			Caller:  m[3],
			Message: strings.TrimSpace(m[4]),
		})
	}
	return entries
}
