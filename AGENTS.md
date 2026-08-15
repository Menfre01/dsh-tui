# AGENTS.md — dsh-tui 开发与发布规范

dsh-tui 是 [deepseek-harness](https://github.com/deepseek-ai/deepseek-harness) 的
Go 终端客户端,渲染层移植自 [waveloom](https://github.com/Menfre01/waveloom)。

## 开发

- **构建/测试**:`make build && make test && make vet`(环境变量指向工作区缓存,
  仓库 `.git` 状态特殊,构建需 `-buildvcs=false`)
- **协议漂移防护**:wire 层全部隔离在 `internal/dsh`,渲染/投影在 `internal/tui`;
  宿主协议变更时先对照 upstream 源码(`@deepseek-ai/*` 包)再改 wire 层
- **新增工具渲染**:宿主新增工具时,`formatToolArgs`(参数摘要)与 `toolSuffix`
  (状态 suffix)必须覆盖,避免摘要行显示原始 JSON
- **测试**:投影/交互/渲染路径的修复必须带回归测试

## Release 规范

**发布前置校验**(必须全部通过后方可继续发布流程):

```sh
make build && make test && make vet
```

任一失败 → 先修复,再重新走校验。

Release notes 以用户可感知的功能变化为描述单位,分类汇总:

- **新增功能** — 新特性、模块、命令
- **修复** — Bug 修复
- **重构** — 重大模块重构
- **性能优化** — 性能相关

`docs` / `chore` / `test` 类型不列入。

**无需列入 changelog 的判定规则**(用户无感原则):

修复/新增功能在上一个正式版中不存在(该特性随当前版本首次发布,无用户受影响),
changelog 中无需列出。典型场景与判定方法:

- **新组件/新模块首次发布**:其内部缺陷修复(协议合规、竞态等)不列修复条目
- **新特性引入问题的修正**:本版本新增特性后对其自身问题的收紧/降级,不列独立
  修复条目,并入对应"新增功能"条目说明边界
- **判定方法**:用 `git show <上一tag>:<涉及文件>` 核对缺陷是否在上一正式版存在:
  - 文件/代码路径在上一 tag 存在且缺陷相同 → 必须列为修复(用户可感知)
  - 文件/代码路径为本次新增 → 不列
  - 边界:跨版本存在的缺陷(即使本轮才修复)必须列入

**Release body 格式**:主体为 changelog 分类汇总:

```
## [vX.Y.Z] — YYYY-MM-DD

### 新增功能
- ...

### 修复
- ...

### 重构
- ...
```

---

📝 [Changelog (English)](https://github.com/Menfre01/dsh-tui/blob/main/CHANGELOG.en.md)

发布由 GitHub Actions 自动完成(tag push `v*` → `.github/workflows/release.yml`)。

手动步骤(release workflow 之前完成):

1. **汇总 changelog** — 从上次 tag 到 HEAD 扫描 commit,按分类汇总,更新
   `CHANGELOG.md`
2. **核对日期** — 检查 `CHANGELOG.md` 中新版本的日期是否为当天日期
   (`date '+%Y-%m-%d'`),防止日期偏移
3. **核对英文锚点** — 检查 `CHANGELOG.md` 中新版本条目末尾是否包含英文
   changelog 锚点(搜索 `📝 [Changelog (English)]`),确保 Release body 末尾有英文入口
4. **审查 Windows 兼容性** — 检查本次变更涉及的代码是否存在平台依赖问题:
   - 路径拼接是否使用 `filepath.Join`,无硬编码 `/` 或 `\`
   - 文件遍历优先使用 `filepath.WalkDir` / `os.ReadDir`,无外部命令
   - 新增依赖是否声明跨平台支持
   - Git diff 中新增的 `/` 分隔符确认是 Go 导入路径(安全)而非文件系统路径
5. **审查 README** — 检查 `README.md`(英文主文档)与 `docs/README.zh-CN.md`
   是否需要同步新功能
6. **文档提交** — 如有文档修改,先 commit(类型 `docs`)
7. **打 tag 并推送** — `git tag vX.Y.Z && git push origin main && git push origin vX.Y.Z`

**Release 重发(发布后修复缺陷)**:

- 重发仅用于发布产物含用户可感知缺陷;首次发布特性内缺陷按用户无感原则不重发
- **操作顺序(关键:先删 release,再动 tag,否则 release 会变草稿)**:
  1. `gh release delete vX.Y.Z --yes` — 先删旧 release(连带资产;不删 tag)
  2. 本地移动 tag:`git tag -f vX.Y.Z <新commit>`(变更已 commit 到 main)
  3. 删远端 tag ref:`gh api -X DELETE repos/<owner>/<repo>/git/refs/tags/vX.Y.Z`
  4. 推送新 tag:`git push origin vX.Y.Z` → workflow 走 create 分支,正式发布
- **坑(实测踩过)**:先删远端 tag 再 push,会把已发布 release 打成 `untagged + draft`;
  workflow 幂等 edit 分支不会 publish,网页不可见。若已发生:删草稿 release
  (`gh release delete --yes`)+ `gh run rerun <run-id>`(view 失败 → create 分支)
- 重发后必须验证:`gh release view vX.Y.Z --json isDraft,publishedAt,url`
  (isDraft=false 且 url 为正常 release 页)
