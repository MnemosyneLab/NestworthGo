# Wails wails3 版本升级流程

本文记录 Nestworth-go 升级 Wails v3 的完整流程。Wails 版本升级不是只替换
一个 Go module：后端 Wails API、前端 runtime、wails3 CLI、TypeScript
bindings 生成器和干净 checkout 的 fallback 必须保持一致。

本文以 v3.0.0-beta.16 为示例。升级其他版本时，将下面的
WAILS_GO_VERSION 和 WAILS_RUNTIME_VERSION 替换成目标版本即可：Go 和
CLI 使用 v 前缀，npm runtime 不使用 v 前缀。

官方资料：

- [Wails releases](https://github.com/wailsapp/wails/releases)
- [Wails v3 documentation](https://v3.wails.io/)
- [Wails bindings documentation](https://v3.wails.io/features/bindings/methods/)

## 升级边界

一次 Wails 升级至少应检查以下位置：

- go.mod、go.sum：后端 Wails module；
- frontend/package.json、frontend/pnpm-lock.yaml：前端
  @wailsio/runtime；
- frontend/scripts/ensure-bindings.mjs：没有全局 wails3 时使用的精确
  版本 fallback；
- frontend/bindings/：由 Wails 生成、被 gitignore 的 TypeScript 文件；
- README 和 docs/development/ 中描述当前版本的开发文档；
- 本机 wails3 CLI：通过 go install 安装，不写入仓库。

本流程不应修改用户数据库、数据库 schema、业务逻辑或手写生成的 bindings。
frontend/dist/、bin/、dist/macos/ 等构建产物也不应作为版本升级的源码
变更提交。

仓库的 GitHub Actions 当前单独固定 pnpm 版本。除非任务明确要求，Wails
升级不等于 pnpm 升级；升级 Wails 后仍需验证现有 pnpm 版本能够安装新的
runtime。

## 1. 升级前检查

所有 Go、Wails 和 Task 命令都从仓库根目录运行，只有 pnpm 命令进入
frontend/。

先记录工作区状态：

~~~bash
git status --short
git diff --stat
~~~

如果工作区已有未提交改动，先确认它们属于当前任务或用户正在进行的工作。
不要用 reset、checkout 或其他破坏性命令覆盖这些改动。

设置本次升级使用的版本和可写缓存。受限环境下不要依赖默认的 Go cache
目录：

~~~bash
WAILS_GO_VERSION=v3.0.0-beta.16
WAILS_RUNTIME_VERSION=3.0.0-beta.16
GOCACHE=/tmp/nestworth-wails-gocache
GOMODCACHE=/tmp/nestworth-wails-gomodcache
export WAILS_GO_VERSION WAILS_RUNTIME_VERSION GOCACHE GOMODCACHE
~~~

检查当前工具链和当前依赖：

~~~bash
go version
node --version
pnpm --version
type -a wails3 2>/dev/null || true
wails3 version 2>/dev/null || true
go list -m github.com/wailsapp/wails/v3
rg -n 'v3\.0\.0-beta|@wailsio/runtime' \
  go.mod frontend/package.json frontend/pnpm-lock.yaml \
  frontend/scripts/ensure-bindings.mjs README.md docs .github
~~~

阅读目标版本的 release notes，特别检查 Changed、Fixed、Removed 和
bindings/runtime 相关内容。beta 版本即使 API 大体稳定，也不能把自动化
测试通过当作原生 GUI 或打包验证通过。

## 2. 安装匹配的 wails3 CLI

Wails v3 CLI 应与 Go module 使用同一个精确版本：

~~~bash
go install "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_GO_VERSION"
PATH="$(go env GOPATH)/bin:$PATH" wails3 version
~~~

输出必须是目标版本，例如：

~~~text
v3.0.0-beta.16
~~~

如果仍输出旧版本，先检查 type -a wails3，再用带有
$(go env GOPATH)/bin 的 PATH 重试。不要只检查某个旧的绝对路径。

## 3. 更新 Go Wails module

在仓库根目录执行：

~~~bash
go get "github.com/wailsapp/wails/v3@$WAILS_GO_VERSION"
go mod tidy
~~~

然后检查 diff：

~~~bash
git diff -- go.mod go.sum
go list -m github.com/wailsapp/wails/v3
~~~

通常应看到 go.mod 的 Wails 版本和 go.sum 对应 checksum 更新。若
go mod tidy 修改了与 Wails 无关的大量依赖，先停止并审查原因，不要把
无关升级混入本次变更。

## 4. 更新前端 runtime 和 lockfile

必须在 frontend/ 目录执行 pnpm 命令，避免在仓库根目录意外生成新的
package.json 或 pnpm-lock.yaml：

~~~bash
cd frontend
pnpm add "@wailsio/runtime@$WAILS_RUNTIME_VERSION" \
  --save-exact --lockfile-only
pnpm install --frozen-lockfile
~~~

检查 frontend/package.json 和 lockfile：

~~~bash
rg -n -C 1 '@wailsio/runtime|3\.0\.0-beta' \
  package.json pnpm-lock.yaml
node -e "console.log(require('./node_modules/@wailsio/runtime/package.json').version)"
cd ..
~~~

node 输出必须是目标 runtime 版本。--save-exact 用于保持 Wails
runtime 与后端 API 的明确对应关系，不要在这里使用范围版本。

## 5. 更新 clean-checkout fallback 和活动文档

打开 frontend/scripts/ensure-bindings.mjs，同步以下两处目标版本：

1. go run github.com/wailsapp/wails/v3/cmd/wails3@... 的版本；
2. 生成失败提示中的安装版本。

这个 fallback 很重要：本机可能已经安装了 wails3，但 CI、干净 checkout
或新开发环境不一定有。fallback 使用旧版本会导致 bindings 由旧生成器
产出，造成前端类型或 runtime 漂移。

更新 README、Engineering Guide 和 Local Development 文档中的当前版本：

~~~bash
rg -n 'v3\.0\.0-beta|@wailsio/runtime' \
  README.md docs/development frontend/scripts/ensure-bindings.mjs
~~~

历史技术评审或已完成阶段的 baseline 应保留原始 review-time 版本；不要用
全局替换改写历史结论。只有描述当前工具链的活动文档需要更新。

## 6. 重新生成 Wails bindings

从仓库根目录运行仓库标准任务：

~~~bash
PATH="$(go env GOPATH)/bin:$PATH" \
  wails3 task generate:bindings
~~~

该任务会先执行 go mod tidy，然后按 production tag 运行等价的命令：

~~~bash
wails3 generate bindings -f '-tags production' -clean=true -ts -i ./...
~~~

./... 不能省略。绑定服务注册在 cmd/nestworth，只扫描仓库根包会漏掉
实际的 Wails service。

如果环境没有全局 CLI，可以使用目标版本直接运行：

~~~bash
go run "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_GO_VERSION" \
  generate bindings -f '-tags production' -clean=true -ts -i ./...
~~~

确认生成器报告了服务、方法、模型和事件数量，并检查：

~~~bash
test -f frontend/bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts
git status --short --ignored frontend/bindings frontend/dist
~~~

frontend/bindings/ 是生成目录，不要手工编辑，也不要把它加入 Git。若
生成后出现 tracked 文件变化，应先确认是否是误取消了 gitignore，而不是
直接提交生成产物。

## 7. 自动化验证

先做静态版本和格式检查：

~~~bash
test -z "$(gofmt -l cmd internal)"
node --check frontend/scripts/ensure-bindings.mjs
git diff --check
~~~

运行 Go 验证。显式设置缓存可以避免默认缓存目录权限问题：

~~~bash
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go test ./...
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go vet ./...
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build ./cmd/nestworth
~~~

运行前端验证：

~~~bash
cd frontend
pnpm install --frozen-lockfile
pnpm run build
pnpm run lint
pnpm run typecheck
pnpm run test
cd ..
~~~

也可以运行仓库的一站式检查：

~~~bash
PATH="$(go env GOPATH)/bin:$PATH" \
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" \
  wails3 task check
~~~

一站式检查和分步检查不要被重复结果混淆；无论采用哪一种，都要记录实际
执行的命令和结果。Go 链接器 warning 不等于测试失败，但必须确认命令的
exit code 为 0，并在交接中记录 warning 内容。

## 8. 原生 GUI 和打包验证

自动化检查不能覆盖 Wails runtime 的原生窗口、菜单、托盘、WebView、事件
分发、文件对话框和打包行为。升级完成后，在 macOS 目标环境至少执行：

~~~bash
SMOKE_DIR="$(mktemp -d /tmp/nestworth-wails-smoke.XXXXXX)"
NESTWORTH_DATABASE_PATH="$SMOKE_DIR/nestworth.db" \
NESTWORTH_SETTINGS_PATH="$SMOKE_DIR/settings.json" \
  PATH="$(go env GOPATH)/bin:$PATH" wails3 task dev
~~~

开发启动 smoke test 应覆盖：

- 首次启动和已有本地数据启动；
- 至少一次前端到 Go service 的调用；
- 事件通知、窗口 resize、菜单和 macOS 托盘（如果启用）；
- 数据库不可用或恢复页面仍能正常显示；
- 关闭窗口和退出应用时没有明显错误。

不要使用真实财务数据库做 smoke test。测试结束后确认临时目录和测试进程
没有继续占用资源。

macOS 本地 release-shaped package 验证：

~~~bash
PATH="$(go env GOPATH)/bin:$PATH" \
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" \
  wails3 task package:release
~~~

检查 dist/macos/Nestworth.app 和对应 DMG 的 bundle ID、版本、build、
arm64 架构、图标和 DMG 可读性。该任务生成的是本地 unsigned/ad-hoc
验证产物；Developer ID 签名、notarization、VoiceOver、200% zoom、窄窗口、
暗色模式和 reduced-motion 仍是独立的发布或人工门禁，不能用 go test 或
前端测试代替。

## 9. 常见失败和处理方式

### wails3 version 仍是旧版本

用下面的命令确认 PATH 顺序：

~~~bash
type -a wails3
PATH="$(go env GOPATH)/bin:$PATH" wails3 version
~~~

不要通过修改仓库脚本来适配错误的全局 PATH。CI 或 clean checkout 应使用
ensure-bindings.mjs 中的精确 fallback。

### Go 报 cache operation not permitted

这通常是默认 cache 目录不可写，不代表 Wails API 不兼容。重试时设置：

~~~bash
GOCACHE=/tmp/nestworth-wails-gocache
GOMODCACHE=/tmp/nestworth-wails-gomodcache
export GOCACHE GOMODCACHE
~~~

然后重新执行失败的 Go 命令。不要为了绕过权限问题修改用户目录权限。

### TypeScript 找不到 frontend/bindings

从仓库根目录重新生成：

~~~bash
PATH="$(go env GOPATH)/bin:$PATH" wails3 task generate:bindings
~~~

如果没有 CLI，使用上一节的 go run ...@$WAILS_GO_VERSION fallback。不要
手写缺失的 bindings 类型来掩盖生成失败。

### pnpm install --frozen-lockfile 失败

确认 pnpm add 是在 frontend/ 运行的，并且 package.json 与 lockfile
中的 runtime 版本一致。重新生成 lockfile 后，再用 frozen install 验证；
不要删除 lockfile，也不要在仓库根目录创建第二套 pnpm manifest。

### 升级后出现 Go API 或 runtime 类型错误

先对照目标版本 release notes，判断是 API 变更、绑定输入 tag 不一致还是
旧生成文件残留。修复应落在实际的 Go service、前端调用或 Taskfile，而不
应通过手改生成文件解决。若需要业务代码迁移，应把它作为独立的兼容性
变更记录，并增加对应测试。

## 10. 完成前检查清单

- [ ] go.mod 的 Wails module、go.sum checksum、前端 runtime 和 CLI
      都是同一个目标版本；
- [ ] frontend/scripts/ensure-bindings.mjs fallback 已同步；
- [ ] 活动开发文档已更新，历史 review baseline 未被全局替换；
- [ ] bindings 已用目标版本重新生成，且没有提交生成目录；
- [ ] go test ./...、go vet ./...、go build ./cmd/nestworth 通过；
- [ ] 前端 build、lint、typecheck、test 通过；
- [ ] git diff --check 通过，diff 没有无关依赖或构建产物；
- [ ] 原生 GUI、package、签名、notarization 和人工可访问性门禁的执行状态
      已分别记录；
- [ ] 用户数据库没有被打开、迁移、修改或清理。

如果只做依赖升级，推荐使用 Conventional Commit：

~~~text
chore(deps): upgrade Wails to v3.0.0-beta.16
~~~
