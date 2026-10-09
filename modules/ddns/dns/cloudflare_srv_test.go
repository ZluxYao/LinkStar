package dns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// cfFakeSRV 有状态的假 Cloudflare DNS：只认 SRV 记录的查、建、改、删
type cfFakeSRV struct {
	mu      sync.Mutex
	records map[string]cfSRVRecord // id -> 记录
	nextID  int
	writes  int // 建 + 改 + 删 的次数，用来断言「没变就不打 API」
	deny    bool
}

func newFakeSRV(t *testing.T, seed ...cfSRVRecord) *cfFakeSRV {
	f := &cfFakeSRV{records: map[string]cfSRVRecord{}}
	for _, r := range seed {
		f.nextID++
		r.ID = fmt.Sprintf("r%d", f.nextID)
		f.records[r.ID] = r
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	old := zonesAPI
	zonesAPI = srv.URL + "/zones"
	t.Cleanup(func() { zonesAPI = old })
	return f
}

func (f *cfFakeSRV) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("content-type", "application/json")
	ok := func(v any) {
		b, _ := json.Marshal(map[string]any{"success": true, "result": v})
		w.Write(b)
	}
	if r.URL.Path == "/zones" {
		ok([]map[string]string{{"id": "z1", "name": "example.com"}})
		return
	}
	if f.deny {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`)
		return
	}
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	switch r.Method {
	case http.MethodGet:
		var out []cfSRVRecord
		for _, rec := range f.records {
			if rec.Type == r.URL.Query().Get("type") && rec.Name == r.URL.Query().Get("name") {
				out = append(out, rec)
			}
		}
		ok(out)
	case http.MethodPost:
		var rec cfSRVRecord
		json.NewDecoder(r.Body).Decode(&rec)
		f.nextID++
		rec.ID = fmt.Sprintf("r%d", f.nextID)
		f.records[rec.ID] = rec
		f.writes++
		ok(rec)
	case http.MethodPut:
		var rec cfSRVRecord
		json.NewDecoder(r.Body).Decode(&rec)
		rec.ID = id
		f.records[id] = rec
		f.writes++
		ok(rec)
	case http.MethodDelete:
		delete(f.records, id)
		f.writes++
		ok(map[string]string{"id": id})
	}
}

func (f *cfFakeSRV) only(t *testing.T) cfSRVRecord {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.records) != 1 {
		t.Fatalf("应该正好一条 SRV，实际 %d 条: %+v", len(f.records), f.records)
	}
	for _, r := range f.records {
		return r
	}
	return cfSRVRecord{}
}

const mcName = "_minecraft._tcp.mc.example.com"

func TestSyncSRVRecord(t *testing.T) {
	t.Run("没有就新建，带上标记", func(t *testing.T) {
		f := newFakeSRV(t)
		cf := NewCloudflare("x")
		changed, err := cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 18083)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		rec := f.only(t)
		if rec.Data.Port != 18083 || rec.Data.Target != "mc.example.com" || rec.Comment != srvRecordComment {
			t.Fatalf("建出来的不对: %+v", rec)
		}
	})

	t.Run("端口变了就改，不新建第二条", func(t *testing.T) {
		f := newFakeSRV(t)
		cf := NewCloudflare("x")
		cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 18083)
		changed, err := cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 20001)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		if rec := f.only(t); rec.Data.Port != 20001 {
			t.Fatalf("端口没改过去: %+v", rec)
		}
	})

	t.Run("什么都没变不打写接口", func(t *testing.T) {
		f := newFakeSRV(t)
		cf := NewCloudflare("x")
		cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 18083)
		before := f.writes
		changed, err := cf.SyncSRVRecord("example.com", mcName, "MC.example.com.", 18083)
		if err != nil || changed || f.writes != before {
			t.Fatalf("changed=%v err=%v 写了 %d 次", changed, err, f.writes-before)
		}
	})

	t.Run("接管用户手建的同名记录，优先级权重不动", func(t *testing.T) {
		// 之前用 Webhook 模板手建过的：没有标记，优先级权重是用户自己填的
		f := newFakeSRV(t, cfSRVRecord{Type: "SRV", Name: mcName, TTL: 1,
			Data: cfSRVData{Priority: 10, Weight: 20, Port: 1111, Target: "mc.example.com"}})
		cf := NewCloudflare("x")
		if _, err := cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 18083); err != nil {
			t.Fatal(err)
		}
		rec := f.only(t)
		if rec.Data.Port != 18083 || rec.Comment != srvRecordComment {
			t.Fatalf("没接管上: %+v", rec)
		}
		if rec.Data.Priority != 10 || rec.Data.Weight != 20 {
			t.Fatalf("用户的优先级权重被改了: %+v", rec)
		}
	})

	t.Run("没 DNS 权限说人话", func(t *testing.T) {
		f := newFakeSRV(t)
		f.deny = true
		_, err := NewCloudflare("x").SyncSRVRecord("example.com", mcName, "mc.example.com", 18083)
		if err == nil || !strings.Contains(err.Error(), "编辑区域 DNS") {
			t.Fatalf("err = %v，应该告诉用户缺 DNS 权限", err)
		}
	})
}

func TestRemoveSRVRecord(t *testing.T) {
	t.Run("自己建的删掉", func(t *testing.T) {
		f := newFakeSRV(t)
		cf := NewCloudflare("x")
		cf.SyncSRVRecord("example.com", mcName, "mc.example.com", 18083)
		removed, err := cf.RemoveSRVRecord("example.com", mcName)
		if err != nil || !removed || len(f.records) != 0 {
			t.Fatalf("removed=%v err=%v 剩 %d 条", removed, err, len(f.records))
		}
	})

	t.Run("用户自己的不动", func(t *testing.T) {
		f := newFakeSRV(t, cfSRVRecord{Type: "SRV", Name: mcName, TTL: 1,
			Data: cfSRVData{Port: 1111, Target: "mc.example.com"}})
		removed, err := NewCloudflare("x").RemoveSRVRecord("example.com", mcName)
		if err != nil || removed || len(f.records) != 1 {
			t.Fatalf("removed=%v err=%v 剩 %d 条，用户的记录不该删", removed, err, len(f.records))
		}
	})

	t.Run("本来就没有", func(t *testing.T) {
		newFakeSRV(t)
		removed, err := NewCloudflare("x").RemoveSRVRecord("example.com", mcName)
		if err != nil || removed {
			t.Fatalf("removed=%v err=%v", removed, err)
		}
	})
}
