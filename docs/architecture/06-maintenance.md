# 维护指南 · 扩展点 / 测试 / 坑

> 面向未来接手者：改代码前先读 00-overview 与对应分册。本文件是
> "动手清单"——怎么加功能、怎么验证、哪些坑踩过。

## 1. 扩展点

### 加一个新笔记软件（如思源）

1. `internal/adapter/` 加 `xxx.go`：实现 `ParseXXX(...) ([]*Document, error)`，
   把软件数据翻译成 Document（ID/Title/Aliases/Type/Path/Refs/Tags/Text）。
2. `cmd/seren/main.go` 的 `parseSource` 加分支识别（如按扩展名/特征）。
3. 若需要类型语义 → 画像（VaultProfile）字段扩展。
4. 遵循安全红线：活数据先快照/只读，凭据类数据绝不碰。

### 加一个打分维度（如依赖分 δ）

- `internal/score`：`Dim` 加一维；`RankOpts` 加权重；`Rank` 里加融合项。
- `roam.Compute` 传入对应分（当前 γ/δ 传 0）。
- 设计修订 #13：v1 只有一组全局默认，场景预设是 v2。

### 加一个 CLI 子命令

- `cmd/seren/main.go`：`main` 的 switch + `cmdXxx` + `usage`。
- 复用 `parseArgs`（支持 `--k=v` / `--k v`）、`fint/ffloat`、`fatal`。

### 加一个 Web 端点

- `web.Handler()` 注册；读图用 `s.mu.RLock()`（refresh 换图持 Lock）。
- 闭包注入模式：能力（如 refresh/touch）由 main 构造传入，web 不感知来源。

## 2. 构建与测试（沙箱环境）

```powershell
# 环境变量（沙箱内必须，否则写不进系统缓存）
$env:GOCACHE='D:\WorkSpace\serendipity-engine\scratch\.gocache'
$env:GOPATH='D:\WorkSpace\serendipity-engine\scratch\.gopath'
$env:GOMODCACHE="$env:GOPATH\pkg\mod"
$env:GOENV='D:\WorkSpace\serendipity-engine\scratch\go.env'
$env:GOTELEMETRY='off'; $env:CGO_ENABLED='0'
go build -o scratch/seren.exe ./cmd/seren
```

- 前端 JS：提取 `<script>` 内容 → `node --check`。
- 接口验证：`Invoke-RestMethod` 打各端点。
- 真实数据测试样本：`scratch/orca_copy.db`（小虎鲸库）、
  `D:\WorkSpace\NoteLib\TestOrca\TestOrca.db`（大库，**用户活库，app 可能独占锁**——
  正好测快照双路径）；`scratch/okf-test/`（OKF 合成库）。
- **PowerShell 大小写不敏感陷阱**：曾因 JSON 键大小写不一致导致前端 undefined——
  用 Python 做大小写敏感校验。
- Windows 沙箱：Go/Python 程序不能直接读工作区外文件（先拷贝到 scratch）。

## 3. 已知坑（按模块）

### adapter/orca
- **to_pinyin stub 必须确定性**：`RegisterDeterministicScalarFunction`（非确定性会
  报 malformed）；生成列 `name_p` 校验需要它。
- **VACUUM INTO 在 app 独占锁下必失败**：锁探测（busy_timeout=0 的 SELECT 1）先行走
  文件拷贝路径，别傻等 busy_timeout。
- **文件拷贝顺序先 wal 后 db**：checkpoint 间隙不丢已落盘数据（wal salt 不匹配被忽略）。
- 聚合后要**清理宿主 Refs 中的悬空目标**（空壳过滤的包含边），否则悬空链接暴涨。

### adapter/obsidian
- 同名文件 ID 冲突 → 相对路径 ID 消歧（否则 store UNIQUE 炸）。
- md 链接只认 `.md` 目标；带协议外链跳过。

### graph / roam
- `Stats().Edges = Σdeg/2`，别按节点数算。
- Resolve 短 ID（虎鲸 "1"）子串命中大量节点是预期的。
- ModeSparse 降级搜索词：纯数字查询词改用锚点 title（`isNumeric`）。

### store
- 列名别用 SQL 保留字（`from` → `src` 踩过）。
- Load 对"存在但从未写入"的文件返回空（无 documents 表判定）。

### web / 前端
- 页面必须 `Cache-Control: no-store`，否则用户看旧页面以为"没反应"。
- 事件委托（document 级 closest）统一处理点击；`a` 链接排除。
- touch/refresh 闭包失败要静默或仅提示，不影响主流程。

## 4. 安全与隐私红线（再次强调）

- 虎鲸 `Repo` 表（API key / 对象存储密钥）**绝不读取、绝不入库、绝不进 git**。
- 用户真实库内容（vault-survey、novel-wiki.yaml、scratch/）**不进 git**。
- 提交前检查：`git ls-files | Select-String 'novel-wiki|scratch|PROGRESS|vault-survey'`。
- 推送到 GitHub 前确认历史无敏感文件（历史重写要谨慎，方案见 PROGRESS_LOG）。

## 5. 版本与发布

发布清单（每版按序走）：

1. `version` 常量在 `cmd/seren/main.go`，bump 到新版本号。
2. **README 徽章版本号同步**（README.md / README.en.md 顶部 shields.io 徽章里的
   `vX.Y.Z`——与 version 常量同值，改 README 时一并改）。
3. 文档同步：00-overview §6 版本行 + 相关 architecture 文档 + PROGRESS_LOG.md。
4. `git tag vX.Y.Z`（与 version 常量一致；tag 缺失会导致后续 push 报
   "src refspec 不匹配"）。
5. 推送（沙箱内无法用 credential helper/askpass——git 强制走 sh 且沙箱禁 sh 信号
   管道；用 URL 内嵌 token）：
   ```powershell
   $tok = gh auth token
   git push "https://x-access-token:$tok@github.com/heptaspirit/serendipity-engine.git" main
   git push "https://x-access-token:$tok@github.com/heptaspirit/serendipity-engine.git" --tags
   git fetch "https://x-access-token:$tok@github.com/heptaspirit/serendipity-engine.git" main:refs/remotes/origin/main
   ```
   用户终端里普通 `git push` 不受此限制。
6. 版本脉络见 00-overview §6；详细过程见 PROGRESS_LOG.md（本地）。
