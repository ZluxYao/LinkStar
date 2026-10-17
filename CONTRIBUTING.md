# 贡献指南

感谢参与 LinkStar 的开发。本指南适用于新提交的代码和 Pull Request；现有 Git 历史不需要重写。

## Commit Message 规范

格式：

```text
<type>[optional scope][!]: <description>

[optional body]

[optional footer(s)]
```

- `type`：必填，表示变更类型，统一使用英文小写。
- `scope`：可选，标明受影响模块。
- `description`：必填，简短说明本次改动，可使用中文或英文。
- `body`：可选，解释修改原因、实现方式和兼容性影响；与标题之间空一行。
- `footer`：可选，用于关联 Issue 或说明破坏性变更。

### Type 类型

| Type | 适用场景 |
| --- | --- |
| `feat` | 新增功能 |
| `fix` | 修复缺陷 |
| `refactor` | 重构代码，不改变对外行为 |
| `perf` | 性能优化 |
| `docs` | 文档变更 |
| `test` | 增加或调整测试 |
| `build` | 构建系统或依赖变更 |
| `ci` | CI 配置或工作流变更 |
| `chore` | 其他维护性变更 |
| `revert` | 回滚已有变更 |

### Scope 范围

Scope 不强制填写。需要标明模块时，使用代码中可识别的名称，例如 `stun`、`ddns`、`cert`、`proxy`、`api`、`web`。

不要为每个文件创建新的 Scope；不要求统一维护 Scope 白名单。

### 示例

```text
fix(stun): 修复探测超时后未正确返回错误的问题
feat(ddns): 支持新的 DNS 服务商
refactor(proxy): 简化路由匹配逻辑
docs: 更新部署说明
test(cert): 补充证书续期失败场景测试
ci: 调整 Go 测试工作流
```

当需要解释原因或验证方式时，在标题下空一行添加正文：

```text
fix(stun): 修复探测超时后未正确返回错误的问题

原实现可能将超时结果视为有效探测数据。
现在明确返回超时错误，并补充回归测试。

Refs: #123
```

对于不兼容变更，在 `type` / `scope` 后添加 `!`，或在 Footer 中使用 `BREAKING CHANGE:` 并说明影响：

```text
feat(api)!: 调整服务配置字段

BREAKING CHANGE: 旧版配置字段需要迁移后才能使用。
```

`feat`、`fix` 和 `BREAKING CHANGE` 分别可用于语义化版本管理的 MINOR、PATCH 和 MAJOR 判断；本指南本身不代表项目已经启用自动发布。

## Pull Request

1. 基于上游仓库的目标分支创建独立的功能或修复分支，避免夹带无关变更。
2. PR 标题建议沿用相同的 Conventional Commits 格式。
3. PR 描述中写明变更目的、主要改动、兼容性影响及实际验证结果。
4. 修改 Go 代码时，尽可能执行相关测试（例如 `go test ./...`）；修改前端时，执行对应前端的构建或测试。不要将未执行的测试标记为通过。

本规范仅面向后续新增提交；不要求重新整理已合并的提交历史。是否启用 Commit Message 的 CI 强制校验，由后续独立变更决定。

