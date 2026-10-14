//go:build unix

package utilsFile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJsonFile不修改权限(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := os.WriteFile(path, []byte(`{"x":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJsonFile[map[string]int](path); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o644 {
		t.Fatalf("读取后文件权限 = %o，不应被改掉", fileInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o755 {
		t.Fatalf("读取后目录权限 = %o，不应被改掉", dirInfo.Mode().Perm())
	}
}

func Test收紧不存在的配置文件会报错(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ProtectConfigFile(filepath.Join(dir, "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "收紧配置文件") {
		t.Fatalf("应收紧失败并带上上下文，实际 %v", err)
	}
	dirInfo, statErr := os.Stat(dir)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if dirInfo.Mode().Perm() != 0o755 {
		t.Fatalf("收紧失败时父目录权限 = %o，不应被改掉", dirInfo.Mode().Perm())
	}
}

func Test是否项目配置目录(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{name: "空路径", dir: "", want: false},
		{name: "当前目录", dir: ".", want: false},
		{name: "根目录", dir: string(filepath.Separator), want: false},
		{name: "data 目录", dir: "data", want: false},
		{name: "名字只是像 config", dir: "config.bak", want: false},
		{name: "前缀不是 config", dir: "myconfig", want: false},
		{name: "相对 config", dir: "config", want: true},
		{name: "点斜杠 config", dir: filepath.Join(".", "config"), want: true},
		{name: "带斜杠的 config", dir: "config" + string(filepath.Separator), want: true},
		{name: "绝对路径以 config 结尾", dir: filepath.Join(string(filepath.Separator), "var", "lib", "config"), want: true},
		{name: "config 的子目录", dir: filepath.Join("config", "nested"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isProjectConfigDir(tc.dir); got != tc.want {
				t.Fatalf("isProjectConfigDir(%q) = %v，想要 %v", tc.dir, got, tc.want)
			}
		})
	}
}

func Test只把项目config目录收成0700(t *testing.T) {
	root := t.TempDir()
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	rootPerm := rootInfo.Mode().Perm()

	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(dataDir, "notes.json")
	if err := os.WriteFile(dataFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ProtectConfigFile(dataFile); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, dataFile, 0o600)
	assertPerm(t, dataDir, 0o755)
	assertPerm(t, root, rootPerm)

	cfgDir := filepath.Join(root, "config")
	if err := os.Mkdir(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(cfgDir, "authConfig.json")
	if err := WriteJsonFile(cfgFile, map[string]string{"jwtSecret": "secret"}); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, cfgFile, 0o600)
	assertPerm(t, cfgDir, 0o700)
	assertPerm(t, root, rootPerm)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s 权限 = %o，想要 %o", path, info.Mode().Perm(), want)
	}
}
