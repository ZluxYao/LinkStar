//go:build windows

package utilsFile

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProtectConfigFile 用 NTFS ACL 把配置收成仅当前用户可访问。
//
// os.Chmod 在 Windows 上只能改只读位，0600 写进去后 Stat 仍是 0666，不能表示「仅属主」。
// 这里改写 DACL 并关掉继承，避免目录上的 Everyone / Users 继续套到密钥文件上。
// 父目录只有在明确是项目的 config 目录时才改，避免通用 JSON 写入动到 data/ 或其他目录。
func ProtectConfigFile(filePath string) error {
	if err := protectPath(filePath, false); err != nil {
		return fmt.Errorf("收紧配置文件 %s 的 ACL 失败，密钥可能仍可被其他用户读到: %w", filePath, err)
	}
	dir := filepath.Dir(filePath)
	if !isProjectConfigDir(dir) {
		return nil
	}
	if err := protectPath(dir, true); err != nil {
		return fmt.Errorf("收紧配置目录 %s 的 ACL 失败，密钥可能仍可被其他用户读到: %w", dir, err)
	}
	return nil
}

// VerifyConfigProtected 核对 DACL：允许访问的账户只能是当前用户。
func VerifyConfigProtected(filePath, dirPath string) error {
	if err := daclAllowsOnlyCurrentUser(filePath); err != nil {
		return err
	}
	if dirPath == "" || !isProjectConfigDir(dirPath) {
		return nil
	}
	return daclAllowsOnlyCurrentUser(dirPath)
}

func protectPath(path string, container bool) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	inherit := uint32(windows.NO_INHERITANCE)
	if container {
		inherit = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inherit,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func daclAllowsOnlyCurrentUser(path string) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("读取 %s 的 ACL 失败: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("读取 %s 的 DACL 失败: %w", path, err)
	}
	if dacl == nil || dacl.AceCount == 0 {
		return fmt.Errorf("%s 没有限制访问的 DACL", path)
	}
	sawUser := false
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return fmt.Errorf("读取 %s 的 ACE 失败: %w", path, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(user) {
			sawUser = true
			continue
		}
		name := sid.String()
		return fmt.Errorf("%s 的 ACL 仍允许其他账户 %s 访问", path, name)
	}
	if !sawUser {
		return fmt.Errorf("%s 的 ACL 没有给当前用户授权", path)
	}
	return nil
}

func currentUserSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid.Copy()
}
