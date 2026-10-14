package utilsFile

import "path/filepath"

// projectConfigDirName 与各模块配置路径 "config/*.json" 的父目录同名。
// 写在这里是为了不让 utils 反向依赖具体模块。
const projectConfigDirName = "config"

// isProjectConfigDir 判断目录是不是项目配置目录。
// 只认目录名恰好是 config。config.bak、myconfig、以及更上层的父目录都不算。
func isProjectConfigDir(dir string) bool {
	clean := filepath.Clean(dir)
	if clean == "" || clean == "." || clean == string(filepath.Separator) {
		return false
	}
	return filepath.Base(clean) == projectConfigDirName
}
