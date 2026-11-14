# try-go

用 Go 与 Bubble Tea 编写的终端项目管理器：搜索并进入目录、新建试验、克隆仓库，以及管理 worktree、重命名、毕业和删除。

## 构建与安装

```sh
nix develop
just                 # 查看开发命令
just build           # bin/try-go，CGO_ENABLED=0
just test
python3 tests/e2e.py bin/try-go  # 真实 PTY / Shell 回归
just install         # 安装到 ~/.local/bin/try-go
```

也可用 `nix build` 构建锁定依赖的包，或 `nix run . -- --help` 直接运行。无需 Nix 时，使用 Go 1.24 或更新版本和 just。运行程序只需二进制；克隆、worktree 等 Git 操作需要 PATH 中有 Git。

`~/.local/bin` 需在 PATH 中。让 `try` 切换当前 Shell 的目录，在 Shell 启动文件中添加对应一行：

```sh
# zsh
eval "$(try-go init zsh)"
# bash
eval "$(try-go init bash)"
# fish
try-go init fish | source
```

`just install` 只安装二进制；Shell 初始化由使用者显式配置。

## 使用

```sh
try                              # 交互搜索并进入目录
try my-project                   # 带初始查询的交互搜索
try new my-project               # 直接新建
try clone https://github.com/owner/repo [NAME]
try https://github.com/owner/repo # URL 快捷克隆
try worktree feature --repo /path/to/repo
try rename old-name new-name
try graduate my-project          # 移到毕业目录
try graduate my-project /path/to/destination
try delete old-one old-two --yes  # 永久删除指定目录
try --date new dated-project     # 显式加日期前缀
try --path /path/to/tries         # 本次覆盖项目根目录
```

交互界面支持模糊搜索；按界面底部提示进行新建、重命名、毕业与批量删除。毕业后原位置保留符号链接；删除符号链接只移除链接。删除需确认。目录默认不加日期，已有日期前缀目录仍可搜索和使用。

## 目录

日常目录布局为：普通创建与浏览使用 `~/codebase/incubator`，clone 与 URL 快捷克隆使用 `~/Downloads/git-repos`，毕业使用 `~/codebase/Ray`。

| 设置 | 用途 |
| --- | --- |
| `TRY_PATH` | 覆盖试验根目录；自定义根目录时 clone 默认也使用此目录 |
| `TRY_CLONE_PATH` | 单独指定 clone 根目录 |
| `TRY_PROJECTS` | 指定毕业根目录 |
| `--path PATH` | 本次统一覆盖浏览、新建与 clone 根目录 |
| `--date` | 为新目录添加日期前缀 |

没有 `~/codebase` 的机器默认使用 `~/src/tries`；没有独立 clone 目录布局时与试验目录共用根目录。毕业目录可通过 `TRY_PROJECTS` 或命令参数指定。

## Nix 维护

`flake.lock` 固定 nixpkgs，`flake.nix` 声明开发工具及 `try-go` 包。修改 Go 依赖后需更新 `vendorHash`：临时设为 `pkgs.lib.fakeHash`，执行 `nix build`，将错误中实际计算出的 `got: sha256-…` 填回，再重新构建。
